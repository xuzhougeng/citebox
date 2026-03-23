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
        this.paperDetailPromises = new Map();
        this.viewState = this.defaultViewState();
        this.dragState = null;
        this.cropState = this.defaultCropState();

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
                event.preventDefault();
                event.stopPropagation();
                void this.previous();
                return;
            }
            if (event.key === 'ArrowRight') {
                event.preventDefault();
                event.stopPropagation();
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
            if (captionInput) {
                this.captionDraft = captionInput.value;
                return;
            }

            const subfigureCaptionInput = event.target.closest('[data-subfigure-field="caption"]');
            if (!subfigureCaptionInput) return;
            const selectionID = subfigureCaptionInput.dataset.selectionId;
            const selection = this.findCropSelection(selectionID);
            if (!selection) return;
            selection.caption = subfigureCaptionInput.value;
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
                if (button.dataset.figureAction === 'toggle-subfigure-crop') {
                    this.toggleSubfigureCropMode();
                }
                if (button.dataset.figureAction === 'cancel-subfigure-crop') {
                    this.cancelSubfigureCropMode();
                }
                if (button.dataset.figureAction === 'clear-subfigure-selections') {
                    this.clearSubfigureSelections();
                }
                if (button.dataset.figureAction === 'save-subfigures') {
                    await this.submitSubfigureSelections();
                }
                return;
            }

            const subfigureButton = event.target.closest('[data-subfigure-action]');
            if (subfigureButton) {
                if (subfigureButton.dataset.subfigureAction === 'remove-selection') {
                    this.removeCropSelection(subfigureButton.dataset.selectionId);
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
            if (!viewport || !this.currentFigure || this.isCropModeEnabled()) return;
            event.preventDefault();
            this.handleViewportWheel(event, viewport);
        }, { passive: false });
        this.body.addEventListener('pointerdown', (event) => {
            const cropOverlay = event.target.closest('[data-figure-crop-overlay]');
            if (cropOverlay && this.currentFigure && this.isCropModeEnabled()) {
                this.beginCropSelection(event, cropOverlay);
                return;
            }
            const viewport = event.target.closest('[data-figure-viewport]');
            if (!viewport || !this.currentFigure || this.isCropModeEnabled()) return;
            this.beginViewportDrag(event, viewport);
        });
        document.addEventListener('pointermove', (event) => {
            this.updateCropSelection(event);
            this.updateViewportDrag(event);
        });
        document.addEventListener('pointerup', (event) => {
            this.endCropSelection(event);
            this.endViewportDrag(event);
        });
        document.addEventListener('pointercancel', (event) => {
            this.endCropSelection(event);
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
        this.resetCropState();
        this.syncCurrentFigureState({ forceDraftFromFigure: true });
        try {
            this.render();
            this.modal.classList.remove('hidden');
            document.body.classList.add('modal-open');
            this.requestCurrentPaperDetail();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    close() {
        this.stopAIAction({ preservePartial: false, silent: true });
        this.endCropSelection();
        this.endViewportDrag();
        this.resetCropState();
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
        if (!this.canMovePrevious() || this.loadingPage || this.aiRequestState?.loading || this.isCropModeEnabled()) return;
        if (this.index > 0) {
            this.index -= 1;
            this.resetViewportState();
            this.resetCropState();
            this.syncCurrentFigureState({ forceDraftFromFigure: true });
            this.render();
            this.requestCurrentPaperDetail();
            return;
        }
        await this.loadAdjacentPage(this.page - 1, 'last');
    },

    async next() {
        if (!this.canMoveNext() || this.loadingPage || this.aiRequestState?.loading || this.isCropModeEnabled()) return;
        if (this.index < this.figures.length - 1) {
            this.index += 1;
            this.resetViewportState();
            this.resetCropState();
            this.syncCurrentFigureState({ forceDraftFromFigure: true });
            this.render();
            this.requestCurrentPaperDetail();
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
            this.resetCropState();
            this.syncCurrentFigureState({ forceDraftFromFigure: true });
            this.requestCurrentPaperDetail();
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

    defaultCropState() {
        return {
            enabled: false,
            selections: [],
            draft: null,
            pointerId: null,
            submitting: false
        };
    },

    resetCropState() {
        this.cropState = this.defaultCropState();
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
        const stage = this.body?.querySelector('[data-figure-image-stage]');
        const scale = Math.min(6, Math.max(1, Number(state.scale) || 1));
        if (!viewport || !stage || scale <= 1) {
            return { scale: 1, x: 0, y: 0 };
        }

        const baseWidth = stage.offsetWidth || stage.clientWidth || 0;
        const baseHeight = stage.offsetHeight || stage.clientHeight || 0;
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
        const stage = this.body?.querySelector('[data-figure-image-stage]');
        if (!viewport || !image || !stage) return;

        this.viewState = this.clampViewState(this.viewState);
        const state = this.viewState;
        stage.style.transform = `translate(${state.x}px, ${state.y}px) scale(${state.scale})`;
        image.draggable = false;
        if (this.isCropModeEnabled()) {
            viewport.style.cursor = 'crosshair';
        } else {
            viewport.style.cursor = state.scale > 1 ? (this.dragState ? 'grabbing' : 'grab') : 'zoom-in';
        }
        viewport.classList.toggle('is-zoomed', state.scale > 1);
        viewport.classList.toggle('is-dragging', Boolean(this.dragState));
        viewport.classList.toggle('is-crop-mode', this.isCropModeEnabled());

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

    isCropModeEnabled() {
        return Boolean(this.cropState?.enabled);
    },

    canExtractSubfigures() {
        return Boolean(this.currentFigure) && !this.currentFigure.parent_figure_id;
    },

    toggleSubfigureCropMode() {
        if (!this.canExtractSubfigures()) {
            Utils.showToast('当前只支持从一级大图提取子图', 'info');
            return;
        }

        if (this.isCropModeEnabled()) {
            this.cancelSubfigureCropMode();
            return;
        }

        this.resetViewportState();
        this.cropState = {
            ...this.defaultCropState(),
            enabled: true
        };
        this.render();
    },

    cancelSubfigureCropMode() {
        this.endCropSelection();
        this.resetCropState();
        this.render();
    },

    clearSubfigureSelections() {
        if (!this.isCropModeEnabled() || this.cropState.submitting) return;
        this.cropState.selections = [];
        this.cropState.draft = null;
        this.render();
    },

    findCropSelection(selectionID) {
        return (this.cropState?.selections || []).find((selection) => selection.id === selectionID);
    },

    removeCropSelection(selectionID) {
        if (!selectionID || !this.isCropModeEnabled() || this.cropState.submitting) return;
        this.cropState.selections = (this.cropState.selections || []).filter((selection) => selection.id !== selectionID);
        this.render();
    },

    cropPointFromOverlay(event, overlay) {
        const rect = overlay.getBoundingClientRect();
        const width = rect.width || 0;
        const height = rect.height || 0;
        if (!width || !height) {
            return null;
        }

        return {
            x: Math.max(0, Math.min(1, (event.clientX - rect.left) / width)),
            y: Math.max(0, Math.min(1, (event.clientY - rect.top) / height))
        };
    },

    beginCropSelection(event, overlay) {
        if (!this.isCropModeEnabled() || this.cropState.submitting || event.button !== 0) return;

        const point = this.cropPointFromOverlay(event, overlay);
        if (!point) return;

        event.preventDefault();
        event.stopPropagation();
        this.cropState.pointerId = event.pointerId;
        this.cropState.draft = {
            startX: point.x,
            startY: point.y,
            x: point.x,
            y: point.y,
            width: 0,
            height: 0
        };
        if (typeof overlay.setPointerCapture === 'function') {
            try {
                overlay.setPointerCapture(event.pointerId);
            } catch (error) {
                // Ignore pointer capture errors from synthetic events.
            }
        }
        this.refreshCropOverlay();
    },

    updateCropSelection(event) {
        if (!this.isCropModeEnabled() || !this.cropState?.draft || event.pointerId !== this.cropState.pointerId) return;
        const overlay = this.body?.querySelector('[data-figure-crop-overlay]');
        if (!overlay) return;

        const point = this.cropPointFromOverlay(event, overlay);
        if (!point) return;

        const draft = this.cropState.draft;
        this.cropState.draft = {
            ...draft,
            x: Math.min(draft.startX, point.x),
            y: Math.min(draft.startY, point.y),
            width: Math.abs(point.x - draft.startX),
            height: Math.abs(point.y - draft.startY)
        };
        this.refreshCropOverlay();
    },

    endCropSelection(event = null) {
        if (!this.isCropModeEnabled() || !this.cropState?.draft) return;
        if (event && event.pointerId !== this.cropState.pointerId) return;

        const overlay = this.body?.querySelector('[data-figure-crop-overlay]');
        if (overlay && this.cropState.pointerId !== null && typeof overlay.hasPointerCapture === 'function' && overlay.hasPointerCapture(this.cropState.pointerId)) {
            try {
                overlay.releasePointerCapture(this.cropState.pointerId);
            } catch (error) {
                // Ignore release failures after pointer cancellation.
            }
        }

        const draft = this.cropState.draft;
        const nextSelection = draft && draft.width >= 0.02 && draft.height >= 0.02 ? {
            id: `subfigure-${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`,
            x: draft.x,
            y: draft.y,
            width: draft.width,
            height: draft.height,
            caption: ''
        } : null;

        this.cropState.pointerId = null;
        this.cropState.draft = null;
        if (nextSelection) {
            this.cropState.selections = [...(this.cropState.selections || []), nextSelection];
            this.render();
            return;
        }

        this.refreshCropOverlay();
    },

    renderCropOverlayContent() {
        if (!this.isCropModeEnabled()) return '';

        const boxes = [...(this.cropState?.selections || [])];
        if (this.cropState?.draft) {
            boxes.push({
                id: 'draft',
                x: this.cropState.draft.x,
                y: this.cropState.draft.y,
                width: this.cropState.draft.width,
                height: this.cropState.draft.height,
                draft: true
            });
        }

        return boxes.map((selection, index) => `
            <div
                class="figure-crop-box ${selection.draft ? 'is-draft' : ''}"
                style="left:${selection.x * 100}%;top:${selection.y * 100}%;width:${selection.width * 100}%;height:${selection.height * 100}%"
            >
                <span>${selection.draft ? '拖拽中' : index + 1}</span>
            </div>
        `).join('');
    },

    refreshCropOverlay() {
        const overlay = this.body?.querySelector('[data-figure-crop-overlay]');
        if (!overlay) return;
        overlay.innerHTML = this.renderCropOverlayContent();
    },

    async buildSubfigureImageData(selection) {
        const image = this.body?.querySelector('[data-figure-image]');
        if (!image || !image.naturalWidth || !image.naturalHeight) {
            throw new Error('当前图片尚未加载完成，无法提取子图');
        }

        const left = Math.max(0, Math.floor(selection.x * image.naturalWidth));
        const top = Math.max(0, Math.floor(selection.y * image.naturalHeight));
        const right = Math.min(image.naturalWidth, Math.ceil((selection.x + selection.width) * image.naturalWidth));
        const bottom = Math.min(image.naturalHeight, Math.ceil((selection.y + selection.height) * image.naturalHeight));
        const width = right - left;
        const height = bottom - top;
        if (width < 2 || height < 2) {
            throw new Error('子图区域过小，请重新选择');
        }

        const canvas = document.createElement('canvas');
        canvas.width = width;
        canvas.height = height;
        const context = canvas.getContext('2d', { alpha: false });
        context.fillStyle = '#ffffff';
        context.fillRect(0, 0, width, height);
        context.drawImage(image, left, top, width, height, 0, 0, width, height);
        return canvas.toDataURL('image/png');
    },

    async submitSubfigureSelections() {
        if (!this.currentFigure || !this.isCropModeEnabled() || this.cropState.submitting) return;
        if (!(this.cropState.selections || []).length) {
            Utils.showToast('请先框选至少一个子图区域', 'error');
            return;
        }

        this.cropState.submitting = true;
        this.render();

        try {
            const regions = [];
            for (const selection of this.cropState.selections) {
                regions.push({
                    x: selection.x,
                    y: selection.y,
                    width: selection.width,
                    height: selection.height,
                    caption: String(selection.caption || '').trim(),
                    image_data: await this.buildSubfigureImageData(selection)
                });
            }

            const payload = await API.createSubfigures(this.currentFigure.id, { regions });
            this.syncPaperMetadata(payload.paper);
            this.resetCropState();
            this.render();
            Utils.showToast(`已提取 ${payload.added_count || 0} 张子图`);
            if (typeof this.onMetaChanged === 'function') {
                await this.onMetaChanged(payload.paper);
            }
        } catch (error) {
            this.cropState.submitting = false;
            this.render();
            Utils.showToast(error.message, 'error');
        }
    },

    currentPaperDetail() {
        return this.paperDetails.get(Number(this.currentFigure?.paper_id)) || null;
    },

    currentFigureDetail() {
        const paper = this.currentPaperDetail();
        if (!paper) {
            return this.currentFigure || null;
        }
        return (paper.figures || []).find((figure) => Number(figure.id) === Number(this.currentFigure?.id)) || this.currentFigure || null;
    },

    currentFigureSubfigures() {
        const figure = this.currentFigureDetail();
        return Array.isArray(figure?.subfigures) ? figure.subfigures : [];
    },

    requestCurrentPaperDetail() {
        const paperID = Number(this.currentFigure?.paper_id || 0);
        if (!paperID || this.paperDetails.has(paperID) || this.paperDetailPromises.has(paperID)) {
            return;
        }

        const promise = API.getPaper(paperID)
            .then((paper) => {
                this.syncPaperMetadata(paper);
                if (Number(this.currentFigure?.paper_id || 0) === paperID) {
                    this.render();
                }
                return paper;
            })
            .catch(() => null)
            .finally(() => {
                this.paperDetailPromises.delete(paperID);
            });
        this.paperDetailPromises.set(paperID, promise);
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
            prevButton.disabled = !this.canMovePrevious() || this.loadingPage || Boolean(this.aiRequestState?.loading) || this.isCropModeEnabled();
        }
        if (nextButton) {
            nextButton.disabled = !this.canMoveNext() || this.loadingPage || Boolean(this.aiRequestState?.loading) || this.isCropModeEnabled();
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

    renderPresetTagButtons() {
        const presets = Array.isArray(Utils.defaultFigureTagPresets) ? Utils.defaultFigureTagPresets : [];
        const existing = new Set(this.currentFigureTagNames().map((tag) => tag.toLowerCase()));
        return presets.map((tagName) => {
            const applied = existing.has(tagName.toLowerCase());
            const action = applied ? 'remove-tag' : 'apply-tag';
            return `
                <button
                    class="figure-tag-preset ${applied ? 'is-applied' : ''}"
                    type="button"
                    data-figure-meta-action="${action}"
                    data-tag-name="${Utils.escapeHTML(tagName)}"
                    aria-pressed="${applied ? 'true' : 'false'}"
                    title="${applied ? `再次点击取消 ${Utils.escapeHTML(tagName)}` : `点击添加 ${Utils.escapeHTML(tagName)}`}"
                >${Utils.escapeHTML(tagName)}</button>
            `;
        }).join('');
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

    renderFigureLocation(figure) {
        const label = figure.display_label || (figure.subfigure_label ? `Fig ${figure.figure_index || '-'}${figure.subfigure_label}` : `Fig ${figure.figure_index || '-'}`);
        const source = figure.source === 'manual' ? ' · 人工提取' : '';
        const parent = figure.parent_figure_id ? ` · 来自 ${figure.parent_display_label || `Fig ${figure.figure_index || '-'}`}` : '';
        return `第 ${figure.page_number || '-'} 页 · ${Utils.escapeHTML(label)}${parent}${source}`;
    },

    renderSubfigureSelectionList() {
        const selections = this.cropState?.selections || [];
        if (!selections.length) {
            return '<div class="figure-subfigure-empty"><p>在图片上拖拽即可框选子图区域。</p></div>';
        }

        return selections.map((selection, index) => `
            <article class="figure-subfigure-selection">
                <div class="figure-subfigure-selection-head">
                    <strong>区域 ${index + 1}</strong>
                    <button class="btn btn-outline btn-small" type="button" data-subfigure-action="remove-selection" data-selection-id="${selection.id}">删除</button>
                </div>
                <div class="figure-subfigure-selection-meta">
                    <span>x ${(selection.x * 100).toFixed(1)}%</span>
                    <span>y ${(selection.y * 100).toFixed(1)}%</span>
                    <span>w ${(selection.width * 100).toFixed(1)}%</span>
                    <span>h ${(selection.height * 100).toFixed(1)}%</span>
                </div>
                <label class="field">
                    <span>Caption / 备注</span>
                    <textarea
                        class="form-textarea figure-subfigure-caption"
                        rows="2"
                        placeholder="可选，保存后作为子图说明"
                        data-subfigure-field="caption"
                        data-selection-id="${selection.id}"
                    >${Utils.escapeHTML(selection.caption || '')}</textarea>
                </label>
            </article>
        `).join('');
    },

    renderSubfigurePanel(figure) {
        const subfigures = this.currentFigureSubfigures();
        const loading = this.paperDetailPromises.has(Number(figure.paper_id));

        if (figure.parent_figure_id) {
            return `
                <section class="figure-lightbox-subfigure">
                    <div class="figure-lightbox-ai-head">
                        <div>
                            <p class="eyebrow">Subfigure</p>
                            <h3>子图来源</h3>
                        </div>
                    </div>
                    <div class="figure-subfigure-summary is-compact">
                        <p>当前查看的是 <strong>${Utils.escapeHTML(figure.display_label || '')}</strong>，来源于 <strong>${Utils.escapeHTML(figure.parent_display_label || '')}</strong>。</p>
                    </div>
                </section>
            `;
        }

        const countLabel = loading ? '加载中...' : `${subfigures.length} 张`;
        return `
            <section class="figure-lightbox-subfigure">
                <div class="figure-lightbox-ai-head">
                    <div>
                        <p class="eyebrow">Subfigure</p>
                        <h3>子图提取</h3>
                    </div>
                    <span class="figure-subfigure-count">已有 ${countLabel}</span>
                </div>
                <div class="figure-subfigure-summary">
                    <p>从当前大图里框选感兴趣的局部，系统会自动命名为不重复的 ${Utils.escapeHTML(figure.display_label || `Fig ${figure.figure_index || '-'}`)}a / b / c ...</p>
                    ${subfigures.length ? `
                        <div class="figure-subfigure-chip-list">
                            ${subfigures.map((item) => `<span class="figure-subfigure-chip">${Utils.escapeHTML(item.display_label || '')}</span>`).join('')}
                        </div>
                    ` : '<p class="muted">当前还没有提取过子图。</p>'}
                </div>
                ${this.isCropModeEnabled() ? `
                    <div class="figure-subfigure-editor">
                        <div class="figure-subfigure-editor-head">
                            <p>在左侧图片上拖拽框选，右侧会累积待保存的子图。</p>
                            <div class="figure-subfigure-editor-actions">
                                <button class="btn btn-outline btn-small" type="button" data-figure-action="clear-subfigure-selections" ${(this.cropState.selections || []).length ? '' : 'disabled'}>清空</button>
                                <button class="btn btn-outline btn-small" type="button" data-figure-action="cancel-subfigure-crop" ${this.cropState.submitting ? 'disabled' : ''}>取消</button>
                                <button class="btn btn-primary btn-small" type="button" data-figure-action="save-subfigures" ${(this.cropState.selections || []).length && !this.cropState.submitting ? '' : 'disabled'}>${this.cropState.submitting ? '保存中...' : '保存子图'}</button>
                            </div>
                        </div>
                        <div class="figure-subfigure-selection-list">${this.renderSubfigureSelectionList()}</div>
                    </div>
                ` : `
                    <div class="figure-subfigure-actions">
                        <button class="btn btn-outline" type="button" data-figure-action="toggle-subfigure-crop">开始子图截取</button>
                    </div>
                `}
            </section>
        `;
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

    isCurrentAINoteWriteDisabled(kind) {
        if (kind !== 'answer') return false;
        const figureID = Number(this.currentFigure?.id || 0);
        return Boolean(
            this.aiRequestState?.loading
            && this.aiRequestState.figureID === figureID
            && this.aiRequestState.action === 'figure_interpretation'
        );
    },

    aiNoteWriteDisabledReason(kind) {
        if (!this.isCurrentAINoteWriteDisabled(kind)) return '';
        return '图片解读输出中，完成后才能写入笔记';
    },

    renderAIHeadActions(options = {}) {
        const {
            copyKind = '',
            noteKind = '',
            disableNote = false,
            noteTitle = ''
        } = options;
        const buttons = [];

        if (copyKind) {
            buttons.push(`<button class="btn btn-outline btn-small" type="button" data-figure-ai-copy="${Utils.escapeHTML(copyKind)}">复制</button>`);
        }
        if (noteKind) {
            const titleAttr = noteTitle ? ` title="${Utils.escapeHTML(noteTitle)}"` : '';
            buttons.push(`<button class="btn btn-outline btn-small" type="button" data-figure-ai-note="${Utils.escapeHTML(noteKind)}" ${disableNote ? 'disabled' : ''}${titleAttr}>写入笔记</button>`);
        }

        if (!buttons.length) return '';
        return `<div class="figure-ai-head-actions">${buttons.join('')}</div>`;
    },

    async appendAIResultToNotes(kind) {
        const disabledReason = this.aiNoteWriteDisabledReason(kind);
        if (disabledReason) {
            Utils.showToast(disabledReason, 'info');
            return;
        }

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
        const label = figure.display_label || (figure.subfigure_label ? `Fig ${figure.figure_index || '-'}${figure.subfigure_label}` : `Fig ${figure.figure_index || '-'}`);
        const parent = figure.parent_figure_id ? `，来自 ${figure.parent_display_label || `Fig ${figure.figure_index || '-'}`}` : '';
        const location = `第 ${figure.page_number || '-'} 页 ${label}${parent}${figure.source === 'manual' ? '（人工提取）' : ''}`;
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
                return this.renderAICachedResult(fallback, currentTagNames, {
                    isWaiting: true,
                    disableNote: this.isCurrentAINoteWriteDisabled('answer'),
                    noteTitle: this.aiNoteWriteDisabledReason('answer')
                });
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
                        ${requestState.answer
                            ? this.renderAIHeadActions({
                                copyKind: 'answer',
                                noteKind: 'answer',
                                disableNote: this.isCurrentAINoteWriteDisabled('answer'),
                                noteTitle: this.aiNoteWriteDisabledReason('answer')
                            })
                            : ''}
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
                        ${requestState.answer
                            ? this.renderAIHeadActions({
                                copyKind: 'answer',
                                noteKind: 'answer'
                            })
                            : ''}
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

        return this.renderAICachedResult(result, currentTagNames, {
            isWaiting: false,
            disableNote: this.isCurrentAINoteWriteDisabled('answer'),
            noteTitle: this.aiNoteWriteDisabledReason('answer')
        });
    },

    renderAICachedResult(result, currentTagNames, options = {}) {
        const {
            isWaiting = false,
            disableNote = false,
            noteTitle = ''
        } = options;
        const tags = (result.suggested_tags || []).map((tag) => `
            <button class="tag-pill neutral figure-ai-tag-button ${currentTagNames.has(tag.trim().toLowerCase()) ? 'is-applied' : ''}" type="button" data-figure-meta-action="apply-tag" data-tag-name="${Utils.escapeHTML(tag)}" ${currentTagNames.has(tag.trim().toLowerCase()) ? 'disabled' : ''}>
                ${Utils.escapeHTML(tag)}
            </button>
        `).join('');

        return `
            <div class="figure-ai-result ${isWaiting ? 'loading' : ''}">
                <div class="figure-ai-head">
                    <p class="figure-ai-status">${Utils.escapeHTML(this.aiActionLabel(result.action))} · ${Utils.escapeHTML(result.provider)} · ${Utils.escapeHTML(result.model)} · ${Utils.escapeHTML(result.mode)}${isWaiting ? ' · 加载中' : ''}</p>
                    ${result.answer
                        ? this.renderAIHeadActions({
                            copyKind: 'answer',
                            noteKind: 'answer',
                            disableNote,
                            noteTitle
                        })
                        : ''}
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
        const aiLoading = Boolean(this.aiRequestState?.loading) || this.isCropModeEnabled();
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

        this.requestCurrentPaperDetail();
        const figure = this.currentFigure;
        const total = this.figures.length;
        const canPrev = this.canMovePrevious();
        const canNext = this.canMoveNext();
        const aiLoading = Boolean(this.aiRequestState?.loading);
        const cropModeActive = this.isCropModeEnabled();
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
                            <span class="figure-lightbox-hint">${cropModeActive ? '拖拽框选子图区域，完成后在右侧保存' : '滚轮缩放，按住左键或中键拖动'}</span>
                            <button class="btn btn-outline" type="button" data-figure-action="reset-view">复原视图</button>
                            <button class="btn btn-outline" type="button" data-figure-action="prev" ${!canPrev || this.loadingPage || aiLoading || cropModeActive ? 'disabled' : ''}>上一张</button>
                            <button class="btn btn-outline" type="button" data-figure-action="next" ${!canNext || this.loadingPage || aiLoading || cropModeActive ? 'disabled' : ''}>下一张</button>
                        </div>
                    </div>
                    <div class="figure-lightbox-media" data-figure-viewport>
                        <div class="figure-lightbox-image-stage" data-figure-image-stage>
                            <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.caption || figure.paper_title)}" data-figure-image>
                            <div class="figure-crop-overlay ${cropModeActive ? 'is-active' : ''}" data-figure-crop-overlay>${this.renderCropOverlayContent()}</div>
                        </div>
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
                            <strong>${this.renderFigureLocation(figure)}</strong>
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
                                <div class="figure-tag-presets">
                                    <span class="figure-tag-presets-label">快捷标签</span>
                                    <div class="figure-tag-presets-list">
                                        ${this.renderPresetTagButtons()}
                                    </div>
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

                    ${this.renderSubfigurePanel(figure)}

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
