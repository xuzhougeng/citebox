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
            if (event.defaultPrevented) return;
            if (typeof Utils !== 'undefined' && typeof Utils.isTopVisibleModal === 'function' && !Utils.isTopVisibleModal(this.modal)) {
                return;
            }
            const target = event.target;
            const isEditableTarget = target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName));
            if (event.key === 'Escape') {
                event.preventDefault();
                event.stopPropagation();
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
                if (button.dataset.figureAction === 'download-image') {
                    await this.downloadCurrentImage();
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
        if (button.dataset.figureMetaAction === 'translate-caption') {
            await this.translateCaptionFromInput();
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

    async translateCaptionFromInput() {
        const caption = this.currentFigureCaptionDraft();
        if (typeof DesktopTranslate === 'undefined' || typeof DesktopTranslate.translateText !== 'function') {
            Utils.showToast('翻译功能暂不可用', 'error');
            return;
        }

        await DesktopTranslate.translateText(caption, {
            title: 'Caption 翻译',
            emptyMessage: '当前没有可翻译的图片说明'
        });
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
            onMetaChanged: this.onMetaChanged,
            returnToFigureViewer: true
        });
    },

    async downloadCurrentImage() {
        if (!this.currentFigure?.image_url) {
            Utils.showToast('当前图片无法下载', 'error');
            return;
        }

        const filename = this.currentFigure.filename || 'figure.png';
        try {
            const response = await fetch(this.currentFigure.image_url, {
                credentials: 'same-origin'
            });
            if (!response.ok) {
                throw new Error(`图片下载失败 (${response.status})`);
            }

            const saved = await Utils.saveBlobDownload(await response.blob(), filename);
            if (saved) {
                Utils.showToast('图片已保存');
            }
        } catch (error) {
            Utils.showToast(error.message || '图片下载失败', 'error');
        }
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
        const canUseDesktopSave = Utils.supportsDesktopSave();
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
                            <div class="figure-caption-action-group">
                                <button class="btn btn-outline btn-small" type="button" data-figure-meta-action="translate-caption">翻译说明</button>
                                <button class="btn btn-outline btn-small" type="button" data-figure-meta-action="save-caption">保存说明</button>
                            </div>
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
                        <a class="btn btn-outline" href="${Utils.resourceViewerURL('image', figure.image_url)}">打开原图</a>
                        ${canUseDesktopSave
                            ? '<button class="btn btn-outline" type="button" data-figure-action="download-image">下载图片</button>'
                            : `<a class="btn btn-outline" href="${figure.image_url}" download="${Utils.escapeHTML(figure.filename || 'figure.png')}">下载图片</a>`}
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
