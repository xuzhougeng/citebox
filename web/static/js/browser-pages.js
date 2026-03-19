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
                    <div class="card-actions paper-list-actions">
                        <button class="btn btn-primary" type="button" data-action="open">查看详情</button>
                        <a class="btn btn-outline" href="/manual?paper_id=${paper.id}">手动标注</a>
                    </div>
                </div>
            </article>
        `;
    },

    renderFigureNotePreview(noteText = '', emptyText = '还没有笔记，可把 AI 解读或人工观察先记在这里。') {
        const normalized = String(noteText || '').replace(/\s+/g, ' ').trim();
        if (!normalized) {
            return `
                <div class="figure-preview-note is-empty">
                    <span class="figure-preview-note-label">图片笔记</span>
                    <p class="figure-preview-note-text">${Utils.escapeHTML(emptyText)}</p>
                </div>
            `;
        }

        const excerpt = normalized.length > 120 ? `${normalized.slice(0, 120)}...` : normalized;
        return `
            <div class="figure-preview-note">
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

        const noteButtonClass = primaryAction === 'note' ? 'btn btn-primary' : 'btn btn-outline';
        const previewButtonClass = primaryAction === 'preview' ? 'btn btn-primary' : 'btn btn-outline';
        const hasNotes = Boolean(String(figure.notes_text || '').trim());
        const mediaLabel = mediaAction === 'note' ? '查看笔记' : '查看大图';

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
                        <strong class="figure-preview-title">${Utils.escapeHTML(figure.paper_title)}</strong>
                    </div>
                    <div class="figure-preview-tags ${figure.tags?.length ? '' : 'is-empty'}">
                        ${figure.tags?.length ? BrowserUI.renderTagChips(figure.tags || []) : '<span class="figure-preview-empty">无标签</span>'}
                    </div>
                    ${showNotesPreview ? BrowserUI.renderFigureNotePreview(figure.notes_text, emptyNotesText) : ''}
                    <div class="card-actions">
                        <button class="${noteButtonClass}" type="button" data-action="note">查看笔记</button>
                        <button class="${previewButtonClass}" type="button" data-action="preview">查看大图</button>
                        <button class="btn btn-outline" type="button" data-action="paper">查看文献</button>
                        <a class="btn btn-outline" href="${Utils.resourceViewerURL('image', figure.image_url)}">原图</a>
                    </div>
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
        const name = window.prompt('新的分组名称', group.name);
        if (name === null) return;
        const description = window.prompt('新的分组说明', group.description || '');
        if (description === null) return;
        try {
            await API.updateGroup(id, { name, description });
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
        this.creatorTitle = document.getElementById('tagCreatorTitle');
        this.creatorHint = document.getElementById('tagCreatorHint');
        this.submitButton = document.getElementById('tagPageSubmit');
        this.scopeSwitch = document.getElementById('tagScopeSwitch');
        this.grid = document.getElementById('tagCardGrid');
        this.headline = document.getElementById('tagHeadline');
        this.scopeHint = document.getElementById('tagScopeHint');
        this.resultList = document.getElementById('tagResultList');
        this.pagination = document.getElementById('tagPagination');
    },

    bind() {
        this.form.addEventListener('submit', async (event) => {
            event.preventDefault();
            try {
                await API.createTag({
                    scope: this.state.scope,
                    name: this.nameInput.value.trim(),
                    color: this.colorInput.value
                });
                this.form.reset();
                this.colorInput.value = '#A45C40';
                Utils.showToast('标签已创建');
                await this.reload();
            } catch (error) {
                Utils.showToast(error.message, 'error');
            }
        });

        this.grid.addEventListener('click', async (event) => {
            const card = event.target.closest('[data-tag-id]');
            if (!card) return;
            const action = event.target.closest('[data-action]');
            const id = card.dataset.tagId;
            if (!action) {
                this.state.selectedTagId = id;
                this.state.page = 1;
                this.renderTagCards();
                await this.loadResults();
                return;
            }
            if (action.dataset.action === 'edit-tag') {
                await this.editTag(Number(id));
            }
            if (action.dataset.action === 'delete-tag') {
                await this.deleteTag(Number(id));
            }
        });

        this.scopeSwitch.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-tag-scope]');
            if (!button || button.dataset.tagScope === this.state.scope) return;
            this.state.scope = button.dataset.tagScope;
            this.state.selectedTagId = '';
            this.state.page = 1;
            await this.loadTags();
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
            <article class="entity-card ${this.state.selectedTagId ? '' : 'active'}" data-tag-id="">
                <div><h3>${isPaperScope ? '全部文献标签' : '全部图片标签'}</h3><p>${isPaperScope ? '查看所有标签下的文献' : '查看所有标签下的图片'}</p></div>
                <strong>${totalCount}</strong>
            </article>
        `;
        this.grid.innerHTML = allCard + this.tags.map((tag) => `
            <article class="entity-card ${String(tag.id) === String(this.state.selectedTagId) ? 'active' : ''}" data-tag-id="${tag.id}">
                <div>
                    <h3 class="tag-line"><span class="tag-dot" style="background:${tag.color}"></span>${Utils.escapeHTML(tag.name)}</h3>
                    <p>${isPaperScope ? `关联文献 ${tag.paper_count || 0} 篇` : `关联图片 ${tag.figure_count || 0} 张`}</p>
                </div>
                <div class="entity-card-actions">
                    <strong>${isPaperScope ? (tag.paper_count || 0) : (tag.figure_count || 0)}</strong>
                    <button class="ghost-btn" type="button" data-action="edit-tag">改名</button>
                    <button class="ghost-btn danger" type="button" data-action="delete-tag">删除</button>
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
        const name = window.prompt('新的标签名称', tag.name);
        if (name === null) return;
        const color = window.prompt('新的标签颜色（HEX）', tag.color || '#A45C40');
        if (color === null) return;
        try {
            await API.updateTag(id, { name, color });
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
