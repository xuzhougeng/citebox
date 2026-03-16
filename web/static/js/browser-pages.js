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
        return `
            <article class="paper-card" data-paper-id="${paper.id}">
                <div class="paper-card-head">
                    <span class="status-pill ${statusClass}">${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}</span>
                    <span class="muted">${Utils.formatDate(paper.created_at)}</span>
                </div>
                <h3>${Utils.escapeHTML(paper.title)}</h3>
                <p class="paper-filename">${Utils.escapeHTML(paper.original_filename)}</p>
                <div class="meta-row">
                    <span>分组</span>
                    <strong>${Utils.escapeHTML(paper.group_name || '未分组')}</strong>
                </div>
                <div class="meta-row">
                    <span>提取图片</span>
                    <strong>${paper.figure_count || 0}</strong>
                </div>
                <div class="chip-row">${tags}</div>
                ${paper.extractor_message ? `<p class="notice ${statusClass}">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}
                <div class="card-actions">
                    <button class="btn btn-primary" type="button" data-action="open">查看详情</button>
                </div>
            </article>
        `;
    },

    renderPagination(container, currentPage, totalPages) {
        if (!container) return;
        if (!totalPages || totalPages <= 1) {
            container.innerHTML = '';
            return;
        }

        const buttons = [];
        for (let page = 1; page <= totalPages; page += 1) {
            buttons.push(`
                <button class="${page === currentPage ? 'active' : ''}" type="button" data-page="${page}">
                    ${page}
                </button>
            `);
        }
        container.innerHTML = buttons.join('');
    }
};

const PaperViewer = {
    init() {
        this.modal = document.getElementById('paperModal');
        this.body = document.getElementById('paperModalBody');
        this.closeButton = document.getElementById('closePaperModal');
        if (!this.modal || this.initialized) return;
        this.initialized = true;

        this.closeButton.addEventListener('click', () => this.close());
        this.modal.addEventListener('click', (event) => {
            if (event.target === this.modal) {
                this.close();
            }
        });

        this.body.addEventListener('submit', async (event) => {
            const form = event.target.closest('#paperViewerForm');
            if (!form) return;
            event.preventDefault();
            await this.save();
        });

        this.body.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-modal-action]');
            if (!button) return;
            if (button.dataset.modalAction === 'reextract-paper') {
                await this.reextract();
            }
            if (button.dataset.modalAction === 'delete-paper') {
                await this.remove();
            }
        });
    },

    async open(id, onChanged) {
        this.init();
        this.onChanged = onChanged;
        try {
            const [paper, groupsPayload] = await Promise.all([API.getPaper(id), API.listGroups()]);
            this.paper = paper;
            this.groups = groupsPayload.groups || [];
            this.render();
            this.modal.classList.remove('hidden');
            document.body.classList.add('modal-open');
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    close() {
        if (!this.modal) return;
        this.modal.classList.add('hidden');
        document.body.classList.remove('modal-open');
    },

    render() {
        const paper = this.paper;
        const groupOptions = ['<option value="">未分组</option>']
            .concat(this.groups.map((group) => `
                <option value="${group.id}" ${String(group.id) === String(paper.group_id || '') ? 'selected' : ''}>
                    ${Utils.escapeHTML(group.name)}
                </option>
            `))
            .join('');
        const figures = paper.figures || [];
        const boxes = paper.boxes ? JSON.stringify(paper.boxes, null, 2) : '';
        const statusClass = Utils.statusTone(paper.extraction_status);
        const figureSection = figures.length ? figures.map((figure) => `
                        <figure class="figure-card">
                            <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.original_name || paper.title)}">
                            <figcaption>
                                <strong>第 ${figure.page_number || '-'} 页</strong>
                                <span>${Utils.escapeHTML(figure.caption || figure.original_name || '未命名图片')}</span>
                            </figcaption>
                        </figure>
                    `).join('') : `<p class="muted">${Utils.isProcessingStatus(paper.extraction_status) ? '后台解析完成后会在这里显示提取图片。' : '没有可展示的提取图片。'}</p>`;

        this.body.innerHTML = `
            <div class="detail-head">
                <div>
                    <p class="eyebrow">文献详情</p>
                    <h2>${Utils.escapeHTML(paper.title)}</h2>
                </div>
                <span class="status-pill ${statusClass}">
                    ${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}
                </span>
            </div>

            <form id="paperViewerForm" class="detail-form">
                <label class="field">
                    <span>标题</span>
                    <input id="paperViewerTitle" class="form-input" type="text" value="${Utils.escapeHTML(paper.title)}">
                </label>
                <label class="field">
                    <span>分组</span>
                    <select id="paperViewerGroup" class="form-input">${groupOptions}</select>
                </label>
                <label class="field">
                    <span>标签</span>
                    <input id="paperViewerTags" class="form-input" type="text" value="${Utils.escapeHTML(Utils.joinTags(paper.tags || []))}">
                </label>
                <div class="detail-actions">
                    <button class="btn btn-primary" type="submit">保存</button>
                    ${(paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') ? '<button class="btn btn-outline" type="button" data-modal-action="reextract-paper">重新解析</button>' : ''}
                    <button class="btn btn-outline danger" type="button" data-modal-action="delete-paper">删除文献</button>
                    <a class="btn btn-outline" href="${paper.pdf_url}" target="_blank" rel="noreferrer">打开 PDF</a>
                </div>
            </form>

            <div class="detail-meta-panel">
                <div><span>原始文件</span><strong>${Utils.escapeHTML(paper.original_filename)}</strong></div>
                <div><span>PDF 大小</span><strong>${Utils.formatFileSize(paper.file_size || 0)}</strong></div>
                <div><span>提取图片</span><strong>${figures.length}</strong></div>
                <div><span>当前标签</span><strong>${BrowserUI.renderTagChips(paper.tags || [])}</strong></div>
            </div>

            ${paper.extractor_message ? `<p class="notice ${statusClass}">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}

            <section class="detail-section">
                <div class="section-head">
                    <h3>提取图片</h3>
                    <span>${figures.length} 张</span>
                </div>
                <div class="figure-grid">
                    ${figureSection}
                </div>
            </section>

            <section class="detail-section">
                <div class="section-head">
                    <h3>框选结果</h3>
                </div>
                <pre class="code-block">${Utils.escapeHTML(boxes || '暂无框选结果')}</pre>
            </section>

            <section class="detail-section">
                <div class="section-head">
                    <h3>PDF 原文</h3>
                </div>
                <pre class="text-block">${Utils.escapeHTML(paper.pdf_text || '暂无 PDF 原文')}</pre>
            </section>
        `;
    },

    async save() {
        try {
            const payload = await API.updatePaper(this.paper.id, {
                title: document.getElementById('paperViewerTitle').value.trim(),
                group_id: document.getElementById('paperViewerGroup').value ? Number(document.getElementById('paperViewerGroup').value) : null,
                tags: Utils.splitTags(document.getElementById('paperViewerTags').value)
            });
            this.paper = payload.paper;
            Utils.showToast('文献信息已更新');
            this.render();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async remove() {
        const confirmed = await Utils.confirm('删除后会移除 PDF、提取图片以及相关关联。');
        if (!confirmed) return;
        try {
            await API.deletePaper(this.paper.id);
            Utils.showToast('文献已删除');
            this.close();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async reextract() {
        try {
            const payload = await API.reextractPaper(this.paper.id);
            this.paper = payload.paper;
            Utils.showToast('文献已重新提交解析', 'info');
            this.render();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    }
};

const FigureViewer = {
    init() {
        this.modal = document.getElementById('figureModal');
        this.body = document.getElementById('figureModalBody');
        this.closeButton = document.getElementById('closeFigureModal');
        if (!this.modal || this.initialized) return;
        this.initialized = true;

        this.handleKeydown = (event) => {
            if (!this.modal || this.modal.classList.contains('hidden')) return;
            if (event.key === 'Escape') {
                this.close();
            }
            if (event.key === 'ArrowLeft') {
                void this.previous();
            }
            if (event.key === 'ArrowRight') {
                void this.next();
            }
        };

        this.closeButton.addEventListener('click', () => this.close());
        this.modal.addEventListener('click', (event) => {
            if (event.target === this.modal) {
                this.close();
            }
        });
        this.body.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-figure-action]');
            if (!button) return;

            if (button.dataset.figureAction === 'prev') {
                await this.previous();
            }
            if (button.dataset.figureAction === 'next') {
                await this.next();
            }
            if (button.dataset.figureAction === 'open-paper' && this.currentFigure) {
                this.close();
                if (typeof this.onOpenPaper === 'function') {
                    await this.onOpenPaper(this.currentFigure.paper_id);
                }
            }
        });
        document.addEventListener('keydown', this.handleKeydown);
    },

    open(options = {}) {
        this.init();
        this.figures = Array.isArray(options.figures) ? options.figures : [];
        this.index = Math.max(0, Math.min(Number(options.index) || 0, this.figures.length - 1));
        this.page = Math.max(1, Number(options.page) || 1);
        this.totalPages = Math.max(1, Number(options.totalPages) || 1);
        this.loadPage = typeof options.loadPage === 'function' ? options.loadPage : null;
        this.onOpenPaper = options.onOpenPaper;
        this.loadingPage = false;
        this.render();
        this.modal.classList.remove('hidden');
        document.body.classList.add('modal-open');
    },

    close() {
        if (!this.modal) return;
        this.modal.classList.add('hidden');
        document.body.classList.remove('modal-open');
    },

    canMovePrevious() {
        return Boolean(this.figures?.length) && (this.index > 0 || this.page > 1);
    },

    canMoveNext() {
        return Boolean(this.figures?.length) && (this.index < this.figures.length - 1 || this.page < this.totalPages);
    },

    async previous() {
        if (!this.canMovePrevious() || this.loadingPage) return;
        if (this.index > 0) {
            this.index -= 1;
            this.render();
            return;
        }
        await this.loadAdjacentPage(this.page - 1, 'last');
    },

    async next() {
        if (!this.canMoveNext() || this.loadingPage) return;
        if (this.index < this.figures.length - 1) {
            this.index += 1;
            this.render();
            return;
        }
        await this.loadAdjacentPage(this.page + 1, 'first');
    },

    async loadAdjacentPage(targetPage, targetIndex) {
        if (!this.loadPage || targetPage < 1 || targetPage > this.totalPages) return;

        this.loadingPage = true;
        this.render();

        try {
            const payload = await this.loadPage(targetPage);
            const figures = payload?.figures || [];
            if (!figures.length) return;

            this.figures = figures;
            this.page = targetPage;
            this.totalPages = Math.max(1, Number(payload.total_pages) || this.totalPages);
            this.index = targetIndex === 'last' ? figures.length - 1 : 0;
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            this.loadingPage = false;
            this.render();
        }
    },

    render() {
        this.currentFigure = this.figures?.[this.index];
        if (!this.currentFigure) {
            this.body.innerHTML = '<div class="empty-state"><h3>没有可展示的图片</h3></div>';
            return;
        }

        const figure = this.currentFigure;
        const total = this.figures.length;
        const tags = BrowserUI.renderTagChips(figure.tags || []);
        const canPrev = this.canMovePrevious();
        const canNext = this.canMoveNext();

        this.body.innerHTML = `
            <div class="figure-lightbox">
                <section class="figure-lightbox-media-panel">
                    <div class="figure-lightbox-toolbar">
                        <div class="figure-lightbox-counter">第 ${this.index + 1} / ${total} 张 · 第 ${this.page} / ${this.totalPages} 页</div>
                        <div class="figure-lightbox-nav">
                            <button class="btn btn-outline" type="button" data-figure-action="prev" ${!canPrev || this.loadingPage ? 'disabled' : ''}>上一张</button>
                            <button class="btn btn-outline" type="button" data-figure-action="next" ${!canNext || this.loadingPage ? 'disabled' : ''}>下一张</button>
                        </div>
                    </div>
                    <div class="figure-lightbox-media">
                        <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.caption || figure.paper_title)}">
                    </div>
                </section>

                <aside class="figure-lightbox-side">
                    <div class="figure-lightbox-head">
                        <p class="eyebrow">Figure Preview</p>
                        <h2>${Utils.escapeHTML(figure.paper_title)}</h2>
                        <p class="figure-lightbox-caption">${Utils.escapeHTML(figure.caption || '未提供图片说明')}</p>
                    </div>

                    <div class="figure-lightbox-meta">
                        <div><span>来源文献</span><strong>${Utils.escapeHTML(figure.paper_title)}</strong></div>
                        <div><span>定位</span><strong>第 ${figure.page_number || '-'} 页 · #${figure.figure_index || '-'}</strong></div>
                        <div><span>分组</span><strong>${Utils.escapeHTML(figure.group_name || '未分组')}</strong></div>
                        <div><span>标签</span><strong>${tags}</strong></div>
                    </div>

                    <div class="figure-lightbox-actions">
                        <button class="btn btn-primary" type="button" data-figure-action="open-paper">查看来源文献</button>
                        <a class="btn btn-outline" href="${figure.image_url}" target="_blank" rel="noreferrer">打开原图</a>
                        <a class="btn btn-outline" href="${figure.image_url}" download="${Utils.escapeHTML(figure.filename || 'figure.png')}">下载图片</a>
                    </div>
                </aside>
            </div>
        `;
    }
};

const FiguresPage = {
    state: { page: 1, pageSize: 8, totalPages: 0, filters: { keyword: '', group_id: '', tag_id: '' } },

    async init() {
        PaperViewer.init();
        FigureViewer.init();
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
            if (action.dataset.action === 'preview') {
                FigureViewer.open({
                    figures: this.figures || [],
                    index,
                    page: this.state.page,
                    totalPages: this.state.totalPages,
                    loadPage: async (page) => {
                        const payload = await this.fetchFigurePage(page);
                        this.renderFigureResults(payload, page);
                        return payload;
                    },
                    onOpenPaper: async (paperID) => {
                        await PaperViewer.open(Number(paperID), async () => {
                            await Promise.all([this.loadGroups(), this.loadTags(), this.load(this.state.page)]);
                        });
                    }
                });
            }
            if (action.dataset.action === 'paper') {
                await PaperViewer.open(Number(card.dataset.paperId), async () => {
                    await Promise.all([this.loadGroups(), this.loadTags(), this.load(this.state.page)]);
                });
            }
        });
        this.pagination.addEventListener('click', async (event) => {
            const button = event.target.closest('button[data-page]');
            if (!button) return;
            await this.load(Number(button.dataset.page));
        });
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
        this.groupFilter.innerHTML = '<option value="">全部分组</option>' + (payload.groups || []).map((group) => `<option value="${group.id}">${Utils.escapeHTML(group.name)}</option>`).join('');
    },

    async loadTags() {
        const payload = await API.listTags();
        this.tagFilter.innerHTML = '<option value="">全部标签</option>' + (payload.tags || []).map((tag) => `<option value="${tag.id}">${Utils.escapeHTML(tag.name)}</option>`).join('');
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
        this.state.page = page;
        this.figures = figures;
        this.state.totalPages = payload.total_pages || 0;
        this.summaryStrip.innerHTML = `
            <div class="stat-card"><span>筛选结果</span><strong>${payload.total || 0}</strong></div>
            <div class="stat-card"><span>当前页图片</span><strong>${figures.length}</strong></div>
            <div class="stat-card"><span>分组筛选</span><strong>${Utils.escapeHTML(this.groupFilter.selectedOptions[0]?.textContent || '全部分组')}</strong></div>
            <div class="stat-card"><span>标签筛选</span><strong>${Utils.escapeHTML(this.tagFilter.selectedOptions[0]?.textContent || '全部标签')}</strong></div>
        `;
        this.pageControls.innerHTML = this.state.totalPages > 1 ? `
            <button class="btn btn-outline" type="button" data-page-step="-1" ${this.state.page <= 1 ? 'disabled' : ''}>Prev</button>
            <span class="figure-page-indicator">第 ${this.state.page} / ${this.state.totalPages} 页</span>
            <button class="btn btn-outline" type="button" data-page-step="1" ${this.state.page >= this.state.totalPages ? 'disabled' : ''}>Next</button>
        ` : '';
        this.grid.innerHTML = figures.length ? figures.map((figure, index) => `
            <article class="figure-preview-card" data-paper-id="${figure.paper_id}" data-figure-index="${index}">
                <div class="figure-preview-stage">
                    <button class="figure-preview-media" type="button" data-action="preview" aria-label="查看大图">
                        <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.paper_title || '提取图片')}">
                    </button>
                    <div class="figure-preview-badges">
                        <span class="figure-badge figure-badge-strong">第 ${figure.page_number || '-'} 页</span>
                        <span class="figure-badge">#${figure.figure_index || '-'}</span>
                        ${figure.group_name ? `<span class="figure-badge">${Utils.escapeHTML(figure.group_name)}</span>` : ''}
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
                    <div class="card-actions">
                        <button class="btn btn-primary" type="button" data-action="preview">查看大图</button>
                        <button class="btn btn-outline" type="button" data-action="paper">查看文献</button>
                        <a class="btn btn-outline" href="${figure.image_url}" target="_blank" rel="noreferrer">原图</a>
                    </div>
                </div>
            </article>
        `).join('') : '<div class="empty-state"><h3>没有可展示的图片</h3><p>先上传文献，或者调整筛选条件。</p></div>';
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
    state: { selectedGroupId: '', page: 1, pageSize: 12, totalPaperCount: 0 },

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

        this.pagination.addEventListener('click', async (event) => {
            const button = event.target.closest('button[data-page]');
            if (!button) return;
            this.state.page = Number(button.dataset.page);
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
            const papers = payload.papers || [];
            this.paperList.innerHTML = papers.length ? papers.map(BrowserUI.renderPaperCard).join('') : '<div class="empty-state"><h3>这个分组下还没有文献</h3></div>';
            BrowserUI.renderPagination(this.pagination, this.state.page, payload.total_pages || 0);
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
    state: { selectedTagId: '', page: 1, pageSize: 12, totalPaperCount: 0 },

    async init() {
        PaperViewer.init();
        this.cache();
        this.bind();
        await this.reload();
    },

    cache() {
        this.form = document.getElementById('tagPageForm');
        this.nameInput = document.getElementById('tagPageNameInput');
        this.colorInput = document.getElementById('tagPageColorInput');
        this.grid = document.getElementById('tagCardGrid');
        this.headline = document.getElementById('tagHeadline');
        this.paperList = document.getElementById('tagPaperList');
        this.pagination = document.getElementById('tagPagination');
    },

    bind() {
        this.form.addEventListener('submit', async (event) => {
            event.preventDefault();
            try {
                await API.createTag({
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
                await this.loadPapers();
                return;
            }
            if (action.dataset.action === 'edit-tag') {
                await this.editTag(Number(id));
            }
            if (action.dataset.action === 'delete-tag') {
                await this.deleteTag(Number(id));
            }
        });

        this.paperList.addEventListener('click', async (event) => {
            const action = event.target.closest('[data-action]');
            const card = event.target.closest('[data-paper-id]');
            if (!action || !card) return;
            await PaperViewer.open(Number(card.dataset.paperId), async () => await this.reload());
        });

        this.pagination.addEventListener('click', async (event) => {
            const button = event.target.closest('button[data-page]');
            if (!button) return;
            this.state.page = Number(button.dataset.page);
            await this.loadPapers();
        });
    },

    async reload() {
        await Promise.all([this.loadTags(), this.loadGlobalPaperCount()]);
        this.renderTagCards();
        await this.loadPapers();
    },

    async loadTags() {
        const payload = await API.listTags();
        this.tags = payload.tags || [];
        if (this.state.selectedTagId && !this.tags.some((tag) => String(tag.id) === String(this.state.selectedTagId))) {
            this.state.selectedTagId = '';
        }
    },

    async loadGlobalPaperCount() {
        const payload = await API.listPapers({ page: 1, page_size: 1 });
        this.state.totalPaperCount = payload.total || 0;
    },

    renderTagCards() {
        const allCard = `
            <article class="entity-card ${this.state.selectedTagId ? '' : 'active'}" data-tag-id="">
                <div><h3>全部标签</h3><p>查看所有标签下的文献</p></div>
                <strong>${this.state.totalPaperCount}</strong>
            </article>
        `;
        this.grid.innerHTML = allCard + this.tags.map((tag) => `
            <article class="entity-card ${String(tag.id) === String(this.state.selectedTagId) ? 'active' : ''}" data-tag-id="${tag.id}">
                <div>
                    <h3 class="tag-line"><span class="tag-dot" style="background:${tag.color}"></span>${Utils.escapeHTML(tag.name)}</h3>
                    <p>按这个标签浏览文献</p>
                </div>
                <div class="entity-card-actions">
                    <strong>${tag.paper_count}</strong>
                    <button class="ghost-btn" type="button" data-action="edit-tag">改名</button>
                    <button class="ghost-btn danger" type="button" data-action="delete-tag">删除</button>
                </div>
            </article>
        `).join('');

        const current = this.tags.find((tag) => String(tag.id) === String(this.state.selectedTagId));
        this.headline.textContent = current ? `标签「${current.name}」下的文献` : '全部标签下的文献';
    },

    async loadPapers() {
        try {
            const payload = await API.listPapers({
                page: this.state.page,
                page_size: this.state.pageSize,
                tag_id: this.state.selectedTagId
            });
            const papers = payload.papers || [];
            this.paperList.innerHTML = papers.length ? papers.map(BrowserUI.renderPaperCard).join('') : '<div class="empty-state"><h3>这个标签下还没有文献</h3></div>';
            BrowserUI.renderPagination(this.pagination, this.state.page, payload.total_pages || 0);
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
