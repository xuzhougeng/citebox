if (typeof window.t !== 'function') window.t = function(k,f){return f||k};
const LibraryPage = {
    VIEW_MODE_KEY: 'citebox.libraryViewMode',
    DEFAULT_FILTERS: {
        keyword: '',
        author: '',
        keyword_scope: 'full_text',
        group_id: '',
        tag_id: '',
        status: '',
        sort_by: 'created_at'
    },
    // Filters that live inside the collapsible "more filters" panel.
    SECONDARY_FILTERS: ['author', 'keyword_scope', 'status', 'sort_by'],
    MAX_LISTED_AUTHORS: 3,
    state: {
        currentPage: 1,
        pageSize: 12,
        groups: [],
        tags: [],
        viewMode: 'detailed',
        filters: {
            keyword: '',
            author: '',
            keyword_scope: 'full_text',
            group_id: '',
            tag_id: '',
            status: '',
            sort_by: 'created_at'
        }
    },

    readViewMode() {
        try {
            const v = window.localStorage.getItem(this.VIEW_MODE_KEY);
            if (v === 'compact' || v === 'detailed') this.state.viewMode = v;
        } catch (e) { /* ignore */ }
    },

    saveViewMode(mode) {
        try { window.localStorage.setItem(this.VIEW_MODE_KEY, mode); } catch (e) { /* ignore */ }
    },

    applyViewMode() {
        if (!this.paperList) return;
        this.paperList.classList.toggle('compact', this.state.viewMode === 'compact');
    },

    async init() {
        this.autoRefreshTimer = null;
        this.loadingPapers = false;
        this.readViewMode();
        this.readLaunchState();
        this.cacheElements();
        this.applyViewMode();
        this.syncFilterMore();
        this.bindEvents();
        this.loadExtractionHint();
        await Promise.all([
            this.loadGroups(),
            this.loadTags()
        ]);
        await this.loadPapers();
        await this.handleLaunchState();
    },

    readLaunchState() {
        const params = new URLSearchParams(window.location.search);
        const paperId = String(params.get('paper_id') || '').trim();
        this.launchState = {
            paperId: paperId && paperId !== '0' ? paperId : '',
            fromDuplicate: params.get('from') === 'duplicate'
        };
    },

    async handleLaunchState() {
        if (!this.launchState?.paperId) return;

        if (this.launchState.fromDuplicate) {
            Utils.showToast(t('library.msg_duplicate_redirect', 'PDF 已存在，已跳转到已有文献'), 'info');
        }

        const paperId = this.launchState.paperId;
        this.clearLaunchState();
        await this.openPaperModal(paperId);
    },

    clearLaunchState() {
        const url = new URL(window.location.href);
        url.searchParams.delete('paper_id');
        url.searchParams.delete('from');
        window.history.replaceState({}, '', `${url.pathname}${url.search}${url.hash}`);
        this.launchState = { paperId: '', fromDuplicate: false };
    },

    cacheElements() {
        this.keywordInput = document.getElementById('keywordInput');
        this.authorInput = document.getElementById('authorInput');
        this.keywordScopeFilter = document.getElementById('keywordScopeFilter');
        this.groupFilter = document.getElementById('groupFilter');
        this.tagFilter = document.getElementById('tagFilter');
        this.statusFilter = document.getElementById('statusFilter');
        this.sortFilter = document.getElementById('sortFilter');
        this.filterMore = document.getElementById('libraryFilterMore');
        this.filterMoreCount = document.getElementById('libraryFilterMoreCount');
        this.paperList = document.getElementById('paperList');
        this.resultMeta = document.getElementById('libraryResultMeta');
        this.extractionHint = document.getElementById('libraryExtractionHint');
        this.pagination = document.getElementById('pagination');
        this.groupForm = document.getElementById('groupForm');
        this.groupNameInput = document.getElementById('groupNameInput');
        this.groupDescriptionInput = document.getElementById('groupDescriptionInput');
        this.groupList = document.getElementById('groupList');
        this.tagForm = document.getElementById('tagForm');
        this.tagNameInput = document.getElementById('tagNameInput');
        this.tagColorInput = document.getElementById('tagColorInput');
        this.tagList = document.getElementById('tagList');
        if (this.keywordScopeFilter) {
            this.keywordScopeFilter.value = this.state.filters.keyword_scope;
            this.updateKeywordPlaceholder();
        }
        if (this.statusFilter) {
            this.statusFilter.value = this.state.filters.status;
        }
        if (this.sortFilter) {
            this.sortFilter.value = this.state.filters.sort_by;
        }
    },

    bindEvents() {
        const debouncedSearch = Utils.debounce(async () => {
            await this.applyFilterChange('keyword', this.keywordInput.value.trim());
        }, 300);

        this.keywordInput.addEventListener('input', debouncedSearch);

        const debouncedAuthorFilter = Utils.debounce(async () => {
            await this.applyFilterChange('author', this.authorInput.value.trim());
        }, 300);

        this.authorInput.addEventListener('input', debouncedAuthorFilter);

        this.keywordScopeFilter.addEventListener('change', async () => {
            this.state.filters.keyword_scope = this.keywordScopeFilter.value || 'full_text';
            this.updateKeywordPlaceholder();
            await this.applyFilterChange('keyword_scope', this.state.filters.keyword_scope);
        });

        this.groupFilter.addEventListener('change', async () => {
            await this.applyFilterChange('group_id', this.groupFilter.value);
        });

        this.tagFilter.addEventListener('change', async () => {
            await this.applyFilterChange('tag_id', this.tagFilter.value);
        });

        this.statusFilter.addEventListener('change', async () => {
            await this.applyFilterChange('status', this.statusFilter.value);
        });

        this.sortFilter.addEventListener('change', async () => {
            await this.applyFilterChange('sort_by', this.sortFilter.value || 'created_at');
        });

        this.paperList.addEventListener('click', async (event) => {
            const card = event.target.closest('[data-paper-id]');
            if (!card) return;
            const paperId = String(card.dataset.paperId ?? '').trim();

            const action = event.target.closest('[data-action]');
            if (!action) {
                // Compact rows open the detail view when clicked anywhere that is
                // not another control (links, menus, buttons keep their own behavior).
                if (card.hasAttribute('data-row-open') && !event.target.closest('a, button, summary, details, input, select, textarea')) {
                    await this.openPaperModal(paperId);
                }
                return;
            }

            if (action.dataset.action === 'open-pdf') {
                this.openPaperPDF(paperId);
            }
            if (action.dataset.action === 'open') {
                await this.openPaperModal(paperId);
            }
            if (action.dataset.action === 'reextract') {
                await this.reextractPaper(paperId);
            }
            if (action.dataset.action === 'delete') {
                await this.deletePaper(paperId);
            }
        });

        if (this.resultMeta) {
            this.resultMeta.addEventListener('click', async (event) => {
                const clear = event.target.closest('[data-clear-filter]');
                if (clear) {
                    await this.clearFilter(clear.dataset.clearFilter);
                    return;
                }
                const btn = event.target.closest('[data-view-mode]');
                if (!btn) return;
                this.setViewMode(btn.dataset.viewMode);
            });
        }

        Utils.bindPagination(this.pagination, async (page) => {
            this.state.currentPage = page;
            await this.loadPapers();
        });

        if (this.groupForm) {
            this.groupForm.addEventListener('submit', async (event) => {
                event.preventDefault();
                await this.createGroup();
            });
        }

        if (this.groupList) {
            this.groupList.addEventListener('click', async (event) => {
                const button = event.target.closest('button[data-action]');
                if (!button) return;
                const id = Number(button.dataset.id);
                if (button.dataset.action === 'edit-group') {
                    await this.editGroup(id);
                }
                if (button.dataset.action === 'delete-group') {
                    await this.deleteGroup(id);
                }
            });
        }

        if (this.tagForm) {
            this.tagForm.addEventListener('submit', async (event) => {
                event.preventDefault();
                await this.createTag();
            });
        }

        if (this.tagList) {
            this.tagList.addEventListener('click', async (event) => {
                const button = event.target.closest('button[data-action]');
                if (!button) return;
                const id = Number(button.dataset.id);
                if (button.dataset.action === 'edit-tag') {
                    await this.editTag(id);
                }
                if (button.dataset.action === 'delete-tag') {
                    await this.deleteTag(id);
                }
            });
        }

        window.addEventListener('beforeunload', () => this.stopAutoRefresh());
    },

    async loadPapers(options = {}) {
        if (this.loadingPapers) return;
        this.loadingPapers = true;

        try {
            const payload = await API.listPapers({
                page: this.state.currentPage,
                page_size: this.state.pageSize,
                keyword: this.state.filters.keyword,
                author: this.state.filters.author,
                keyword_scope: this.state.filters.keyword_scope,
                group_id: this.state.filters.group_id,
                tag_id: this.state.filters.tag_id,
                status: this.state.filters.status,
                sort_by: this.state.filters.sort_by
            });
            this.state.papers = payload.papers || [];
            this.state.total = payload.total || 0;
            this.state.totalPages = payload.total_pages || 0;
            this.state.currentPage = this.state.totalPages ? Math.min(this.state.currentPage, this.state.totalPages) : 1;
            this.renderResultMeta();
            this.renderPaperList();
            this.renderPagination();
            this.syncAutoRefresh();
        } catch (error) {
            if (!options.silent) {
                Utils.showToast(error.message, 'error');
            }
        } finally {
            this.loadingPapers = false;
        }
    },

    async loadGroups() {
        try {
            const payload = await API.listGroups();
            this.state.groups = payload.groups || [];
            this.renderGroups();
            this.renderGroupFilter();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async loadTags() {
        try {
            const payload = await API.listTags({ scope: 'paper' });
            this.state.tags = payload.tags || [];
            this.renderTags();
            this.renderTagFilter();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async applyFilterChange(key, value) {
        this.state.filters[key] = value;
        this.state.currentPage = 1;
        this.syncFilterMore();
        await this.loadPapers();
    },

    isFilterActive(key) {
        return String(this.state.filters[key] ?? '') !== String(this.DEFAULT_FILTERS[key] ?? '');
    },

    // Keeps the "more filters" panel open while any of its fields differs from
    // the default, and shows how many of them are set when it is collapsed.
    syncFilterMore() {
        const activeCount = this.SECONDARY_FILTERS.filter((key) => this.isFilterActive(key)).length;
        if (this.filterMore && activeCount > 0) {
            this.filterMore.open = true;
        }
        if (this.filterMoreCount) {
            this.filterMoreCount.hidden = activeCount === 0;
            this.filterMoreCount.textContent = activeCount > 0 ? String(activeCount) : '';
        }
    },

    async clearFilter(key) {
        const keys = key === 'all'
            ? Object.keys(this.DEFAULT_FILTERS)
            : (Object.prototype.hasOwnProperty.call(this.DEFAULT_FILTERS, key) ? [key] : []);
        if (!keys.length) return;

        keys.forEach((name) => {
            this.state.filters[name] = this.DEFAULT_FILTERS[name];
        });
        const controls = {
            keyword: this.keywordInput,
            author: this.authorInput,
            keyword_scope: this.keywordScopeFilter,
            group_id: this.groupFilter,
            tag_id: this.tagFilter,
            status: this.statusFilter,
            sort_by: this.sortFilter
        };
        keys.forEach((name) => {
            if (controls[name]) controls[name].value = this.state.filters[name];
        });
        this.updateKeywordPlaceholder();
        this.state.currentPage = 1;
        this.syncFilterMore();
        await this.loadPapers();
    },

    activeFilterChips() {
        const filters = this.state.filters;
        const chips = [];
        if (filters.keyword) {
            chips.push({ key: 'keyword', label: `${t('library.result_keyword', '关键词')}：${filters.keyword}` });
        }
        if (filters.keyword && this.isFilterActive('keyword_scope')) {
            chips.push({ key: 'keyword_scope', label: `${t('library.result_scope_label', '范围')}：${this.keywordScopeLabel()}` });
        }
        if (filters.author) {
            chips.push({ key: 'author', label: `${t('library.result_author', '作者')}：${filters.author}` });
        }
        if (filters.group_id) {
            const group = (this.state.groups || []).find((item) => String(item.id) === String(filters.group_id));
            chips.push({ key: 'group_id', label: `${t('library.filter_group', '分组')}：${group ? group.name : filters.group_id}` });
        }
        if (filters.tag_id) {
            const tag = (this.state.tags || []).find((item) => String(item.id) === String(filters.tag_id));
            chips.push({ key: 'tag_id', label: `${t('library.result_tag', '标签')}：${tag ? tag.name : filters.tag_id}` });
        }
        if (filters.status) {
            chips.push({ key: 'status', label: `${t('library.result_status', '状态')}：${Utils.statusLabel(filters.status)}` });
        }
        return chips;
    },

    renderResultMeta() {
        if (!this.resultMeta) return;

        const chips = this.activeFilterChips();
        const removeLabel = t('library.filter_remove', '移除筛选');
        const chipsHTML = chips.length ? `
            <div class="active-filter-chips">
                ${chips.map((chip) => `
                    <button type="button" class="active-filter-chip" data-clear-filter="${chip.key}" aria-label="${Utils.escapeHTML(`${removeLabel}：${chip.label}`)}">
                        <span>${Utils.escapeHTML(chip.label)}</span>
                        <span class="active-filter-chip-x" aria-hidden="true">×</span>
                    </button>
                `).join('')}
                ${chips.length > 1 ? `<button type="button" class="active-filter-clear" data-clear-filter="all">${t('library.filter_clear_all', '清除全部')}</button>` : ''}
            </div>
        ` : '';

        const vm = this.state.viewMode;
        this.resultMeta.innerHTML = `
            <h2>${t('library.result_found', '找到')} ${this.state.total || 0} ${t('library.result_papers', '篇文献')}</h2>
            ${chipsHTML}
            <span class="library-view-toggle" role="group" aria-label="${t('library.view_mode_aria', '视图模式')}">
                <button type="button" class="view-toggle-btn ${vm === 'detailed' ? 'active' : ''}" data-view-mode="detailed" aria-pressed="${vm === 'detailed'}">${t('library.view_detailed', '详细')}</button>
                <button type="button" class="view-toggle-btn ${vm === 'compact' ? 'active' : ''}" data-view-mode="compact" aria-pressed="${vm === 'compact'}">${t('library.view_compact', '紧凑')}</button>
            </span>
        `;
    },

    setViewMode(mode) {
        if (mode !== 'compact' && mode !== 'detailed') return;
        if (this.state.viewMode === mode) return;
        this.state.viewMode = mode;
        this.saveViewMode(mode);
        this.applyViewMode();
        this.renderResultMeta();
        this.renderPaperList();
    },

    keywordScopeLabel() {
        return this.state.filters.keyword_scope === 'title_abstract' ? t('library.scope_title_abstract', '标题 + 摘要 + DOI') : t('library.scope_full_text', '全文 + DOI');
    },

    sortLabel() {
        return this.state.filters.sort_by === 'updated_at' ? t('library.sort_updated', '按文献更新时间') : t('library.sort_created', '按文献创建时间');
    },

    updateKeywordPlaceholder() {
        if (!this.keywordInput) return;
        this.keywordInput.placeholder = this.state.filters.keyword_scope === 'title_abstract'
            ? t('library.filter_keyword_placeholder_title', '标题、摘要或 DOI')
            : t('library.filter_keyword_placeholder', '标题、摘要、正文或 DOI');
    },

    async loadExtractionHint() {
        if (!this.extractionHint || typeof API === 'undefined' || typeof API.getExtractorSettings !== 'function') return;
        try {
            const settings = await API.getExtractorSettings();
            const profile = String(settings?.extractor_profile || '').trim();
            // The manual profile is a deliberate choice; the built-in AI profile
            // is configured on the AI page. Only hint when a service URL is missing.
            const missing = profile !== 'manual' && profile !== 'open_source_vision' && !settings?.effective_extractor_url;
            this.extractionHint.hidden = !missing;
            this.extractionHint.innerHTML = missing ? `
                <span>${t('library.extraction_hint', '未配置自动解析服务：新文献会直接入库，图片可通过手动标注补录。')}</span>
                <a href="/settings#settings-section-12">${t('library.extraction_hint_link', '前往配置')}</a>
            ` : '';
        } catch (error) {
            this.extractionHint.hidden = true;
        }
    },

    // "A, B, C" for short lists, "A, B, C et al." beyond MAX_LISTED_AUTHORS.
    formatAuthors(value) {
        const raw = String(value || '').trim();
        if (!raw) return '';
        // "Last, First; Last, First" lists keep their semicolons.
        const semicolons = /[;；]/.test(raw);
        const separator = semicolons ? '; ' : ', ';
        const names = (semicolons ? raw.split(/[;；]/) : raw.split(/[,，]/))
            .map((name) => name.trim())
            .filter(Boolean);
        if (names.length <= this.MAX_LISTED_AUTHORS) return names.join(separator);
        return t('library.authors_et_al', '{names} 等').replace('{names}', names.slice(0, this.MAX_LISTED_AUTHORS).join(separator));
    },

    publishedYear(value) {
        const raw = String(value || '').trim();
        const match = raw.match(/\b(\d{4})\b/);
        return match ? match[1] : raw;
    },

    formatShortDate(value) {
        if (!value) return '';
        const date = new Date(value);
        if (Number.isNaN(date.getTime())) return '';
        const locale = typeof document !== 'undefined' && document.documentElement?.lang ? document.documentElement.lang : 'zh-CN';
        return date.toLocaleDateString(locale, { year: 'numeric', month: '2-digit', day: '2-digit' });
    },

    paperMetaLine(paper) {
        return [this.formatAuthors(paper.authors_text), String(paper.journal || '').trim(), this.publishedYear(paper.published_at)]
            .filter(Boolean)
            .map((part) => Utils.escapeHTML(part))
            .join('<span class="paper-meta-sep" aria-hidden="true"> · </span>');
    },

    // Status is shown only when it carries information (anything but completed).
    renderStatusPill(paper) {
        if (!paper.extraction_status || paper.extraction_status === 'completed') return '';
        return `<span class="status-pill ${Utils.statusTone(paper.extraction_status)}">${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}</span>`;
    },

    shouldShowExtractorMessage(paper) {
        return Boolean(paper.extractor_message) && ['failed', 'cancelled', 'queued', 'running'].includes(paper.extraction_status);
    },

    canReextract(paper) {
        return paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled';
    },

    renderTitleButton(paper) {
        return `<button type="button" class="paper-title-link" data-action="open" title="${t('library.meta_click_detail', '点击查看详情')}">${Utils.escapeHTML(paper.title)}</button>`;
    },

    renderRowMenu(paper, options = {}) {
        const moreLabel = t('library.btn_more_actions', '更多操作');
        return `
            <details class="action-menu no-caret">
                <summary class="btn btn-outline btn-small action-menu-trigger" aria-label="${moreLabel}" title="${moreLabel}">⋯</summary>
                <div class="action-menu-list">
                    ${options.includeReadPDF ? `<button type="button" class="action-menu-item" data-action="open-pdf">${t('library.btn_read_pdf', '阅读 PDF')}</button>` : ''}
                    <button type="button" class="action-menu-item" data-action="open">${t('library.btn_view_detail', '查看详情')}</button>
                    <a class="action-menu-item" href="/manual?paper_id=${paper.id}">${t('library.btn_manual', '手动标注')}</a>
                    ${this.canReextract(paper) ? `<button type="button" class="action-menu-item" data-action="reextract">${t('library.btn_reextract', '重新解析')}</button>` : ''}
                    <div class="action-menu-separator"></div>
                    <button type="button" class="action-menu-item danger" data-action="delete">${t('btn.delete', '删除')}</button>
                </div>
            </details>
        `;
    },

    renderPaperList() {
        if (!this.state.papers.length) {
            const filtered = Object.keys(this.DEFAULT_FILTERS).some((key) => key !== 'sort_by' && this.isFilterActive(key));
            this.paperList.innerHTML = `
                <div class="empty-state">
                    <h3>${filtered ? t('library.empty_title', '还没有符合条件的文献') : t('library.empty_library_title', '文献库还是空的')}</h3>
                    <p>${filtered ? t('library.empty_filtered', '调整或清除当前的筛选条件后再试。') : t('library.empty_library_desc', '上传 PDF 后，文献会出现在这里。')}</p>
                    ${filtered
                        ? `<button class="btn btn-outline" type="button" data-empty-clear-filters>${t('library.filter_clear_all', '清除全部')}</button>`
                        : `<a class="btn btn-primary" href="/upload">${t('library.btn_upload', '上传文献')}</a>`}
                </div>
            `;
            this.paperList.querySelector?.('[data-empty-clear-filters]')?.addEventListener('click', () => this.clearFilter('all'));
            return;
        }

        if (this.state.viewMode === 'compact') {
            this.paperList.innerHTML = this.renderCompactTable();
            return;
        }

        this.paperList.innerHTML = this.state.papers.map((paper) => {
            const tags = (paper.tags || []).map((tag) => `<span class="chip" style="--chip-color:${tag.color}">${Utils.escapeHTML(tag.name)}</span>`).join('');
            const statusClass = Utils.statusTone(paper.extraction_status);
            const summary = paper.abstract_text || paper.paper_notes_text || paper.notes_text;
            const metaLine = this.paperMetaLine(paper);
            const quiet = [
                paper.group_name ? Utils.escapeHTML(paper.group_name) : '',
                t('library.meta_figure_count', '{count} 张图片').replace('{count}', paper.figure_count || 0)
            ].filter(Boolean).join('<span class="paper-meta-sep" aria-hidden="true"> · </span>');

            return `
                <article class="paper-row" data-paper-id="${paper.id}">
                    <div class="paper-row-main">
                        <h3 class="paper-row-title">${this.renderStatusPill(paper)}${this.renderTitleButton(paper)}</h3>
                        ${metaLine ? `<p class="paper-row-meta">${metaLine}</p>` : ''}
                        ${summary ? `<p class="paper-row-summary">${Utils.escapeHTML(summary)}</p>` : ''}
                        ${this.shouldShowExtractorMessage(paper) ? `<p class="notice ${statusClass} paper-row-notice">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}
                        <div class="paper-row-foot">
                            <span class="paper-row-quiet">${quiet}</span>
                            ${tags ? `<span class="paper-row-tags">${tags}</span>` : ''}
                        </div>
                    </div>
                    <div class="paper-row-actions">
                        <button class="btn btn-outline btn-small" type="button" data-action="open-pdf">${t('library.btn_read_pdf', '阅读 PDF')}</button>
                        ${this.renderRowMenu(paper)}
                    </div>
                </article>
            `;
        }).join('');
    },

    renderCompactTable() {
        const sortedByUpdate = this.state.filters.sort_by === 'updated_at';
        const dateLabel = sortedByUpdate ? t('library.col_updated', '更新时间') : t('library.col_added', '添加时间');
        const rows = this.state.papers.map((paper) => {
            const authors = this.formatAuthors(paper.authors_text);
            const year = this.publishedYear(paper.published_at);
            const journal = String(paper.journal || '').trim();
            const date = this.formatShortDate(sortedByUpdate ? (paper.updated_at || paper.created_at) : paper.created_at);
            return `
                <tr class="paper-table-row" data-paper-id="${paper.id}" data-row-open>
                    <td class="paper-table-title">${this.renderTitleButton(paper)}${this.renderStatusPill(paper)}</td>
                    <td class="paper-table-authors" title="${Utils.escapeHTML(paper.authors_text || '')}">${Utils.escapeHTML(authors) || '<span class="muted">—</span>'}</td>
                    <td class="paper-table-year">${Utils.escapeHTML(year) || '<span class="muted">—</span>'}</td>
                    <td class="paper-table-journal" title="${Utils.escapeHTML(journal)}">${Utils.escapeHTML(journal) || '<span class="muted">—</span>'}</td>
                    <td class="paper-table-group">${paper.group_name ? Utils.escapeHTML(paper.group_name) : '<span class="muted">—</span>'}</td>
                    <td class="paper-table-figures">${paper.figure_count || 0}</td>
                    <td class="paper-table-date">${Utils.escapeHTML(date)}</td>
                    <td class="paper-table-actions">${this.renderRowMenu(paper, { includeReadPDF: true })}</td>
                </tr>
            `;
        }).join('');

        return `
            <div class="paper-table-wrap">
                <table class="paper-table">
                    <thead>
                        <tr>
                            <th scope="col" class="paper-table-title">${t('library.col_title', '标题')}</th>
                            <th scope="col" class="paper-table-authors">${t('library.col_authors', '作者')}</th>
                            <th scope="col" class="paper-table-year">${t('library.col_year', '年份')}</th>
                            <th scope="col" class="paper-table-journal">${t('library.col_journal', '期刊')}</th>
                            <th scope="col" class="paper-table-group">${t('library.col_group', '分组')}</th>
                            <th scope="col" class="paper-table-figures">${t('library.col_figures', '图片')}</th>
                            <th scope="col" class="paper-table-date" aria-sort="descending">${dateLabel}<span class="paper-table-sort" aria-hidden="true">↓</span></th>
                            <th scope="col" class="paper-table-actions"><span class="paper-table-hidden-label">${t('library.col_actions', '操作')}</span></th>
                        </tr>
                    </thead>
                    <tbody>${rows}</tbody>
                </table>
            </div>
        `;
    },

    renderPagination() {
        Utils.renderPagination(this.pagination, this.state.currentPage, this.state.totalPages);
    },

    syncAutoRefresh() {
        const needsRefresh = (this.state.papers || []).some((paper) => Utils.isProcessingStatus(paper.extraction_status));
        if (needsRefresh && !this.autoRefreshTimer) {
            this.autoRefreshTimer = window.setInterval(() => {
                this.loadPapers({ silent: true });
            }, 4000);
            return;
        }
        if (!needsRefresh) {
            this.stopAutoRefresh();
        }
    },

    stopAutoRefresh() {
        if (this.autoRefreshTimer) {
            window.clearInterval(this.autoRefreshTimer);
            this.autoRefreshTimer = null;
        }
    },

    renderGroupFilter() {
        const current = this.state.filters.group_id;
        this.groupFilter.innerHTML = `<option value="">${t('library.filter_all_groups', '全部分组')}</option>`;
        this.state.groups.forEach((group) => {
            this.groupFilter.insertAdjacentHTML(
                'beforeend',
                `<option value="${group.id}" ${String(group.id) === String(current) ? 'selected' : ''}>${Utils.escapeHTML(group.name)}</option>`
            );
        });
    },

    renderTagFilter() {
        const current = this.state.filters.tag_id;
        this.tagFilter.innerHTML = `<option value="">${t('library.filter_all_tags', '全部文献标签')}</option>`;
        this.state.tags.forEach((tag) => {
            this.tagFilter.insertAdjacentHTML(
                'beforeend',
                `<option value="${tag.id}" ${String(tag.id) === String(current) ? 'selected' : ''}>${Utils.escapeHTML(tag.name)}</option>`
            );
        });
    },

    renderGroups() {
        if (!this.groupList) return;
        this.groupList.innerHTML = this.state.groups.map((group) => `
            <div class="manager-item">
                <div>
                    <strong>${Utils.escapeHTML(group.name)}</strong>
                    <p>${Utils.escapeHTML(group.description || t('library.group_no_desc', '无描述'))}</p>
                </div>
                <div class="manager-item-actions">
                    <span>${group.paper_count}</span>
                    <button class="ghost-btn" type="button" data-action="edit-group" data-id="${group.id}">${t('library.btn_rename', '改名')}</button>
                    <button class="ghost-btn danger" type="button" data-action="delete-group" data-id="${group.id}">${t('btn.delete', '删除')}</button>
                </div>
            </div>
        `).join('') || `<p class="muted">${t('library.group_empty', '暂无分组')}</p>`;
    },

    renderTags() {
        if (!this.tagList) return;
        this.tagList.innerHTML = this.state.tags.map((tag) => `
            <div class="manager-item">
                <div class="tag-line">
                    <span class="tag-dot" style="background:${tag.color}"></span>
                    <strong>${Utils.escapeHTML(tag.name)}</strong>
                </div>
                <div class="manager-item-actions">
                    <span>${tag.paper_count}</span>
                    <button class="ghost-btn" type="button" data-action="edit-tag" data-id="${tag.id}">${t('library.btn_rename', '改名')}</button>
                    <button class="ghost-btn danger" type="button" data-action="delete-tag" data-id="${tag.id}">${t('btn.delete', '删除')}</button>
                </div>
            </div>
        `).join('') || `<p class="muted">${t('library.tag_empty', '暂无标签')}</p>`;
    },

    async createGroup() {
        try {
            await API.createGroup({
                name: this.groupNameInput.value.trim(),
                description: this.groupDescriptionInput.value.trim()
            });
            this.groupForm.reset();
            Utils.showToast(t('library.msg_group_created', '分组已创建'));
            await this.loadGroups();
            await this.loadPapers();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async editGroup(id) {
        const group = this.state.groups.find((item) => item.id === id);
        if (!group) return;
        const name = window.prompt(t('library.prompt_group_name', '新的分组名称'), group.name);
        if (name === null) return;
        const description = window.prompt(t('library.prompt_group_desc', '新的分组说明'), group.description || '');
        if (description === null) return;

        try {
            await API.updateGroup(id, { name, description });
            Utils.showToast(t('library.msg_group_updated', '分组已更新'));
            await this.loadGroups();
            await this.loadPapers();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async deleteGroup(id) {
        const confirmed = await Utils.confirm(t('library.msg_delete_group_confirm', '删除分组后，文献只会失去分组，不会删除文献本身。'));
        if (!confirmed) return;

        try {
            await API.deleteGroup(id);
            Utils.showToast(t('library.msg_group_deleted', '分组已删除'));
            await this.loadGroups();
            await this.loadPapers();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async createTag() {
        try {
            await API.createTag({
                scope: 'paper',
                name: this.tagNameInput.value.trim(),
                color: this.tagColorInput.value
            });
            this.tagForm.reset();
            this.tagColorInput.value = '#A45C40';
            Utils.showToast(t('library.msg_tag_created', '标签已创建'));
            await this.loadTags();
            await this.loadPapers();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async editTag(id) {
        const tag = this.state.tags.find((item) => item.id === id);
        if (!tag) return;
        const name = window.prompt(t('library.prompt_tag_name', '新的标签名称'), tag.name);
        if (name === null) return;
        const color = window.prompt(t('library.prompt_tag_color', '新的标签颜色（HEX）'), tag.color || '#A45C40');
        if (color === null) return;

        try {
            await API.updateTag(id, { name, color });
            Utils.showToast(t('library.msg_tag_updated', '标签已更新'));
            await this.loadTags();
            await this.loadPapers();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async deleteTag(id) {
        const confirmed = await Utils.confirm(t('library.msg_delete_tag_confirm', '删除标签后，相关关联也会一并移除。'));
        if (!confirmed) return;

        try {
            await API.deleteTag(id);
            Utils.showToast(t('library.msg_tag_deleted', '标签已删除'));
            await this.loadTags();
            await this.loadPapers();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async openPaperModal(id) {
        await PaperViewer.open(id, async () => {
            await Promise.all([this.loadGroups(), this.loadTags(), this.loadPapers()]);
        });
    },

    openPaperPDF(id) {
        const paperId = String(id ?? '').trim();
        const paper = (this.state.papers || []).find((item) => String(item.id ?? '').trim() === paperId);
        if (!paper?.pdf_url) {
            Utils.showToast(t('shared.paper.no_pdf_url', '当前文献缺少 PDF 文件地址'), 'error');
            return;
        }
        if (typeof Utils.openResourceViewer === 'function') {
            Utils.openResourceViewer('pdf', paper.pdf_url, window.location.href, { paperId });
            return;
        }
        window.location.href = Utils.resourceViewerURL('pdf', paper.pdf_url, window.location.href, { paperId });
    },

    async deletePaper(id) {
        const confirmed = await Utils.confirm(t('library.msg_delete_paper_confirm', '删除后会移除 PDF、提取图片以及相关标签关系。'));
        if (!confirmed) return;

        try {
            await API.deletePaper(id);
            Utils.showToast(t('library.msg_paper_deleted', '文献已删除'));
            await Promise.all([this.loadGroups(), this.loadTags(), this.loadPapers()]);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async reextractPaper(id) {
        try {
            await API.reextractPaper(id);
            Utils.showToast(t('library.msg_reextract_submitted', '文献已重新提交解析'), 'info');
            await Promise.all([this.loadGroups(), this.loadTags(), this.loadPapers()]);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },


};
