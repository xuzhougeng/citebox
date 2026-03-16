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
        const summary = paper.abstract_text || paper.notes_text;
        return `
            <article class="paper-list-row" data-paper-id="${paper.id}">
                <div class="paper-list-main">
                    <div class="paper-list-head">
                        <span class="status-pill ${statusClass}">${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}</span>
                        <h3>${Utils.escapeHTML(paper.title)}</h3>
                    </div>
                    <div class="paper-list-meta">
                        <span>文件：${Utils.escapeHTML(paper.original_filename)}</span>
                        <span>分组：${Utils.escapeHTML(paper.group_name || '未分组')}</span>
                        <span>图片：${paper.figure_count || 0}</span>
                        <span>更新：${Utils.formatDate(paper.updated_at || paper.created_at)}</span>
                    </div>
                    ${summary ? `<p class="paper-list-summary">${Utils.escapeHTML(summary)}</p>` : ''}
                    ${paper.extractor_message ? `<p class="notice ${statusClass} paper-list-notice">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}
                </div>
                <div class="paper-list-side">
                    <div class="paper-list-tags">${tags}</div>
                    <div class="card-actions paper-list-actions">
                        <button class="btn btn-primary" type="button" data-action="open">查看详情</button>
                    </div>
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

const FigureViewer = {
    init() {
        this.modal = document.getElementById('figureModal');
        this.body = document.getElementById('figureModalBody');
        this.closeButton = document.getElementById('closeFigureModal');
        if (!this.modal) {
            const shell = document.createElement('div');
            shell.id = 'figureModal';
            shell.className = 'modal-shell hidden';
            shell.innerHTML = `
                <div class="modal-dialog figure-modal-dialog">
                    <button id="closeFigureModal" class="modal-close" type="button" aria-label="关闭">×</button>
                    <div id="figureModalBody"></div>
                </div>
            `;
            document.body.appendChild(shell);
            this.modal = shell;
            this.body = shell.querySelector('#figureModalBody');
            this.closeButton = shell.querySelector('#closeFigureModal');
        }
        if (!this.modal || this.initialized) return;
        this.initialized = true;
        this.aiCache = new Map();
        this.activeAIByPaper = new Map();
        this.aiRequestState = null;
        this.groups = [];
        this.paperDetails = new Map();

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
            if (button) {
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
                return;
            }

            const aiButton = event.target.closest('[data-figure-ai-action]');
            if (aiButton) {
                await this.runAIAction(aiButton.dataset.figureAiAction);
                return;
            }

            const stopButton = event.target.closest('[data-figure-ai-stop]');
            if (stopButton) {
                this.stopAIAction();
                return;
            }

            const copyButton = event.target.closest('[data-figure-ai-copy]');
            if (copyButton) {
                await this.copyAIResult(copyButton.dataset.figureAiCopy);
                return;
            }

            const metaButton = event.target.closest('[data-figure-meta-action]');
            if (metaButton) {
                await this.handleMetaAction(metaButton);
            }
        });
        this.body.addEventListener('change', async (event) => {
            const groupSelect = event.target.closest('#figurePaperGroup');
            if (!groupSelect || !this.currentFigure) return;
            await this.updateCurrentPaperMetadata(this.currentDraftMetadata(), '文献分组已更新');
        });
        this.body.addEventListener('keydown', async (event) => {
            const tagInput = event.target.closest('#figurePaperTagInput');
            if (!tagInput || event.key !== 'Enter') return;
            event.preventDefault();
            await this.addTagFromInput();
        });
        document.addEventListener('keydown', this.handleKeydown);
    },

    async open(options = {}) {
        this.init();
        this.figures = Array.isArray(options.figures) ? options.figures : [];
        this.index = Math.max(0, Math.min(Number(options.index) || 0, this.figures.length - 1));
        this.page = Math.max(1, Number(options.page) || 1);
        this.totalPages = Math.max(1, Number(options.totalPages) || 1);
        this.loadPage = typeof options.loadPage === 'function' ? options.loadPage : null;
        this.onOpenPaper = options.onOpenPaper;
        this.onMetaChanged = options.onMetaChanged;
        this.loadingPage = false;
        try {
            await this.ensureGroupsLoaded();
            this.render();
            this.modal.classList.remove('hidden');
            document.body.classList.add('modal-open');
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    close() {
        this.stopAIAction({ preservePartial: false, silent: true });
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
        if (!this.canMovePrevious() || this.loadingPage || this.aiRequestState?.loading) return;
        if (this.index > 0) {
            this.index -= 1;
            this.render();
            return;
        }
        await this.loadAdjacentPage(this.page - 1, 'last');
    },

    async next() {
        if (!this.canMoveNext() || this.loadingPage || this.aiRequestState?.loading) return;
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

    aiCacheKey(paperID, action) {
        return `${paperID}:${action}`;
    },

    activeAIAction() {
        if (!this.currentFigure?.paper_id) return '';
        return this.activeAIByPaper.get(this.currentFigure.paper_id) || '';
    },

    currentAIResult() {
        if (!this.currentFigure?.paper_id) return null;
        const action = this.activeAIAction();
        if (!action) return null;
        return this.aiCache.get(this.aiCacheKey(this.currentFigure.paper_id, action)) || null;
    },

    refreshAIResultPanel() {
        if (!this.modal || this.modal.classList.contains('hidden')) return;
        const panel = this.body.querySelector('[data-figure-ai-panel]');
        if (!panel) return;
        panel.innerHTML = this.renderAIResultPanel();
    },

    refreshAIActions() {
        if (!this.modal || this.modal.classList.contains('hidden')) return;
        const actions = this.body.querySelector('[data-figure-ai-actions]');
        if (!actions) return;
        actions.innerHTML = this.renderAIActionButtons();
    },

    refreshNavigationButtons() {
        if (!this.modal || this.modal.classList.contains('hidden')) return;
        const prevButton = this.body.querySelector('[data-figure-action="prev"]');
        const nextButton = this.body.querySelector('[data-figure-action="next"]');
        if (prevButton) {
            prevButton.disabled = !this.canMovePrevious() || this.loadingPage || Boolean(this.aiRequestState?.loading);
        }
        if (nextButton) {
            nextButton.disabled = !this.canMoveNext() || this.loadingPage || Boolean(this.aiRequestState?.loading);
        }
    },

    refreshAIState() {
        this.refreshAIActions();
        this.refreshAIResultPanel();
        this.refreshNavigationButtons();
    },

    async runAIAction(action) {
        if (!this.currentFigure || this.aiRequestState?.loading) return;

        const paperID = Number(this.currentFigure.paper_id);
        const cacheKey = this.aiCacheKey(paperID, action);
        this.activeAIByPaper.set(paperID, action);

        if (this.aiCache.has(cacheKey)) {
            this.aiRequestState = null;
            this.refreshAIState();
            return;
        }

        this.aiRequestState = {
            loading: true,
            paperID,
            action
        };
        this.refreshAIState();

        if (this.isStreamingAction(action)) {
            await this.runStreamingAIAction(action, paperID, cacheKey);
            return;
        }

        await this.runBufferedAIAction(action, paperID, cacheKey);
    },

    async runBufferedAIAction(action, paperID, cacheKey) {
        try {
            const result = await API.readPaperWithAI({
                paper_id: paperID,
                action,
                question: this.buildAIQuestion(action, this.currentFigure)
            });
            this.aiCache.set(cacheKey, result);
            this.aiRequestState = null;
            this.refreshAIState();
        } catch (error) {
            this.aiRequestState = {
                loading: false,
                paperID,
                action,
                error: error.message
            };
            this.refreshAIState();
            Utils.showToast(error.message, 'error');
        }
    },

    async runStreamingAIAction(action, paperID, cacheKey) {
        const requestState = {
            loading: true,
            paperID,
            action,
            answer: '',
            provider: '',
            model: '',
            mode: '',
            includedFigures: 0,
            abortController: new AbortController(),
            stopped: false,
            silentAbort: false,
            discardOnAbort: false
        };
        this.aiRequestState = requestState;
        this.refreshAIState();

        try {
            await API.readPaperWithAIStream({
                paper_id: paperID,
                action,
                question: this.buildAIQuestion(action, this.currentFigure)
            }, {
                signal: requestState.abortController.signal,
                onEvent: (event) => {
                    if (this.aiRequestState !== requestState) return;

                    if (event.type === 'error') {
                        throw new Error(event.error || '流式解读失败');
                    }
                    if (event.type === 'meta' && event.result) {
                        requestState.provider = event.result.provider || '';
                        requestState.model = event.result.model || '';
                        requestState.mode = event.result.mode || '';
                        requestState.includedFigures = event.result.included_figures || 0;
                        this.refreshAIResultPanel();
                        return;
                    }
                    if (event.type === 'delta') {
                        requestState.answer += event.delta || '';
                        this.refreshAIResultPanel();
                        return;
                    }
                    if (event.type === 'final' && event.result) {
                        this.aiCache.set(cacheKey, event.result);
                        this.aiRequestState = null;
                        this.refreshAIState();
                    }
                }
            });
        } catch (error) {
            if (error.name === 'AbortError') {
                if (this.aiRequestState !== requestState) return;
                if (requestState.discardOnAbort) {
                    this.aiRequestState = null;
                    return;
                }
                requestState.loading = false;
                requestState.stopped = true;
                delete requestState.abortController;
                this.aiRequestState = requestState;
                if (!requestState.silentAbort) {
                    this.refreshAIState();
                }
                return;
            }

            if (this.aiRequestState !== requestState) return;
            this.aiRequestState = {
                loading: false,
                paperID,
                action,
                answer: requestState.answer || '',
                provider: requestState.provider || '',
                model: requestState.model || '',
                mode: requestState.mode || '',
                includedFigures: requestState.includedFigures || 0,
                error: error.message
            };
            this.refreshAIState();
            Utils.showToast(error.message, 'error');
        }
    },

    stopAIAction(options = {}) {
        if (!this.aiRequestState?.loading || !this.aiRequestState.abortController) return;
        this.aiRequestState.discardOnAbort = options.preservePartial === false;
        this.aiRequestState.silentAbort = Boolean(options.silent);
        this.aiRequestState.abortController.abort();
    },

    isStreamingAction(action) {
        return action === 'figure_interpretation';
    },

    async ensureGroupsLoaded(force = false) {
        if (!force && this.groups.length > 0) return this.groups;
        const payload = await API.listGroups();
        this.groups = payload.groups || [];
        return this.groups;
    },

    async ensureCurrentPaperDetail() {
        const paperID = Number(this.currentFigure?.paper_id);
        if (!paperID) return null;
        if (this.paperDetails.has(paperID)) {
            return this.paperDetails.get(paperID);
        }

        const paper = await API.getPaper(paperID);
        this.paperDetails.set(paperID, paper);
        return paper;
    },

    currentDraftMetadata() {
        const groupSelect = this.body.querySelector('#figurePaperGroup');
        return {
            groupID: groupSelect?.value ? Number(groupSelect.value) : null,
            tags: Utils.splitTags(Utils.joinTags(this.currentFigure?.tags || []))
        };
    },

    async handleMetaAction(button) {
        if (!this.currentFigure) return;

        if (button.dataset.figureMetaAction === 'apply-tag') {
            await this.applySuggestedTag(button.dataset.tagName || '');
            return;
        }
        if (button.dataset.figureMetaAction === 'apply-group') {
            await this.applySuggestedGroup(button.dataset.groupName || '');
            return;
        }
        if (button.dataset.figureMetaAction === 'add-tag') {
            await this.addTagFromInput();
            return;
        }
        if (button.dataset.figureMetaAction === 'remove-tag') {
            await this.removeTag(button.dataset.tagName || '');
        }
    },

    async applySuggestedTag(tagName) {
        const normalized = tagName.trim();
        if (!normalized) return;

        const draft = this.currentDraftMetadata();
        const existing = new Set(draft.tags.map((tag) => tag.toLowerCase()));
        if (existing.has(normalized.toLowerCase())) {
            Utils.showToast('这个标签已经存在', 'info');
            return;
        }

        draft.tags.push(normalized);
        await this.updateCurrentPaperMetadata(draft, `已添加标签：${normalized}`);
    },

    async addTagFromInput() {
        const input = this.body.querySelector('#figurePaperTagInput');
        const value = input?.value.trim() || '';
        if (!value) return;
        await this.applySuggestedTag(value);
    },

    async removeTag(tagName) {
        const normalized = tagName.trim();
        if (!normalized) return;

        const draft = this.currentDraftMetadata();
        draft.tags = draft.tags.filter((tag) => tag.toLowerCase() !== normalized.toLowerCase());
        await this.updateCurrentPaperMetadata(draft, `已移除标签：${normalized}`);
    },

    async applySuggestedGroup(groupName) {
        const normalized = groupName.trim();
        if (!normalized) return;

        await this.ensureGroupsLoaded();
        let group = this.groups.find((item) => item.name.trim().toLowerCase() === normalized.toLowerCase());
        if (!group) {
            const payload = await API.createGroup({ name: normalized, description: '' });
            group = payload.group;
            await this.ensureGroupsLoaded(true);
        }

        const draft = this.currentDraftMetadata();
        draft.groupID = group?.id || null;
        await this.updateCurrentPaperMetadata(draft, `已加入分组：${normalized}`);
    },

    async updateCurrentPaperMetadata(draft, successMessage) {
        if (!this.currentFigure) return;

        try {
            const paper = await this.ensureCurrentPaperDetail();
            const payload = await API.updatePaper(this.currentFigure.paper_id, {
                title: paper.title,
                abstract_text: paper.abstract_text || '',
                notes_text: paper.notes_text || '',
                group_id: draft.groupID,
                tags: draft.tags
            });
            this.syncPaperMetadata(payload.paper);
            Utils.showToast(successMessage);
            this.render();
            if (typeof this.onMetaChanged === 'function') {
                await this.onMetaChanged(payload.paper);
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    syncPaperMetadata(paper) {
        this.paperDetails.set(paper.id, paper);
        this.figures = (this.figures || []).map((figure) => {
            if (Number(figure.paper_id) !== Number(paper.id)) return figure;
            return {
                ...figure,
                paper_title: paper.title,
                group_id: paper.group_id,
                group_name: paper.group_name || '',
                tags: paper.tags || []
            };
        });
    },

    async copyAIResult(kind) {
        const text = this.copyTextForCurrentResult(kind);
        if (!text) {
            Utils.showToast('当前没有可复制的内容', 'error');
            return;
        }

        try {
            await navigator.clipboard.writeText(text);
            Utils.showToast('已复制');
        } catch (error) {
            Utils.showToast('复制失败', 'error');
        }
    },

    copyTextForCurrentResult(kind) {
        const paperID = Number(this.currentFigure?.paper_id);
        const action = this.activeAIAction();
        const requestState = this.aiRequestState;
        if (requestState?.loading && requestState.paperID === paperID && requestState.action === action) {
            return requestState.answer || '';
        }
        if (requestState?.stopped && requestState.paperID === paperID && requestState.action === action) {
            return requestState.answer || '';
        }
        if (requestState?.error && requestState.paperID === paperID && requestState.action === action) {
            return requestState.error || '';
        }

        const result = this.currentAIResult();
        if (!result) return '';

        if (kind === 'group' && result.suggested_group) {
            return result.suggested_group;
        }
        if (kind === 'tags' && (result.suggested_tags || []).length) {
            return result.suggested_tags.join(', ');
        }
        return result.answer || '';
    },

    buildAIQuestion(action, figure) {
        const location = `第 ${figure.page_number || '-'} 页图 ${figure.figure_index || '-'}`;
        const caption = figure.caption ? `；caption：${figure.caption}` : '';

        switch (action) {
            case 'figure_interpretation':
                return `请优先围绕当前查看的图片进行解读：${location}${caption}。说明这张图展示了什么、支持了什么结论，以及它和全文主线的关系。`;
            case 'tag_suggestion':
                return `我正在查看这篇文献中的 ${location}${caption}。请结合全文和图片给出适合归档检索的标签建议，优先复用现有标签。`;
            case 'group_suggestion':
                return `我正在查看这篇文献中的 ${location}${caption}。请结合全文和图片判断这篇文献最适合放入哪个分组，并说明理由。`;
            default:
                return '';
        }
    },

    renderAIResultPanel() {
        if (!this.currentFigure) return '';

        const paperID = Number(this.currentFigure.paper_id);
        const action = this.activeAIAction();
        const requestState = this.aiRequestState;
        const isLoading = Boolean(requestState?.loading && requestState.paperID === paperID);
        const activeLabel = this.aiActionLabel(action);
        const currentTagNames = new Set((this.currentFigure.tags || []).map((tag) => {
            const name = typeof tag === 'string' ? tag : tag.name || '';
            return name.trim().toLowerCase();
        }));
        const currentGroupName = (this.currentFigure.group_name || '').trim().toLowerCase();

        if (isLoading) {
            return `
                <div class="figure-ai-result loading">
                    <div class="figure-ai-head">
                        <p class="figure-ai-status">${Utils.escapeHTML(activeLabel)}进行中</p>
                        ${requestState.answer ? '<button class="btn btn-outline btn-small" type="button" data-figure-ai-copy="answer">Copy</button>' : ''}
                    </div>
                    <div class="figure-ai-answer">${Utils.escapeHTML(requestState.answer || '正在结合全文、摘要、标签和图片生成结果。')}</div>
                    <div class="figure-ai-stream-actions">
                        <button class="btn btn-outline" type="button" data-figure-ai-stop>Stop</button>
                    </div>
                </div>
            `;
        }

        if (requestState?.error && requestState.paperID === paperID && requestState.action === action) {
            return `
                <div class="figure-ai-result error">
                    <div class="figure-ai-head">
                        <p class="figure-ai-status">${Utils.escapeHTML(activeLabel)}失败</p>
                        ${requestState.error ? '<button class="btn btn-outline btn-small" type="button" data-figure-ai-copy="answer">Copy</button>' : ''}
                    </div>
                    <div class="figure-ai-answer">${Utils.escapeHTML(requestState.error)}</div>
                </div>
            `;
        }

        if (requestState?.stopped && requestState.paperID === paperID && requestState.action === action) {
            return `
                <div class="figure-ai-result">
                    <div class="figure-ai-head">
                        <p class="figure-ai-status">${Utils.escapeHTML(activeLabel)}已停止</p>
                        ${requestState.answer ? '<button class="btn btn-outline btn-small" type="button" data-figure-ai-copy="answer">Copy</button>' : ''}
                    </div>
                    <div class="figure-ai-answer">${Utils.escapeHTML(requestState.answer || '这次解读已被手动停止。')}</div>
                </div>
            `;
        }

        const result = this.currentAIResult();
        if (!result) {
            return `
                <div class="figure-ai-result empty">
                    <p class="figure-ai-status">选择一个快捷动作</p>
                    <div class="figure-ai-answer">这里会显示图片解读、Tag 建议或 Group 建议的返回结果。</div>
                </div>
            `;
        }

        const tags = (result.suggested_tags || []).map((tag) => `
            <button class="tag-pill neutral figure-ai-tag-button ${currentTagNames.has(tag.trim().toLowerCase()) ? 'is-applied' : ''}" type="button" data-figure-meta-action="apply-tag" data-tag-name="${Utils.escapeHTML(tag)}" ${currentTagNames.has(tag.trim().toLowerCase()) ? 'disabled' : ''}>
                ${Utils.escapeHTML(tag)}
            </button>
        `).join('');
        const groupApplied = (result.suggested_group || '').trim().toLowerCase() === currentGroupName;

        return `
            <div class="figure-ai-result">
                <div class="figure-ai-head">
                    <p class="figure-ai-status">${Utils.escapeHTML(this.aiActionLabel(result.action))} · ${Utils.escapeHTML(result.provider)} · ${Utils.escapeHTML(result.model)} · ${Utils.escapeHTML(result.mode)}</p>
                    ${result.answer ? '<button class="btn btn-outline btn-small" type="button" data-figure-ai-copy="answer">Copy</button>' : ''}
                </div>
                <div class="figure-ai-answer">${Utils.escapeHTML(result.answer || '模型没有返回文本结果。')}</div>
                ${(result.suggested_tags || []).length ? `
                    <div class="figure-ai-supplement">
                        <span>Tag 建议</span>
                        <div class="figure-ai-tag-list">${tags}</div>
                    </div>
                ` : ''}
                ${result.suggested_group ? `
                    <div class="figure-ai-supplement">
                        <span>Group 建议</span>
                        <div class="figure-ai-suggestion-row">
                            <strong>${Utils.escapeHTML(result.suggested_group)}</strong>
                            <button class="btn btn-outline btn-small" type="button" data-figure-meta-action="apply-group" data-group-name="${Utils.escapeHTML(result.suggested_group)}" ${groupApplied ? 'disabled' : ''}>
                                ${groupApplied ? '已添加' : '直接添加'}
                            </button>
                        </div>
                    </div>
                ` : ''}
            </div>
        `;
    },

    aiActionLabel(action) {
        const labels = {
            figure_interpretation: '图片解读',
            tag_suggestion: 'Tag 建议',
            group_suggestion: 'Group 建议'
        };
        return labels[action] || 'AI 结果';
    },

    renderAIActionButtons() {
        const aiLoading = Boolean(this.aiRequestState?.loading);
        return `
            <button class="btn btn-outline ${this.activeAIAction() === 'figure_interpretation' ? 'active' : ''}" type="button" data-figure-ai-action="figure_interpretation" ${aiLoading ? 'disabled' : ''}>图片解读</button>
            <button class="btn btn-outline ${this.activeAIAction() === 'tag_suggestion' ? 'active' : ''}" type="button" data-figure-ai-action="tag_suggestion" ${aiLoading ? 'disabled' : ''}>Tag 建议</button>
            <button class="btn btn-outline ${this.activeAIAction() === 'group_suggestion' ? 'active' : ''}" type="button" data-figure-ai-action="group_suggestion" ${aiLoading ? 'disabled' : ''}>Group 建议</button>
        `;
    },

    render() {
        this.currentFigure = this.figures?.[this.index];
        if (!this.currentFigure) {
            this.body.innerHTML = '<div class="empty-state"><h3>没有可展示的图片</h3></div>';
            return;
        }

        const figure = this.currentFigure;
        const total = this.figures.length;
        const canPrev = this.canMovePrevious();
        const canNext = this.canMoveNext();
        const aiLoading = Boolean(this.aiRequestState?.loading);
        const editableTags = (figure.tags || []).map((tag) => `
            <button class="figure-editable-tag" type="button" data-figure-meta-action="remove-tag" data-tag-name="${Utils.escapeHTML(typeof tag === 'string' ? tag : tag.name || '')}" aria-label="移除标签 ${Utils.escapeHTML(typeof tag === 'string' ? tag : tag.name || '')}">
                <span>${Utils.escapeHTML(typeof tag === 'string' ? tag : tag.name || '')}</span>
                <span aria-hidden="true">+</span>
            </button>
        `).join('');
        const groupOptions = ['<option value="">未分组</option>']
            .concat((this.groups || []).map((group) => `
                <option value="${group.id}" ${String(group.id) === String(figure.group_id || '') ? 'selected' : ''}>
                    ${Utils.escapeHTML(group.name)}
                </option>
            `))
            .join('');

        this.body.innerHTML = `
            <div class="figure-lightbox">
                <section class="figure-lightbox-media-panel">
                    <div class="figure-lightbox-toolbar">
                        <div class="figure-lightbox-counter">第 ${this.index + 1} / ${total} 张 · 第 ${this.page} / ${this.totalPages} 页</div>
                        <div class="figure-lightbox-nav">
                            <button class="btn btn-outline" type="button" data-figure-action="prev" ${!canPrev || this.loadingPage || aiLoading ? 'disabled' : ''}>上一张</button>
                            <button class="btn btn-outline" type="button" data-figure-action="next" ${!canNext || this.loadingPage || aiLoading ? 'disabled' : ''}>下一张</button>
                        </div>
                    </div>
                    <div class="figure-lightbox-media">
                        <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.caption || figure.paper_title)}">
                    </div>
                    ${figure.caption ? `
                        <div class="figure-lightbox-caption">
                            ${Utils.escapeHTML(figure.caption)}
                        </div>
                    ` : ''}
                </section>

                <aside class="figure-lightbox-side">
                    <div class="figure-lightbox-head">
                        <p class="eyebrow">Figure Preview</p>
                        <h2>${Utils.escapeHTML(figure.paper_title)}</h2>
                    </div>

                    <div class="figure-lightbox-meta">
                        <div class="figure-lightbox-meta-item">
                            <span>来源文献</span>
                            <strong>${Utils.escapeHTML(figure.paper_title)}</strong>
                        </div>
                        <div class="figure-lightbox-meta-item">
                            <span>定位</span>
                            <strong>第 ${figure.page_number || '-'} 页 · #${figure.figure_index || '-'}</strong>
                        </div>
                        <label class="figure-lightbox-meta-item figure-lightbox-meta-item-editable">
                            <span>分组</span>
                            <select id="figurePaperGroup" class="form-input figure-meta-select">${groupOptions}</select>
                        </label>
                        <div class="figure-lightbox-meta-item figure-lightbox-meta-item-editable">
                            <span>标签</span>
                            <div class="figure-tag-editor">
                                <div class="figure-tag-editor-list ${figure.tags?.length ? '' : 'is-empty'}">
                                    ${figure.tags?.length ? editableTags : '<span class="figure-tag-empty">暂无标签</span>'}
                                </div>
                                <div class="figure-tag-add">
                                    <input id="figurePaperTagInput" class="form-input" type="text" placeholder="添加标签">
                                    <button class="btn btn-outline btn-small" type="button" data-figure-meta-action="add-tag" aria-label="添加标签">+</button>
                                </div>
                            </div>
                        </div>
                    </div>

                    <div class="figure-lightbox-actions">
                        <button class="btn btn-primary" type="button" data-figure-action="open-paper">查看来源文献</button>
                        <a class="btn btn-outline" href="${figure.image_url}" target="_blank" rel="noreferrer">打开原图</a>
                        <a class="btn btn-outline" href="${figure.image_url}" download="${Utils.escapeHTML(figure.filename || 'figure.png')}">下载图片</a>
                    </div>

                    <section class="figure-lightbox-ai">
                        <div class="figure-lightbox-ai-head">
                            <div>
                                <p class="eyebrow">AI Shortcut</p>
                                <h3>快速辅助阅读</h3>
                            </div>
                            <a class="btn btn-outline" href="/ai?paper_id=${figure.paper_id}" target="_blank" rel="noreferrer">自由提问</a>
                        </div>
                        <div class="figure-lightbox-ai-actions" data-figure-ai-actions>${this.renderAIActionButtons()}</div>
                        <div data-figure-ai-panel>${this.renderAIResultPanel()}</div>
                    </section>
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
                await FigureViewer.open({
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
                    },
                    onMetaChanged: async () => {
                        await Promise.all([this.loadGroups(), this.loadTags(), this.load(this.state.page)]);
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
        const selected = String(this.state.filters.group_id || '');
        this.groupFilter.innerHTML = '<option value="">全部分组</option>' + (payload.groups || []).map((group) => `
            <option value="${group.id}" ${String(group.id) === selected ? 'selected' : ''}>${Utils.escapeHTML(group.name)}</option>
        `).join('');
    },

    async loadTags() {
        const payload = await API.listTags();
        const selected = String(this.state.filters.tag_id || '');
        this.tagFilter.innerHTML = '<option value="">全部标签</option>' + (payload.tags || []).map((tag) => `
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
    state: { selectedTagId: '', page: 1, pageSize: 20, totalPaperCount: 0 },

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
