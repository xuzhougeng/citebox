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
        this.aiCacheMaxSize = 200;
        this.activeAIByFigure = new Map();
        this.aiRequestState = null;
        this.paperDetails = new Map();
        this.paperDetailPromises = new Map();
        this.figureTagCatalog = new Map();
        this.figureTagCatalogPromise = null;
        this.viewState = this.defaultViewState();
        this.dragState = null;
        this.cropState = this.defaultCropState();
        this.paletteRequestState = this.defaultPaletteRequestState();
        this.tagInputDraft = '';

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
                return;
            }
            const tagShortcuts = {
                '1': '图 1', '2': '图 2', '3': '图 3', '4': '图 4',
                '5': '图 5', '6': '图 6', '7': '图 7',
                '9': '附图', '0': '图片摘要'
            };
            const shortcutTag = tagShortcuts[event.key];
            if (shortcutTag) {
                event.preventDefault();
                event.stopPropagation();
                void this.toggleTag(shortcutTag);
            }
        };

        this.closeButton.addEventListener('click', () => this.close());
        this.modal.addEventListener('click', (event) => {
            if (event.target === this.modal) {
                this.close();
            }
        });
        this.body.addEventListener('input', (event) => {
            const tagInput = event.target.closest('#figurePaperTagInput');
            if (tagInput) {
                this.tagInputDraft = tagInput.value;
                this.refreshFigureTagSuggestions();
                return;
            }

            const captionInput = event.target.closest('#figureCaptionInput');
            if (captionInput) {
                this.captionDraft = captionInput.value;
                return;
            }

            const subfigureFieldInput = event.target.closest('[data-subfigure-field]');
            if (!subfigureFieldInput) return;
            const selectionID = subfigureFieldInput.dataset.selectionId;
            const selection = this.findCropSelection(selectionID);
            if (!selection) return;
            if (subfigureFieldInput.dataset.subfigureField === 'label') {
                const normalized = this.normalizeSubfigureLabelInput(subfigureFieldInput.value);
                if (subfigureFieldInput.value !== normalized) {
                    subfigureFieldInput.value = normalized;
                }
                selection.label = normalized;
                return;
            }
            selection.caption = subfigureFieldInput.value;
        });
        this.body.addEventListener('focusin', (event) => {
            const tagInput = event.target.closest('#figurePaperTagInput');
            if (!tagInput) return;
            this.refreshFigureTagSuggestions();
        });
        this.body.addEventListener('click', async (event) => {
            const tagSuggestion = event.target.closest('[data-figure-tag-suggestion]');
            if (tagSuggestion) {
                await this.applyExistingTagSuggestion(tagSuggestion.dataset.figureTagSuggestion || '');
                return;
            }

            if (!event.target.closest('.figure-tag-add-input')) {
                this.hideFigureTagSuggestions();
            }

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
                if (subfigureButton.dataset.subfigureAction === 'extract-palette') {
                    await this.extractPaletteForFigure(Number(subfigureButton.dataset.figureId || 0));
                }
                if (subfigureButton.dataset.subfigureAction === 'copy-palette') {
                    await this.copyPaletteForFigure(Number(subfigureButton.dataset.figureId || 0));
                }
                if (subfigureButton.dataset.subfigureAction === 'delete-palette') {
                    await this.deletePaletteForFigure(Number(subfigureButton.dataset.figureId || 0));
                }
                if (subfigureButton.dataset.subfigureAction === 'delete-figure') {
                    await this.deleteSubfigure(Number(subfigureButton.dataset.figureId || 0));
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
        this.paletteRequestState = this.defaultPaletteRequestState();
        this.resetViewportState();
        this.resetCropState();
        this.syncCurrentFigureState({ forceDraftFromFigure: true });
        this.ensureFigureTagCatalogLoaded();
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
        if (!document.querySelector('.modal-shell:not(.hidden)')) {
            document.body.classList.remove('modal-open');
        }
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
            workspaceOpen: false,
            selections: [],
            draft: null,
            pointerId: null,
            submitting: false
        };
    },

    defaultPaletteRequestState() {
        return {
            loadingFigureIDs: new Set(),
            failedFigureMessages: new Map(),
            autoQueuedFigureIDs: new Set()
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

    isSubfigureWorkspaceOpen() {
        return Boolean(this.cropState?.workspaceOpen);
    },

    canExtractSubfigures() {
        return Boolean(this.currentFigure) && !this.currentFigure.parent_figure_id;
    },

    toggleSubfigureCropMode() {
        if (!this.canExtractSubfigures()) {
            Utils.showToast('当前只支持从一级大图提取子图', 'info');
            return;
        }

        if (this.isSubfigureWorkspaceOpen()) {
            this.cancelSubfigureCropMode();
            return;
        }

        this.resetViewportState();
        this.cropState = {
            ...this.defaultCropState(),
            enabled: true,
            workspaceOpen: true
        };
        this.render();
        this.queueAutoPaletteGeneration();
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

    normalizeSubfigureLabelInput(value) {
        return String(value || '').toLowerCase().replace(/[^a-z]/g, '');
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
            label: '',
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

    resetSubfigureWorkspaceSelections() {
        if (!this.isSubfigureWorkspaceOpen()) {
            this.resetCropState();
            return;
        }
        this.cropState = {
            ...this.defaultCropState(),
            enabled: true,
            workspaceOpen: true
        };
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
            const previousFigureIDs = new Set(this.currentFigureSubfigures().map((figure) => Number(figure.id || 0)));
            const regions = [];
            for (const selection of this.cropState.selections) {
                regions.push({
                    x: selection.x,
                    y: selection.y,
                    width: selection.width,
                    height: selection.height,
                    label: String(selection.label || '').trim(),
                    caption: String(selection.caption || '').trim()
                });
            }

            const payload = await API.createSubfigures(this.currentFigure.id, { regions });
            this.syncPaperMetadata(payload.paper);
            const createdFigureIDs = this.currentFigureSubfigures()
                .map((figure) => Number(figure.id || 0))
                .filter((figureID) => figureID > 0 && !previousFigureIDs.has(figureID));
            const generation = await this.generatePalettesForFigures(createdFigureIDs, {
                silentSuccess: true,
                silentError: true,
                deferMetaChanged: true
            });
            const latestPaper = generation.paper || payload.paper;
            this.resetSubfigureWorkspaceSelections();
            this.render();
            if (generation.failedCount) {
                Utils.showToast(`已提取 ${payload.added_count || 0} 张子图，${generation.generatedCount || 0} 张已生成配色`, 'info');
            } else if (generation.generatedCount) {
                Utils.showToast(`已提取 ${payload.added_count || 0} 张子图，并生成 ${generation.generatedCount} 组配色`);
            } else {
                Utils.showToast(`已提取 ${payload.added_count || 0} 张子图`);
            }
            if (typeof this.onMetaChanged === 'function') {
                await this.onMetaChanged(latestPaper);
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

    subfigureSelectionTitle(selection, index) {
        const label = this.normalizeSubfigureLabelInput(selection?.label || '');
        const figureIndex = Number(this.currentFigure?.figure_index || 0);
        if (label && figureIndex > 0) {
            return `Fig ${figureIndex}${label}`;
        }
        if (label) {
            return label;
        }
        return `区域 ${index + 1}`;
    },

    findFigureInCurrentPaper(figureID) {
        const normalizedID = Number(figureID || 0);
        if (!normalizedID) return null;

        const paper = this.currentPaperDetail();
        if (paper) {
            return (paper.figures || []).find((figure) => Number(figure.id) === normalizedID) || null;
        }

        if (Number(this.currentFigure?.id || 0) === normalizedID) {
            return this.currentFigure;
        }
        return null;
    },

    isPaletteLoading(figureID) {
        return this.paletteRequestState?.loadingFigureIDs?.has(Number(figureID || 0));
    },

    setPaletteLoading(figureID, loading) {
        const normalizedID = Number(figureID || 0);
        if (!normalizedID) return;
        if (loading) {
            this.paletteRequestState.loadingFigureIDs.add(normalizedID);
        } else {
            this.paletteRequestState.loadingFigureIDs.delete(normalizedID);
        }
    },

    paletteFailureMessage(figureID) {
        return this.paletteRequestState?.failedFigureMessages?.get(Number(figureID || 0)) || '';
    },

    setPaletteFailure(figureID, message = '') {
        const normalizedID = Number(figureID || 0);
        if (!normalizedID) return;
        if (!message) {
            this.paletteRequestState.failedFigureMessages.delete(normalizedID);
            return;
        }
        this.paletteRequestState.failedFigureMessages.set(normalizedID, String(message || '').trim());
    },

    figureHasPalette(figure) {
        return Boolean(Number(figure?.palette_count || 0) > 0 || (figure?.palette_colors || []).length > 0);
    },

    queueAutoPaletteGeneration() {
        const current = this.currentFigureDetail();
        if (!current) return;

        const targets = [];
        if (current.parent_figure_id) {
            targets.push(current);
        } else if (this.isSubfigureWorkspaceOpen()) {
            targets.push(...this.currentFigureSubfigures());
        }
        if (!targets.length) return;

        const figureIDs = targets
            .filter((figure) => figure?.parent_figure_id)
            .filter((figure) => !this.figureHasPalette(figure))
            .map((figure) => Number(figure.id || 0))
            .filter((figureID) => figureID > 0)
            .filter((figureID) => !this.isPaletteLoading(figureID))
            .filter((figureID) => !this.paletteFailureMessage(figureID))
            .filter((figureID) => !this.paletteRequestState.autoQueuedFigureIDs.has(figureID));
        if (!figureIDs.length) return;

        figureIDs.forEach((figureID) => this.paletteRequestState.autoQueuedFigureIDs.add(figureID));
        void this.generatePalettesForFigures(figureIDs, { silentSuccess: true, silentError: true, deferMetaChanged: true })
            .then(async ({ paper, failedCount }) => {
                if (paper && typeof this.onMetaChanged === 'function') {
                    await this.onMetaChanged(paper);
                }
                if (failedCount) {
                    this.render();
                }
            })
            .catch(() => {
                this.render();
            });
    },

    requestCurrentPaperDetail() {
        const paperID = Number(this.currentFigure?.paper_id || 0);
        if (!paperID || this.paperDetails.has(paperID) || this.paperDetailPromises.has(paperID)) {
            return;
        }

        const promise = API.getPaper(paperID)
            .then((paper) => {
                this.syncPaperMetadata(paper);
                this.queueAutoPaletteGeneration();
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

    async refreshPaperDetail(paperID) {
        const normalizedID = Number(paperID || 0);
        if (!normalizedID) return null;

        const paper = await API.getPaper(normalizedID);
        this.syncPaperMetadata(paper);
        return paper;
    },

    syncCurrentFigureState(options = {}) {
        const { forceDraftFromFigure = false } = options;
        const previousFigureID = Number(this.currentFigure?.id || 0);
        this.currentFigure = this.figures?.[this.index];
        if (previousFigureID !== Number(this.currentFigure?.id || 0)) {
            this.tagInputDraft = '';
        }
        if (forceDraftFromFigure || typeof this.captionDraft !== 'string') {
            this.captionDraft = this.currentFigure?.caption || '';
        }
    },

    mergeFigureTagCatalog(tags = []) {
        (Array.isArray(tags) ? tags : []).forEach((rawTag) => {
            const name = String(typeof rawTag === 'string' ? rawTag : rawTag?.name || '').trim();
            if (!name) return;

            const key = name.toLowerCase();
            const existing = this.figureTagCatalog.get(key) || {};
            const rawCount = typeof rawTag === 'string' ? 1 : Number(rawTag?.figure_count ?? 0);
            this.figureTagCatalog.set(key, {
                name,
                figure_count: Math.max(Number(existing.figure_count || 0), rawCount)
            });
        });
    },

    mergeFigureTagCatalogFromPaper(paper) {
        if (!paper) return;
        (paper.figures || []).forEach((figure) => {
            this.mergeFigureTagCatalog(figure.tags || []);
        });
    },

    ensureFigureTagCatalogLoaded() {
        if (this.figureTagCatalog.size > 0) {
            return Promise.resolve(this.figureTagCatalog);
        }
        if (this.figureTagCatalogPromise) {
            return this.figureTagCatalogPromise;
        }

        this.figureTagCatalogPromise = API.listTags({ scope: 'figure' })
            .then((payload) => {
                this.mergeFigureTagCatalog(payload?.tags || []);
                this.refreshFigureTagSuggestions();
                return this.figureTagCatalog;
            })
            .catch(() => this.figureTagCatalog)
            .finally(() => {
                this.figureTagCatalogPromise = null;
            });

        return this.figureTagCatalogPromise;
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
            this.evictMapIfNeeded(this.aiCache, this.aiCacheMaxSize);
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
                        this.evictMapIfNeeded(this.aiCache, this.aiCacheMaxSize);
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

    filteredFigureTagSuggestions(query = this.tagInputDraft) {
        const normalizedQuery = String(query || '').trim().toLowerCase();
        if (!normalizedQuery) {
            return [];
        }

        const appliedTags = new Set(this.currentFigureTagNames().map((tag) => tag.toLowerCase()));
        return Array.from(this.figureTagCatalog.values())
            .filter((tag) => tag?.name)
            .filter((tag) => !appliedTags.has(tag.name.toLowerCase()))
            .filter((tag) => tag.name.toLowerCase().includes(normalizedQuery))
            .sort((first, second) => {
                const firstName = String(first.name || '');
                const secondName = String(second.name || '');
                const firstLower = firstName.toLowerCase();
                const secondLower = secondName.toLowerCase();
                const firstStarts = firstLower.startsWith(normalizedQuery) ? 0 : 1;
                const secondStarts = secondLower.startsWith(normalizedQuery) ? 0 : 1;
                if (firstStarts !== secondStarts) {
                    return firstStarts - secondStarts;
                }

                const firstIndex = firstLower.indexOf(normalizedQuery);
                const secondIndex = secondLower.indexOf(normalizedQuery);
                if (firstIndex !== secondIndex) {
                    return firstIndex - secondIndex;
                }

                const usageDiff = Number(second.figure_count || 0) - Number(first.figure_count || 0);
                if (usageDiff !== 0) {
                    return usageDiff;
                }

                if (firstName.length !== secondName.length) {
                    return firstName.length - secondName.length;
                }
                return firstName.localeCompare(secondName, 'zh-CN', { sensitivity: 'base' });
            })
            .slice(0, 8);
    },

    renderFigureTagSuggestionList(query = this.tagInputDraft) {
        const suggestions = this.filteredFigureTagSuggestions(query);
        if (!suggestions.length) {
            return '';
        }

        return suggestions.map((tag) => `
            <button
                class="figure-tag-suggestion"
                type="button"
                data-figure-tag-suggestion="${Utils.escapeHTML(tag.name)}"
            >
                <span>${Utils.escapeHTML(tag.name)}</span>
                ${Number(tag.figure_count || 0) > 0 ? `<small>已用 ${Number(tag.figure_count)} 次</small>` : ''}
            </button>
        `).join('');
    },

    refreshFigureTagSuggestions() {
        const panel = this.body?.querySelector('[data-figure-tag-suggestions]');
        if (!panel) return;

        const markup = this.renderFigureTagSuggestionList(this.tagInputDraft);
        panel.innerHTML = markup;
        panel.classList.toggle('hidden', !markup);
    },

    hideFigureTagSuggestions() {
        const panel = this.body?.querySelector('[data-figure-tag-suggestions]');
        if (!panel) return;
        panel.classList.add('hidden');
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
        if (!normalized) return false;

        const draftTags = this.currentFigureTagNames();
        const existing = new Set(draftTags.map((tag) => tag.toLowerCase()));
        if (existing.has(normalized.toLowerCase())) {
            Utils.showToast('这个标签已经存在', 'info');
            return false;
        }

        draftTags.push(normalized);
        return this.updateCurrentFigureTags(draftTags, `已添加标签：${normalized}`);
    },

    async addTagFromInput() {
        const input = this.body.querySelector('#figurePaperTagInput');
        const value = input?.value.trim() || '';
        if (!value) return;
        const added = await this.applySuggestedTag(value);
        if (!added) return;

        this.tagInputDraft = '';
        if (input) {
            input.value = '';
        }
        this.refreshFigureTagSuggestions();
    },

    async applyExistingTagSuggestion(tagName) {
        const normalized = String(tagName || '').trim();
        if (!normalized) return;

        const applied = await this.applySuggestedTag(normalized);
        if (!applied) return;

        this.tagInputDraft = '';
        const input = this.body.querySelector('#figurePaperTagInput');
        if (input) {
            input.value = '';
        }
        this.refreshFigureTagSuggestions();
    },

    async removeTag(tagName) {
        const normalized = tagName.trim();
        if (!normalized) return;

        const draftTags = this.currentFigureTagNames().filter((tag) => tag.toLowerCase() !== normalized.toLowerCase());
        await this.updateCurrentFigureTags(draftTags, `已移除标签：${normalized}`);
    },

    async toggleTag(tagName) {
        if (!this.currentFigure) return;
        const existing = new Set(this.currentFigureTagNames().map((t) => t.toLowerCase()));
        if (existing.has(tagName.toLowerCase())) {
            await this.removeTag(tagName);
        } else {
            await this.applySuggestedTag(tagName);
        }
    },

    async updateCurrentFigureTags(tags, successMessage) {
        if (!this.currentFigure) return;

        try {
            const figureID = Number(this.currentFigure.id);
            const preserveActions = ['figure_interpretation'];
            if (this.activeAIAction() === 'tag_suggestion') {
                preserveActions.push('tag_suggestion');
            }
            const payload = await API.updateFigure(this.currentFigure.id, {
                tags,
                caption: this.currentFigureCaptionDraft(),
                notes_text: this.currentFigureNotesDraft()
            });
            this.clearFigureAIState(figureID, { preserveActions });
            this.syncPaperMetadata(payload.paper);
            this.mergeFigureTagCatalog(tags);
            Utils.showToast(successMessage);
            this.render();
            if (typeof this.onMetaChanged === 'function') {
                await this.onMetaChanged(payload.paper);
            }
            return true;
        } catch (error) {
            Utils.showToast(error.message, 'error');
            return false;
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
        this.evictMapIfNeeded(this.paperDetails, 50);
        this.mergeFigureTagCatalogFromPaper(paper);
        this.figures = mergeFigureCollectionWithPaper(this.figures, paper);
        this.syncCurrentFigureState({ forceDraftFromFigure: true });
    },

    renderPaletteSwatches(colors = [], options = {}) {
        const { compact = false } = options;
        const list = Array.isArray(colors) ? colors.filter((color) => String(color || '').trim()) : [];
        if (!list.length) {
            return '<span class="figure-preview-empty">还没有保存配色</span>';
        }

        return `
            <div class="palette-swatch-list ${compact ? 'is-compact' : ''}">
                ${list.map((color) => `
                    <span class="palette-swatch" title="${Utils.escapeHTML(color)}">
                        <span class="palette-swatch-chip" style="background:${Utils.escapeHTML(color)}"></span>
                        <span class="palette-swatch-label">${Utils.escapeHTML(color)}</span>
                    </span>
                `).join('')}
            </div>
        `;
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
            return '<div class="figure-subfigure-empty"><p>左侧拖拽后，新的截取结果会出现在这里。</p></div>';
        }

        return selections.map((selection, index) => `
            <article class="figure-subfigure-selection">
                <div class="figure-subfigure-selection-head">
                    <strong>${Utils.escapeHTML(this.subfigureSelectionTitle(selection, index))}</strong>
                    <button class="btn btn-outline btn-small" type="button" data-subfigure-action="remove-selection" data-selection-id="${selection.id}">删除</button>
                </div>
                <div class="figure-subfigure-selection-meta">
                    <span>x ${(selection.x * 100).toFixed(1)}%</span>
                    <span>y ${(selection.y * 100).toFixed(1)}%</span>
                    <span>w ${(selection.width * 100).toFixed(1)}%</span>
                    <span>h ${(selection.height * 100).toFixed(1)}%</span>
                </div>
                <label class="field">
                    <span>名称 / Label</span>
                    <input
                        class="form-input"
                        type="text"
                        maxlength="12"
                        placeholder="只支持 a-z；留空自动补 a / b / c"
                        data-subfigure-field="label"
                        data-selection-id="${selection.id}"
                        value="${Utils.escapeHTML(this.normalizeSubfigureLabelInput(selection.label || ''))}"
                    >
                </label>
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

    renderSubfigurePaletteCard(figure, options = {}) {
        if (!figure) return '';

        const { standalone = false } = options;
        const loading = this.isPaletteLoading(figure.id);
        const hasPalette = this.figureHasPalette(figure);
        const failure = this.paletteFailureMessage(figure.id);
        const caption = String(figure.caption || '').trim();
        const overlayButtonLabel = loading ? '提取中...' : (failure ? '重新提取配色' : '提取配色');
        const paletteStatus = loading
            ? '配色提取中'
            : (hasPalette ? (figure.palette_name || '已保存配色') : (failure ? '提取失败' : '待提取配色'));
        const paletteHint = loading
            ? '正在自动提取这张子图的主色。'
            : (hasPalette
                ? `这组颜色已经绑定到 ${figure.display_label || '当前子图'}。`
                : (failure ? '点击图片上的按钮重新提取配色。' : '点击图片上的按钮即可提取配色。'));
        const actions = [];
        if (hasPalette) {
            actions.push(`
                <button class="btn btn-outline btn-small" type="button" data-subfigure-action="copy-palette" data-figure-id="${figure.id}">
                    复制配色
                </button>
            `);
            actions.push(`
                <button class="btn btn-outline btn-small danger" type="button" data-subfigure-action="delete-palette" data-figure-id="${figure.id}">
                    删除配色
                </button>
            `);
        }
        actions.push(`
            <button class="btn btn-outline btn-small danger" type="button" data-subfigure-action="delete-figure" data-figure-id="${figure.id}">
                删除子图
            </button>
        `);

        return `
            <article class="figure-subfigure-card ${standalone ? 'is-standalone' : ''}">
                <div class="figure-subfigure-card-media">
                    <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.display_label || figure.paper_title || '子图')}">
                    ${hasPalette ? '' : `
                        <button
                            class="figure-subfigure-card-overlay-action"
                            type="button"
                            data-subfigure-action="extract-palette"
                            data-figure-id="${figure.id}"
                            ${loading ? 'disabled' : ''}
                        >${Utils.escapeHTML(overlayButtonLabel)}</button>
                    `}
                    <div class="figure-subfigure-card-badges">
                        <span class="figure-badge figure-badge-strong">${Utils.escapeHTML(figure.display_label || '子图')}</span>
                        ${figure.parent_display_label ? `<span class="figure-badge">来自 ${Utils.escapeHTML(figure.parent_display_label)}</span>` : ''}
                        ${hasPalette ? '<span class="figure-badge figure-badge-accent">已绑定配色</span>' : '<span class="figure-badge">未提取配色</span>'}
                    </div>
                </div>
                <div class="figure-subfigure-card-body">
                    <div class="figure-subfigure-card-head">
                        <strong>${Utils.escapeHTML(figure.display_label || '子图')}</strong>
                        <span>${Utils.escapeHTML(paletteStatus)}</span>
                    </div>
                    <div class="figure-subfigure-card-palette">
                        <span class="figure-subfigure-card-palette-label">提取颜色</span>
                        ${this.renderPaletteSwatches(figure.palette_colors || [], { compact: true })}
                    </div>
                    <p class="figure-subfigure-card-hint">${Utils.escapeHTML(paletteHint)}</p>
                    <p class="figure-subfigure-card-caption ${caption ? '' : 'is-empty'}">${Utils.escapeHTML(caption || '这个子图还没有单独说明。')}</p>
                    <div class="figure-subfigure-card-actions">
                        ${actions.join('')}
                    </div>
                </div>
            </article>
        `;
    },

    renderSubfigurePanel(figure) {
        const subfigures = this.currentFigureSubfigures();
        const loading = this.paperDetailPromises.has(Number(figure.paper_id));
        const workspaceOpen = this.isSubfigureWorkspaceOpen();

        if (figure.parent_figure_id) {
            return `
                <section class="figure-lightbox-subfigure is-subfigure-view">
                    <div class="figure-lightbox-ai-head">
                        <div>
                            <p class="eyebrow">Subfigure</p>
                            <h3>子图配色</h3>
                        </div>
                        <span class="figure-subfigure-count">仅保留配色相关信息</span>
                    </div>
                    <div class="figure-subfigure-summary is-compact">
                        <p>当前查看的是 <strong>${Utils.escapeHTML(figure.display_label || '')}</strong>，来源于 <strong>${Utils.escapeHTML(figure.parent_display_label || '')}</strong>。</p>
                        <p>子图不会进入独立图片库或独立笔记流，这里只保留预览和配色操作。</p>
                    </div>
                    <div class="figure-subfigure-card-list is-single">
                        ${this.renderSubfigurePaletteCard(figure, { standalone: true })}
                    </div>
                    <div class="figure-subfigure-actions">
                        <button class="btn btn-outline" type="button" data-figure-action="open-paper">查看来源文献</button>
                        <a class="btn btn-outline" href="/palettes">查看配色库</a>
                    </div>
                </section>
            `;
        }

        const countLabel = loading ? '加载中...' : `${subfigures.length} 张`;
        if (!workspaceOpen) {
            const countBadge = loading ? '...' : String(subfigures.length);
            return `
                <section class="figure-lightbox-subfigure is-entry">
                    <button
                        class="figure-subfigure-entry-trigger"
                        type="button"
                        data-figure-action="toggle-subfigure-crop"
                        aria-label="打开子图提取或查看，当前已有 ${Utils.escapeHTML(countLabel)}"
                    >
                        <div class="figure-subfigure-entry">
                            <div class="figure-subfigure-entry-copy">
                                <p class="eyebrow">子图</p>
                                <strong>子图提取或查看</strong>
                                <span>点击后进入工作台，查看已有子图或继续提取新的区域。</span>
                            </div>
                            <span class="figure-subfigure-entry-count">${Utils.escapeHTML(countBadge)}</span>
                        </div>
                    </button>
                </section>
            `;
        }

        return `
            <section class="figure-lightbox-subfigure is-workspace">
                <div class="figure-lightbox-ai-head">
                    <div>
                        <p class="eyebrow">Subfigure</p>
                        <h3>子图提取工作台</h3>
                    </div>
                    <span class="figure-subfigure-count">已有 ${countLabel}</span>
                </div>
                <div class="figure-subfigure-summary">
                    <p>左侧直接拖拽框选子图区域，右侧集中查看待保存结果和已有子图。</p>
                    <p>命名只支持 <strong>a-z</strong>；如果你输入大写，保存时会自动转成小写。留空则按 ${Utils.escapeHTML(figure.display_label || `Fig ${figure.figure_index || '-'}`)}a / b / c ... 自动补位。</p>
                </div>
                <div class="figure-subfigure-workspace-block">
                    <div class="figure-subfigure-workspace-head">
                        <strong>当前截取结果</strong>
                        <span>${(this.cropState.selections || []).length} 个待保存区域</span>
                    </div>
                    <div class="figure-subfigure-editor">
                        <div class="figure-subfigure-editor-head">
                            <p>左侧当前大图保持可操作状态；每框出一个区域，右侧都会累积一条待保存结果。</p>
                            <div class="figure-subfigure-editor-actions">
                                <button class="btn btn-outline btn-small" type="button" data-figure-action="clear-subfigure-selections" ${(this.cropState.selections || []).length ? '' : 'disabled'}>清空</button>
                                <button class="btn btn-outline btn-small" type="button" data-figure-action="cancel-subfigure-crop" ${this.cropState.submitting ? 'disabled' : ''}>关闭工作台</button>
                                <button class="btn btn-primary btn-small" type="button" data-figure-action="save-subfigures" ${(this.cropState.selections || []).length && !this.cropState.submitting ? '' : 'disabled'}>${this.cropState.submitting ? '保存中...' : '保存子图'}</button>
                            </div>
                        </div>
                        <div class="figure-subfigure-selection-list">${this.renderSubfigureSelectionList()}</div>
                    </div>
                </div>
                <div class="figure-subfigure-workspace-block">
                    <div class="figure-subfigure-workspace-head">
                        <strong>已保存子图</strong>
                        <span>${countLabel}</span>
                    </div>
                    ${subfigures.length
                        ? `<div class="figure-subfigure-card-list is-scrollable">${subfigures.map((item) => this.renderSubfigurePaletteCard(item)).join('')}</div>`
                        : '<div class="figure-subfigure-empty"><p>当前还没有提取过子图。</p></div>'}
                </div>
            </section>
        `;
    },

    async copyPaletteForFigure(figureID) {
        const figure = this.findFigureInCurrentPaper(figureID);
        const colors = Array.isArray(figure?.palette_colors) ? figure.palette_colors.filter(Boolean) : [];
        if (!colors.length) {
            Utils.showToast('当前子图还没有保存配色', 'error');
            return;
        }

        try {
            await navigator.clipboard.writeText(colors.join(', '));
            Utils.showToast(`${figure.display_label || '子图'} 配色已复制`);
        } catch (error) {
            Utils.showToast('复制失败', 'error');
        }
    },

    async deletePaletteForFigure(figureID) {
        const figure = this.findFigureInCurrentPaper(figureID);
        if (!figure?.palette_id) {
            Utils.showToast('当前子图还没有可删除的配色', 'error');
            return;
        }

        const confirmed = await Utils.confirm(`删除后只会移除 ${figure.display_label || '当前子图'} 的配色，不会删除子图本身。`);
        if (!confirmed) return;

        try {
            await API.deletePalette(figure.palette_id);
            const paper = await this.refreshPaperDetail(figure.paper_id);
            Utils.showToast('子图配色已删除');
            this.render();
            if (paper && typeof this.onMetaChanged === 'function') {
                await this.onMetaChanged(paper);
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async deleteSubfigure(figureID) {
        const figure = this.findFigureInCurrentPaper(figureID);
        if (!figure) {
            Utils.showToast('当前子图不存在或已被删除', 'error');
            return;
        }
        if (!figure.parent_figure_id) {
            Utils.showToast('这里只支持删除子图', 'error');
            return;
        }

        const confirmed = await Utils.confirm(`删除后会移除 ${figure.display_label || '当前子图'} 以及它绑定的配色。`);
        if (!confirmed) return;

        try {
            const payload = await API.deleteFigure(figure.id);
            const isCurrentFigure = Number(this.currentFigure?.id || 0) === Number(figure.id || 0);

            if (payload.paper) {
                this.syncPaperMetadata(payload.paper);
            }

            Utils.showToast('子图已删除');
            if (payload.paper && typeof this.onMetaChanged === 'function') {
                await this.onMetaChanged(payload.paper);
            }

            if (isCurrentFigure) {
                this.close();
                return;
            }

            this.render();
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    loadImageForPaletteExtraction(imageURL) {
        return new Promise((resolve, reject) => {
            const image = new Image();
            image.decoding = 'async';
            image.onload = () => resolve(image);
            image.onerror = () => reject(new Error('子图图片加载失败，暂时无法提取配色'));
            image.src = imageURL;
        });
    },

    rgbToHex(r, g, b) {
        const toHex = (value) => Math.max(0, Math.min(255, value)).toString(16).padStart(2, '0').toUpperCase();
        return `#${toHex(r)}${toHex(g)}${toHex(b)}`;
    },

    quantizePaletteChannel(value, step = 24) {
        return Math.max(0, Math.min(255, Math.round(value / step) * step));
    },

    colorDistance(first, second) {
        const dr = first.r - second.r;
        const dg = first.g - second.g;
        const db = first.b - second.b;
        return Math.sqrt(dr * dr + dg * dg + db * db);
    },

    collectPaletteCandidates(data, options = {}) {
        const { includeNeutral = false, limit = 6 } = options;
        const buckets = new Map();

        for (let index = 0; index < data.length; index += 4) {
            const alpha = data[index + 3];
            if (alpha < 180) continue;

            const red = data[index];
            const green = data[index + 1];
            const blue = data[index + 2];
            const max = Math.max(red, green, blue);
            const min = Math.min(red, green, blue);
            const saturation = max - min;
            const brightness = 0.299 * red + 0.587 * green + 0.114 * blue;

            if (!includeNeutral) {
                if (brightness > 246 && saturation < 18) continue;
                if (brightness < 18 && saturation < 18) continue;
                if (saturation < 14) continue;
            }

            const r = this.quantizePaletteChannel(red);
            const g = this.quantizePaletteChannel(green);
            const b = this.quantizePaletteChannel(blue);
            const key = `${r},${g},${b}`;
            const weight = includeNeutral ? 1 : (1 + saturation / 64);
            const current = buckets.get(key) || { r, g, b, count: 0, saturation };
            current.count += weight;
            current.saturation = Math.max(current.saturation, saturation);
            buckets.set(key, current);
        }

        const sorted = Array.from(buckets.values()).sort((first, second) => {
            if (second.count !== first.count) {
                return second.count - first.count;
            }
            return second.saturation - first.saturation;
        });

        const selected = [];
        for (const item of sorted) {
            if (selected.some((existing) => this.colorDistance(existing, item) < 46)) {
                continue;
            }
            selected.push(item);
            if (selected.length >= limit) {
                break;
            }
        }

        return selected.map((item) => this.rgbToHex(item.r, item.g, item.b));
    },

    async extractPaletteColorsFromImageURL(imageURL) {
        const image = await this.loadImageForPaletteExtraction(imageURL);
        const maxSize = 96;
        const ratio = Math.min(1, maxSize / Math.max(image.naturalWidth || 1, image.naturalHeight || 1));
        const width = Math.max(24, Math.round((image.naturalWidth || 1) * ratio));
        const height = Math.max(24, Math.round((image.naturalHeight || 1) * ratio));
        const canvas = document.createElement('canvas');
        canvas.width = width;
        canvas.height = height;

        const context = canvas.getContext('2d', { willReadFrequently: true });
        context.drawImage(image, 0, 0, width, height);
        const imageData = context.getImageData(0, 0, width, height);
        const primary = this.collectPaletteCandidates(imageData.data, { includeNeutral: false, limit: 6 });
        if (primary.length >= 3) {
            return primary;
        }

        const fallback = this.collectPaletteCandidates(imageData.data, { includeNeutral: true, limit: 6 });
        if (!fallback.length) {
            throw new Error('这张子图里没有提取到有效配色');
        }
        return fallback;
    },

    async extractPaletteForFigure(figureID) {
        return this.extractPaletteForFigureWithOptions(figureID, {});
    },

    async extractPaletteForFigureWithOptions(figureID, options = {}) {
        const {
            silentSuccess = false,
            silentError = false,
            deferMetaChanged = false
        } = options;
        const figure = this.findFigureInCurrentPaper(figureID);
        if (!figure) {
            if (!silentError) {
                Utils.showToast('没有找到对应的子图', 'error');
            }
            return null;
        }
        if (!figure.parent_figure_id) {
            if (!silentError) {
                Utils.showToast('当前只支持对子图提取配色', 'error');
            }
            return null;
        }
        if (this.isPaletteLoading(figureID)) {
            return null;
        }

        this.setPaletteFailure(figureID, '');
        this.setPaletteLoading(figureID, true);
        this.render();

        try {
            const colors = await this.extractPaletteColorsFromImageURL(figure.image_url);
            const payload = await API.createFigurePalette(figureID, { colors });
            this.syncPaperMetadata(payload.paper);
            this.setPaletteFailure(figureID, '');
            if (!silentSuccess) {
                Utils.showToast(`${figure.display_label || '子图'} 配色已保存`);
            }
            if (!deferMetaChanged && typeof this.onMetaChanged === 'function') {
                await this.onMetaChanged(payload.paper);
            }
            return payload;
        } catch (error) {
            this.setPaletteFailure(figureID, error.message || '配色提取失败');
            this.paletteRequestState.autoQueuedFigureIDs.delete(Number(figureID || 0));
            if (!silentError) {
                Utils.showToast(error.message, 'error');
            }
            return null;
        } finally {
            this.setPaletteLoading(figureID, false);
            this.render();
        }
    },

    async generatePalettesForFigures(figureIDs = [], options = {}) {
        const uniqueIDs = Array.from(new Set((Array.isArray(figureIDs) ? figureIDs : [])
            .map((figureID) => Number(figureID || 0))
            .filter((figureID) => figureID > 0)));
        let latestPaper = null;
        let generatedCount = 0;
        let failedCount = 0;

        for (const figureID of uniqueIDs) {
            const figure = this.findFigureInCurrentPaper(figureID);
            if (!figure || !figure.parent_figure_id || this.figureHasPalette(figure)) {
                continue;
            }

            const payload = await this.extractPaletteForFigureWithOptions(figureID, options);
            if (payload?.paper) {
                latestPaper = payload.paper;
                generatedCount += 1;
                continue;
            }
            if (this.paletteFailureMessage(figureID)) {
                failedCount += 1;
            }
        }

        return { paper: latestPaper, generatedCount, failedCount };
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

        const filename = this.currentFigure.original_name || this.currentFigure.filename || 'figure.png';
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
        const subfigureWorkspaceOpen = this.isSubfigureWorkspaceOpen();
        const minimalFigureWorkspace = subfigureWorkspaceOpen || Boolean(figure.parent_figure_id);
        const canUseDesktopSave = Utils.supportsDesktopSave();
        const captionDraft = this.captionDraft ?? (figure.caption || '');
        const notePreview = String(figure.notes_text || '').replace(/\s+/g, ' ').trim();
        const mediaHint = subfigureWorkspaceOpen
            ? '左侧拖拽框选，右侧集中保存子图结果'
            : (figure.parent_figure_id ? '当前是子图预览，右侧仅保留配色相关内容' : '滚轮缩放，按住左键或中键拖动');
        const editableTags = (figure.tags || []).map((tag) => `
            <button class="figure-editable-tag" type="button" data-figure-meta-action="remove-tag" data-tag-name="${Utils.escapeHTML(typeof tag === 'string' ? tag : tag.name || '')}" aria-label="移除标签 ${Utils.escapeHTML(typeof tag === 'string' ? tag : tag.name || '')}">
                <span>${Utils.escapeHTML(typeof tag === 'string' ? tag : tag.name || '')}</span>
                <span aria-hidden="true">+</span>
            </button>
        `).join('');
        this.body.innerHTML = `
            <div class="figure-lightbox ${subfigureWorkspaceOpen ? 'is-subfigure-workspace' : ''}">
                <section class="figure-lightbox-media-panel">
                    <div class="figure-lightbox-toolbar">
                        <div class="figure-lightbox-counter">第 ${this.index + 1} / ${total} 张 · 第 ${this.page} / ${this.totalPages} 页</div>
                        <div class="figure-lightbox-nav">
                            <span class="figure-lightbox-hint">${Utils.escapeHTML(mediaHint)}</span>
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
                    ${minimalFigureWorkspace ? '' : `
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
                    `}
                </section>

                <aside class="figure-lightbox-side">
                    <div class="figure-lightbox-head">
                        <p class="eyebrow">Image Library</p>
                        <h2>${Utils.escapeHTML(figure.paper_title)}</h2>
                    </div>

                    ${this.renderSubfigurePanel(figure)}

                    ${minimalFigureWorkspace ? '' : `
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
                                        <div class="figure-tag-add-input">
                                            <input id="figurePaperTagInput" class="form-input" type="text" placeholder="添加标签" value="${Utils.escapeHTML(this.tagInputDraft || '')}" autocomplete="off" spellcheck="false">
                                            <div class="figure-tag-autocomplete hidden" data-figure-tag-suggestions></div>
                                        </div>
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
                    `}
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
        this.refreshFigureTagSuggestions();
    },

    evictMapIfNeeded(map, maxSize) {
        if (map.size <= maxSize) return;
        const keysToDelete = Array.from(map.keys()).slice(0, map.size - maxSize);
        for (const key of keysToDelete) {
            map.delete(key);
        }
    }
};
