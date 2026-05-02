(function () {
    'use strict';

    const state = {
        seed: null,
        activeTab: 'references',
        list: [],
        offset: 0,
        limit: 20,
        filters: { yearMin: null, yearMax: null, minCites: 0, influentialOnly: false },
        basket: [],
        history: [],
        searchHistory: [],
    };

    const STORAGE_KEY = 'citebox_research_state';
    const SEARCH_HISTORY_LIMIT = 5;
    const AUTOCOMPLETE_DEBOUNCE_MS = 180;
    const AUTOCOMPLETE_MIN_LEN = 2;
    let rateWarningTimer = null;

    const autocomplete = {
        items: [],
        active: -1,
        timer: null,
        seq: 0,
        suppress: false,
    };

    function $(id) { return document.getElementById(id); }

    function loadPersistedState() {
        try {
            const raw = localStorage.getItem(STORAGE_KEY);
            if (!raw) return null;
            return JSON.parse(raw);
        } catch (e) {
            return null;
        }
    }

    function savePersistedState() {
        try {
            const snap = {
                seed: state.seed,
                activeTab: state.activeTab,
                searchHistory: state.searchHistory,
            };
            localStorage.setItem(STORAGE_KEY, JSON.stringify(snap));
        } catch (e) {
            // localStorage disabled or full — silently ignore
        }
    }

    function pushSearchHistory(query) {
        const q = (query || '').trim();
        if (!q) return;
        const list = (state.searchHistory || []).filter(x => x.toLowerCase() !== q.toLowerCase());
        list.unshift(q);
        state.searchHistory = list.slice(0, SEARCH_HISTORY_LIMIT);
        savePersistedState();
        renderSearchHistory();
    }

    function clearSearchHistory() {
        state.searchHistory = [];
        savePersistedState();
        renderSearchHistory();
    }

    function renderSearchHistory() {
        const el = $('research-search-history');
        if (!el) return;
        const list = state.searchHistory || [];
        if (!list.length) {
            el.innerHTML = '';
            el.hidden = true;
            return;
        }
        const labelTxt = t('research.history.recent', '最近搜索');
        const clearTxt = t('research.history.clear', '清空');
        el.innerHTML = `
            <span class="research-history-label">${escapeHtml(labelTxt)}</span>
            ${list.map(q => `<button class="research-history-chip" type="button" data-q="${escapeHtml(q)}">${escapeHtml(q)}</button>`).join('')}
            <button class="research-history-clear" type="button">${escapeHtml(clearTxt)}</button>
        `;
        el.hidden = false;
    }

    async function api(path, opts = {}) {
        const res = await fetch(path, {
            ...opts,
            headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) },
        });
        const text = await res.text();
        if (!res.ok) {
            const parsed = parseResearchError(res.status, text);
            if (parsed.rateLimited) {
                showRateWarning(parsed.message);
            }
            throw new Error(parsed.message);
        }
        return text ? JSON.parse(text) : {};
    }

    function parseResearchError(status, bodyText) {
        const parser = window.CiteBoxResearchErrors && window.CiteBoxResearchErrors.parseResearchErrorResponse;
        if (typeof parser === 'function') {
            return parser(status, bodyText, t);
        }
        return {
            rateLimited: status === 503,
            usedAPIKey: null,
            message: bodyText || String(status),
        };
    }

    function showRateWarning(message) {
        const el = $('research-rate-warning');
        if (!el) return;
        el.textContent = message || t('research.error.rateLimited', 'Semantic Scholar 限流，请稍后重试。');
        el.classList.remove('hidden');
        if (rateWarningTimer) {
            clearTimeout(rateWarningTimer);
        }
        rateWarningTimer = setTimeout(() => {
            el.classList.add('hidden');
        }, 4000);
    }

    function hideAutocomplete() {
        autocomplete.items = [];
        autocomplete.active = -1;
        const el = $('research-autocomplete-list');
        if (el) {
            el.hidden = true;
            el.innerHTML = '';
        }
        const input = $('research-search-input');
        if (input) {
            input.setAttribute('aria-expanded', 'false');
            input.setAttribute('aria-activedescendant', '');
        }
    }

    function renderAutocomplete(items) {
        const el = $('research-autocomplete-list');
        const input = $('research-search-input');
        if (!el || !input) return;
        autocomplete.items = items;
        autocomplete.active = -1;
        if (!items.length) {
            hideAutocomplete();
            return;
        }
        const moreLabel = t('research.autocomplete.more', '查看全部搜索结果');
        const moreIdx = items.length;
        const itemsHTML = items.map((it, i) => {
            const meta = it.authorsYear ? `<span class="research-autocomplete-meta">${escapeHtml(it.authorsYear)}</span>` : '';
            return `<li id="research-autocomplete-item-${i}" class="research-autocomplete-item" role="option" data-id="${escapeHtml(it.paperId || '')}" data-i="${i}">
                <span class="research-autocomplete-title">${escapeHtml(it.title || '')}</span>${meta}
            </li>`;
        }).join('');
        const moreHTML = `<li id="research-autocomplete-item-${moreIdx}" class="research-autocomplete-more" role="option" data-more="1" data-i="${moreIdx}">
            <span class="research-autocomplete-more-label">${escapeHtml(moreLabel)}</span>
            <span class="research-autocomplete-more-arrow" aria-hidden="true">→</span>
        </li>`;
        el.innerHTML = itemsHTML + moreHTML;
        el.hidden = false;
        input.setAttribute('aria-expanded', 'true');
    }

    function setActiveSuggestion(idx) {
        const list = $('research-autocomplete-list');
        const input = $('research-search-input');
        if (!list || !input) return;
        // navigable count = papers + the trailing "more" row
        const max = autocomplete.items.length + 1;
        if (max <= 1) return;
        autocomplete.active = ((idx % max) + max) % max;
        list.querySelectorAll('[data-i]').forEach((el) => {
            const i = parseInt(el.dataset.i, 10);
            const active = i === autocomplete.active;
            el.classList.toggle('is-active', active);
            if (active) {
                input.setAttribute('aria-activedescendant', el.id);
                el.scrollIntoView({ block: 'nearest' });
            }
        });
    }

    async function fetchAutocomplete(query) {
        const seq = ++autocomplete.seq;
        try {
            const data = await api(`/api/research/autocomplete?q=${encodeURIComponent(query)}`);
            if (seq !== autocomplete.seq) return; // a newer query has been issued
            renderAutocomplete(Array.isArray(data.items) ? data.items : []);
        } catch (e) {
            // network error — silently dismiss; user can still hit Enter to search
            if (seq === autocomplete.seq) hideAutocomplete();
        }
    }

    function scheduleAutocomplete() {
        clearTimeout(autocomplete.timer);
        const input = $('research-search-input');
        if (!input) return;
        if (autocomplete.suppress) {
            autocomplete.suppress = false;
            return;
        }
        const q = input.value.trim();
        if (q.length < AUTOCOMPLETE_MIN_LEN) {
            autocomplete.seq++; // invalidate any in-flight response
            hideAutocomplete();
            return;
        }
        autocomplete.timer = setTimeout(() => fetchAutocomplete(q), AUTOCOMPLETE_DEBOUNCE_MS);
    }

    async function selectAutocomplete(idx) {
        const input = $('research-search-input');
        // The trailing "more" row sits at index === items.length and runs the full search.
        if (idx === autocomplete.items.length) {
            const q = input ? input.value.trim() : '';
            hideAutocomplete();
            if (q) await searchSeed(q);
            return;
        }
        const item = autocomplete.items[idx];
        if (!item || !item.paperId) return;
        autocomplete.suppress = true;
        if (input) input.value = item.title || input.value;
        hideAutocomplete();
        try {
            await setSeed(item.paperId);
        } catch (e) {
            if (input && input.value.trim()) await searchSeed(input.value.trim());
        }
    }

    async function searchSeed(query) {
        pushSearchHistory(query);
        const data = await api(`/api/research/search?q=${encodeURIComponent(query)}&limit=20`);
        state.history = [];
        state.seed = null;
        renderSearchResults(data.items || []);
        savePersistedState();
    }

    function renderSearchResults(items) {
        state.activeTab = 'search';
        renderSeedPane(`<h4>${escapeHtml(t('research.tab.search', '搜索结果'))}</h4>`, items);
    }

    async function setSeed(s2PaperID) {
        const previousView = currentViewSnapshot();
        const paper = await api(`/api/research/paper/${encodeURIComponent(s2PaperID)}`);
        if (!paper || !paper.paperId) return;
        pushHistory(previousView);
        state.seed = paper;
        state.activeTab = 'references';
        savePersistedState();
        await loadActiveTab();
        $('research-seed-pane')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    async function loadActiveTab() {
        const seed = state.seed;
        const activeTab = state.activeTab;
        if (!seed || !seed.paperId) return;
        const id = encodeURIComponent(seed.paperId);
        let items = [];
        if (activeTab === 'references') {
            const data = await api(`/api/research/paper/${id}/references?offset=0&limit=20`);
            items = data.items || [];
        } else if (activeTab === 'citations') {
            const params = state.filters.influentialOnly ? '&influential_only=true' : '';
            const data = await api(`/api/research/paper/${id}/citations?offset=0&limit=20${params}`);
            items = data.items || [];
        } else if (activeTab === 'recommendations') {
            const data = await api(`/api/research/paper/${id}/recommendations`);
            items = data.items || [];
        }
        if (!state.seed || state.seed.paperId !== seed.paperId || state.activeTab !== activeTab) return;
        renderSeedPane(buildSeedHeader(seed, activeTab), items);
    }

    function buildSeedHeader(seed, activeTab) {
        const s = seed || state.seed;
        if (!s) return '';
        const currentTab = activeTab || state.activeTab;
        const ids = s.externalIds || {};
        const titleHtml = `<b>${escapeHtml(paperTitle(s))}</b>`;
        const metaParts = [formatAuthors(s.authors), s.year, s.venue, formatPaperStats(s)].filter(Boolean);
        const metaHtml = metaParts.length
            ? `<div class="research-seed-meta">${metaParts.map(escapeHtml).join(' · ')}</div>`
            : '';
        const tldr = s.tldr || (s.abstract && truncate(s.abstract, 320));
        const tldrHtml = tldr ? `<div class="research-seed-tldr">${escapeHtml(tldr)}</div>` : '';
        const linkBits = [];
        if (ids.DOI) linkBits.push(`<a href="https://doi.org/${encodeURIComponent(ids.DOI)}" target="_blank" rel="noopener">DOI</a>`);
        if (ids.ArXiv) linkBits.push(`<a href="https://arxiv.org/abs/${encodeURIComponent(ids.ArXiv)}" target="_blank" rel="noopener">arXiv</a>`);
        if (ids.PubMed) linkBits.push(`<a href="https://pubmed.ncbi.nlm.nih.gov/${encodeURIComponent(ids.PubMed)}/" target="_blank" rel="noopener">PubMed</a>`);
        if (s.openAccessPdfUrl) linkBits.push(`<a href="${escapeHtml(s.openAccessPdfUrl)}" target="_blank" rel="noopener">PDF</a>`);
        const linksHtml = linkBits.length ? `<div class="research-item-links">${linkBits.join(' · ')}</div>` : '';
        return `
            <div class="research-seed-card">
                <div class="research-seed-title-row">
                    <div class="research-seed-title">${titleHtml}</div>
                    ${s.paperId ? renderCandidateButton('add-seed-to-basket', '') : ''}
                </div>
                ${metaHtml}
                ${tldrHtml}
                ${linksHtml}
            </div>
            <div class="research-tabs">
                <span class="research-tab ${currentTab === 'references' ? 'active' : ''}" data-tab="references">${escapeHtml(t('research.tab.references', '引用了'))}</span>
                <span class="research-tab ${currentTab === 'citations' ? 'active' : ''}" data-tab="citations">${escapeHtml(t('research.tab.citations', '被引用'))}</span>
                <span class="research-tab ${currentTab === 'recommendations' ? 'active' : ''}" data-tab="recommendations">${escapeHtml(t('research.tab.recommendations', '相似'))}</span>
            </div>
        `;
    }

    function renderSeedPane(headerHTML, items) {
        state.list = items;
        const listHTML = items.map(p => renderListItem(p)).join('');
        const emptyHTML = `<div class="research-empty">${escapeHtml(emptyMessageForCurrentView())}</div>`;
        $('research-seed-pane').innerHTML = `${renderBackBar()}${headerHTML}<div class="research-list">${listHTML || emptyHTML}</div>`;
    }

    function emptyMessageForCurrentView() {
        const seed = state.seed;
        if (state.activeTab === 'search') {
            return t('research.empty.searchNoResults', '暂无搜索结果');
        }
        if (state.activeTab === 'basketRec') {
            return t('research.empty.basketRecNoResults', '暂无候选推荐');
        }
        if (state.activeTab === 'references') {
            if (seed && seed.referenceCount > 0) {
                return formatI18n(
                    'research.empty.referencesUnavailableWithCount',
                    'Semantic Scholar 显示这篇文章有 {count} 篇参考文献，但当前没有返回可展示的引用列表。',
                    { count: seed.referenceCount },
                );
            }
            return t('research.empty.referencesUnavailable', 'Semantic Scholar 暂未返回这篇文章的参考文献列表。');
        }
        if (state.activeTab === 'citations') {
            if (seed && seed.citationCount > 0) {
                return formatI18n(
                    'research.empty.citationsUnavailableWithCount',
                    'Semantic Scholar 显示这篇文章有 {count} 次被引，但当前没有返回可展示的被引列表。',
                    { count: seed.citationCount },
                );
            }
            return t('research.empty.citationsNone', 'Semantic Scholar 暂无被引记录。');
        }
        if (state.activeTab === 'recommendations') {
            return t('research.empty.recommendationsNone', 'Semantic Scholar 暂未返回相似文献。');
        }
        return t('research.empty.noResults', '暂无结果');
    }

    function renderBackBar() {
        if (!state.history.length) return '';
        const label = t('research.history.back', '返回上一级');
        return `
            <div class="research-back-row">
                <button class="research-back-button" type="button" data-action="go-back" aria-label="${escapeHtml(label)}">
                    <span aria-hidden="true">←</span>
                    <span>${escapeHtml(label)}</span>
                </button>
            </div>
        `;
    }

    function currentViewSnapshot() {
        return {
            seed: state.seed,
            activeTab: state.activeTab,
            list: Array.isArray(state.list) ? state.list.slice() : [],
        };
    }

    function pushHistory(snapshot) {
        state.history.push(snapshot);
        if (state.history.length > 20) state.history.shift();
    }

    function restorePreviousView() {
        const snapshot = state.history.pop();
        if (!snapshot) return;
        state.seed = snapshot.seed || null;
        state.activeTab = snapshot.activeTab || 'references';

        if (state.activeTab === 'search') {
            renderSearchResults(snapshot.list || []);
        } else if (state.activeTab === 'basketRec') {
            renderSeedPane(`<h4>${escapeHtml(t('research.tab.basketRec', '候选推荐'))}</h4>`, snapshot.list || []);
        } else if (state.seed) {
            renderSeedPane(buildSeedHeader(state.seed, state.activeTab), snapshot.list || []);
        } else {
            renderSeedPane(`<h4>${escapeHtml(t('research.tab.search', '搜索结果'))}</h4>`, snapshot.list || []);
        }

        savePersistedState();
        $('research-seed-pane')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    function renderListItem(p) {
        const ids = p.externalIds || {};
        const paperID = p.paperId || '';
        const detailLabel = t('research.action.openPaperDetail', '查看文献详情');
        const titleHtml = paperID
            ? `<button class="research-item-title research-title-button" type="button" data-action="set-as-seed" data-id="${escapeHtml(paperID)}" aria-label="${escapeHtml(detailLabel)}" title="${escapeHtml(detailLabel)}">${escapeHtml(paperTitle(p))}</button>`
            : `<b class="research-item-title">${escapeHtml(paperTitle(p))}</b>`;

        const metaParts = [formatAuthors(p.authors), p.year, p.venue, formatPaperStats(p)].filter(Boolean);
        const metaHtml = metaParts.length
            ? `<div class="research-meta">${metaParts.map(escapeHtml).join(' · ')}</div>`
            : '';

        const tldr = p.tldr || (p.abstract && truncate(p.abstract, 240));
        const tldrHtml = tldr ? `<div class="research-item-tldr">${escapeHtml(tldr)}</div>` : '';

        const linkBits = [];
        if (ids.DOI) linkBits.push(`<a href="https://doi.org/${encodeURIComponent(ids.DOI)}" target="_blank" rel="noopener">DOI</a>`);
        if (ids.ArXiv) linkBits.push(`<a href="https://arxiv.org/abs/${encodeURIComponent(ids.ArXiv)}" target="_blank" rel="noopener">arXiv</a>`);
        if (ids.PubMed) linkBits.push(`<a href="https://pubmed.ncbi.nlm.nih.gov/${encodeURIComponent(ids.PubMed)}/" target="_blank" rel="noopener">PubMed</a>`);
        if (p.openAccessPdfUrl) linkBits.push(`<a href="${escapeHtml(p.openAccessPdfUrl)}" target="_blank" rel="noopener">PDF</a>`);
        const linksHtml = linkBits.length ? `<div class="research-item-links">${linkBits.join(' · ')}</div>` : '';

        return `
            <div class="research-list-item" data-id="${escapeHtml(paperID)}">
                <div class="research-item-head">
                    ${titleHtml}
                    ${paperID ? renderCandidateButton('add-to-basket', paperID) : ''}
                </div>
                ${metaHtml}
                ${tldrHtml}
                ${linksHtml}
            </div>
        `;
    }

    function paperTitle(p) {
        return p.title || t('research.paper.untitled', '未命名文献');
    }

    function renderCandidateButton(action, paperID) {
        const label = t('research.action.addToBasket', '加入候选');
        const idAttr = paperID ? ` data-id="${escapeHtml(paperID)}"` : '';
        return `
            <button class="research-icon-button research-add-candidate" type="button" data-action="${escapeHtml(action)}"${idAttr} aria-label="${escapeHtml(label)}" title="${escapeHtml(label)}">
                <span aria-hidden="true">+</span>
            </button>
        `;
    }

    function formatAuthors(authors) {
        if (!authors || !authors.length) return '';
        return authors.slice(0, 3).map(a => a.name).join(', ') + (authors.length > 3 ? ' et al.' : '');
    }

    function formatPaperStats(p) {
        const parts = [];
        const refs = p.referenceCount || 0;
        const c = p.citationCount || 0;
        const inf = p.influentialCitationCount || 0;
        if (refs > 0) {
            parts.push(formatI18n('research.metric.references', '{count} references', { count: refs }));
        }
        if (c > 0 || inf > 0) {
            const citationText = formatI18n('research.metric.citations', '{count} citations', { count: c });
            parts.push(inf > 0 ? `${citationText} (★ ${inf})` : citationText);
        }
        return parts.join(' / ');
    }

    function formatI18n(key, fallback, values) {
        let text = t(key, fallback);
        Object.entries(values || {}).forEach(([name, value]) => {
            text = text.split(`{${name}}`).join(value);
        });
        return text;
    }

    function truncate(s, n) {
        if (!s) return '';
        if (s.length <= n) return s;
        return s.slice(0, n).trimEnd() + '…';
    }

    function escapeHtml(s) {
        if (s == null) return '';
        return String(s)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    async function refreshBasket() {
        const data = await api('/api/research/basket');
        state.basket = data.items || [];
        renderBasket();
    }

    function renderBasket() {
        if (!state.basket.length) {
            $('research-basket-list').innerHTML = `<li class="research-basket-empty">${escapeHtml(t('research.basket.empty', '暂无候选文献'))}</li>`;
            return;
        }
        const removeLabel = t('research.action.removeFromBasket', '移出候选');
        $('research-basket-list').innerHTML = state.basket.map(p => `
            <li class="research-basket-item" data-id="${escapeHtml(p.paperId)}">
                <span>${escapeHtml(paperTitle(p))}</span>
                <button data-action="remove-from-basket" data-id="${escapeHtml(p.paperId)}" aria-label="${escapeHtml(removeLabel)}" title="${escapeHtml(removeLabel)}">×</button>
            </li>
        `).join('');
    }

    async function addToBasket(s2PaperID) {
        await api('/api/research/basket', {
            method: 'POST',
            body: JSON.stringify({ s2_paper_id: s2PaperID }),
        });
        await refreshBasket();
    }

    async function removeFromBasket(s2PaperID) {
        await api(`/api/research/basket/${encodeURIComponent(s2PaperID)}`, { method: 'DELETE' });
        await refreshBasket();
    }

    async function importBasketToLibrary() {
        const ids = state.basket.map(p => p.paperId);
        if (!ids.length) return;
        await api('/api/research/basket/import-to-library', {
            method: 'POST',
            body: JSON.stringify({ ids }),
        });
        await refreshBasket();
    }

    async function exportBasket() {
        const res = await fetch('/api/research/basket/export');
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'research-shortlist.md';
        a.click();
        URL.revokeObjectURL(url);
    }

    async function recommendFromBasket() {
        const ids = state.basket.map(p => p.paperId);
        if (!ids.length) return;
        const data = await api('/api/research/recommendations', {
            method: 'POST',
            body: JSON.stringify({ positive: ids, negative: [] }),
        });
        pushHistory(currentViewSnapshot());
        state.activeTab = 'basketRec';
        renderSeedPane(`<h4>${escapeHtml(t('research.tab.basketRec', '候选推荐'))}</h4>`, data.items || []);
    }

    function bindEvents() {
        $('research-search-btn').addEventListener('click', () => {
            hideAutocomplete();
            const q = $('research-search-input').value.trim();
            if (q) searchSeed(q);
        });
        const input = $('research-search-input');
        input.addEventListener('input', scheduleAutocomplete);
        input.addEventListener('focus', () => {
            // Re-show when refocusing if we still have suggestions for the current query.
            if (autocomplete.items.length) {
                $('research-autocomplete-list').hidden = false;
                input.setAttribute('aria-expanded', 'true');
            }
        });
        input.addEventListener('blur', () => {
            // Delay so a click on the dropdown can land before we hide.
            setTimeout(hideAutocomplete, 120);
        });
        input.addEventListener('keydown', (e) => {
            const visible = autocomplete.items.length > 0 && !$('research-autocomplete-list').hidden;
            if (visible && e.key === 'ArrowDown') {
                e.preventDefault();
                setActiveSuggestion(autocomplete.active + 1);
                return;
            }
            if (visible && e.key === 'ArrowUp') {
                e.preventDefault();
                setActiveSuggestion(autocomplete.active - 1);
                return;
            }
            if (visible && e.key === 'Escape') {
                hideAutocomplete();
                return;
            }
            if (e.key === 'Enter') {
                if (visible && autocomplete.active >= 0) {
                    e.preventDefault();
                    selectAutocomplete(autocomplete.active);
                    return;
                }
                $('research-search-btn').click();
            }
        });
        $('research-autocomplete-list').addEventListener('mousedown', (e) => {
            // mousedown so we beat the input's blur and prevent it from firing.
            const li = e.target.closest('[data-i]');
            if (!li) return;
            e.preventDefault();
            const i = parseInt(li.dataset.i, 10);
            if (Number.isFinite(i)) selectAutocomplete(i);
        });
        $('research-basket-import-all').addEventListener('click', importBasketToLibrary);
        $('research-basket-export').addEventListener('click', exportBasket);
        $('research-basket-recommend').addEventListener('click', recommendFromBasket);

        // Event delegation for title drill-down, candidate actions, and tab switches.
        $('research-seed-pane').addEventListener('click', async (e) => {
            const tabEl = e.target.closest('[data-tab]');
            const tab = tabEl && tabEl.dataset.tab;
            if (tab) {
                state.activeTab = tab;
                savePersistedState();
                await loadActiveTab();
                return;
            }
            const actionEl = e.target.closest('[data-action]');
            const action = actionEl && actionEl.dataset.action;
            const id = actionEl && actionEl.dataset.id;
            if (action === 'go-back') {
                restorePreviousView();
                return;
            }
            if (action === 'set-as-seed' && id) await setSeed(id);
            if (action === 'add-to-basket' && id) await addToBasket(id);
            if (action === 'add-seed-to-basket' && state.seed) await addToBasket(state.seed.paperId);
        });

        $('research-basket-list').addEventListener('click', async (e) => {
            const actionEl = e.target.closest('[data-action]');
            const action = actionEl && actionEl.dataset.action;
            const id = actionEl && actionEl.dataset.id;
            if (action === 'remove-from-basket' && id) await removeFromBasket(id);
        });

        $('research-search-history').addEventListener('click', async (e) => {
            const chip = e.target.closest('button.research-history-chip');
            if (chip && chip.dataset.q) {
                $('research-search-input').value = chip.dataset.q;
                hideAutocomplete();
                await searchSeed(chip.dataset.q);
                return;
            }
            if (e.target.closest('button.research-history-clear')) {
                clearSearchHistory();
            }
        });
    }

    async function init() {
        bindEvents();

        const persisted = loadPersistedState();
        if (persisted) {
            state.searchHistory = Array.isArray(persisted.searchHistory) ? persisted.searchHistory : [];
            if (persisted.seed && persisted.seed.paperId) {
                state.seed = persisted.seed;
                state.activeTab = persisted.activeTab || 'references';
            }
        }
        renderSearchHistory();

        await refreshBasket();

        if (state.seed) {
            try {
                await loadActiveTab();
            } catch (e) {
                // upstream / network errors on resume — show cached seed only
                renderSeedPane(buildSeedHeader(state.seed, state.activeTab), []);
            }
        }
    }

    document.addEventListener('DOMContentLoaded', init);
})();
