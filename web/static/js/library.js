if (typeof window.t !== 'function') window.t = function(k,f){return f||k};
const LibraryPage = {
    state: {
        currentPage: 1,
        pageSize: 12,
        groups: [],
        tags: [],
        filters: {
            keyword: '',
            keyword_scope: 'full_text',
            group_id: '',
            tag_id: '',
            status: 'completed',
            sort_by: 'created_at'
        }
    },

    async init() {
        this.autoRefreshTimer = null;
        this.loadingPapers = false;
        this.readLaunchState();
        this.cacheElements();
        this.bindEvents();
        await Promise.all([
            this.loadGroups(),
            this.loadTags()
        ]);
        await this.loadPapers();
        await this.handleLaunchState();
    },

    readLaunchState() {
        const params = new URLSearchParams(window.location.search);
        const paperId = Number(params.get('paper_id') || 0);
        this.launchState = {
            paperId: paperId > 0 ? paperId : 0,
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
        this.launchState = { paperId: 0, fromDuplicate: false };
    },

    cacheElements() {
        this.keywordInput = document.getElementById('keywordInput');
        this.keywordScopeFilter = document.getElementById('keywordScopeFilter');
        this.groupFilter = document.getElementById('groupFilter');
        this.tagFilter = document.getElementById('tagFilter');
        this.statusFilter = document.getElementById('statusFilter');
        this.sortFilter = document.getElementById('sortFilter');
        this.paperList = document.getElementById('paperList');
        this.resultMeta = document.getElementById('libraryResultMeta');
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
            this.state.filters.keyword = this.keywordInput.value.trim();
            this.state.currentPage = 1;
            await this.loadPapers();
        }, 300);

        this.keywordInput.addEventListener('input', debouncedSearch);

        this.keywordScopeFilter.addEventListener('change', async () => {
            this.state.filters.keyword_scope = this.keywordScopeFilter.value || 'full_text';
            this.state.currentPage = 1;
            this.updateKeywordPlaceholder();
            await this.loadPapers();
        });

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

        this.sortFilter.addEventListener('change', async () => {
            this.state.filters.sort_by = this.sortFilter.value || 'created_at';
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

    renderResultMeta() {
        if (!this.resultMeta) return;

        const statusLabel = this.state.filters.status
            ? Utils.statusLabel(this.state.filters.status)
            : t('library.filter_all_status', '全部状态');
        const scopeLabel = this.keywordScopeLabel();
        const sortLabel = this.sortLabel();

        this.resultMeta.innerHTML = `
            <div>
                <p class="eyebrow">Result Set</p>
                <h2>${t('library.result_found', '找到')} ${this.state.total || 0} ${t('library.result_papers', '篇文献')}</h2>
                <p>${t('library.result_focus', '当前聚焦')}：${Utils.escapeHTML(statusLabel)}。${t('library.result_scope', '关键词检索范围')}：${Utils.escapeHTML(scopeLabel)}。${t('library.result_sort', '排序')}：${Utils.escapeHTML(sortLabel)}。</p>
            </div>
            <div class="library-result-meta-tags">
                <span class="tag-pill neutral">${t('library.result_current_page', '当前页')} ${this.state.papers.length} ${t('library.result_papers_unit', '篇')}</span>
                ${this.state.filters.keyword ? `<span class="tag-pill neutral">${t('library.result_keyword', '关键词')}：${Utils.escapeHTML(this.state.filters.keyword)}</span>` : ''}
                <span class="tag-pill neutral">${t('library.result_scope_label', '范围')}：${Utils.escapeHTML(scopeLabel)}</span>
                <span class="tag-pill neutral">${t('library.result_sort', '排序')}：${Utils.escapeHTML(sortLabel)}</span>
                ${this.state.filters.group_id ? `<span class="tag-pill neutral">${t('library.result_group_limited', '已限定分组')}</span>` : ''}
                ${this.state.filters.tag_id ? `<span class="tag-pill neutral">${t('library.result_tag_limited', '已限定标签')}</span>` : ''}
            </div>
        `;
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

    renderPaperList() {
        if (!this.state.papers.length) {
            this.paperList.innerHTML = `
                <div class="empty-state">
                    <h3>${t('library.empty_title', '还没有符合条件的文献')}</h3>
                    <p>${this.state.filters.status === 'completed' ? t('library.empty_completed', '当前还没有完成解析的文献，先上传 PDF 或把状态切回全部。') : t('library.empty_default', '先上传 PDF，或者调整当前的筛选条件。')}</p>
                    <a class="btn btn-primary" href="/upload">${t('library.btn_upload', '上传文献')}</a>
                </div>
            `;
            return;
        }

        this.paperList.innerHTML = this.state.papers.map((paper) => {
            const tags = (paper.tags || []).map((tag) => `<span class="chip" style="--chip-color:${tag.color}">${Utils.escapeHTML(tag.name)}</span>`).join('');
            const statusClass = Utils.statusTone(paper.extraction_status);
            const summary = paper.abstract_text || paper.paper_notes_text || paper.notes_text;

            return `
                <article class="paper-list-row" data-paper-id="${paper.id}">
                    <div class="paper-list-main">
                        <div class="paper-list-head">
                            <span class="status-pill ${statusClass}">${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}</span>
                            <h3>${Utils.escapeHTML(paper.title)}</h3>
                            <div class="paper-list-head-actions">
                                <button class="btn btn-outline danger" type="button" data-action="delete">${t('btn.delete', '删除')}</button>
                            </div>
                        </div>
                        <div class="paper-list-meta">
                            <span class="paper-list-meta-item paper-list-meta-file" data-action="open" role="button" title="${t('library.meta_click_detail', '点击查看详情')}">
                                <span class="paper-list-meta-label">${t('library.meta_file', '文件')}</span>
                                <span class="paper-list-meta-value">${Utils.escapeHTML(paper.original_filename)}</span>
                            </span>
                            <span class="paper-list-meta-item">
                                <span class="paper-list-meta-label">${t('library.meta_group', '分组')}</span>
                                <span class="paper-list-meta-value">${Utils.escapeHTML(paper.group_name || t('library.meta_no_group', '未分组'))}</span>
                            </span>
                            ${paper.doi ? `
                                <span class="paper-list-meta-item">
                                    <span class="paper-list-meta-label">${t('library.meta_doi', 'DOI')}</span>
                                    <span class="paper-list-meta-value">${Utils.escapeHTML(paper.doi)}</span>
                                </span>
                            ` : ''}
                            <span class="paper-list-meta-item">
                                <span class="paper-list-meta-label">${t('library.meta_figures', '图片')}</span>
                                <span class="paper-list-meta-value">${paper.figure_count || 0}</span>
                            </span>
                            <span class="paper-list-meta-item">
                                <span class="paper-list-meta-label">${t('library.meta_updated', '更新')}</span>
                                <span class="paper-list-meta-value">${Utils.formatDate(paper.updated_at || paper.created_at)}</span>
                            </span>
                        </div>
                        ${summary ? `<p class="paper-list-summary">${Utils.escapeHTML(summary)}</p>` : ''}
                        ${paper.extractor_message ? `<p class="notice ${statusClass} paper-list-notice">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}
                    </div>
                    <div class="paper-list-footer">
                        <div class="paper-list-tags">${tags || `<span class="muted">${t('library.meta_no_tags', '无标签')}</span>`}</div>
                        <div class="paper-list-footer-actions">
                            <a class="btn btn-outline" href="/manual?paper_id=${paper.id}">${t('library.btn_manual', '手动标注')}</a>
                            ${(paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') ? `<button class="btn btn-outline" type="button" data-action="reextract">${t('library.btn_reextract', '重新解析')}</button>` : ''}
                        </div>
                    </div>
                </article>
            `;
        }).join('');
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
