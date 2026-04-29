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
    };

    function $(id) { return document.getElementById(id); }

    async function api(path, opts = {}) {
        const res = await fetch(path, {
            ...opts,
            headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) },
        });
        if (res.status === 503) {
            $('research-rate-warning').classList.remove('hidden');
            setTimeout(() => $('research-rate-warning').classList.add('hidden'), 4000);
        }
        if (!res.ok) {
            const text = await res.text();
            throw new Error(`${res.status}: ${text}`);
        }
        return res.json();
    }

    async function searchSeed(query) {
        const data = await api(`/api/research/search?q=${encodeURIComponent(query)}&limit=20`);
        renderSearchResults(data.items || []);
    }

    function renderSearchResults(items) {
        state.activeTab = 'search';
        renderSeedPane(`<h4 data-i18n="research.tab.search">搜索结果</h4>`, items);
    }

    async function setSeed(s2PaperID) {
        if (state.seed) state.history.push(state.seed.paperId);
        const paper = await api(`/api/research/paper/${encodeURIComponent(s2PaperID)}`);
        state.seed = paper;
        state.activeTab = 'references';
        await loadActiveTab();
    }

    async function loadActiveTab() {
        if (!state.seed) return;
        const id = encodeURIComponent(state.seed.paperId);
        let items = [];
        if (state.activeTab === 'references') {
            const data = await api(`/api/research/paper/${id}/references?offset=0&limit=20`);
            items = data.items || [];
        } else if (state.activeTab === 'citations') {
            const params = state.filters.influentialOnly ? '&influential_only=true' : '';
            const data = await api(`/api/research/paper/${id}/citations?offset=0&limit=20${params}`);
            items = data.items || [];
        } else if (state.activeTab === 'recommendations') {
            const data = await api(`/api/research/paper/${id}/recommendations`);
            items = data.items || [];
        }
        renderSeedPane(buildSeedHeader(), items);
    }

    function buildSeedHeader() {
        const s = state.seed;
        const ids = s.externalIds || {};
        const titleHref = s.paperId ? `https://www.semanticscholar.org/paper/${encodeURIComponent(s.paperId)}` : '';
        const titleHtml = titleHref
            ? `<a class="research-item-title" href="${escapeHtml(titleHref)}" target="_blank" rel="noopener">${escapeHtml(s.title)}</a>`
            : `<b>${escapeHtml(s.title)}</b>`;
        const metaParts = [formatAuthors(s.authors), s.year, s.venue, formatCites(s)].filter(Boolean);
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
                <div class="research-seed-title">${titleHtml}</div>
                ${metaHtml}
                ${tldrHtml}
                ${linksHtml}
                <div class="research-seed-actions">
                    <button data-action="add-seed-to-basket">+ 篮子</button>
                </div>
            </div>
            <div class="research-tabs">
                <span class="research-tab ${state.activeTab === 'references' ? 'active' : ''}" data-tab="references" data-i18n="research.tab.references">引用了</span>
                <span class="research-tab ${state.activeTab === 'citations' ? 'active' : ''}" data-tab="citations" data-i18n="research.tab.citations">被引用</span>
                <span class="research-tab ${state.activeTab === 'recommendations' ? 'active' : ''}" data-tab="recommendations" data-i18n="research.tab.recommendations">相似</span>
            </div>
        `;
    }

    function renderSeedPane(headerHTML, items) {
        state.list = items;
        const listHTML = items.map(p => renderListItem(p)).join('');
        $('research-seed-pane').innerHTML = `${headerHTML}<div class="research-list">${listHTML || '<div class="research-empty">无结果</div>'}</div>`;
    }

    function renderListItem(p) {
        const ids = p.externalIds || {};
        const titleHref = p.paperId ? `https://www.semanticscholar.org/paper/${encodeURIComponent(p.paperId)}` : '';
        const titleHtml = titleHref
            ? `<a class="research-item-title" href="${escapeHtml(titleHref)}" target="_blank" rel="noopener">${escapeHtml(p.title)}</a>`
            : `<b>${escapeHtml(p.title)}</b>`;

        const metaParts = [formatAuthors(p.authors), p.year, p.venue, formatCites(p)].filter(Boolean);
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
            <div class="research-list-item" data-id="${escapeHtml(p.paperId)}">
                ${titleHtml}
                ${metaHtml}
                ${tldrHtml}
                ${linksHtml}
                <div class="research-list-actions">
                    <button data-action="add-to-basket" data-id="${escapeHtml(p.paperId)}">+ 篮</button>
                    <button data-action="set-as-seed" data-id="${escapeHtml(p.paperId)}">设为种子</button>
                </div>
            </div>
        `;
    }

    function formatAuthors(authors) {
        if (!authors || !authors.length) return '';
        return authors.slice(0, 3).map(a => a.name).join(', ') + (authors.length > 3 ? ' et al.' : '');
    }

    function formatCites(p) {
        const c = p.citationCount || 0;
        const inf = p.influentialCitationCount || 0;
        if (!c && !inf) return '';
        return inf > 0 ? `${c} cites (★ ${inf})` : `${c} cites`;
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
        $('research-basket-list').innerHTML = state.basket.map(p => `
            <li class="research-basket-item" data-id="${escapeHtml(p.paperId)}">
                <span>${escapeHtml(p.title)}</span>
                <button data-action="remove-from-basket" data-id="${escapeHtml(p.paperId)}">×</button>
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
        a.download = 'research-basket.md';
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
        state.activeTab = 'basketRec';
        renderSeedPane(`<h4 data-i18n="research.tab.basketRec">基于篮子推荐</h4>`, data.items || []);
    }

    function bindEvents() {
        $('research-search-btn').addEventListener('click', () => {
            const q = $('research-search-input').value.trim();
            if (q) searchSeed(q);
        });
        $('research-search-input').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') $('research-search-btn').click();
        });
        $('research-basket-import-all').addEventListener('click', importBasketToLibrary);
        $('research-basket-export').addEventListener('click', exportBasket);
        $('research-basket-recommend').addEventListener('click', recommendFromBasket);

        // Event delegation for the seed pane (set-as-seed, add-to-basket, tab switches)
        $('research-seed-pane').addEventListener('click', async (e) => {
            const tab = e.target.dataset.tab;
            if (tab) {
                state.activeTab = tab;
                await loadActiveTab();
                return;
            }
            const action = e.target.dataset.action;
            const id = e.target.dataset.id;
            if (action === 'set-as-seed' && id) await setSeed(id);
            if (action === 'add-to-basket' && id) await addToBasket(id);
            if (action === 'add-seed-to-basket' && state.seed) await addToBasket(state.seed.paperId);
        });

        $('research-basket-list').addEventListener('click', async (e) => {
            const action = e.target.dataset.action;
            const id = e.target.dataset.id;
            if (action === 'remove-from-basket' && id) await removeFromBasket(id);
        });
    }

    async function init() {
        bindEvents();
        await refreshBasket();
    }

    document.addEventListener('DOMContentLoaded', init);
})();
