const BrowserUI = {
    renderTagChips(tags = []) {
        if (!tags.length) {
            return '<span class="muted">无标签</span>';
        }
        return tags.map((tag) => `<span class="chip" style="--chip-color:${tag.color}">${Utils.escapeHTML(tag.name)}</span>`).join('');
    },

    renderPaperCard(paper) {
        const tags = BrowserUI.renderTagChips(paper.tags || []);
        const statusClass = Utils.statusTone(paper.extraction_status);
        const summary = paper.abstract_text || paper.paper_notes_text || paper.notes_text;
        return `
            <article class="paper-list-row" data-paper-id="${paper.id}">
                <div class="paper-list-main">
                    <div class="paper-list-head">
                        <span class="status-pill ${statusClass}">${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}</span>
                        <h3>${Utils.escapeHTML(paper.title)}</h3>
                    </div>
                    <div class="paper-list-meta">
                        <span class="paper-list-meta-item paper-list-meta-file" data-action="open" role="button" title="点击查看详情">
                            <span class="paper-list-meta-label">文件</span>
                            <span class="paper-list-meta-value">${Utils.escapeHTML(paper.original_filename)}</span>
                        </span>
                        <span class="paper-list-meta-item">
                            <span class="paper-list-meta-label">分组</span>
                            <span class="paper-list-meta-value">${Utils.escapeHTML(paper.group_name || '未分组')}</span>
                        </span>
                        <span class="paper-list-meta-item">
                            <span class="paper-list-meta-label">图片</span>
                            <span class="paper-list-meta-value">${paper.figure_count || 0}</span>
                        </span>
                        <span class="paper-list-meta-item">
                            <span class="paper-list-meta-label">更新</span>
                            <span class="paper-list-meta-value">${Utils.formatDate(paper.updated_at || paper.created_at)}</span>
                        </span>
                    </div>
                    ${summary ? `<p class="paper-list-summary">${Utils.escapeHTML(summary)}</p>` : ''}
                    ${paper.extractor_message ? `<p class="notice ${statusClass} paper-list-notice">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}
                </div>
                <div class="paper-list-footer">
                    <div class="paper-list-tags">${tags}</div>
                </div>
            </article>
        `;
    },

    renderFigureNotePreview(noteText = '', emptyText = '还没有笔记，可把 AI 解读或人工观察先记在这里。', options = {}) {
        const { interactive = false } = options;
        const interactiveClass = interactive ? ' figure-preview-note-action' : '';
        const interactiveAttrs = interactive ? ' data-action="note" role="button" tabindex="0"' : '';
        const normalized = String(noteText || '').replace(/\s+/g, ' ').trim();
        if (!normalized) {
            return `
                <div class="figure-preview-note is-empty${interactiveClass}"${interactiveAttrs}>
                    <span class="figure-preview-note-label">图片笔记</span>
                    <p class="figure-preview-note-text">${Utils.escapeHTML(emptyText)}</p>
                </div>
            `;
        }

        const excerpt = normalized.length > 120 ? `${normalized.slice(0, 120)}...` : normalized;
        return `
            <div class="figure-preview-note${interactiveClass}"${interactiveAttrs}>
                <span class="figure-preview-note-label">图片笔记</span>
                <p class="figure-preview-note-text">${Utils.escapeHTML(excerpt)}</p>
            </div>
        `;
    },

    renderFigureCard(figure, index, options = {}) {
        const {
            mediaAction = 'preview',
            primaryAction = 'note',
            showNotesPreview = false,
            emptyNotesText = '还没有笔记，可把 AI 解读或人工观察先记在这里。'
        } = options;

        const hasNotes = Boolean(String(figure.notes_text || '').trim());
        const mediaLabel = mediaAction === 'note' ? '查看笔记' : '查看大图';
        const notePreview = showNotesPreview
            ? BrowserUI.renderFigureNotePreview(figure.notes_text, emptyNotesText, { interactive: true })
            : '';

        return `
            <article class="figure-preview-card" data-paper-id="${figure.paper_id}" data-figure-index="${index}">
                <div class="figure-preview-stage">
                    <button class="figure-preview-media" type="button" data-action="${mediaAction}" aria-label="${mediaLabel}">
                        <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.paper_title || '提取图片')}">
                    </button>
                    <div class="figure-preview-badges">
                        <span class="figure-badge figure-badge-strong">第 ${figure.page_number || '-'} 页</span>
                        <span class="figure-badge">#${figure.figure_index || '-'}</span>
                        ${hasNotes ? '<span class="figure-badge figure-badge-accent">有笔记</span>' : ''}
                        ${figure.source === 'manual' ? '<span class="figure-badge">人工提取</span>' : ''}
                    </div>
                </div>
                <div class="figure-preview-body">
                    <div class="figure-preview-head">
                        <span class="figure-preview-label">来源文献</span>
                        <div class="figure-preview-title-row">
                            <strong class="figure-preview-title figure-preview-title-action" data-action="paper" role="button" tabindex="0">${Utils.escapeHTML(figure.paper_title)}</strong>
                            <a class="figure-preview-origin-link" href="${Utils.resourceViewerURL('image', figure.image_url)}">原图</a>
                        </div>
                    </div>
                    <div class="figure-preview-tags ${figure.tags?.length ? '' : 'is-empty'}">
                        ${figure.tags?.length ? BrowserUI.renderTagChips(figure.tags || []) : '<span class="figure-preview-empty">无标签</span>'}
                    </div>
                    ${notePreview}
                </div>
            </article>
        `;
    },

    renderPagination(container, currentPage, totalPages) {
        Utils.renderPagination(container, currentPage, totalPages);
    }
};

function mergeFigureCollectionWithPaper(figures = [], paper) {
    if (!paper) {
        return Array.isArray(figures) ? figures : [];
    }

    const figuresByID = new Map((paper.figures || []).map((figure) => [Number(figure.id), figure]));
    return (Array.isArray(figures) ? figures : []).map((figure) => {
        if (Number(figure.paper_id) !== Number(paper.id)) {
            return figure;
        }

        const updatedFigure = figuresByID.get(Number(figure.id));
        return {
            ...figure,
            paper_title: paper.title,
            group_id: paper.group_id,
            group_name: paper.group_name || '',
            filename: updatedFigure?.filename || figure.filename,
            image_url: updatedFigure?.image_url || figure.image_url,
            tags: updatedFigure?.tags || [],
            caption: updatedFigure?.caption ?? figure.caption,
            source: updatedFigure?.source || figure.source,
            notes_text: updatedFigure?.notes_text ?? figure.notes_text ?? ''
        };
    });
}

function createFigureCollectionActions(options = {}) {
    const normalizePage = (value) => Math.max(1, Number(value) || 1);
    const buildModalOptions = (index) => ({
        figures: typeof options.getFigures === 'function' ? options.getFigures() || [] : [],
        index,
        page: normalizePage(typeof options.getPage === 'function' ? options.getPage() : 1),
        totalPages: normalizePage(typeof options.getTotalPages === 'function' ? options.getTotalPages() : 1),
        loadPage: typeof options.loadPage === 'function' ? options.loadPage : null,
        onOpenPaper: typeof options.openPaper === 'function'
            ? async (paperID) => {
                await options.openPaper(Number(paperID));
            }
            : null,
        onMetaChanged: typeof options.onMetaChanged === 'function' ? options.onMetaChanged : null
    });

    return {
        openNote: async (index) => {
            await NoteViewer.open(buildModalOptions(index));
        },
        openPreview: async (index) => {
            await FigureViewer.open(buildModalOptions(index));
        },
        openPaper: async (paperID) => {
            if (typeof options.openPaper === 'function') {
                await options.openPaper(Number(paperID));
            }
        }
    };
}

const FiguresPage = {
    state: { page: 1, pageSize: 8, totalPages: 0, filters: { keyword: '', group_id: '', tag_id: '' } },

    async init() {
        PaperViewer.init();
        FigureViewer.init();
        NoteViewer.init();
        this.cache();
        this.bind();
        await Promise.all([this.loadGroups(), this.loadTags()]);
        await this.load();
    },

    cache() {
        this.keywordInput = document.getElementById('figureKeywordInput');
        this.groupFilter = document.getElementById('figureGroupFilter');
        this.tagFilter = document.getElementById('figureTagFilter');
        this.summaryStrip = document.getElementById('figureSummaryStrip');
        this.grid = document.getElementById('figureGrid');
        this.pagination = document.getElementById('figurePagination');
        this.pageControls = document.getElementById('figurePageControls');
    },

    bind() {
        this.figureActions = createFigureCollectionActions({
            getFigures: () => this.figures || [],
            getPage: () => this.state.page,
            getTotalPages: () => this.state.totalPages,
            loadPage: async (page) => {
                const payload = await this.fetchFigurePage(page);
                this.renderFigureResults(payload, page);
                return payload;
            },
            openPaper: async (paperID) => {
                await PaperViewer.open(Number(paperID), async () => {
                    await Promise.all([this.loadGroups(), this.loadTags(), this.load(this.state.page)]);
                });
            },
            onMetaChanged: async () => {
                await Promise.all([this.loadGroups(), this.loadTags(), this.load(this.state.page)]);
            }
        });

        const debouncedSearch = Utils.debounce(async () => {
            this.state.filters.keyword = this.keywordInput.value.trim();
            await this.load(1);
        }, 250);
        this.keywordInput.addEventListener('input', debouncedSearch);
        this.groupFilter.addEventListener('change', async () => {
            this.state.filters.group_id = this.groupFilter.value;
            await this.load(1);
        });
        this.tagFilter.addEventListener('change', async () => {
            this.state.filters.tag_id = this.tagFilter.value;
            await this.load(1);
        });
        this.grid.addEventListener('click', async (event) => {
            const action = event.target.closest('[data-action]');
            const card = event.target.closest('[data-figure-index]');
            if (!card) return;
            if (!action) return;

            const index = Number(card.dataset.figureIndex);
            if (action.dataset.action === 'note') {
                await this.figureActions.openNote(index);
                return;
            }
            if (action.dataset.action === 'preview') {
                await this.figureActions.openPreview(index);
                return;
            }
            if (action.dataset.action === 'paper') {
                await this.figureActions.openPaper(card.dataset.paperId);
            }
        });
        Utils.bindPagination(this.pagination, async (page) => await this.load(page));
        this.pageControls.addEventListener('click', async (event) => {
            const button = event.target.closest('button[data-page-step]');
            if (!button || button.disabled) return;

            const step = Number(button.dataset.pageStep);
            const nextPage = this.state.page + step;
            if (nextPage < 1 || nextPage > this.state.totalPages) return;

            await this.load(nextPage);
        });
    },

    async loadGroups() {
        const payload = await API.listGroups();
        const selected = String(this.state.filters.group_id || '');
        this.groupFilter.innerHTML = '<option value="">全部分组</option>' + (payload.groups || []).map((group) => `
            <option value="${group.id}" ${String(group.id) === selected ? 'selected' : ''}>${Utils.escapeHTML(group.name)}</option>
        `).join('');
    },

    async loadTags() {
        const payload = await API.listTags({ scope: 'figure' });
        const selected = String(this.state.filters.tag_id || '');
        this.tagFilter.innerHTML = '<option value="">全部图片标签</option>' + (payload.tags || []).map((tag) => `
            <option value="${tag.id}" ${String(tag.id) === selected ? 'selected' : ''}>${Utils.escapeHTML(tag.name)}</option>
        `).join('');
    },

    buildFigureParams(page = this.state.page) {
        return {
            page,
            page_size: this.state.pageSize,
            keyword: this.state.filters.keyword,
            group_id: this.state.filters.group_id,
            tag_id: this.state.filters.tag_id
        };
    },

    async fetchFigurePage(page = this.state.page) {
        return API.listFigures(this.buildFigureParams(page));
    },

    renderFigureResults(payload, page = this.state.page) {
        const figures = payload.figures || [];
        const totalPages = payload.total_pages || 0;
        this.state.page = totalPages ? Math.min(page, totalPages) : 1;
        this.figures = figures;
        this.state.totalPages = totalPages;
        this.summaryStrip.innerHTML = `
            <div class="stat-card"><span>筛选结果</span><strong>${payload.total || 0}</strong></div>
            <div class="stat-card"><span>当前页图片</span><strong>${figures.length}</strong></div>
            <div class="stat-card"><span>来源分组筛选</span><strong>${Utils.escapeHTML(this.groupFilter.selectedOptions[0]?.textContent || '全部分组')}</strong></div>
            <div class="stat-card"><span>图片标签筛选</span><strong>${Utils.escapeHTML(this.tagFilter.selectedOptions[0]?.textContent || '全部图片标签')}</strong></div>
        `;
        this.pageControls.innerHTML = this.state.totalPages > 1 ? `
            <button class="btn btn-outline" type="button" data-page-step="-1" ${this.state.page <= 1 ? 'disabled' : ''}>上一页</button>
            <span class="figure-page-indicator">第 ${this.state.page} / ${this.state.totalPages} 页</span>
            <button class="btn btn-outline" type="button" data-page-step="1" ${this.state.page >= this.state.totalPages ? 'disabled' : ''}>下一页</button>
        ` : '';
        this.grid.innerHTML = figures.length
            ? figures.map((figure, index) => BrowserUI.renderFigureCard(figure, index, {
                mediaAction: 'preview',
                primaryAction: 'note',
                showNotesPreview: true
            })).join('')
            : '<div class="empty-state"><h3>没有可展示的图片</h3><p>先上传文献，或者调整筛选条件。</p></div>';
        BrowserUI.renderPagination(this.pagination, this.state.page, this.state.totalPages);
    },

    async load(page = this.state.page) {
        try {
            const payload = await this.fetchFigurePage(page);
            this.renderFigureResults(payload, page);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    }
};

const GroupsPage = {
    state: { selectedGroupId: '', page: 1, pageSize: 20, totalPaperCount: 0 },

    async init() {
        PaperViewer.init();
        this.cache();
        this.bind();
        await this.reload();
    },

    cache() {
        this.form = document.getElementById('groupPageForm');
        this.nameInput = document.getElementById('groupPageNameInput');
        this.descriptionInput = document.getElementById('groupPageDescriptionInput');
        this.grid = document.getElementById('groupCardGrid');
        this.headline = document.getElementById('groupHeadline');
        this.paperList = document.getElementById('groupPaperList');
        this.pagination = document.getElementById('groupPagination');
    },

    bind() {
        this.figureActions = createFigureCollectionActions({
            getFigures: () => this.figures || [],
            getPage: () => this.state.page,
            getTotalPages: () => this.totalPages || 1,
            loadPage: async (page) => {
                const payload = await this.fetchFigureResults(page);
                this.renderFigureResults(payload, page);
                return payload;
            },
            openPaper: async (paperID) => {
                await PaperViewer.open(Number(paperID), async () => await this.reload());
            },
            onMetaChanged: async () => {
                await this.reload();
            }
        });

        this.form.addEventListener('submit', async (event) => {
            event.preventDefault();
            try {
                await API.createGroup({
                    name: this.nameInput.value.trim(),
                    description: this.descriptionInput.value.trim()
                });
                this.form.reset();
                Utils.showToast('分组已创建');
                await this.reload();
            } catch (error) {
                Utils.showToast(error.message, 'error');
            }
        });

        this.grid.addEventListener('click', async (event) => {
            const card = event.target.closest('[data-group-id]');
            if (!card) return;
            const action = event.target.closest('[data-action]');
            const id = card.dataset.groupId;
            if (!action) {
                this.state.selectedGroupId = id;
                this.state.page = 1;
                this.renderGroupCards();
                await this.loadPapers();
                return;
            }
            if (action.dataset.action === 'edit-group') {
                await this.editGroup(Number(id));
            }
            if (action.dataset.action === 'delete-group') {
                await this.deleteGroup(Number(id));
            }
        });

        this.paperList.addEventListener('click', async (event) => {
            const action = event.target.closest('[data-action]');
            const card = event.target.closest('[data-paper-id]');
            if (!action || !card) return;
            await PaperViewer.open(Number(card.dataset.paperId), async () => await this.reload());
        });

        Utils.bindPagination(this.pagination, async (page) => {
            this.state.page = page;
            await this.loadPapers();
        });
    },

    async reload() {
        await Promise.all([this.loadGroups(), this.loadGlobalPaperCount()]);
        this.renderGroupCards();
        await this.loadPapers();
    },

    async loadGroups() {
        const payload = await API.listGroups();
        this.groups = payload.groups || [];
        if (this.state.selectedGroupId && !this.groups.some((group) => String(group.id) === String(this.state.selectedGroupId))) {
            this.state.selectedGroupId = '';
        }
    },

    async loadGlobalPaperCount() {
        const payload = await API.listPapers({ page: 1, page_size: 1 });
        this.state.totalPaperCount = payload.total || 0;
    },

    renderGroupCards() {
        const allCard = `
            <article class="entity-card ${this.state.selectedGroupId ? '' : 'active'}" data-group-id="">
                <div><h3>全部文献</h3><p>查看所有分组下的文献</p></div>
                <strong>${this.state.totalPaperCount}</strong>
            </article>
        `;
        this.grid.innerHTML = allCard + this.groups.map((group) => `
            <article class="entity-card ${String(group.id) === String(this.state.selectedGroupId) ? 'active' : ''}" data-group-id="${group.id}">
                <div>
                    <h3>${Utils.escapeHTML(group.name)}</h3>
                    <p>${Utils.escapeHTML(group.description || '无说明')}</p>
                </div>
                <div class="entity-card-actions">
                    <strong>${group.paper_count}</strong>
                    <button class="ghost-btn" type="button" data-action="edit-group">改名</button>
                    <button class="ghost-btn danger" type="button" data-action="delete-group">删除</button>
                </div>
            </article>
        `).join('');

        const current = this.groups.find((group) => String(group.id) === String(this.state.selectedGroupId));
        this.headline.textContent = current ? `分组「${current.name}」中的文献` : '全部文献';
    },

    async loadPapers() {
        try {
            const payload = await API.listPapers({
                page: this.state.page,
                page_size: this.state.pageSize,
                group_id: this.state.selectedGroupId
            });
            const totalPages = payload.total_pages || 0;
            this.state.page = totalPages ? Math.min(this.state.page, totalPages) : 1;
            const papers = payload.papers || [];
            this.paperList.innerHTML = papers.length ? papers.map(BrowserUI.renderPaperCard).join('') : '<div class="empty-state"><h3>这个分组下还没有文献</h3></div>';
            BrowserUI.renderPagination(this.pagination, this.state.page, totalPages);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async editGroup(id) {
        const group = this.groups.find((item) => item.id === id);
        if (!group) return;
        const values = await Utils.promptFields({
            title: '修改分组',
            description: '更新分组名称和说明，便于后续筛选和归类。',
            confirmLabel: '保存分组',
            fields: [
                { name: 'name', label: '分组名称', value: group.name || '', required: true, placeholder: '例如：单细胞图谱' },
                { name: 'description', label: '分组说明', value: group.description || '', placeholder: '一句话说明这个分组的用途' }
            ]
        });
        if (!values) return;
        try {
            await API.updateGroup(id, {
                name: values.name.trim(),
                description: values.description.trim()
            });
            Utils.showToast('分组已更新');
            await this.reload();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async deleteGroup(id) {
        const confirmed = await Utils.confirm('删除分组后，文献仍会保留，只是不再属于该分组。');
        if (!confirmed) return;
        try {
            await API.deleteGroup(id);
            Utils.showToast('分组已删除');
            await this.reload();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    }
};

const TagsPage = {
    defaultTagColor: '#A45C40',
    colorStorageKey: 'citebox_tag_page_color',
    colorPresets: [
        { value: '#A45C40', label: '陶土棕' },
        { value: '#C97A40', label: '琥珀橙' },
        { value: '#D4A017', label: '金黄' },
        { value: '#6E9F5B', label: '橄榄绿' },
        { value: '#2F7D6B', label: '青绿' },
        { value: '#416788', label: '钢蓝' },
        { value: '#5B5F97', label: '靛蓝' },
        { value: '#A05C7B', label: '莓果紫' }
    ],
    state: { scope: 'paper', selectedTagId: '', page: 1, totalPaperCount: 0, totalFigureCount: 0 },

    async init() {
        PaperViewer.init();
        FigureViewer.init();
        NoteViewer.init();
        this.cache();
        this.bind();
        await this.reload();
    },

    cache() {
        this.form = document.getElementById('tagPageForm');
        this.nameInput = document.getElementById('tagPageNameInput');
        this.colorInput = document.getElementById('tagPageColorInput');
        this.colorPresetList = document.getElementById('tagColorPresetList');
        this.tagPresetPanel = document.getElementById('tagPresetPanel');
        this.tagPresetList = document.getElementById('tagPresetList');
        this.creatorTitle = document.getElementById('tagCreatorTitle');
        this.creatorHint = document.getElementById('tagCreatorHint');
        this.submitButton = document.getElementById('tagPageSubmit');
        this.scopeSwitch = document.getElementById('tagScopeSwitch');
        this.grid = document.getElementById('tagCardGrid');
        this.headline = document.getElementById('tagHeadline');
        this.scopeHint = document.getElementById('tagScopeHint');
        this.resultList = document.getElementById('tagResultList');
        this.pagination = document.getElementById('tagPagination');
        this.contextMenu = document.getElementById('tagContextMenu');

        if (!this.contextMenu) {
            this.contextMenu = document.createElement('div');
            this.contextMenu.id = 'tagContextMenu';
            this.contextMenu.className = 'tag-context-menu hidden';
            this.contextMenu.innerHTML = `
                <button type="button" data-tag-menu-action="edit">改名</button>
                <button type="button" class="danger" data-tag-menu-action="delete">删除</button>
            `;
            document.body.appendChild(this.contextMenu);
        }

        if (this.colorInput) {
            this.colorInput.value = this.loadSavedTagColor();
        }
    },

    bind() {
        this.form.addEventListener('submit', async (event) => {
            event.preventDefault();
            try {
                const color = this.currentTagColor();
                await API.createTag({
                    scope: this.state.scope,
                    name: this.nameInput.value.trim(),
                    color
                });
                this.nameInput.value = '';
                this.setTagColor(color);
                this.nameInput.focus();
                Utils.showToast('标签已创建');
                await this.reload();
            } catch (error) {
                Utils.showToast(error.message, 'error');
            }
        });

        this.colorPresetList?.addEventListener('click', (event) => {
            const button = event.target.closest('[data-tag-color]');
            if (!button) return;
            this.setTagColor(button.dataset.tagColor);
        });

        this.tagPresetList?.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-tag-preset-name]');
            if (!button) return;
            await this.handleTagPreset(button.dataset.tagPresetName || '');
        });

        this.colorInput?.addEventListener('input', () => {
            this.setTagColor(this.colorInput.value);
        });

        this.grid.addEventListener('click', async (event) => {
            this.hideContextMenu();
            const card = event.target.closest('[data-tag-id]');
            if (!card) return;
            const id = card.dataset.tagId;
            this.state.selectedTagId = id;
            this.state.page = 1;
            this.renderTagCards();
            await this.loadResults();
        });

        this.grid.addEventListener('contextmenu', (event) => {
            const card = event.target.closest('[data-tag-id]');
            const id = String(card?.dataset.tagId || '');
            if (!card || !id) return;
            event.preventDefault();
            this.showContextMenu(id, event.clientX, event.clientY);
        });

        this.contextMenu.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-tag-menu-action]');
            if (!button) return;
            const id = Number(this.contextMenu.dataset.tagId || 0);
            this.hideContextMenu();
            if (!id) return;
            if (button.dataset.tagMenuAction === 'edit') {
                await this.editTag(id);
                return;
            }
            if (button.dataset.tagMenuAction === 'delete') {
                await this.deleteTag(id);
            }
        });

        document.addEventListener('click', (event) => {
            if (this.contextMenu.classList.contains('hidden')) return;
            if (event.target.closest('#tagContextMenu')) return;
            this.hideContextMenu();
        });

        document.addEventListener('keydown', (event) => {
            if (event.key === 'Escape') {
                this.hideContextMenu();
            }
        });

        document.addEventListener('scroll', () => {
            this.hideContextMenu();
        });

        this.scopeSwitch.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-tag-scope]');
            if (!button || button.dataset.tagScope === this.state.scope) return;
            this.state.scope = button.dataset.tagScope;
            this.state.selectedTagId = '';
            this.state.page = 1;
            await this.loadTags();
            this.hideContextMenu();
            this.renderTagCreator();
            this.renderScopeSwitch();
            this.renderTagCards();
            await this.loadResults();
        });

        this.resultList.addEventListener('click', async (event) => {
            if (this.state.scope === 'paper') {
                const action = event.target.closest('[data-action]');
                const card = event.target.closest('[data-paper-id]');
                if (!action || !card) return;
                await PaperViewer.open(Number(card.dataset.paperId), async () => await this.reload());
                return;
            }

            const action = event.target.closest('[data-action]');
            const card = event.target.closest('[data-figure-index]');
            if (!action || !card) return;

            const index = Number(card.dataset.figureIndex);
            if (action.dataset.action === 'note') {
                await this.figureActions.openNote(index);
                return;
            }
            if (action.dataset.action === 'preview') {
                await this.figureActions.openPreview(index);
                return;
            }
            if (action.dataset.action === 'paper') {
                await this.figureActions.openPaper(card.dataset.paperId);
            }
        });

        Utils.bindPagination(this.pagination, async (page) => {
            this.state.page = page;
            await this.loadResults();
        });
    },

    async reload() {
        await Promise.all([this.loadTags(), this.loadGlobalCounts()]);
        this.renderTagCreator();
        this.renderScopeSwitch();
        this.renderTagCards();
        await this.loadResults();
    },

    async loadTags() {
        const payload = await API.listTags({ scope: this.state.scope });
        this.tags = payload.tags || [];
        if (this.state.selectedTagId && !this.tags.some((tag) => String(tag.id) === String(this.state.selectedTagId))) {
            this.state.selectedTagId = '';
        }
    },

    renderTagCreator() {
        const isPaperScope = this.state.scope === 'paper';
        this.creatorTitle.textContent = isPaperScope ? '新建文献标签' : '新建图片标签';
        this.creatorHint.textContent = isPaperScope
            ? '给文献补充主题、方法或阅读状态等检索维度。'
            : '给图片补充内容、实验类型或局部特征等检索维度。';
        this.nameInput.placeholder = isPaperScope ? '例如：review' : '例如：细胞分裂';
        this.submitButton.textContent = isPaperScope ? '创建文献标签' : '创建图片标签';
        this.renderColorPresets();
        this.renderTagPresets();
    },

    renderTagPresets() {
        if (!this.tagPresetPanel || !this.tagPresetList) return;

        const isFigureScope = this.state.scope === 'figure';
        this.tagPresetPanel.hidden = !isFigureScope;
        if (!isFigureScope) {
            this.tagPresetList.innerHTML = '';
            return;
        }

        const presets = Array.isArray(Utils.defaultFigureTagPresets) ? Utils.defaultFigureTagPresets : [];
        const existing = new Set((this.tags || []).map((tag) => String(tag.name || '').trim().toLowerCase()).filter(Boolean));
        const selectedTag = this.tags.find((tag) => String(tag.id) === String(this.state.selectedTagId));
        const selectedName = String(selectedTag?.name || '').trim().toLowerCase();

        this.tagPresetList.innerHTML = presets.map((name) => {
            const normalized = String(name || '').trim();
            const key = normalized.toLowerCase();
            const isExisting = existing.has(key);
            const isActive = key && key === selectedName;
            let helper = '点击创建';
            if (isExisting) {
                helper = isActive ? '当前已选中' : '点击筛选';
            }
            return `
                <button
                    class="tag-preset-pill ${isExisting ? 'is-existing' : ''} ${isActive ? 'is-active' : ''}"
                    type="button"
                    data-tag-preset-name="${Utils.escapeHTML(normalized)}"
                    title="${helper}"
                >${Utils.escapeHTML(normalized)}</button>
            `;
        }).join('');
    },

    normalizeTagColor(value) {
        const normalized = String(value || '').trim().toUpperCase();
        if (/^#[0-9A-F]{6}$/.test(normalized)) {
            return normalized;
        }
        return this.defaultTagColor;
    },

    loadSavedTagColor() {
        try {
            return this.normalizeTagColor(window.localStorage.getItem(this.colorStorageKey));
        } catch (error) {
            return this.defaultTagColor;
        }
    },

    currentTagColor() {
        return this.normalizeTagColor(this.colorInput?.value || this.loadSavedTagColor());
    },

    setTagColor(color) {
        const normalized = this.normalizeTagColor(color);
        if (this.colorInput) {
            this.colorInput.value = normalized;
        }
        try {
            window.localStorage.setItem(this.colorStorageKey, normalized);
        } catch (error) {
            // Ignore storage failures and keep the current in-memory value.
        }
        this.syncColorPresetSelection(normalized);
    },

    renderColorPresets() {
        if (!this.colorPresetList) return;
        const currentColor = this.currentTagColor();
        this.colorPresetList.innerHTML = this.colorPresets.map((preset) => `
            <button
                class="tag-color-preset ${preset.value === currentColor ? 'active' : ''}"
                type="button"
                data-tag-color="${preset.value}"
                aria-label="${preset.label}"
                aria-pressed="${preset.value === currentColor ? 'true' : 'false'}"
                title="${preset.label}"
                style="--tag-preset-color:${preset.value}"
            ></button>
        `).join('');
    },

    syncColorPresetSelection(color = this.currentTagColor()) {
        if (!this.colorPresetList) return;
        this.colorPresetList.querySelectorAll('[data-tag-color]').forEach((button) => {
            const isActive = this.normalizeTagColor(button.dataset.tagColor) === color;
            button.classList.toggle('active', isActive);
            button.setAttribute('aria-pressed', isActive ? 'true' : 'false');
        });
    },

    async handleTagPreset(name) {
        const normalized = String(name || '').trim();
        if (!normalized || this.state.scope !== 'figure') return;

        const existing = (this.tags || []).find((tag) => String(tag.name || '').trim().toLowerCase() === normalized.toLowerCase());
        if (existing) {
            this.state.selectedTagId = String(existing.id);
            this.state.page = 1;
            this.renderTagCards();
            this.renderTagPresets();
            await this.loadResults();
            return;
        }

        try {
            await API.createTag({
                scope: 'figure',
                name: normalized,
                color: this.currentTagColor()
            });
            await this.loadTags();
            const created = (this.tags || []).find((tag) => String(tag.name || '').trim().toLowerCase() === normalized.toLowerCase());
            this.state.selectedTagId = created ? String(created.id) : '';
            this.state.page = 1;
            Utils.showToast(`已创建图片标签：${normalized}`);
            this.renderTagCreator();
            this.renderTagCards();
            await this.loadResults();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async loadGlobalCounts() {
        const [papersPayload, figuresPayload] = await Promise.all([
            API.listPapers({ page: 1, page_size: 1 }),
            API.listFigures({ page: 1, page_size: 1 })
        ]);
        this.state.totalPaperCount = papersPayload.total || 0;
        this.state.totalFigureCount = figuresPayload.total || 0;
    },

    renderScopeSwitch() {
        this.scopeSwitch.innerHTML = `
            <button class="btn ${this.state.scope === 'paper' ? 'btn-primary' : 'btn-outline'}" type="button" data-tag-scope="paper">文献标签</button>
            <button class="btn ${this.state.scope === 'figure' ? 'btn-primary' : 'btn-outline'}" type="button" data-tag-scope="figure">图片标签</button>
        `;
    },

    renderTagCards() {
        const isPaperScope = this.state.scope === 'paper';
        const totalCount = isPaperScope ? this.state.totalPaperCount : this.state.totalFigureCount;
        const allCard = `
            <article class="entity-card tag-entity-card ${this.state.selectedTagId ? '' : 'active'}" data-tag-id="" title="点击查看全部">
                <div class="tag-entity-main">
                    <div class="tag-entity-chip tag-entity-chip-global">
                        <span class="tag-entity-label">${isPaperScope ? '全部文献' : '全部图片'}</span>
                        <span class="tag-card-count">${totalCount}</span>
                    </div>
                </div>
            </article>
        `;
        this.grid.innerHTML = allCard + this.tags.map((tag) => `
            <article class="entity-card tag-entity-card ${String(tag.id) === String(this.state.selectedTagId) ? 'active' : ''}" data-tag-id="${tag.id}" title="点击筛选，右键可改名或删除">
                <div class="tag-entity-main">
                    <div class="tag-entity-chip">
                        <span class="tag-dot" style="background:${tag.color}"></span>
                        <span class="tag-entity-label">${Utils.escapeHTML(tag.name)}</span>
                        <span class="tag-card-count">${isPaperScope ? (tag.paper_count || 0) : (tag.figure_count || 0)}</span>
                    </div>
                </div>
            </article>
        `).join('');

        const current = this.tags.find((tag) => String(tag.id) === String(this.state.selectedTagId));
        this.headline.textContent = current
            ? `标签「${current.name}」下的${isPaperScope ? '文献' : '图片'}`
            : `全部${isPaperScope ? '文献标签' : '图片标签'}下的${isPaperScope ? '文献' : '图片'}`;
        this.scopeHint.textContent = isPaperScope
            ? '这里展示带有当前标签的文献列表。'
            : '这里展示带有当前标签的图片列表。';
    },

    showContextMenu(id, clientX, clientY) {
        if (!this.contextMenu) return;
        this.contextMenu.dataset.tagId = String(id);
        this.contextMenu.classList.remove('hidden');
        this.contextMenu.style.left = '0px';
        this.contextMenu.style.top = '0px';

        const rect = this.contextMenu.getBoundingClientRect();
        const left = Math.max(12, Math.min(clientX, window.innerWidth - rect.width - 12));
        const top = Math.max(12, Math.min(clientY, window.innerHeight - rect.height - 12));
        this.contextMenu.style.left = `${left}px`;
        this.contextMenu.style.top = `${top}px`;
    },

    hideContextMenu() {
        if (!this.contextMenu) return;
        this.contextMenu.classList.add('hidden');
        this.contextMenu.dataset.tagId = '';
    },

    pageSize() {
        return this.state.scope === 'paper' ? 20 : 12;
    },

    async loadResults() {
        if (this.state.scope === 'paper') {
            await this.loadPapers();
            return;
        }
        await this.loadFigures();
    },

    async loadPapers() {
        try {
            const payload = await API.listPapers({
                page: this.state.page,
                page_size: this.pageSize(),
                tag_id: this.state.selectedTagId
            });
            const totalPages = payload.total_pages || 0;
            this.state.page = totalPages ? Math.min(this.state.page, totalPages) : 1;
            const papers = payload.papers || [];
            this.resultList.className = 'paper-grid paper-list-mode';
            this.resultList.innerHTML = papers.length ? papers.map(BrowserUI.renderPaperCard).join('') : '<div class="empty-state"><h3>这个标签下还没有文献</h3></div>';
            BrowserUI.renderPagination(this.pagination, this.state.page, totalPages);
            this.figures = [];
            this.totalPages = totalPages;
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async fetchFigureResults(page = this.state.page) {
        return API.listFigures({
            page,
            page_size: this.pageSize(),
            tag_id: this.state.selectedTagId
        });
    },

    renderFigureResults(payload, page = this.state.page) {
        const figures = payload.figures || [];
        const totalPages = payload.total_pages || 0;
        this.state.page = totalPages ? Math.min(page, totalPages) : 1;
        this.figures = figures;
        this.totalPages = totalPages;
        this.resultList.className = 'figure-preview-grid';
        this.resultList.innerHTML = figures.length
            ? figures.map((figure, index) => BrowserUI.renderFigureCard(figure, index, {
                mediaAction: 'preview',
                primaryAction: 'note',
                showNotesPreview: true
            })).join('')
            : '<div class="empty-state"><h3>这个标签下还没有图片</h3></div>';
        BrowserUI.renderPagination(this.pagination, this.state.page, this.totalPages);
    },

    async loadFigures() {
        try {
            const payload = await this.fetchFigureResults(this.state.page);
            this.renderFigureResults(payload, this.state.page);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async editTag(id) {
        const tag = this.tags.find((item) => item.id === id);
        if (!tag) return;
        const values = await Utils.promptFields({
            title: '编辑标签',
            description: '可以同时调整标签名称和颜色。',
            confirmLabel: '保存标签',
            fields: [
                { name: 'name', label: '标签名称', value: tag.name || '', required: true, placeholder: '例如：review' },
                { name: 'color', label: '标签颜色', type: 'color', value: this.normalizeTagColor(tag.color || this.defaultTagColor), required: true }
            ]
        });
        if (!values) return;
        try {
            await API.updateTag(id, {
                name: values.name.trim(),
                color: this.normalizeTagColor(values.color)
            });
            Utils.showToast('标签已更新');
            await this.reload();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async deleteTag(id) {
        const confirmed = await Utils.confirm('删除标签后，相关关联也会一并移除。');
        if (!confirmed) return;
        try {
            await API.deleteTag(id);
            Utils.showToast('标签已删除');
            await this.reload();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    }
};

const NotesPage = {
    state: { mode: 'paper', page: 1, pageSize: 8, totalPages: 0, filters: { keyword: '', group_id: '', tag_id: '' } },

    async init() {
        PaperViewer.init();
        FigureViewer.init();
        NoteViewer.init();
        PaperNoteViewer.init();
        this.cache();
        this.bind();
        this.syncModeUI();
        await Promise.all([this.loadGroups(), this.loadTags()]);
        await this.load();
    },

    cache() {
        this.keywordInput = document.getElementById('notesKeywordInput');
        this.groupFilter = document.getElementById('notesGroupFilter');
        this.tagFilter = document.getElementById('notesTagFilter');
        this.tagFilterLabel = document.getElementById('notesTagFilterLabel');
        this.typeSwitch = document.getElementById('notesTypeSwitch');
        this.filterDescription = document.getElementById('notesFilterDescription');
        this.summaryStrip = document.getElementById('notesSummaryStrip');
        this.grid = document.getElementById('notesGrid');
        this.pagination = document.getElementById('notesPagination');
        this.pageControls = document.getElementById('notesPageControls');
    },

    bind() {
        this.figureActions = createFigureCollectionActions({
            getFigures: () => this.figures || [],
            getPage: () => this.state.page,
            getTotalPages: () => this.state.totalPages,
            loadPage: async (page) => {
                const payload = await this.fetchFigurePage(page);
                this.renderFigureResults(payload, page);
                return payload;
            },
            openPaper: async (paperID) => {
                await this.openPaper(Number(paperID));
            },
            onMetaChanged: async () => {
                await Promise.all([this.loadGroups(), this.loadTags(), this.load(this.state.page)]);
            }
        });

        const debouncedSearch = Utils.debounce(async () => {
            this.state.filters.keyword = this.keywordInput.value.trim();
            await this.load(1);
        }, 250);
        this.keywordInput.addEventListener('input', debouncedSearch);
        this.groupFilter.addEventListener('change', async () => {
            this.state.filters.group_id = this.groupFilter.value;
            await this.load(1);
        });
        this.tagFilter.addEventListener('change', async () => {
            this.state.filters.tag_id = this.tagFilter.value;
            await this.load(1);
        });
        this.typeSwitch.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-notes-mode]');
            if (!button) return;
            const nextMode = button.dataset.notesMode === 'figure' ? 'figure' : 'paper';
            if (nextMode === this.state.mode) return;
            this.state.mode = nextMode;
            this.state.page = 1;
            this.state.totalPages = 0;
            this.state.filters.tag_id = '';
            this.syncModeUI();
            await this.loadTags();
            await this.load(1);
        });
        this.grid.addEventListener('click', async (event) => {
            const action = event.target.closest('[data-action]');
            const card = event.target.closest('[data-note-kind]');
            if (!card || !action) return;

            if (card.dataset.noteKind === 'paper') {
                const paperID = Number(card.dataset.paperId);
                if (action.dataset.action === 'note') {
                    await this.openPaperNote(paperID);
                    return;
                }
                if (action.dataset.action === 'paper') {
                    await this.openPaper(paperID);
                    return;
                }
                if (action.dataset.action === 'ai') {
                    window.location.href = `/ai?paper_id=${paperID}`;
                }
                return;
            }

            const index = Number(card.dataset.figureIndex);
            if (action.dataset.action === 'note') {
                await this.figureActions.openNote(index);
                return;
            }
            if (action.dataset.action === 'preview') {
                await this.figureActions.openPreview(index);
                return;
            }
            if (action.dataset.action === 'paper') {
                await this.figureActions.openPaper(card.dataset.paperId);
            }
        });
        Utils.bindPagination(this.pagination, async (page) => await this.load(page));
        this.pageControls.addEventListener('click', async (event) => {
            const button = event.target.closest('button[data-page-step]');
            if (!button || button.disabled) return;

            const step = Number(button.dataset.pageStep);
            const nextPage = this.state.page + step;
            if (nextPage < 1 || nextPage > this.state.totalPages) return;

            await this.load(nextPage);
        });
    },

    syncModeUI() {
        const isPaperMode = this.state.mode === 'paper';
        this.keywordInput.placeholder = isPaperMode ? '文献标题、摘要、文献笔记、标签' : '文献标题、图片说明、图片标签、图片笔记';
        this.filterDescription.textContent = isPaperMode
            ? '这里只显示已经写过文献笔记的条目，你可以继续编辑、回看内容或跳转到来源文献。'
            : '这里只显示已经写过图片笔记的条目，你可以继续编辑、回看大图或跳转到来源文献。';
        this.tagFilterLabel.textContent = isPaperMode ? '文献标签' : '图片标签';

        Array.from(this.typeSwitch.querySelectorAll('[data-notes-mode]')).forEach((button) => {
            const active = button.dataset.notesMode === this.state.mode;
            button.classList.toggle('btn-primary', active);
            button.classList.toggle('btn-outline', !active);
            button.setAttribute('aria-pressed', active ? 'true' : 'false');
        });
    },

    currentTagScope() {
        return this.state.mode === 'paper' ? 'paper' : 'figure';
    },

    async loadGroups() {
        const payload = await API.listGroups();
        const selected = String(this.state.filters.group_id || '');
        this.groupFilter.innerHTML = '<option value="">全部分组</option>' + (payload.groups || []).map((group) => `
            <option value="${group.id}" ${String(group.id) === selected ? 'selected' : ''}>${Utils.escapeHTML(group.name)}</option>
        `).join('');
    },

    async loadTags() {
        const scope = this.currentTagScope();
        const payload = await API.listTags({ scope });
        const selected = String(this.state.filters.tag_id || '');
        const label = scope === 'paper' ? '全部文献标签' : '全部图片标签';
        this.tagFilter.innerHTML = `<option value="">${label}</option>` + (payload.tags || []).map((tag) => `
            <option value="${tag.id}" ${String(tag.id) === selected ? 'selected' : ''}>${Utils.escapeHTML(tag.name)}</option>
        `).join('');
    },

    buildPaperParams(page = this.state.page) {
        return {
            page,
            page_size: this.state.pageSize,
            keyword: this.state.filters.keyword,
            group_id: this.state.filters.group_id,
            tag_id: this.state.filters.tag_id,
            has_paper_notes: true
        };
    },

    buildFigureParams(page = this.state.page) {
        return {
            page,
            page_size: this.state.pageSize,
            keyword: this.state.filters.keyword,
            group_id: this.state.filters.group_id,
            tag_id: this.state.filters.tag_id,
            has_notes: true
        };
    },

    async fetchPaperPage(page = this.state.page) {
        return API.listPapers(this.buildPaperParams(page));
    },

    async fetchFigurePage(page = this.state.page) {
        return API.listFigures(this.buildFigureParams(page));
    },

    renderPageControls() {
        this.pageControls.innerHTML = this.state.totalPages > 1 ? `
            <button class="btn btn-outline" type="button" data-page-step="-1" ${this.state.page <= 1 ? 'disabled' : ''}>上一页</button>
            <span class="figure-page-indicator">第 ${this.state.page} / ${this.state.totalPages} 页</span>
            <button class="btn btn-outline" type="button" data-page-step="1" ${this.state.page >= this.state.totalPages ? 'disabled' : ''}>下一页</button>
        ` : '';
    },

    renderPaperResults(payload, page = this.state.page) {
        const papers = payload.papers || [];
        const totalPages = payload.total_pages || 0;
        this.state.page = totalPages ? Math.min(page, totalPages) : 1;
        this.papers = papers;
        this.state.totalPages = totalPages;
        this.summaryStrip.innerHTML = `
            <div class="stat-card"><span>带笔记文献</span><strong>${payload.total || 0}</strong></div>
            <div class="stat-card"><span>当前页</span><strong>${papers.length}</strong></div>
            <div class="stat-card"><span>来源分组</span><strong>${Utils.escapeHTML(this.groupFilter.selectedOptions[0]?.textContent || '全部分组')}</strong></div>
            <div class="stat-card"><span>文献标签</span><strong>${Utils.escapeHTML(this.tagFilter.selectedOptions[0]?.textContent || '全部文献标签')}</strong></div>
        `;
        this.renderPageControls();
        this.grid.innerHTML = papers.length
            ? papers.map((paper) => this.renderPaperNoteRow(paper)).join('')
            : '<div class="empty-state"><h3>还没有可管理的文献笔记</h3><p>先在 AI伴读或文献详情里沉淀文献笔记，再回到这里统一整理。</p></div>';
        BrowserUI.renderPagination(this.pagination, this.state.page, this.state.totalPages);
    },

    renderPaperNoteRow(paper) {
        const noteText = String(paper.paper_notes_text || '').trim().replace(/\s+/g, ' ');
        const preview = noteText.length > 320 ? noteText.slice(0, 320) + '...' : noteText;
        const tags = BrowserUI.renderTagChips(paper.tags || []);

        return `
            <article class="note-row note-row-paper" data-note-kind="paper" data-paper-id="${paper.id}">
                <div class="note-row-body">
                    <div class="note-row-head">
                        <span class="note-row-source" data-action="paper" role="button">${Utils.escapeHTML(paper.title)}</span>
                        <span class="note-row-page">${Utils.escapeHTML(paper.group_name || '未分组')} · 图片 ${paper.figure_count || 0}</span>
                    </div>
                    <div class="note-row-text" data-action="note" role="button">${Utils.escapeHTML(preview) || '<span class="muted">空笔记</span>'}</div>
                    <div class="note-row-foot">
                        <div class="note-row-tags">${tags}</div>
                        <div class="note-row-actions">
                            <button class="btn btn-small btn-primary" type="button" data-action="note">编辑笔记</button>
                            <button class="btn btn-small btn-outline" type="button" data-action="paper">文献详情</button>
                            <button class="btn btn-small btn-outline" type="button" data-action="ai">AI伴读</button>
                        </div>
                    </div>
                </div>
            </article>
        `;
    },

    renderFigureResults(payload, page = this.state.page) {
        const figures = payload.figures || [];
        const totalPages = payload.total_pages || 0;
        this.state.page = totalPages ? Math.min(page, totalPages) : 1;
        this.figures = figures;
        this.state.totalPages = totalPages;
        this.summaryStrip.innerHTML = `
            <div class="stat-card"><span>带笔记图片</span><strong>${payload.total || 0}</strong></div>
            <div class="stat-card"><span>当前页</span><strong>${figures.length}</strong></div>
            <div class="stat-card"><span>来源分组</span><strong>${Utils.escapeHTML(this.groupFilter.selectedOptions[0]?.textContent || '全部分组')}</strong></div>
            <div class="stat-card"><span>图片标签</span><strong>${Utils.escapeHTML(this.tagFilter.selectedOptions[0]?.textContent || '全部图片标签')}</strong></div>
        `;
        this.renderPageControls();
        this.grid.innerHTML = figures.length
            ? figures.map((figure, index) => this.renderFigureNoteRow(figure, index)).join('')
            : '<div class="empty-state"><h3>还没有可管理的图片笔记</h3><p>先在图片库里为图片补充笔记，再回到这里统一整理。</p></div>';
        BrowserUI.renderPagination(this.pagination, this.state.page, this.state.totalPages);
    },

    renderFigureNoteRow(figure, index) {
        const noteText = String(figure.notes_text || '').trim().replace(/\s+/g, ' ');
        const preview = noteText.length > 280 ? noteText.slice(0, 280) + '...' : noteText;
        const tags = BrowserUI.renderTagChips(figure.tags || []);

        return `
            <article class="note-row" data-note-kind="figure" data-paper-id="${figure.paper_id}" data-figure-index="${index}">
                <div class="note-row-thumb">
                    <button class="note-row-img" type="button" data-action="preview" aria-label="查看大图">
                        <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.paper_title || '')}">
                    </button>
                </div>
                <div class="note-row-body">
                    <div class="note-row-head">
                        <span class="note-row-source" data-action="paper" role="button">${Utils.escapeHTML(figure.paper_title)}</span>
                        <span class="note-row-page">第 ${figure.page_number || '-'} 页 · #${figure.figure_index || '-'}</span>
                    </div>
                    <div class="note-row-text" data-action="note" role="button">${Utils.escapeHTML(preview) || '<span class="muted">空笔记</span>'}</div>
                    <div class="note-row-foot">
                        <div class="note-row-tags">${tags}</div>
                        <div class="note-row-actions">
                            <button class="btn btn-small btn-primary" type="button" data-action="note">编辑笔记</button>
                            <button class="btn btn-small btn-outline" type="button" data-action="preview">大图</button>
                            <button class="btn btn-small btn-outline" type="button" data-action="paper">文献</button>
                        </div>
                    </div>
                </div>
            </article>
        `;
    },

    async openPaper(paperID) {
        await PaperViewer.open(Number(paperID), async () => {
            await Promise.all([this.loadGroups(), this.loadTags(), this.load(this.state.page)]);
        });
    },

    async openPaperNote(paperID) {
        const paper = await API.getPaper(Number(paperID));
        PaperNoteViewer.open({
            paper,
            onChanged: async () => {
                await Promise.all([this.loadGroups(), this.loadTags(), this.load(this.state.page)]);
            },
            onOpenPaper: async (targetPaperID) => {
                await this.openPaper(Number(targetPaperID));
            }
        });
    },

    async load(page = this.state.page) {
        try {
            if (this.state.mode === 'paper') {
                const payload = await this.fetchPaperPage(page);
                this.renderPaperResults(payload, page);
                return;
            }
            const payload = await this.fetchFigurePage(page);
            this.renderFigureResults(payload, page);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    }
};
