const LibraryPage = {
    state: {
        currentPage: 1,
        pageSize: 12,
        groups: [],
        tags: [],
        modalPaperId: null,
        filters: {
            keyword: '',
            group_id: '',
            tag_id: '',
            status: ''
        }
    },

    async init() {
        this.autoRefreshTimer = null;
        this.loadingPapers = false;
        this.cacheElements();
        this.bindEvents();
        await Promise.all([
            this.loadGroups(),
            this.loadTags()
        ]);
        await this.loadPapers();
    },

    cacheElements() {
        this.keywordInput = document.getElementById('keywordInput');
        this.groupFilter = document.getElementById('groupFilter');
        this.tagFilter = document.getElementById('tagFilter');
        this.statusFilter = document.getElementById('statusFilter');
        this.paperList = document.getElementById('paperList');
        this.summaryStrip = document.getElementById('summaryStrip');
        this.pagination = document.getElementById('pagination');
        this.groupForm = document.getElementById('groupForm');
        this.groupNameInput = document.getElementById('groupNameInput');
        this.groupDescriptionInput = document.getElementById('groupDescriptionInput');
        this.groupList = document.getElementById('groupList');
        this.tagForm = document.getElementById('tagForm');
        this.tagNameInput = document.getElementById('tagNameInput');
        this.tagColorInput = document.getElementById('tagColorInput');
        this.tagList = document.getElementById('tagList');
        this.paperModal = document.getElementById('paperModal');
        this.paperModalBody = document.getElementById('paperModalBody');
        this.closePaperModal = document.getElementById('closePaperModal');
    },

    bindEvents() {
        const debouncedSearch = Utils.debounce(async () => {
            this.state.filters.keyword = this.keywordInput.value.trim();
            this.state.currentPage = 1;
            await this.loadPapers();
        }, 300);

        this.keywordInput.addEventListener('input', debouncedSearch);

        this.groupFilter.addEventListener('change', async () => {
            this.state.filters.group_id = this.groupFilter.value;
            this.state.currentPage = 1;
            await this.loadPapers();
        });

        this.tagFilter.addEventListener('change', async () => {
            this.state.filters.tag_id = this.tagFilter.value;
            this.state.currentPage = 1;
            await this.loadPapers();
        });

        this.statusFilter.addEventListener('change', async () => {
            this.state.filters.status = this.statusFilter.value;
            this.state.currentPage = 1;
            await this.loadPapers();
        });

        this.paperList.addEventListener('click', async (event) => {
            const action = event.target.closest('[data-action]');
            if (!action) return;

            const card = event.target.closest('[data-paper-id]');
            if (!card) return;

            const paperId = Number(card.dataset.paperId);
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

        this.pagination.addEventListener('click', async (event) => {
            const button = event.target.closest('button[data-page]');
            if (!button) return;
            this.state.currentPage = Number(button.dataset.page);
            await this.loadPapers();
        });

        this.groupForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.createGroup();
        });

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

        this.tagForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.createTag();
        });

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

        this.closePaperModal.addEventListener('click', () => this.closeModal());
        this.paperModal.addEventListener('click', (event) => {
            if (event.target === this.paperModal) {
                this.closeModal();
            }
        });

        this.paperModalBody.addEventListener('submit', async (event) => {
            const form = event.target.closest('#paperEditForm');
            if (!form) return;
            event.preventDefault();
            await this.savePaper();
        });

        this.paperModalBody.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-modal-action]');
            if (!button) return;
            if (button.dataset.modalAction === 'reextract-paper') {
                await this.reextractPaper(this.state.modalPaperId, true);
            }
            if (button.dataset.modalAction === 'delete-paper') {
                await this.deletePaper(this.state.modalPaperId);
            }
        });

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
                group_id: this.state.filters.group_id,
                tag_id: this.state.filters.tag_id,
                status: this.state.filters.status
            });
            this.state.papers = payload.papers || [];
            this.state.total = payload.total || 0;
            this.state.totalPages = payload.total_pages || 0;
            this.renderSummary();
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
            const payload = await API.listTags();
            this.state.tags = payload.tags || [];
            this.renderTags();
            this.renderTagFilter();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    renderSummary() {
        const processing = this.state.papers.filter((paper) => Utils.isProcessingStatus(paper.extraction_status)).length;
        const completed = this.state.papers.filter((paper) => paper.extraction_status === 'completed').length;
        const failed = this.state.papers.filter((paper) => paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled').length;
        const figureTotal = this.state.papers.reduce((sum, paper) => sum + (paper.figure_count || 0), 0);

        this.summaryStrip.innerHTML = `
            <div class="stat-card">
                <span>当前页文献</span>
                <strong>${this.state.papers.length}</strong>
            </div>
            <div class="stat-card">
                <span>当前页图片</span>
                <strong>${figureTotal}</strong>
            </div>
            <div class="stat-card">
                <span>等待 / 解析中</span>
                <strong>${processing}</strong>
            </div>
            <div class="stat-card">
                <span>解析完成</span>
                <strong>${completed}</strong>
            </div>
            <div class="stat-card">
                <span>解析异常</span>
                <strong>${failed}</strong>
            </div>
        `;
    },

    renderPaperList() {
        if (!this.state.papers.length) {
            this.paperList.innerHTML = `
                <div class="empty-state">
                    <h3>还没有符合条件的文献</h3>
                    <p>先上传 PDF，或者调整当前的筛选条件。</p>
                    <a class="btn btn-primary" href="/upload">上传文献</a>
                </div>
            `;
            return;
        }

        this.paperList.innerHTML = this.state.papers.map((paper) => {
            const tags = (paper.tags || []).map((tag) => `<span class="chip" style="--chip-color:${tag.color}">${Utils.escapeHTML(tag.name)}</span>`).join('');
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
                    <div class="chip-row">${tags || '<span class="muted">无 Tag</span>'}</div>
                    ${paper.extractor_message ? `<p class="notice ${statusClass}">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}
                    <div class="card-actions">
                        <button class="btn btn-primary" type="button" data-action="open">查看详情</button>
                        ${(paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') ? '<button class="btn btn-outline" type="button" data-action="reextract">重新解析</button>' : ''}
                        <button class="btn btn-outline danger" type="button" data-action="delete">删除</button>
                    </div>
                </article>
            `;
        }).join('');
    },

    renderPagination() {
        if (!this.state.totalPages || this.state.totalPages <= 1) {
            this.pagination.innerHTML = '';
            return;
        }

        const buttons = [];
        for (let page = 1; page <= this.state.totalPages; page += 1) {
            buttons.push(`
                <button class="${page === this.state.currentPage ? 'active' : ''}" type="button" data-page="${page}">
                    ${page}
                </button>
            `);
        }
        this.pagination.innerHTML = buttons.join('');
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
        this.groupFilter.innerHTML = '<option value="">全部分组</option>';
        this.state.groups.forEach((group) => {
            this.groupFilter.insertAdjacentHTML(
                'beforeend',
                `<option value="${group.id}" ${String(group.id) === String(current) ? 'selected' : ''}>${Utils.escapeHTML(group.name)}</option>`
            );
        });
    },

    renderTagFilter() {
        const current = this.state.filters.tag_id;
        this.tagFilter.innerHTML = '<option value="">全部标签</option>';
        this.state.tags.forEach((tag) => {
            this.tagFilter.insertAdjacentHTML(
                'beforeend',
                `<option value="${tag.id}" ${String(tag.id) === String(current) ? 'selected' : ''}>${Utils.escapeHTML(tag.name)}</option>`
            );
        });
    },

    renderGroups() {
        this.groupList.innerHTML = this.state.groups.map((group) => `
            <div class="manager-item">
                <div>
                    <strong>${Utils.escapeHTML(group.name)}</strong>
                    <p>${Utils.escapeHTML(group.description || '无描述')}</p>
                </div>
                <div class="manager-item-actions">
                    <span>${group.paper_count}</span>
                    <button class="ghost-btn" type="button" data-action="edit-group" data-id="${group.id}">改名</button>
                    <button class="ghost-btn danger" type="button" data-action="delete-group" data-id="${group.id}">删除</button>
                </div>
            </div>
        `).join('') || '<p class="muted">暂无分组</p>';
    },

    renderTags() {
        this.tagList.innerHTML = this.state.tags.map((tag) => `
            <div class="manager-item">
                <div class="tag-line">
                    <span class="tag-dot" style="background:${tag.color}"></span>
                    <strong>${Utils.escapeHTML(tag.name)}</strong>
                </div>
                <div class="manager-item-actions">
                    <span>${tag.paper_count}</span>
                    <button class="ghost-btn" type="button" data-action="edit-tag" data-id="${tag.id}">改名</button>
                    <button class="ghost-btn danger" type="button" data-action="delete-tag" data-id="${tag.id}">删除</button>
                </div>
            </div>
        `).join('') || '<p class="muted">暂无标签</p>';
    },

    async createGroup() {
        try {
            await API.createGroup({
                name: this.groupNameInput.value.trim(),
                description: this.groupDescriptionInput.value.trim()
            });
            this.groupForm.reset();
            Utils.showToast('分组已创建');
            await this.loadGroups();
            await this.loadPapers();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async editGroup(id) {
        const group = this.state.groups.find((item) => item.id === id);
        if (!group) return;
        const name = window.prompt('新的分组名称', group.name);
        if (name === null) return;
        const description = window.prompt('新的分组说明', group.description || '');
        if (description === null) return;

        try {
            await API.updateGroup(id, { name, description });
            Utils.showToast('分组已更新');
            await this.loadGroups();
            await this.loadPapers();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async deleteGroup(id) {
        const confirmed = await Utils.confirm('删除分组后，文献只会失去分组，不会删除文献本身。');
        if (!confirmed) return;

        try {
            await API.deleteGroup(id);
            Utils.showToast('分组已删除');
            await this.loadGroups();
            await this.loadPapers();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async createTag() {
        try {
            await API.createTag({
                name: this.tagNameInput.value.trim(),
                color: this.tagColorInput.value
            });
            this.tagForm.reset();
            this.tagColorInput.value = '#A45C40';
            Utils.showToast('标签已创建');
            await this.loadTags();
            await this.loadPapers();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async editTag(id) {
        const tag = this.state.tags.find((item) => item.id === id);
        if (!tag) return;
        const name = window.prompt('新的标签名称', tag.name);
        if (name === null) return;
        const color = window.prompt('新的标签颜色（HEX）', tag.color || '#A45C40');
        if (color === null) return;

        try {
            await API.updateTag(id, { name, color });
            Utils.showToast('标签已更新');
            await this.loadTags();
            await this.loadPapers();
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
            await this.loadTags();
            await this.loadPapers();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async openPaperModal(id) {
        try {
            const paper = await API.getPaper(id);
            this.state.modalPaperId = id;
            this.renderPaperModal(paper);
            this.paperModal.classList.remove('hidden');
            document.body.classList.add('modal-open');
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    closeModal() {
        this.state.modalPaperId = null;
        this.paperModal.classList.add('hidden');
        document.body.classList.remove('modal-open');
    },

    renderPaperModal(paper) {
        const groupOptions = ['<option value="">未分组</option>']
            .concat(this.state.groups.map((group) => `
                <option value="${group.id}" ${String(group.id) === String(paper.group_id || '') ? 'selected' : ''}>
                    ${Utils.escapeHTML(group.name)}
                </option>
            `))
            .join('');

        const tags = (paper.tags || []).map((tag) => `<span class="chip" style="--chip-color:${tag.color}">${Utils.escapeHTML(tag.name)}</span>`).join('');
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

        this.paperModalBody.innerHTML = `
            <div class="detail-head">
                <div>
                    <p class="eyebrow">文献详情</p>
                    <h2>${Utils.escapeHTML(paper.title)}</h2>
                </div>
                <span class="status-pill ${statusClass}">
                    ${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}
                </span>
            </div>

            <form id="paperEditForm" class="detail-form">
                <label class="field">
                    <span>标题</span>
                    <input id="paperTitleInput" class="form-input" type="text" value="${Utils.escapeHTML(paper.title)}">
                </label>
                <label class="field">
                    <span>分组</span>
                    <select id="paperGroupInput" class="form-input">${groupOptions}</select>
                </label>
                <label class="field">
                    <span>标签</span>
                    <input id="paperTagsInput" class="form-input" type="text" value="${Utils.escapeHTML(Utils.joinTags(paper.tags || []))}">
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
                <div><span>当前标签</span><strong>${tags || '无'}</strong></div>
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

    async savePaper() {
        if (!this.state.modalPaperId) return;

        try {
            const payload = await API.updatePaper(this.state.modalPaperId, {
                title: document.getElementById('paperTitleInput').value.trim(),
                group_id: document.getElementById('paperGroupInput').value ? Number(document.getElementById('paperGroupInput').value) : null,
                tags: Utils.splitTags(document.getElementById('paperTagsInput').value)
            });
            Utils.showToast('文献信息已更新');
            await Promise.all([this.loadGroups(), this.loadTags(), this.loadPapers()]);
            this.renderPaperModal(payload.paper);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async deletePaper(id) {
        const confirmed = await Utils.confirm('删除后会移除 PDF、提取图片以及相关标签关系。');
        if (!confirmed) return;

        try {
            await API.deletePaper(id);
            Utils.showToast('文献已删除');
            this.closeModal();
            await Promise.all([this.loadGroups(), this.loadTags(), this.loadPapers()]);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async reextractPaper(id, keepModalOpen = false) {
        try {
            const payload = await API.reextractPaper(id);
            Utils.showToast('文献已重新提交解析', 'info');
            await Promise.all([this.loadGroups(), this.loadTags(), this.loadPapers()]);
            if (keepModalOpen) {
                this.state.modalPaperId = id;
                this.renderPaperModal(payload.paper);
                this.paperModal.classList.remove('hidden');
                document.body.classList.add('modal-open');
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    }
};
