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
                        <a class="btn btn-outline" href="${figure.image_url}" target="_blank" rel="noreferrer">原图</a>
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
        this.activeAIByFigure = new Map();
        this.aiRequestState = null;
        this.paperDetails = new Map();
        this.viewState = this.defaultViewState();
        this.dragState = null;

        this.handleKeydown = (event) => {
            if (!this.modal || this.modal.classList.contains('hidden')) return;
            const target = event.target;
            const isEditableTarget = target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName));
            if (event.key === 'Escape') {
                this.close();
                return;
            }
            if (isEditableTarget) {
                return;
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
        this.body.addEventListener('input', (event) => {
            const captionInput = event.target.closest('#figureCaptionInput');
            if (!captionInput) return;
            this.captionDraft = captionInput.value;
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
                if (button.dataset.figureAction === 'reset-view') {
                    this.resetViewportState();
                    this.applyViewTransform();
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

            const noteButton = event.target.closest('[data-figure-ai-note]');
            if (noteButton) {
                await this.appendAIResultToNotes(noteButton.dataset.figureAiNote);
                return;
            }

            const metaButton = event.target.closest('[data-figure-meta-action]');
            if (metaButton) {
                await this.handleMetaAction(metaButton);
            }
        });
        this.body.addEventListener('keydown', async (event) => {
            const tagInput = event.target.closest('#figurePaperTagInput');
            if (!tagInput || event.key !== 'Enter') return;
            event.preventDefault();
            await this.addTagFromInput();
        });
        this.body.addEventListener('keydown', async (event) => {
            const notesInput = event.target.closest('#figureNotesInput');
            if (!notesInput || event.key !== 'Enter' || (!event.metaKey && !event.ctrlKey)) return;
            event.preventDefault();
            await this.saveNotesFromInput();
        });
        this.body.addEventListener('keydown', async (event) => {
            const captionInput = event.target.closest('#figureCaptionInput');
            if (!captionInput || event.key !== 'Enter' || (!event.metaKey && !event.ctrlKey)) return;
            event.preventDefault();
            await this.saveCaptionFromInput();
        });
        this.body.addEventListener('wheel', (event) => {
            const viewport = event.target.closest('[data-figure-viewport]');
            if (!viewport || !this.currentFigure) return;
            event.preventDefault();
            this.handleViewportWheel(event, viewport);
        }, { passive: false });
        this.body.addEventListener('pointerdown', (event) => {
            const viewport = event.target.closest('[data-figure-viewport]');
            if (!viewport || !this.currentFigure) return;
            this.beginViewportDrag(event, viewport);
        });
        document.addEventListener('pointermove', (event) => {
            this.updateViewportDrag(event);
        });
        document.addEventListener('pointerup', (event) => {
            this.endViewportDrag(event);
        });
        document.addEventListener('pointercancel', (event) => {
            this.endViewportDrag(event);
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
        this.captionDraft = '';
        this.resetViewportState();
        this.syncCurrentFigureState({ forceDraftFromFigure: true });
        try {
            this.render();
            this.modal.classList.remove('hidden');
            document.body.classList.add('modal-open');
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    close() {
        this.stopAIAction({ preservePartial: false, silent: true });
        this.endViewportDrag();
        this.resetViewportState();
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
            this.resetViewportState();
            this.syncCurrentFigureState({ forceDraftFromFigure: true });
            this.render();
            return;
        }
        await this.loadAdjacentPage(this.page - 1, 'last');
    },

    async next() {
        if (!this.canMoveNext() || this.loadingPage || this.aiRequestState?.loading) return;
        if (this.index < this.figures.length - 1) {
            this.index += 1;
            this.resetViewportState();
            this.syncCurrentFigureState({ forceDraftFromFigure: true });
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
            this.resetViewportState();
            this.syncCurrentFigureState({ forceDraftFromFigure: true });
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            this.loadingPage = false;
            this.render();
        }
    },

    defaultViewState() {
        return {
            scale: 1,
            x: 0,
            y: 0
        };
    },

    resetViewportState() {
        this.viewState = this.defaultViewState();
        this.dragState = null;
    },

    hasViewportTransform() {
        const state = this.viewState || this.defaultViewState();
        return Math.abs(state.scale - 1) > 0.001 || Math.abs(state.x) > 0.5 || Math.abs(state.y) > 0.5;
    },

    clampViewState(state = this.viewState || this.defaultViewState()) {
        const viewport = this.body?.querySelector('[data-figure-viewport]');
        const image = this.body?.querySelector('[data-figure-image]');
        const scale = Math.min(6, Math.max(1, Number(state.scale) || 1));
        if (!viewport || !image || scale <= 1) {
            return { scale: 1, x: 0, y: 0 };
        }

        const baseWidth = image.offsetWidth || image.clientWidth || 0;
        const baseHeight = image.offsetHeight || image.clientHeight || 0;
        if (!baseWidth || !baseHeight) {
            return { scale, x: state.x || 0, y: state.y || 0 };
        }

        const maxX = Math.max(0, (baseWidth * scale - viewport.clientWidth) / 2);
        const maxY = Math.max(0, (baseHeight * scale - viewport.clientHeight) / 2);
        return {
            scale,
            x: Math.min(maxX, Math.max(-maxX, Number(state.x) || 0)),
            y: Math.min(maxY, Math.max(-maxY, Number(state.y) || 0))
        };
    },

    applyViewTransform() {
        const viewport = this.body?.querySelector('[data-figure-viewport]');
        const image = this.body?.querySelector('[data-figure-image]');
        if (!viewport || !image) return;

        this.viewState = this.clampViewState(this.viewState);
        const state = this.viewState;
        image.style.transform = `translate(${state.x}px, ${state.y}px) scale(${state.scale})`;
        image.draggable = false;
        viewport.style.cursor = state.scale > 1 ? (this.dragState ? 'grabbing' : 'grab') : 'zoom-in';
        viewport.classList.toggle('is-zoomed', state.scale > 1);
        viewport.classList.toggle('is-dragging', Boolean(this.dragState));

        const resetButton = this.body.querySelector('[data-figure-action="reset-view"]');
        if (resetButton) {
            resetButton.disabled = !this.hasViewportTransform();
        }
    },

    handleViewportWheel(event, viewport) {
        const previous = this.viewState || this.defaultViewState();
        const zoomFactor = event.deltaY < 0 ? 1.18 : 1 / 1.18;
        const nextScale = Math.min(6, Math.max(1, previous.scale * zoomFactor));
        if (Math.abs(nextScale - previous.scale) < 0.001) return;

        const rect = viewport.getBoundingClientRect();
        const pointX = event.clientX - (rect.left + rect.width / 2);
        const pointY = event.clientY - (rect.top + rect.height / 2);
        const ratio = nextScale / previous.scale;

        this.viewState = {
            scale: nextScale,
            x: pointX - ratio * (pointX - previous.x),
            y: pointY - ratio * (pointY - previous.y)
        };
        this.applyViewTransform();
    },

    beginViewportDrag(event, viewport) {
        if ((event.button !== 0 && event.button !== 1) || (this.viewState?.scale || 1) <= 1) return;
        event.preventDefault();
        this.dragState = {
            pointerID: event.pointerId,
            startX: event.clientX,
            startY: event.clientY,
            originX: this.viewState.x,
            originY: this.viewState.y
        };
        if (typeof viewport.setPointerCapture === 'function') {
            try {
                viewport.setPointerCapture(event.pointerId);
            } catch (error) {
                // Ignore capture failures from synthetic or non-primary pointer events.
            }
        }
        this.applyViewTransform();
    },

    updateViewportDrag(event) {
        if (!this.dragState || event.pointerId !== this.dragState.pointerID) return;
        this.viewState = {
            ...this.viewState,
            x: this.dragState.originX + (event.clientX - this.dragState.startX),
            y: this.dragState.originY + (event.clientY - this.dragState.startY)
        };
        this.applyViewTransform();
    },

    endViewportDrag(event = null) {
        if (!this.dragState) return;
        if (event && event.pointerId !== this.dragState.pointerID) return;

        const viewport = this.body?.querySelector('[data-figure-viewport]');
        if (viewport && typeof viewport.hasPointerCapture === 'function' && viewport.hasPointerCapture(this.dragState.pointerID)) {
            try {
                viewport.releasePointerCapture(this.dragState.pointerID);
            } catch (error) {
                // Ignore release failures when the pointer has already been cancelled.
            }
        }
        this.dragState = null;
        this.applyViewTransform();
    },

    syncCurrentFigureState(options = {}) {
        const { forceDraftFromFigure = false } = options;
        this.currentFigure = this.figures?.[this.index];
        if (forceDraftFromFigure || typeof this.captionDraft !== 'string') {
            this.captionDraft = this.currentFigure?.caption || '';
        }
    },

    aiCacheKey(figureID, action) {
        return `${figureID}:${action}`;
    },

    activeAIAction() {
        if (!this.currentFigure?.id) return '';
        return this.activeAIByFigure.get(this.currentFigure.id) || '';
    },

    currentAIResult() {
        if (!this.currentFigure?.id) return null;
        const action = this.activeAIAction();
        if (!action) return null;
        return this.aiCache.get(this.aiCacheKey(this.currentFigure.id, action)) || null;
    },

    refreshAIResultPanel() {
        if (!this.modal || this.modal.classList.contains('hidden')) return;
        if (this.aiRequestState?.waitingForContent) return;
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
        const figureID = Number(this.currentFigure.id);
        const cacheKey = this.aiCacheKey(figureID, action);
        this.activeAIByFigure.set(figureID, action);

        if (this.aiCache.has(cacheKey)) {
            this.aiRequestState = null;
            this.refreshAIState();
            return;
        }

        this.aiRequestState = {
            loading: true,
            paperID,
            figureID,
            action,
            waitingForContent: true
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
                figure_id: this.currentFigure.id,
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
                figureID: Number(this.currentFigure?.id) || 0,
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
            figureID: Number(this.currentFigure.id),
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
                figure_id: this.currentFigure.id,
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
                        requestState.waitingForContent = false;
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
            if (!requestState.answer && this.shouldFallbackToBufferedAI(error)) {
                this.aiRequestState = {
                    loading: true,
                    paperID,
                    figureID: requestState.figureID,
                    action,
                    waitingForContent: true
                };
                this.refreshAIState();
                await this.runBufferedAIAction(action, paperID, cacheKey);
                return;
            }
            this.aiRequestState = {
                loading: false,
                paperID,
                figureID: requestState.figureID,
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

    shouldFallbackToBufferedAI(error) {
        const message = String(error?.message || '').toLowerCase();
        return message.includes('401')
            || message.includes('unauthorized')
            || message.includes('invalid jwt token')
            || message.includes('token is malformed')
            || (message.includes('stream') && message.includes('unsupported'));
    },

    tagNames(tags = []) {
        return Utils.splitTags(Utils.joinTags(tags));
    },

    currentFigureTagNames() {
        return this.tagNames(this.currentFigure?.tags || []);
    },

    currentFigureCaptionDraft() {
        return this.body.querySelector('#figureCaptionInput')?.value ?? this.captionDraft ?? (this.currentFigure?.caption || '');
    },

    currentFigureNotesDraft() {
        return this.body.querySelector('#figureNotesInput')?.value ?? (this.currentFigure?.notes_text || '');
    },

    clearFigureAIState(figureID, options = {}) {
        const preserveActions = new Set((options.preserveActions || []).map((action) => String(action || '')));
        if (!figureID) return;
        const activeAction = this.activeAIByFigure.get(figureID) || '';
        if (!activeAction || !preserveActions.has(activeAction)) {
            this.activeAIByFigure.delete(figureID);
        }
        if (!preserveActions.has('figure_interpretation')) {
            this.aiCache.delete(this.aiCacheKey(figureID, 'figure_interpretation'));
        }
        if (!preserveActions.has('tag_suggestion')) {
            this.aiCache.delete(this.aiCacheKey(figureID, 'tag_suggestion'));
        }
        if (this.aiRequestState?.figureID === figureID && !this.aiRequestState.loading && !preserveActions.has(this.aiRequestState.action || '')) {
            this.aiRequestState = null;
        }
    },

    async handleMetaAction(button) {
        if (!this.currentFigure) return;

        if (button.dataset.figureMetaAction === 'apply-tag') {
            await this.applySuggestedTag(button.dataset.tagName || '');
            return;
        }
        if (button.dataset.figureMetaAction === 'add-tag') {
            await this.addTagFromInput();
            return;
        }
        if (button.dataset.figureMetaAction === 'remove-tag') {
            await this.removeTag(button.dataset.tagName || '');
            return;
        }
        if (button.dataset.figureMetaAction === 'save-caption') {
            await this.saveCaptionFromInput();
            return;
        }
        if (button.dataset.figureMetaAction === 'open-notes') {
            await this.openNotes();
            return;
        }
        if (button.dataset.figureMetaAction === 'save-notes') {
            await this.saveNotesFromInput();
        }
    },

    async applySuggestedTag(tagName) {
        const normalized = tagName.trim();
        if (!normalized) return;

        const draftTags = this.currentFigureTagNames();
        const existing = new Set(draftTags.map((tag) => tag.toLowerCase()));
        if (existing.has(normalized.toLowerCase())) {
            Utils.showToast('这个标签已经存在', 'info');
            return;
        }

        draftTags.push(normalized);
        await this.updateCurrentFigureTags(draftTags, `已添加标签：${normalized}`);
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

        const draftTags = this.currentFigureTagNames().filter((tag) => tag.toLowerCase() !== normalized.toLowerCase());
        await this.updateCurrentFigureTags(draftTags, `已移除标签：${normalized}`);
    },

    async updateCurrentFigureTags(tags, successMessage) {
        if (!this.currentFigure) return;

        try {
            const figureID = Number(this.currentFigure.id);
            const preserveActions = this.activeAIAction() === 'tag_suggestion' ? ['tag_suggestion'] : [];
            const payload = await API.updateFigure(this.currentFigure.id, {
                tags,
                caption: this.currentFigureCaptionDraft(),
                notes_text: this.currentFigureNotesDraft()
            });
            this.clearFigureAIState(figureID, { preserveActions });
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

    async saveCaptionFromInput() {
        await this.updateCurrentFigureCaption(this.currentFigureCaptionDraft(), '图片说明已保存');
    },

    async updateCurrentFigureCaption(caption, successMessage) {
        if (!this.currentFigure) return;

        try {
            const figureID = Number(this.currentFigure.id);
            const payload = await API.updateFigure(this.currentFigure.id, {
                caption,
                notes_text: this.currentFigureNotesDraft()
            });
            this.clearFigureAIState(figureID);
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

    async saveNotesFromInput() {
        await this.updateCurrentFigureNotes(this.currentFigureNotesDraft(), '图片笔记已保存');
    },

    async updateCurrentFigureNotes(notesText, successMessage) {
        if (!this.currentFigure) return;

        try {
            const figureID = Number(this.currentFigure.id);
            const payload = await API.updateFigure(this.currentFigure.id, {
                caption: this.currentFigureCaptionDraft(),
                notes_text: notesText
            });
            this.clearFigureAIState(figureID);
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
        this.figures = mergeFigureCollectionWithPaper(this.figures, paper);
        this.syncCurrentFigureState({ forceDraftFromFigure: true });
    },

    async openNotes() {
        if (!this.currentFigure) return;

        this.close();
        await NoteViewer.open({
            figures: this.figures || [],
            index: this.index,
            page: this.page,
            totalPages: this.totalPages,
            loadPage: this.loadPage,
            onOpenPaper: this.onOpenPaper,
            onMetaChanged: this.onMetaChanged
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

    async appendAIResultToNotes(kind) {
        const text = this.copyTextForCurrentResult(kind);
        if (!text) {
            Utils.showToast('当前没有可写入笔记的内容', 'error');
            return;
        }

        const normalized = text.trim();
        if (!normalized) {
            Utils.showToast('当前没有可写入笔记的内容', 'error');
            return;
        }

        const currentNotes = this.currentFigureNotesDraft().trim();
        const nextNotes = currentNotes ? `${currentNotes}\n\n${normalized}` : normalized;
        await this.updateCurrentFigureNotes(nextNotes, 'AI 内容已写入图片笔记');
    },

    copyTextForCurrentResult(kind) {
        const figureID = Number(this.currentFigure?.id);
        const action = this.activeAIAction();
        const requestState = this.aiRequestState;
        if (requestState?.loading && requestState.figureID === figureID && requestState.action === action) {
            return requestState.answer || '';
        }
        if (requestState?.stopped && requestState.figureID === figureID && requestState.action === action) {
            return requestState.answer || '';
        }
        if (requestState?.error && requestState.figureID === figureID && requestState.action === action) {
            return requestState.error || '';
        }

        const result = this.currentAIResult();
        if (!result) return '';

        if (kind === 'tags' && (result.suggested_tags || []).length) {
            return result.suggested_tags.join(', ');
        }
        return result.answer || '';
    },

    buildAIQuestion(action, figure) {
        const location = `第 ${figure.page_number || '-'} 页图 ${figure.figure_index || '-'}${figure.source === 'manual' ? '（人工提取）' : ''}`;
        const caption = figure.caption ? `；caption：${figure.caption}` : '';

        switch (action) {
            case 'figure_interpretation':
                return `当前查看图片：${location}${caption}。请只围绕这张图片回答。`;
            case 'tag_suggestion':
                return `当前查看图片：${location}${caption}。请只针对这张图片给出结果。`;
            default:
                return '';
        }
    },

    renderAIResultPanel() {
        if (!this.currentFigure) return '';

        const figureID = Number(this.currentFigure.id);
        const action = this.activeAIAction();
        const requestState = this.aiRequestState;
        const isLoading = Boolean(requestState?.loading && requestState.figureID === figureID);
        const activeLabel = this.aiActionLabel(action);
        const currentTagNames = new Set((this.currentFigure.tags || []).map((tag) => {
            const name = typeof tag === 'string' ? tag : tag.name || '';
            return name.trim().toLowerCase();
        }));

        if (isLoading && requestState.waitingForContent) {
            const fallback = this.currentAIResult();
            if (fallback) {
                return this.renderAICachedResult(fallback, currentTagNames, true);
            }
            return `
                <div class="figure-ai-result loading">
                    <div class="figure-ai-head">
                        <p class="figure-ai-status">${Utils.escapeHTML(activeLabel)}准备中</p>
                    </div>
                    <div class="figure-ai-answer">正在结合全文、摘要、标签和图片生成结果。</div>
                </div>
            `;
        }

        if (isLoading) {
            return `
                <div class="figure-ai-result loading">
                    <div class="figure-ai-head">
                        <p class="figure-ai-status">${Utils.escapeHTML(activeLabel)}进行中</p>
                        ${requestState.answer ? `
                            <div class="figure-ai-head-actions">
                                <button class="btn btn-outline btn-small" type="button" data-figure-ai-copy="answer">复制</button>
                                <button class="btn btn-outline btn-small" type="button" data-figure-ai-note="answer">写入笔记</button>
                            </div>
                        ` : ''}
                    </div>
                    <div class="figure-ai-answer">${Utils.escapeHTML(requestState.answer || '正在结合全文、摘要、标签和图片生成结果。')}</div>
                    <div class="figure-ai-stream-actions">
                        <button class="btn btn-outline" type="button" data-figure-ai-stop>Stop</button>
                    </div>
                </div>
            `;
        }

        if (requestState?.error && requestState.figureID === figureID && requestState.action === action) {
            return `
                <div class="figure-ai-result error">
                    <div class="figure-ai-head">
                        <p class="figure-ai-status">${Utils.escapeHTML(activeLabel)}失败</p>
                        ${requestState.error ? '<button class="btn btn-outline btn-small" type="button" data-figure-ai-copy="answer">复制</button>' : ''}
                    </div>
                    <div class="figure-ai-answer">${Utils.escapeHTML(requestState.error)}</div>
                </div>
            `;
        }

        if (requestState?.stopped && requestState.figureID === figureID && requestState.action === action) {
            return `
                <div class="figure-ai-result">
                    <div class="figure-ai-head">
                        <p class="figure-ai-status">${Utils.escapeHTML(activeLabel)}已停止</p>
                        ${requestState.answer ? `
                            <div class="figure-ai-head-actions">
                                <button class="btn btn-outline btn-small" type="button" data-figure-ai-copy="answer">复制</button>
                                <button class="btn btn-outline btn-small" type="button" data-figure-ai-note="answer">写入笔记</button>
                            </div>
                        ` : ''}
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
                    <div class="figure-ai-answer">这里会显示图片解读或 Tag 建议的返回结果。</div>
                </div>
            `;
        }

        return this.renderAICachedResult(result, currentTagNames, false);
    },

    renderAICachedResult(result, currentTagNames, isWaiting) {
        const tags = (result.suggested_tags || []).map((tag) => `
            <button class="tag-pill neutral figure-ai-tag-button ${currentTagNames.has(tag.trim().toLowerCase()) ? 'is-applied' : ''}" type="button" data-figure-meta-action="apply-tag" data-tag-name="${Utils.escapeHTML(tag)}" ${currentTagNames.has(tag.trim().toLowerCase()) ? 'disabled' : ''}>
                ${Utils.escapeHTML(tag)}
            </button>
        `).join('');

        return `
            <div class="figure-ai-result ${isWaiting ? 'loading' : ''}">
                <div class="figure-ai-head">
                    <p class="figure-ai-status">${Utils.escapeHTML(this.aiActionLabel(result.action))} · ${Utils.escapeHTML(result.provider)} · ${Utils.escapeHTML(result.model)} · ${Utils.escapeHTML(result.mode)}${isWaiting ? ' · 加载中' : ''}</p>
                    ${result.answer ? `
                        <div class="figure-ai-head-actions">
                            <button class="btn btn-outline btn-small" type="button" data-figure-ai-copy="answer">复制</button>
                            <button class="btn btn-outline btn-small" type="button" data-figure-ai-note="answer">写入笔记</button>
                        </div>
                    ` : ''}
                </div>
                <div class="figure-ai-answer">${Utils.escapeHTML(result.answer || '模型没有返回文本结果。')}</div>
                ${(result.suggested_tags || []).length ? `
                    <div class="figure-ai-supplement">
                        <span>Tag 建议</span>
                        <div class="figure-ai-tag-list">${tags}</div>
                    </div>
                ` : ''}
            </div>
        `;
    },

    aiActionLabel(action) {
        const labels = {
            figure_interpretation: '图片解读',
            tag_suggestion: 'Tag 建议'
        };
        return labels[action] || 'AI 结果';
    },

    renderAIActionButtons() {
        const aiLoading = Boolean(this.aiRequestState?.loading);
        return `
            <button class="btn btn-outline ${this.activeAIAction() === 'figure_interpretation' ? 'active' : ''}" type="button" data-figure-ai-action="figure_interpretation" ${aiLoading ? 'disabled' : ''}>图片解读</button>
            <button class="btn btn-outline ${this.activeAIAction() === 'tag_suggestion' ? 'active' : ''}" type="button" data-figure-ai-action="tag_suggestion" ${aiLoading ? 'disabled' : ''}>Tag 建议</button>
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
        const captionDraft = this.captionDraft ?? (figure.caption || '');
        const notePreview = String(figure.notes_text || '').replace(/\s+/g, ' ').trim();
        const editableTags = (figure.tags || []).map((tag) => `
            <button class="figure-editable-tag" type="button" data-figure-meta-action="remove-tag" data-tag-name="${Utils.escapeHTML(typeof tag === 'string' ? tag : tag.name || '')}" aria-label="移除标签 ${Utils.escapeHTML(typeof tag === 'string' ? tag : tag.name || '')}">
                <span>${Utils.escapeHTML(typeof tag === 'string' ? tag : tag.name || '')}</span>
                <span aria-hidden="true">+</span>
            </button>
        `).join('');
        this.body.innerHTML = `
            <div class="figure-lightbox">
                <section class="figure-lightbox-media-panel">
                    <div class="figure-lightbox-toolbar">
                        <div class="figure-lightbox-counter">第 ${this.index + 1} / ${total} 张 · 第 ${this.page} / ${this.totalPages} 页</div>
                        <div class="figure-lightbox-nav">
                            <span class="figure-lightbox-hint">滚轮缩放，按住左键或中键拖动</span>
                            <button class="btn btn-outline" type="button" data-figure-action="reset-view">复原视图</button>
                            <button class="btn btn-outline" type="button" data-figure-action="prev" ${!canPrev || this.loadingPage || aiLoading ? 'disabled' : ''}>上一张</button>
                            <button class="btn btn-outline" type="button" data-figure-action="next" ${!canNext || this.loadingPage || aiLoading ? 'disabled' : ''}>下一张</button>
                        </div>
                    </div>
                    <div class="figure-lightbox-media" data-figure-viewport>
                        <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.caption || figure.paper_title)}" data-figure-image>
                    </div>
                    <div class="figure-lightbox-caption figure-lightbox-caption-editor">
                        <label class="field">
                            <span>图片说明（Caption）</span>
                            <textarea id="figureCaptionInput" class="form-textarea figure-caption-input" rows="4" placeholder="解析 caption 有误时，可在这里直接修正。">${Utils.escapeHTML(captionDraft)}</textarea>
                        </label>
                        <div class="figure-notes-actions figure-caption-actions">
                            <span class="muted">caption 会参与检索；按 Ctrl/Cmd + Enter 可快速保存。</span>
                            <button class="btn btn-outline btn-small" type="button" data-figure-meta-action="save-caption">保存说明</button>
                        </div>
                    </div>
                </section>

                <aside class="figure-lightbox-side">
                    <div class="figure-lightbox-head">
                        <p class="eyebrow">Image Library</p>
                        <h2>${Utils.escapeHTML(figure.paper_title)}</h2>
                    </div>

                    <div class="figure-lightbox-meta">
                        <div class="figure-lightbox-meta-item">
                            <span>来源文献</span>
                            <strong>${Utils.escapeHTML(figure.paper_title)}</strong>
                        </div>
                        <div class="figure-lightbox-meta-item">
                            <span>定位</span>
                            <strong>第 ${figure.page_number || '-'} 页 · #${figure.figure_index || '-'}${figure.source === 'manual' ? ' · 人工提取' : ''}</strong>
                        </div>
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
                        <div class="figure-lightbox-meta-item figure-lightbox-meta-item-editable">
                            <span>图片笔记</span>
                            <button class="figure-note-inline-trigger ${notePreview ? '' : 'is-empty'}" type="button" data-figure-meta-action="open-notes" aria-label="打开图片笔记编辑器">
                                <span class="figure-note-inline-text">${Utils.escapeHTML(notePreview || '还没有图片笔记，点击后在独立笔记面板中编辑。')}</span>
                                <span class="figure-note-inline-action">打开笔记</span>
                            </button>
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
                            <a class="btn btn-outline" href="/ai?paper_id=${figure.paper_id}">自由提问</a>
                        </div>
                        <div class="figure-lightbox-ai-actions" data-figure-ai-actions>${this.renderAIActionButtons()}</div>
                        <div data-figure-ai-panel>${this.renderAIResultPanel()}</div>
                    </section>
                </aside>
            </div>
        `;

        const image = this.body.querySelector('[data-figure-image]');
        if (image) {
            if (image.complete) {
                this.applyViewTransform();
            } else {
                image.addEventListener('load', () => this.applyViewTransform(), { once: true });
            }
        }
    }
};

const NoteViewer = {
    init() {
        this.modal = document.getElementById('noteModal');
        this.body = document.getElementById('noteModalBody');
        this.closeButton = document.getElementById('closeNoteModal');
        if (!this.modal) {
            const shell = document.createElement('div');
            shell.id = 'noteModal';
            shell.className = 'modal-shell hidden';
            shell.innerHTML = `
                <div class="modal-dialog figure-modal-dialog note-modal-dialog">
                    <button id="closeNoteModal" class="modal-close" type="button" aria-label="关闭">×</button>
                    <div id="noteModalBody"></div>
                </div>
            `;
            document.body.appendChild(shell);
            this.modal = shell;
            this.body = shell.querySelector('#noteModalBody');
            this.closeButton = shell.querySelector('#closeNoteModal');
        }
        if (!this.modal || this.initialized) return;
        this.initialized = true;

        this.handleKeydown = (event) => {
            if (!this.modal || this.modal.classList.contains('hidden')) return;
            const target = event.target;
            const isEditableTarget = target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName));
            if (event.key === 'Escape') {
                event.preventDefault();
                event.stopPropagation();
                void this.openPreview();
                return;
            }
            if (isEditableTarget) {
                return;
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
        this.body.addEventListener('input', (event) => {
            const notesInput = event.target.closest('#noteViewerInput');
            if (!notesInput) return;
            this.noteDraft = notesInput.value;
        });
        this.body.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-note-action]');
            if (!button) return;

            if (button.dataset.noteAction === 'prev') {
                await this.previous();
                return;
            }
            if (button.dataset.noteAction === 'next') {
                await this.next();
                return;
            }
            if (button.dataset.noteAction === 'set-mode') {
                this.noteMode = button.dataset.noteMode === 'preview' ? 'preview' : 'write';
                this.render();
                return;
            }
            if (button.dataset.noteAction === 'save-notes') {
                await this.saveNotesFromInput();
                return;
            }
            if (button.dataset.noteAction === 'open-preview') {
                await this.openPreview();
                return;
            }
            if (button.dataset.noteAction === 'open-paper') {
                await this.openPaper();
            }
        });
        this.body.addEventListener('keydown', async (event) => {
            const notesInput = event.target.closest('#noteViewerInput');
            if (!notesInput || event.key !== 'Enter' || (!event.metaKey && !event.ctrlKey)) return;
            event.preventDefault();
            await this.saveNotesFromInput();
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
        this.noteDraft = '';
        this.noteMode = 'write';
        this.syncCurrentFigureState({ resetMode: true });

        try {
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
            this.syncCurrentFigureState({ resetMode: true });
            this.render();
            return;
        }
        await this.loadAdjacentPage(this.page - 1, 'last');
    },

    async next() {
        if (!this.canMoveNext() || this.loadingPage) return;
        if (this.index < this.figures.length - 1) {
            this.index += 1;
            this.syncCurrentFigureState({ resetMode: true });
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
            this.syncCurrentFigureState({ resetMode: true });
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            this.loadingPage = false;
            this.render();
        }
    },

    currentFigureNotesDraft() {
        return this.body.querySelector('#noteViewerInput')?.value ?? this.noteDraft ?? (this.currentFigure?.notes_text || '');
    },

    async saveNotesFromInput() {
        await this.updateCurrentFigureNotes(this.currentFigureNotesDraft(), '图片笔记已保存');
    },

    async updateCurrentFigureNotes(notesText, successMessage) {
        if (!this.currentFigure) return;

        try {
            const payload = await API.updateFigure(this.currentFigure.id, {
                notes_text: notesText
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
        this.figures = mergeFigureCollectionWithPaper(this.figures, paper);
        this.syncCurrentFigureState({ resetMode: false, forceDraftFromFigure: true });
    },

    syncCurrentFigureState(options = {}) {
        const { resetMode = false, forceDraftFromFigure = false } = options;
        this.currentFigure = this.figures?.[this.index];
        const figureNotes = this.currentFigure?.notes_text || '';

        if (forceDraftFromFigure || typeof this.noteDraft !== 'string' || resetMode) {
            this.noteDraft = figureNotes;
        }
        if (resetMode || !this.noteMode) {
            this.noteMode = figureNotes.trim() ? 'preview' : 'write';
        }
    },

    async openPreview() {
        if (!this.currentFigure) return;

        this.close();
        await FigureViewer.open({
            figures: this.figures || [],
            index: this.index,
            page: this.page,
            totalPages: this.totalPages,
            loadPage: this.loadPage,
            onOpenPaper: this.onOpenPaper,
            onMetaChanged: this.onMetaChanged
        });
    },

    async openPaper() {
        if (!this.currentFigure) return;

        this.close();
        if (typeof this.onOpenPaper === 'function') {
            await this.onOpenPaper(this.currentFigure.paper_id);
        }
    },

    render() {
        this.currentFigure = this.figures?.[this.index];
        if (!this.currentFigure) {
            this.body.innerHTML = '<div class="empty-state"><h3>没有可展示的笔记</h3></div>';
            return;
        }

        const figure = this.currentFigure;
        const total = this.figures.length;
        const canPrev = this.canMovePrevious();
        const canNext = this.canMoveNext();
        const noteText = this.currentFigureNotesDraft();
        const isPreviewMode = this.noteMode === 'preview';

        this.body.innerHTML = `
            <div class="note-lightbox">
                <section class="note-lightbox-main">
                    <div class="figure-lightbox-toolbar">
                        <div class="figure-lightbox-counter">第 ${this.index + 1} / ${total} 张 · 第 ${this.page} / ${this.totalPages} 页</div>
                        <div class="figure-lightbox-nav">
                            <button class="btn btn-outline" type="button" data-note-action="prev" ${!canPrev || this.loadingPage ? 'disabled' : ''}>上一条</button>
                            <button class="btn btn-outline" type="button" data-note-action="next" ${!canNext || this.loadingPage ? 'disabled' : ''}>下一条</button>
                        </div>
                    </div>

                    <div class="note-lightbox-editor-card">
                        <div class="note-lightbox-head">
                            <div class="note-lightbox-head-row">
                                <div>
                                    <p class="eyebrow">Figure Notes</p>
                                    <h2>${Utils.escapeHTML(figure.paper_title)}</h2>
                                    <p class="note-lightbox-subtitle">第 ${figure.page_number || '-'} 页 · #${figure.figure_index || '-'}${figure.source === 'manual' ? ' · 人工提取' : ''}</p>
                                </div>
                                <div class="note-lightbox-mode-switch">
                                    <button class="btn ${isPreviewMode ? 'btn-outline' : 'btn-primary'}" type="button" data-note-action="set-mode" data-note-mode="write">编辑</button>
                                    <button class="btn ${isPreviewMode ? 'btn-primary' : 'btn-outline'}" type="button" data-note-action="set-mode" data-note-mode="preview">Markdown 预览</button>
                                </div>
                            </div>
                        </div>

                        <div class="note-lightbox-meta-grid">
                            <div class="note-lightbox-meta-item">
                                <span>图片标签</span>
                                <div class="figure-preview-tags ${figure.tags?.length ? '' : 'is-empty'}">
                                    ${figure.tags?.length ? BrowserUI.renderTagChips(figure.tags || []) : '<span class="figure-preview-empty">无标签</span>'}
                                </div>
                            </div>
                            <div class="note-lightbox-meta-item">
                                <span>来源分组</span>
                                <strong>${Utils.escapeHTML(figure.group_name || '未分组')}</strong>
                            </div>
                        </div>

                        ${isPreviewMode ? `
                            <section class="note-lightbox-render-panel">
                                <span class="note-lightbox-panel-label">Markdown 预览</span>
                                <div class="markdown-preview">${Utils.renderMarkdown(noteText)}</div>
                            </section>
                        ` : `
                            <label class="field note-lightbox-field">
                                <span>图片笔记</span>
                                <textarea id="noteViewerInput" class="form-textarea note-lightbox-textarea" rows="16" placeholder="记录这张图的观察、AI 解读摘要、方法要点或后续检索关键词">${Utils.escapeHTML(noteText)}</textarea>
                            </label>
                        `}
                        <div class="figure-notes-actions">
                            <span class="muted">${isPreviewMode ? '预览基于当前草稿渲染；切回编辑可继续修改。' : '支持多行内容，按 Ctrl/Cmd + Enter 可快速保存。'}</span>
                            <button class="btn btn-primary" type="button" data-note-action="save-notes">保存笔记</button>
                        </div>
                    </div>
                </section>

                <aside class="note-lightbox-side">
                    <div class="note-lightbox-preview-card">
                        <div class="note-lightbox-preview-media">
                            <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.caption || figure.paper_title)}">
                        </div>
                        ${figure.caption ? `
                            <div class="figure-lightbox-caption">
                                ${Utils.escapeHTML(figure.caption)}
                            </div>
                        ` : ''}
                    </div>

                    <div class="note-lightbox-actions">
                        <button class="btn btn-outline" type="button" data-note-action="open-preview">查看大图</button>
                        <button class="btn btn-outline" type="button" data-note-action="open-paper">查看文献</button>
                        <a class="btn btn-outline" href="${figure.image_url}" target="_blank" rel="noreferrer">原图</a>
                    </div>

                    <div class="note-lightbox-tip">
                        <span>整理建议</span>
                        <p>先把可检索的关键词和 AI 解读写进笔记，再决定是否回到大图页补标签。</p>
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
                await NoteViewer.open({
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
                return;
            }
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
                await NoteViewer.open({
                    figures: this.figures || [],
                    index,
                    page: this.state.page,
                    totalPages: this.totalPages || 1,
                    loadPage: async (page) => {
                        const payload = await this.fetchFigureResults(page);
                        this.renderFigureResults(payload, page);
                        return payload;
                    },
                    onOpenPaper: async (paperID) => {
                        await PaperViewer.open(Number(paperID), async () => await this.reload());
                    },
                    onMetaChanged: async () => {
                        await this.reload();
                    }
                });
                return;
            }
            if (action.dataset.action === 'preview') {
                await FigureViewer.open({
                    figures: this.figures || [],
                    index,
                    page: this.state.page,
                    totalPages: this.totalPages || 1,
                    loadPage: async (page) => {
                        const payload = await this.fetchFigureResults(page);
                        this.renderFigureResults(payload, page);
                        return payload;
                    },
                    onOpenPaper: async (paperID) => {
                        await PaperViewer.open(Number(paperID), async () => await this.reload());
                    },
                    onMetaChanged: async () => {
                        await this.reload();
                    }
                });
                return;
            }
            if (action.dataset.action === 'paper') {
                await PaperViewer.open(Number(card.dataset.paperId), async () => await this.reload());
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
                await NoteViewer.open({
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
                        await this.openPaper(Number(paperID));
                    },
                    onMetaChanged: async () => {
                        await Promise.all([this.loadGroups(), this.loadTags(), this.load(this.state.page)]);
                    }
                });
                return;
            }
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
                        await this.openPaper(Number(paperID));
                    },
                    onMetaChanged: async () => {
                        await Promise.all([this.loadGroups(), this.loadTags(), this.load(this.state.page)]);
                    }
                });
                return;
            }
            if (action.dataset.action === 'paper') {
                await this.openPaper(Number(card.dataset.paperId));
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
