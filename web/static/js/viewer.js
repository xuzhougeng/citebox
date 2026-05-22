const ResourceViewerPage = {
    init() {
        if (typeof t !== 'function') window.t = function(k, f) { return f || k; };
        this.stage = document.getElementById('viewerStage');
        this.closeButton = document.getElementById('viewerCloseButton');
        this.kindLabel = document.getElementById('viewerKindLabel');
        this.title = document.getElementById('viewerTitle');
        this.toolbarActions = document.getElementById('viewerToolbarActions');
        this.resetButton = document.getElementById('viewerResetButton');
        this.pdfControls = document.getElementById('viewerPdfControls');
        this.pdfPrevButton = document.getElementById('viewerPdfPrevButton');
        this.pdfNextButton = document.getElementById('viewerPdfNextButton');
        this.pdfPageInput = document.getElementById('viewerPdfPageInput');
        this.pdfPageTotal = document.getElementById('viewerPdfPageTotal');
        this.pdfZoomOutButton = document.getElementById('viewerPdfZoomOutButton');
        this.pdfZoomInButton = document.getElementById('viewerPdfZoomInButton');
        this.pdfZoomLabel = document.getElementById('viewerPdfZoomLabel');
        this.pdfFitButton = document.getElementById('viewerPdfFitButton');
        this.pdfDetailLink = document.getElementById('viewerPdfDetailLink');
        this.selectionMenu = document.getElementById('viewerSelectionMenu');
        this.pdfAIAskDialog = null;
        this.closing = false;
        this.viewState = this.defaultViewState();
        this.pdfLoadToken = 0;
        this.pdfState = this.defaultPDFState();
        this.dragState = null;
        this.resizeTimer = null;
        this.pdfSelectionTimer = null;
        this.pdfHighlightRenderTimer = null;

        this.closeButton?.addEventListener('click', () => this.close());
        this.resetButton?.addEventListener('click', () => {
            this.resetImageView();
            this.applyImageTransform();
        });
        document.addEventListener('keydown', (event) => {
            this.handleEscapeKey(event);
        });
        this.stage?.addEventListener('wheel', (event) => {
            const viewport = event.target.closest('[data-viewer-viewport]');
            if (!viewport) return;
            event.preventDefault();
            this.handleImageWheel(event, viewport);
        }, { passive: false });
        this.stage?.addEventListener('pointerdown', (event) => {
            const viewport = event.target.closest('[data-viewer-viewport]');
            if (!viewport) return;
            this.beginImageDrag(event, viewport);
        });
        document.addEventListener('pointermove', (event) => {
            this.updateImageDrag(event);
        });
        document.addEventListener('pointerup', (event) => {
            this.endImageDrag(event);
        });
        document.addEventListener('pointercancel', (event) => {
            this.endImageDrag(event);
        });
        document.addEventListener('selectionchange', () => {
            this.schedulePDFSelectionMenuRefresh();
        });
        document.addEventListener('pointerup', () => {
            this.schedulePDFSelectionMenuRefresh();
        });
        window.addEventListener('resize', () => {
            this.applyImageTransform();
            this.schedulePDFRerender();
        });
        this.bindPDFControls();
        this.bindPDFSelectionMenu();
        this.stage?.addEventListener('click', (event) => {
            if (event.target === this.stage) {
                this.close();
            }
        });

        this.render();
    },

    handleEscapeKey(event) {
        if (event.key !== 'Escape') return;
        event.preventDefault();
        event.stopPropagation?.();
        event.stopImmediatePropagation?.();
        if (this.selectionMenu && !this.selectionMenu.classList.contains('hidden')) {
            this.clearPDFSelection();
            return;
        }
        if (this.hasOpenTranslateDialog()) {
            return;
        }
        if (this.hasOpenPDFAIAskDialog()) {
            return;
        }
        this.close({ deferNavigation: true });
    },

    hasOpenTranslateDialog() {
        return Boolean(document.querySelector?.('.translate-dialog-overlay:not(.hidden)'));
    },

    hasOpenPDFAIAskDialog() {
        return Boolean(document.querySelector?.('.viewer-ai-ask-overlay:not(.hidden)'));
    },

    async render() {
        try {
            const resource = this.resolveResource();
            this.endImageDrag();
            this.resetImageView();
            this.destroyPDFState();
            document.title = `${resource.label} - CiteBox`;
            this.kindLabel.textContent = resource.label;
            this.title.textContent = resource.name;

            if (resource.kind === 'image') {
                this.toggleImageToolbar(true);
                this.togglePDFToolbar(false);
                this.stage.className = 'viewer-stage image-mode';
                this.stage.innerHTML = `
                    <div class="viewer-image-viewport" data-viewer-viewport>
                        <img class="viewer-image" src="${resource.href}" alt="${this.escapeHTML(resource.name)}" data-viewer-image>
                    </div>
                `;
                const image = this.stage.querySelector('[data-viewer-image]');
                if (image) {
                    if (image.complete) {
                        this.applyImageTransform();
                    } else {
                        image.addEventListener('load', () => this.applyImageTransform(), { once: true });
                    }
                }
                return;
            }

            this.toggleImageToolbar(false);
            if (resource.kind === 'pdf') {
                await this.renderPDFResource(resource);
                return;
            }
        } catch (error) {
            this.toggleImageToolbar(false);
            this.togglePDFToolbar(false);
            document.title = t('viewer.err_title_failed', '文件查看失败 - CiteBox');
            this.kindLabel.textContent = t('viewer.err_kind_label', '文件查看');
            this.title.textContent = t('viewer.err_cannot_open', '无法打开资源');
            this.stage.className = 'viewer-stage';
            this.stage.innerHTML = `
                <div class="viewer-empty">
                    <h1>${t('viewer.err_cannot_open_title', '无法打开这个资源')}</h1>
                    <p>${this.escapeHTML(error.message || t('viewer.err_invalid_resource', '资源地址无效或不受支持。'))}</p>
                </div>
            `;
        }
    },

    bindPDFControls() {
        this.pdfPrevButton?.addEventListener('click', async () => {
            await this.goToPDFPage((this.pdfState.pageNumber || 1) - 1);
        });
        this.pdfNextButton?.addEventListener('click', async () => {
            await this.goToPDFPage((this.pdfState.pageNumber || 1) + 1);
        });
        this.pdfPageInput?.addEventListener('change', async () => {
            await this.goToPDFPage(Number(this.pdfPageInput.value || 1));
        });
        this.pdfPageInput?.addEventListener('keydown', async (event) => {
            if (event.key !== 'Enter') return;
            event.preventDefault();
            await this.goToPDFPage(Number(this.pdfPageInput.value || 1));
        });
        this.pdfZoomOutButton?.addEventListener('click', async () => {
            await this.setPDFScale((this.pdfState.scale || 1) / 1.2);
        });
        this.pdfZoomInButton?.addEventListener('click', async () => {
            await this.setPDFScale((this.pdfState.scale || 1) * 1.2);
        });
        this.pdfFitButton?.addEventListener('click', async () => {
            this.pdfState.fitMode = true;
            this.applyPDFViewerScale();
            this.syncPDFToolbar();
        });
    },

    bindPDFSelectionMenu() {
        this.selectionMenu?.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-pdf-selection-action]');
            if (!button) return;
            event.preventDefault();
            event.stopPropagation();
            if (button.dataset.pdfSelectionAction === 'copy') {
                await this.copyPDFSelection();
            }
            if (button.dataset.pdfSelectionAction === 'translate') {
                await this.translatePDFSelection();
            }
            if (button.dataset.pdfSelectionAction === 'highlight') {
                await this.highlightPDFSelection();
            }
            if (button.dataset.pdfSelectionAction === 'ask-ai') {
                await this.openPDFSelectionAIAsk();
            }
        });

        document.addEventListener('scroll', () => {
            this.clearPDFSelection();
        }, true);
    },

    defaultViewState() {
        return {
            scale: 1,
            x: 0,
            y: 0
        };
    },

    defaultPDFState() {
        return {
            pdfjsLib: null,
            pdfjsViewerLib: null,
            loadingTask: null,
            pdfDocument: null,
            resource: null,
            eventBus: null,
            linkService: null,
            pdfViewer: null,
            loadToken: 0,
            pageNumber: 1,
            pageCount: 0,
            scale: 1,
            fitMode: true,
            selectionText: '',
            selectionClientRect: null,
            highlights: [],
            targetAnnotationId: '',
            targetAnnotationApplied: false,
            targetAnnotationHighlightActive: false,
            targetAnnotationTimer: null
        };
    },

    resetImageView() {
        this.viewState = this.defaultViewState();
        this.dragState = null;
    },

    toggleImageToolbar(visible) {
        if (!this.toolbarActions) return;
        this.toolbarActions.hidden = !visible;
        if (!visible && this.resetButton) {
            this.resetButton.disabled = true;
        }
    },

    togglePDFToolbar(visible) {
        if (!this.pdfControls) return;
        this.pdfControls.hidden = !visible;
        if (!visible && this.pdfDetailLink) {
            this.pdfDetailLink.hidden = true;
        }
    },

    async ensurePDFJSReady(loadToken = this.pdfState?.loadToken) {
        if (this.pdfState.pdfjsLib) {
            return this.pdfState.pdfjsLib;
        }
        const pdfjsLib = await import('/static/vendor/pdfjs/build/pdf.mjs');
        pdfjsLib.GlobalWorkerOptions.workerSrc = '/static/vendor/pdfjs/build/pdf.worker.mjs';
        if (this.isCurrentPDFLoad(loadToken)) {
            this.pdfState.pdfjsLib = pdfjsLib;
        }
        return pdfjsLib;
    },

    async ensurePDFViewerReady(loadToken = this.pdfState?.loadToken) {
        const pdfjsLib = await this.ensurePDFJSReady(loadToken);
        let pdfjsViewerLib = this.pdfState.pdfjsViewerLib;
        if (!pdfjsViewerLib) {
            this.ensurePDFViewerCompatibility();
            this.ensurePDFViewerStyles();
            pdfjsViewerLib = await import('/static/vendor/pdfjs/web/pdf_viewer.mjs');
            if (this.isCurrentPDFLoad(loadToken)) {
                this.pdfState.pdfjsViewerLib = pdfjsViewerLib;
            }
        }
        return {
            pdfjsLib,
            pdfjsViewerLib
        };
    },

    ensurePDFViewerCompatibility() {
        if (typeof Map === 'undefined' || typeof Map.prototype.getOrInsertComputed === 'function') return;
        Object.defineProperty(Map.prototype, 'getOrInsertComputed', {
            configurable: true,
            writable: true,
            value(key, callback) {
                if (this.has(key)) {
                    return this.get(key);
                }
                const value = callback(key);
                this.set(key, value);
                return value;
            }
        });
    },

    ensurePDFViewerStyles() {
        if (document.getElementById?.('pdfjsViewerStylesheet')) return;
        const link = document.createElement('link');
        link.id = 'pdfjsViewerStylesheet';
        link.rel = 'stylesheet';
        link.href = '/static/vendor/pdfjs/web/pdf_viewer.css';
        document.head.appendChild(link);
    },

    async renderPDFResource(resource) {
        this.togglePDFToolbar(true);
        const previousPDFState = this.pdfState || this.defaultPDFState();
        const loadToken = (this.pdfLoadToken || 0) + 1;
        this.pdfLoadToken = loadToken;
        this.pdfState = {
            ...this.defaultPDFState(),
            pdfjsLib: previousPDFState.pdfjsLib,
            pdfjsViewerLib: previousPDFState.pdfjsViewerLib,
            resource,
            loadToken,
            pageNumber: this.initialPDFPageNumber(),
            scale: this.isNarrowPDFViewport() ? 0.9 : 1,
            fitMode: !this.isNarrowPDFViewport(),
            highlights: [],
            targetAnnotationId: this.initialPDFAnnotationID(resource),
            targetAnnotationApplied: false,
            targetAnnotationHighlightActive: false,
            targetAnnotationTimer: null
        };
        this.stage.className = 'viewer-stage pdf-mode';
        this.stage.innerHTML = `
            <div class="viewer-pdf-scroll" data-pdf-scroll tabindex="0">
                <div class="pdfViewer viewer-pdf-official" data-pdf-viewer></div>
                <div class="viewer-pdf-loading" data-pdf-loading>${t('viewer.pdf_loading', '正在加载 PDF...')}</div>
            </div>
        `;

        const paperId = String(resource.paperId || '').trim();
        if (this.pdfDetailLink && paperId) {
            this.pdfDetailLink.href = `/ai?paper_id=${encodeURIComponent(paperId)}`;
            this.pdfDetailLink.hidden = false;
        }
        this.loadPDFAnnotations(resource, loadToken);

        const loaded = await this.loadPDFDocument(resource.href, loadToken);
        if (!loaded || !this.isCurrentPDFLoad(loadToken)) {
            return;
        }
        const pdfViewer = this.pdfState.pdfViewer;
        const pageCount = Math.max(this.pdfState.pageCount || pdfViewer?.pagesCount || 0, 1);
        this.pdfState.pageNumber = Math.min(Math.max(this.pdfState.pageNumber, 1), pageCount);
        if (pdfViewer) {
            try {
                pdfViewer.currentPageNumber = this.pdfState.pageNumber;
            } catch (error) {
                // PDF.js may reject page changes before pagesinit; page state is synced by events.
            }
        }
        this.syncPDFToolbar();
    },

    initialPDFPageNumber() {
        const page = Number(new URLSearchParams(window.location.search).get('page') || 1);
        return page > 0 ? Math.floor(page) : 1;
    },

    initialPDFAnnotationID(resource = null) {
        const fromResource = String(resource?.annotationId || resource?.annotation_id || '').trim();
        if (fromResource && fromResource !== '0') return fromResource;
        const fromURL = String(new URLSearchParams(window.location.search).get('annotation_id') || '').trim();
        return fromURL && fromURL !== '0' ? fromURL : '';
    },

    isCurrentPDFLoad(loadToken, pdfViewer = null) {
        if (!this.pdfState || this.pdfState.loadToken !== loadToken) {
            return false;
        }
        return !pdfViewer || this.pdfState.pdfViewer === pdfViewer;
    },

    async loadPDFDocument(href, loadToken) {
        const { pdfjsLib, pdfjsViewerLib } = await this.ensurePDFViewerReady(loadToken);
        if (!this.isCurrentPDFLoad(loadToken)) {
            return false;
        }
        const scrollElement = this.stage?.querySelector('[data-pdf-scroll]');
        const viewerElement = this.stage?.querySelector('[data-pdf-viewer]');
        const loadingElement = this.stage?.querySelector('[data-pdf-loading]');
        if (!scrollElement || !viewerElement) {
            throw new Error(t('viewer.err_invalid_resource', '资源地址无效或不受支持。'));
        }

        const eventBus = new pdfjsViewerLib.EventBus();
        const linkService = new pdfjsViewerLib.PDFLinkService({ eventBus });
        const textLayerMode = pdfjsViewerLib.TextLayerMode?.ENABLE ?? 1;
        const pdfViewer = new pdfjsViewerLib.PDFViewer({
            container: scrollElement,
            viewer: viewerElement,
            eventBus,
            linkService,
            textLayerMode,
            annotationMode: pdfjsLib.AnnotationMode?.DISABLE ?? 0
        });
        linkService.setViewer(pdfViewer);

        this.pdfState.eventBus = eventBus;
        this.pdfState.linkService = linkService;
        this.pdfState.pdfViewer = pdfViewer;

        eventBus.on('pagesinit', () => {
            if (!this.isCurrentPDFLoad(loadToken, pdfViewer)) return;
            const pageCount = Math.max(pdfViewer.pagesCount || this.pdfState.pageCount || 0, 1);
            const pageNumber = Math.min(Math.max(this.pdfState.pageNumber || 1, 1), pageCount);
            this.pdfState.pageNumber = pageNumber;
            if (pdfViewer.currentPageNumber !== pageNumber) {
                pdfViewer.currentPageNumber = pageNumber;
            }
            this.applyPDFViewerScale();
            this.syncPDFToolbar();
            if (loadingElement) {
                loadingElement.hidden = true;
            }
            this.renderPDFHighlights();
        });
        eventBus.on('pagesloaded', () => {
            if (!this.isCurrentPDFLoad(loadToken, pdfViewer)) return;
            this.pdfState.pageCount = pdfViewer.pagesCount || this.pdfState.pageCount;
            this.syncPDFToolbar();
            this.renderPDFHighlights();
        });
        eventBus.on('pagechanging', (event) => {
            if (!this.isCurrentPDFLoad(loadToken, pdfViewer)) return;
            this.pdfState.pageNumber = event.pageNumber || pdfViewer.currentPageNumber || 1;
            this.clearPDFSelection();
            this.syncPDFToolbar();
            this.renderPDFHighlights();
        });
        eventBus.on('scalechanging', () => {
            if (!this.isCurrentPDFLoad(loadToken, pdfViewer)) return;
            this.pdfState.scale = pdfViewer.currentScale || this.pdfState.scale || 1;
            this.clearPDFSelection();
            this.syncPDFToolbar();
            this.schedulePDFHighlightsRender();
        });
        eventBus.on('pagerendered', () => {
            if (!this.isCurrentPDFLoad(loadToken, pdfViewer)) return;
            this.renderPDFHighlights();
        });
        eventBus.on('textlayerrendered', () => {
            if (!this.isCurrentPDFLoad(loadToken, pdfViewer)) return;
            this.renderPDFHighlights();
        });

        const loadingTask = pdfjsLib.getDocument({
            url: href,
            cMapUrl: '/static/vendor/pdfjs/cmaps/',
            cMapPacked: true,
            standardFontDataUrl: '/static/vendor/pdfjs/standard_fonts/',
            wasmUrl: '/static/vendor/pdfjs/wasm/'
        });
        this.pdfState.loadingTask = loadingTask;
        let pdfDocument = null;
        try {
            pdfDocument = await loadingTask.promise;
        } catch (error) {
            if (!this.isCurrentPDFLoad(loadToken, pdfViewer)) {
                return false;
            }
            throw error;
        }
        if (!this.isCurrentPDFLoad(loadToken, pdfViewer)) {
            if (pdfDocument && typeof pdfDocument.destroy === 'function') {
                pdfDocument.destroy().catch(() => {});
            }
            if (typeof loadingTask.destroy === 'function') {
                loadingTask.destroy().catch(() => {});
            }
            return false;
        }
        this.pdfState.pdfDocument = pdfDocument;
        this.pdfState.pageCount = pdfDocument.numPages || 0;
        pdfViewer.setDocument(pdfDocument);
        linkService.setDocument(pdfDocument, null);
        this.syncPDFToolbar();
        return true;
    },

    isNarrowPDFViewport() {
        return (window.innerWidth || 0) <= 720;
    },

    clampPDFScale(scale) {
        return Math.min(4, Math.max(0.35, Number(scale) || 1));
    },

    async goToPDFPage(pageNumber) {
        const pdfViewer = this.pdfState.pdfViewer;
        if (!pdfViewer) return;
        const pageCount = pdfViewer.pagesCount || this.pdfState.pageCount || 0;
        const nextPage = Math.min(Math.max(Math.floor(Number(pageNumber) || 1), 1), Math.max(pageCount, 1));
        if (nextPage === this.pdfState.pageNumber && pdfViewer.currentPageNumber === nextPage) {
            this.syncPDFToolbar();
            return;
        }
        this.pdfState.pageNumber = nextPage;
        pdfViewer.currentPageNumber = nextPage;
        this.clearPDFSelection();
        this.syncPDFToolbar();
    },

    async setPDFScale(scale) {
        if (!this.pdfState.pdfViewer) return;
        this.pdfState.fitMode = false;
        this.pdfState.scale = this.clampPDFScale(scale);
        this.applyPDFViewerScale();
        this.clearPDFSelection();
        this.syncPDFToolbar();
    },

    applyPDFViewerScale() {
        const pdfViewer = this.pdfState.pdfViewer;
        if (!pdfViewer) return;
        if (this.pdfState.fitMode) {
            pdfViewer.currentScaleValue = 'page-width';
            this.pdfState.scale = pdfViewer.currentScale || this.pdfState.scale || 1;
            return;
        }
        pdfViewer.currentScale = this.clampPDFScale(this.pdfState.scale);
        this.pdfState.scale = pdfViewer.currentScale || this.pdfState.scale || 1;
    },

    syncPDFToolbar() {
        if (!this.pdfControls || this.pdfControls.hidden) return;
        const pdfViewer = this.pdfState.pdfViewer;
        const pageCount = pdfViewer?.pagesCount || this.pdfState.pageCount || 0;
        const currentPage = pdfViewer?.pagesCount
            ? pdfViewer.currentPageNumber || this.pdfState.pageNumber || 1
            : this.pdfState.pageNumber || 1;
        const pageNumber = Math.min(Math.max(currentPage, 1), Math.max(pageCount, 1));
        this.pdfState.pageCount = pageCount;
        this.pdfState.pageNumber = pageNumber;
        if (pdfViewer?.currentScale) {
            this.pdfState.scale = pdfViewer.currentScale;
        }
        if (this.pdfPageInput) {
            this.pdfPageInput.max = String(Math.max(pageCount, 1));
            this.pdfPageInput.value = String(pageNumber);
        }
        if (this.pdfPageTotal) {
            this.pdfPageTotal.textContent = `/ ${pageCount || 1}`;
        }
        if (this.pdfPrevButton) {
            this.pdfPrevButton.disabled = pageNumber <= 1;
        }
        if (this.pdfNextButton) {
            this.pdfNextButton.disabled = !pageCount || pageNumber >= pageCount;
        }
        if (this.pdfZoomLabel) {
            this.pdfZoomLabel.textContent = `${Math.round((this.pdfState.scale || 1) * 100)}%`;
        }
    },

    schedulePDFRerender() {
        if (!this.pdfState?.pdfViewer || !this.pdfState.fitMode) return;
        window.clearTimeout(this.resizeTimer);
        this.resizeTimer = window.setTimeout(() => {
            this.applyPDFViewerScale();
            this.syncPDFToolbar();
        }, 120);
    },

    destroyPDFState() {
        const previousPDFState = this.pdfState || this.defaultPDFState();
        this.pdfLoadToken = (this.pdfLoadToken || 0) + 1;
        window.clearTimeout(this.resizeTimer);
        this.resizeTimer = null;
        window.clearTimeout(this.pdfSelectionTimer);
        this.pdfSelectionTimer = null;
        window.clearTimeout(this.pdfHighlightRenderTimer);
        this.pdfHighlightRenderTimer = null;
        window.clearTimeout(previousPDFState.targetAnnotationTimer);
        if (previousPDFState.pdfViewer && typeof previousPDFState.pdfViewer.setDocument === 'function') {
            try {
                previousPDFState.pdfViewer.setDocument(null);
            } catch (error) {
                // Ignore PDF.js teardown races while navigating away.
            }
        }
        if (previousPDFState.linkService && typeof previousPDFState.linkService.setDocument === 'function') {
            try {
                previousPDFState.linkService.setDocument(null, null);
            } catch (error) {
                // Ignore PDF.js teardown races while navigating away.
            }
        }
        if (previousPDFState.loadingTask && typeof previousPDFState.loadingTask.destroy === 'function') {
            previousPDFState.loadingTask.destroy().catch(() => {});
        }
        const { pdfjsLib, pdfjsViewerLib } = previousPDFState;
        this.pdfState = {
            ...this.defaultPDFState(),
            pdfjsLib,
            pdfjsViewerLib,
            loadToken: this.pdfLoadToken
        };
    },

    currentPDFSelectionText(selection = window.getSelection?.()) {
        const nativeText = String(selection?.toString?.() || '').trim();
        if (nativeText && this.selectionBelongsToPDFViewer(selection)) {
            return nativeText;
        }
        return String(this.pdfState?.selectionText || '').trim();
    },

    selectionBelongsToPDFViewer(selection = window.getSelection?.()) {
        const scroll = this.stage?.querySelector?.('[data-pdf-scroll]');
        if (!scroll || !selection) return false;
        return this.nodeBelongsToElement(selection.anchorNode, scroll)
            && this.nodeBelongsToElement(selection.focusNode, scroll);
    },

    nodeBelongsToElement(node, element) {
        if (!node || !element) return false;
        const textNodeType = typeof Node !== 'undefined' ? Node.TEXT_NODE : 3;
        const candidate = node.nodeType === textNodeType ? node.parentElement : node;
        return !!candidate && typeof element.contains === 'function' && element.contains(candidate);
    },

    pdfSelectionClientRects(selection = window.getSelection?.()) {
        if (!selection?.rangeCount) return null;
        const rects = [];
        for (let index = 0; index < selection.rangeCount; index += 1) {
            const range = selection.getRangeAt(index);
            const clientRects = Array.from(range.getClientRects?.() || []);
            rects.push(...clientRects.filter((rect) => rect.width > 0 && rect.height > 0));
            if (!clientRects.length) {
                const bounds = range.getBoundingClientRect?.();
                if (bounds?.width > 0 && bounds?.height > 0) {
                    rects.push(bounds);
                }
            }
        }
        return rects;
    },

    pdfSelectionClientRect(selection = window.getSelection?.()) {
        const rects = this.pdfSelectionClientRects(selection) || [];
        if (!rects.length) return null;
        const left = Math.min(...rects.map((rect) => rect.left));
        const top = Math.min(...rects.map((rect) => rect.top));
        const right = Math.max(...rects.map((rect) => rect.right));
        const bottom = Math.max(...rects.map((rect) => rect.bottom));
        const result = {
            left,
            top,
            right,
            bottom,
            width: right - left,
            height: bottom - top
        };
        return result;
    },

    schedulePDFSelectionMenuRefresh() {
        if (!this.pdfState?.pdfDocument) return;
        window.clearTimeout(this.pdfSelectionTimer);
        this.pdfSelectionTimer = window.setTimeout(() => {
            this.refreshPDFSelectionMenu();
        }, 0);
    },

    refreshPDFSelectionMenu() {
        if (!this.pdfState?.pdfDocument) return;
        const selection = window.getSelection?.();
        if (!this.selectionBelongsToPDFViewer(selection)) {
            this.hidePDFSelectionMenu();
            this.pdfState.selectionText = '';
            this.pdfState.selectionClientRect = null;
            return;
        }
        const text = String(selection?.toString?.() || '').trim();
        if (!text) {
            this.hidePDFSelectionMenu();
            this.pdfState.selectionText = '';
            this.pdfState.selectionClientRect = null;
            return;
        }
        const rect = this.pdfSelectionClientRect(selection);
        if (!rect) {
            this.hidePDFSelectionMenu();
            this.pdfState.selectionText = '';
            this.pdfState.selectionClientRect = null;
            return;
        }
        this.pdfState.selectionText = text;
        this.pdfState.selectionClientRect = rect;
        this.showPDFSelectionMenu(rect);
    },

    showPDFSelectionMenu(rect) {
        if (!this.selectionMenu) return;
        this.selectionMenu.classList.remove('hidden');
        this.selectionMenu.style.left = '0px';
        this.selectionMenu.style.top = '0px';

        const menuRect = this.selectionMenu.getBoundingClientRect();
        const left = Math.max(12, Math.min(rect.left, window.innerWidth - menuRect.width - 12));
        const topCandidate = rect.top - menuRect.height - 8;
        const top = topCandidate > 12
            ? topCandidate
            : Math.min(window.innerHeight - menuRect.height - 12, rect.bottom + 8);
        this.selectionMenu.style.left = `${left}px`;
        this.selectionMenu.style.top = `${Math.max(12, top)}px`;
    },

    hidePDFSelectionMenu() {
        if (!this.selectionMenu) return;
        this.selectionMenu.classList.add('hidden');
    },

    clearPDFSelection() {
        this.hidePDFSelectionMenu();
        this.pdfState.selectionText = '';
        this.pdfState.selectionClientRect = null;
        window.getSelection?.()?.removeAllRanges?.();
    },

    currentPDFPaperID(resource = this.pdfState?.resource) {
        return String(resource?.paperId || '').trim();
    },

    async loadPDFAnnotations(resource = this.pdfState?.resource, loadToken = this.pdfState?.loadToken) {
        const paperId = this.currentPDFPaperID(resource);
        if (!paperId || typeof API === 'undefined' || typeof API.listPDFAnnotations !== 'function') return;
        try {
            const response = await API.listPDFAnnotations(paperId);
            if (!this.isCurrentPDFLoad(loadToken)) return;
            const annotations = Array.isArray(response?.annotations) ? response.annotations : [];
            this.pdfState.highlights = annotations
                .map((annotation) => this.normalizePDFHighlight(annotation))
                .filter(Boolean);
            this.renderPDFHighlights();
        } catch (error) {
            if (!this.isCurrentPDFLoad(loadToken)) return;
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(t('viewer.pdf_highlight_load_failed', '高亮加载失败'), 'error');
            }
        }
    },

    normalizePDFHighlight(highlight) {
        if (!highlight || typeof highlight !== 'object') return null;
        const fragments = Array.isArray(highlight.fragments)
            ? highlight.fragments.map((fragment) => this.normalizePDFHighlightFragment(fragment)).filter(Boolean)
            : [];
        if (!fragments.length) return null;
        const quoteText = String(highlight.quote_text || highlight.text || '').slice(0, 10000);
        return {
            id: highlight.id,
            paper_id: highlight.paper_id,
            type: String(highlight.type || 'highlight'),
            page_start: Math.floor(Number(highlight.page_start) || fragments[0].page || 1),
            page_end: Math.floor(Number(highlight.page_end) || fragments[fragments.length - 1].page || 1),
            quote_text: quoteText,
            color: String(highlight.color || 'yellow'),
            note_text: String(highlight.note_text || ''),
            created_at: String(highlight.created_at || ''),
            updated_at: String(highlight.updated_at || ''),
            fragments
        };
    },

    normalizePDFHighlightFragment(fragment) {
        if (!fragment || typeof fragment !== 'object') return null;
        const page = Math.floor(Number(fragment.page) || 0);
        const left = this.roundPDFHighlightValue(Number(fragment.left));
        const top = this.roundPDFHighlightValue(Number(fragment.top));
        const width = this.roundPDFHighlightValue(Number(fragment.width));
        const height = this.roundPDFHighlightValue(Number(fragment.height));
        if (page <= 0 || !Number.isFinite(left) || !Number.isFinite(top)
            || !Number.isFinite(width) || !Number.isFinite(height)
            || width <= 0 || height <= 0) {
            return null;
        }
        return {
            page,
            left: this.clampPDFHighlightUnit(left),
            top: this.clampPDFHighlightUnit(top),
            width: this.clampPDFHighlightUnit(width),
            height: this.clampPDFHighlightUnit(height)
        };
    },

    clampPDFHighlightUnit(value) {
        return Math.min(1, Math.max(0, Number(value) || 0));
    },

    roundPDFHighlightValue(value) {
        return Math.round((Number(value) || 0) * 1000000) / 1000000;
    },

    pdfPageElements() {
        const viewerElement = this.stage?.querySelector?.('[data-pdf-viewer]');
        return Array.from(viewerElement?.querySelectorAll?.('.page[data-page-number], .page') || []);
    },

    pdfPageNumber(pageElement, fallbackIndex = 0) {
        const raw = pageElement?.dataset?.pageNumber
            || pageElement?.getAttribute?.('data-page-number')
            || '';
        const pageNumber = Math.floor(Number(raw) || 0);
        return pageNumber > 0 ? pageNumber : fallbackIndex + 1;
    },

    pdfPageElementForClientRect(clientRect, pageElements = this.pdfPageElements()) {
        let best = null;
        let bestArea = 0;
        pageElements.forEach((pageElement, index) => {
            const pageRect = pageElement.getBoundingClientRect?.();
            if (!pageRect?.width || !pageRect?.height) return;
            const left = Math.max(clientRect.left, pageRect.left);
            const top = Math.max(clientRect.top, pageRect.top);
            const right = Math.min(clientRect.right, pageRect.right);
            const bottom = Math.min(clientRect.bottom, pageRect.bottom);
            const area = Math.max(0, right - left) * Math.max(0, bottom - top);
            if (area > bestArea) {
                bestArea = area;
                best = {
                    pageElement,
                    pageNumber: this.pdfPageNumber(pageElement, index)
                };
            }
        });
        return best;
    },

    normalizePDFHighlightClientRect(clientRect, pageElement) {
        const pageRect = pageElement?.getBoundingClientRect?.();
        if (!pageRect?.width || !pageRect?.height) return null;
        const left = Math.max(clientRect.left, pageRect.left);
        const top = Math.max(clientRect.top, pageRect.top);
        const right = Math.min(clientRect.right, pageRect.right);
        const bottom = Math.min(clientRect.bottom, pageRect.bottom);
        const width = right - left;
        const height = bottom - top;
        if (width <= 0 || height <= 0) return null;
        return {
            left: this.roundPDFHighlightValue((left - pageRect.left) / pageRect.width),
            top: this.roundPDFHighlightValue((top - pageRect.top) / pageRect.height),
            width: this.roundPDFHighlightValue(width / pageRect.width),
            height: this.roundPDFHighlightValue(height / pageRect.height)
        };
    },

    buildPDFHighlightFromSelection(selection = window.getSelection?.()) {
        if (!this.selectionBelongsToPDFViewer(selection)) return null;
        const text = String(selection?.toString?.() || '').trim();
        if (!text) return null;
        const clientRects = this.pdfSelectionClientRects(selection) || [];
        const pageElements = this.pdfPageElements();
        const fragments = clientRects.map((clientRect) => {
            const page = this.pdfPageElementForClientRect(clientRect, pageElements);
            if (!page) return null;
            const normalized = this.normalizePDFHighlightClientRect(clientRect, page.pageElement);
            if (!normalized) return null;
            return {
                page: page.pageNumber,
                ...normalized
            };
        }).filter(Boolean);
        if (!fragments.length) return null;
        return {
            type: 'highlight',
            quote_text: text,
            color: 'yellow',
            fragments
        };
    },

    async highlightPDFSelection() {
        const highlight = this.buildPDFHighlightFromSelection(window.getSelection?.());
        this.clearPDFSelection();
        if (!highlight) {
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(t('viewer.pdf_highlight_no_selection', '请先划选需要高亮的 PDF 内容'), 'error');
            }
            return;
        }
        const paperId = this.currentPDFPaperID();
        if (!paperId || typeof API === 'undefined' || typeof API.createPDFAnnotation !== 'function') {
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(t('viewer.pdf_highlight_requires_paper', '当前 PDF 缺少文献 ID，无法保存高亮'), 'error');
            }
            return;
        }
        try {
            const response = await API.createPDFAnnotation(paperId, highlight);
            const annotation = this.normalizePDFHighlight(response?.annotation);
            if (!annotation) {
                throw new Error('invalid annotation response');
            }
            const current = Array.isArray(this.pdfState.highlights) ? this.pdfState.highlights : [];
            this.pdfState.highlights = [...current, annotation];
            this.renderPDFHighlights();
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(t('viewer.pdf_highlight_added', '已高亮所选内容'));
            }
        } catch (error) {
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(t('viewer.pdf_highlight_save_failed', '高亮保存失败'), 'error');
            }
        }
    },

    async deletePDFHighlight(annotationID) {
        const paperId = this.currentPDFPaperID();
        const id = String(annotationID || '').trim();
        if (!paperId || !id || typeof API === 'undefined' || typeof API.deletePDFAnnotation !== 'function') return;
        if (typeof Utils === 'undefined' || typeof Utils.confirm !== 'function') return;
        const confirmed = await Utils.confirm(t('viewer.pdf_highlight_delete_confirm', '删除这条高亮？'));
        if (!confirmed) {
            return;
        }
        try {
            await API.deletePDFAnnotation(paperId, id);
            const current = Array.isArray(this.pdfState.highlights) ? this.pdfState.highlights : [];
            this.pdfState.highlights = current.filter((highlight) => String(highlight.id) !== id);
            this.renderPDFHighlights();
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(t('viewer.pdf_highlight_deleted', '高亮已删除'));
            }
        } catch (error) {
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(t('viewer.pdf_highlight_delete_failed', '高亮删除失败'), 'error');
            }
        }
    },

    schedulePDFHighlightsRender() {
        window.clearTimeout(this.pdfHighlightRenderTimer);
        this.pdfHighlightRenderTimer = window.setTimeout(() => {
            this.pdfHighlightRenderTimer = null;
            this.renderPDFHighlights();
        }, 0);
    },

    renderPDFHighlights() {
        const pageElements = this.pdfPageElements();
        if (!pageElements.length) return;
        pageElements.forEach((pageElement) => {
            Array.from(pageElement.querySelectorAll?.('.viewer-pdf-highlight-layer') || [])
                .forEach((layer) => layer.remove?.());
        });
        const highlights = Array.isArray(this.pdfState?.highlights) ? this.pdfState.highlights : [];
        if (!highlights.length) return;
        const pageIndex = new Map(pageElements.map((pageElement, index) => [
            this.pdfPageNumber(pageElement, index),
            pageElement
        ]));
        const layers = new Map();
        highlights.forEach((highlight) => {
            const fragments = Array.isArray(highlight.fragments) ? highlight.fragments : [];
            fragments.forEach((fragment) => {
                const pageElement = pageIndex.get(Math.floor(Number(fragment.page) || 0));
                if (!pageElement) return;
                let layer = layers.get(pageElement);
                if (!layer) {
                    layer = document.createElement('div');
                    layer.className = 'viewer-pdf-highlight-layer';
                    layer.classList?.add('viewer-pdf-highlight-layer');
                    layer.setAttribute?.('aria-hidden', 'true');
                    pageElement.appendChild(layer);
                    layers.set(pageElement, layer);
                }
                const marker = document.createElement('span');
                marker.className = 'viewer-pdf-highlight-fragment';
                marker.classList?.add('viewer-pdf-highlight-fragment');
                marker.dataset.highlightId = String(highlight.id || '');
                marker.title = highlight.quote_text || '';
                marker.style.left = `${this.clampPDFHighlightUnit(fragment.left) * 100}%`;
                marker.style.top = `${this.clampPDFHighlightUnit(fragment.top) * 100}%`;
                marker.style.width = `${this.clampPDFHighlightUnit(fragment.width) * 100}%`;
                marker.style.height = `${this.clampPDFHighlightUnit(fragment.height) * 100}%`;
                marker.addEventListener?.('click', (event) => {
                    event.preventDefault?.();
                    event.stopPropagation?.();
                    this.deletePDFHighlight(highlight.id);
                });
                layer.appendChild(marker);
            });
        });
        this.applyPDFTargetAnnotation();
    },

    applyPDFTargetAnnotation() {
        const targetID = String(this.pdfState?.targetAnnotationId || '').trim();
        if (!targetID) return false;
        const marker = this.findPDFTargetMarker(targetID);
        if (!marker) return false;

        const state = this.pdfState;
        if (!state.targetAnnotationApplied) {
            state.targetAnnotationApplied = true;
            state.targetAnnotationHighlightActive = true;
            const scrollTarget = typeof marker.scrollIntoView === 'function' ? marker : marker.parentElement;
            scrollTarget?.scrollIntoView?.({ block: 'center', inline: 'nearest', behavior: 'smooth' });
            window.clearTimeout(state.targetAnnotationTimer);
            state.targetAnnotationTimer = window.setTimeout(() => {
                state.targetAnnotationHighlightActive = false;
                if (this.pdfState === state) {
                    this.findPDFTargetMarker(targetID)?.classList?.remove('is-target-highlight');
                    state.targetAnnotationTimer = null;
                }
            }, 2200);
        }
        if (!state.targetAnnotationHighlightActive) return false;
        marker.classList?.add('is-target-highlight');
        return true;
    },

    findPDFTargetMarker(targetID) {
        const id = String(targetID || '').trim();
        if (!id) return null;
        return Array.from(this.stage?.querySelectorAll?.('[data-highlight-id]') || [])
            .find((node) => String(node?.dataset?.highlightId || '') === id) || null;
    },

    async copyPDFSelection() {
        const text = this.currentPDFSelectionText();
        this.clearPDFSelection();
        if (!text) {
            return;
        }
        try {
            await this.writeClipboardText(text);
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(t('viewer.pdf_copied', '已复制所选内容'));
            }
        } catch (error) {
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(t('viewer.pdf_copy_failed', '复制失败，请手动选择文本'), 'error');
            }
        }
    },

    async translatePDFSelection() {
        const text = this.currentPDFSelectionText();
        this.clearPDFSelection();
        if (!text) {
            return;
        }
        if (typeof DesktopTranslate === 'undefined' || typeof DesktopTranslate.translateText !== 'function') {
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(t('shared.translate.no_content_to_translate', '没有可翻译的内容'), 'error');
            }
            return;
        }
        await DesktopTranslate.translateText(text, {
            title: t('viewer.pdf_translate_title', 'PDF 划选翻译')
        });
    },

    async openPDFSelectionAIAsk() {
        const text = this.currentPDFSelectionText();
        const paperId = Number(this.pdfState?.resource?.paperId || 0);
        this.clearPDFSelection();
        if (!text) {
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(t('viewer.pdf_ai_ask_no_selection', '请先划选需要提问的 PDF 内容'), 'error');
            }
            return;
        }
        if (!Number.isFinite(paperId) || paperId <= 0) {
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(t('viewer.pdf_ai_ask_no_paper', '当前 PDF 缺少文献 ID，无法发起 AI 提问'), 'error');
            }
            return;
        }
        this.renderPDFSelectionAIAskDialog(text, paperId);
    },

    renderPDFSelectionAIAskDialog(selectionText, paperId) {
        this.closePDFSelectionAIAskDialog();
        const overlay = document.createElement('div');
        overlay.className = 'dialog-overlay viewer-ai-ask-overlay';
        overlay.setAttribute('tabindex', '-1');
        overlay.innerHTML = `
            <div class="dialog-box viewer-ai-ask-box">
                <button class="modal-close" type="button" data-pdf-ai-ask-action="close" aria-label="${t('viewer.pdf_ai_ask_close', '关闭')}">×</button>
                <div class="viewer-ai-ask-head">
                    <span class="viewer-ai-ask-badge">${t('viewer.pdf_ai_ask_badge', 'PDF AI')}</span>
                    <h3>${t('viewer.pdf_ai_ask_title', 'AI 提问')}</h3>
                    <p>${t('viewer.pdf_ai_ask_intro', '基于你刚才划选的 PDF 内容，做一轮临时解释或追问。')}</p>
                </div>
                <section class="viewer-ai-ask-section">
                    <span class="viewer-ai-ask-label">${t('viewer.pdf_ai_ask_selected_label', '你划选的内容')}</span>
                    <pre class="viewer-ai-ask-selection" data-native-context-menu="true">${this.escapeHTML(selectionText)}</pre>
                </section>
                <label class="viewer-ai-ask-section">
                    <span class="viewer-ai-ask-label">${t('viewer.pdf_ai_ask_question_label', '你的问题')}</span>
                    <textarea class="form-textarea viewer-ai-ask-question" rows="4" data-native-context-menu="true" placeholder="${t('viewer.pdf_ai_ask_placeholder', '例如：这段话是什么意思？为什么这里要这样做？不输入则默认解释这段内容。')}"></textarea>
                </label>
                <section class="viewer-ai-ask-section">
                    <div class="viewer-ai-ask-actions">
                        <span class="viewer-ai-ask-label">${t('viewer.pdf_ai_ask_answer_label', 'AI 回答')}</span>
                        <div class="viewer-ai-ask-actions-main">
                            <button class="btn btn-outline" type="button" data-pdf-ai-ask-action="close">${t('viewer.pdf_ai_ask_close', '关闭')}</button>
                            <button class="btn btn-primary" type="button" data-pdf-ai-ask-action="submit">${t('viewer.pdf_ai_ask_submit', '开始答疑')}</button>
                        </div>
                    </div>
                    <div class="viewer-ai-ask-answer is-muted" data-pdf-ai-ask-answer>${t('viewer.pdf_ai_ask_waiting', '等待提问')}</div>
                </section>
            </div>
        `;

        const dialogState = {
            overlay,
            selectionText,
            paperId,
            abortController: null,
            loading: false,
            answer: ''
        };
        this.pdfAIAskDialog = dialogState;

        overlay.addEventListener('click', async (event) => {
            if (event.target === overlay) {
                this.closePDFSelectionAIAskDialog();
                return;
            }
            const target = event.target instanceof Element ? event.target : null;
            const button = target?.closest('[data-pdf-ai-ask-action]');
            if (!button) return;

            if (button.dataset.pdfAiAskAction === 'close') {
                this.closePDFSelectionAIAskDialog();
                return;
            }
            if (button.dataset.pdfAiAskAction === 'submit') {
                await this.submitPDFSelectionAIAsk(dialogState);
            }
        });
        overlay.addEventListener('keydown', async (event) => {
            if (event.key === 'Escape') {
                event.preventDefault();
                event.stopPropagation();
                event.stopImmediatePropagation?.();
                this.closePDFSelectionAIAskDialog();
                return;
            }
            if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
                event.preventDefault();
                await this.submitPDFSelectionAIAsk(dialogState);
            }
        });

        document.body.appendChild(overlay);
        overlay.focus?.({ preventScroll: true });
        overlay.querySelector('.viewer-ai-ask-question')?.focus?.({ preventScroll: true });
    },

    async submitPDFSelectionAIAsk(dialogState) {
        if (!dialogState || dialogState.loading) return;
        if (typeof API === 'undefined' || typeof API.readPaperWithAIStream !== 'function') {
            this.renderPDFSelectionAIAnswer(dialogState, t('viewer.pdf_ai_ask_api_missing', '当前环境无法调用 AI 阅读接口'), { error: true });
            return;
        }

        const questionInput = dialogState.overlay.querySelector('.viewer-ai-ask-question');
        const submitButton = dialogState.overlay.querySelector('[data-pdf-ai-ask-action="submit"]');
        const question = String(questionInput?.value || '').trim();
        const prompt = this.buildPDFSelectionAIQuestion(dialogState.selectionText, question);
        dialogState.abortController = new AbortController();
        dialogState.loading = true;
        dialogState.answer = '';
        if (submitButton) {
            submitButton.disabled = true;
            submitButton.textContent = t('viewer.pdf_ai_ask_submitting', '答疑中...');
        }
        this.renderPDFSelectionAIAnswer(dialogState, t('viewer.pdf_ai_ask_loading', '正在调用 AI，请稍候。'), { muted: true });

        try {
            await API.readPaperWithAIStream({
                paper_id: dialogState.paperId,
                action: 'paper_qa',
                include_figures: false,
                question: prompt
            }, {
                signal: dialogState.abortController.signal,
                onEvent: (event) => {
                    if (this.pdfAIAskDialog !== dialogState) return;
                    if (event.type === 'error') {
                        throw new Error(event.error || t('shared.api.stream_error', '流式解读失败'));
                    }
                    if (event.type === 'delta') {
                        dialogState.answer += event.delta || '';
                        this.renderPDFSelectionAIAnswer(dialogState, dialogState.answer);
                        return;
                    }
                    if (event.type === 'final' && event.result && !dialogState.answer.trim()) {
                        dialogState.answer = event.result.answer || '';
                        this.renderPDFSelectionAIAnswer(dialogState, dialogState.answer || t('viewer.pdf_ai_ask_empty_answer', '模型没有返回文本结果。'));
                    }
                }
            });
            if (!dialogState.answer.trim()) {
                this.renderPDFSelectionAIAnswer(dialogState, t('viewer.pdf_ai_ask_empty_answer', '模型没有返回文本结果。'));
            }
        } catch (error) {
            if (error?.name !== 'AbortError' && this.pdfAIAskDialog === dialogState) {
                this.renderPDFSelectionAIAnswer(dialogState, error.message || t('viewer.pdf_ai_ask_failed', 'AI 答疑失败'), { error: true });
            }
        } finally {
            if (this.pdfAIAskDialog === dialogState) {
                dialogState.loading = false;
                dialogState.abortController = null;
                if (submitButton) {
                    submitButton.disabled = false;
                    submitButton.textContent = t('viewer.pdf_ai_ask_submit', '开始答疑');
                }
            }
        }
    },

    buildPDFSelectionAIQuestion(selectionText, userQuestion = '') {
        const selected = String(selectionText || '').trim();
        const question = String(userQuestion || '').trim() || '请解释这段内容的核心意思、背景和它在论文论证中的作用。';
        return [
            '请只围绕下面这段 PDF 划选内容回答；不要泛泛总结整篇论文，除非需要用论文上下文解释这段话。',
            '',
            '【PDF 划选内容】',
            selected,
            '',
            '【我的问题】',
            question
        ].join('\n');
    },

    renderPDFSelectionAIAnswer(dialogState, text, options = {}) {
        const answer = dialogState?.overlay?.querySelector?.('[data-pdf-ai-ask-answer]');
        if (!answer) return;
        answer.classList.toggle('is-muted', Boolean(options.muted));
        answer.classList.toggle('is-error', Boolean(options.error));
        const content = String(text || '').trim();
        if (!content) {
            answer.textContent = t('viewer.pdf_ai_ask_waiting', '等待提问');
            answer.classList.add('is-muted');
            return;
        }
        if (options.muted || options.error || typeof Utils === 'undefined' || typeof Utils.renderMarkdown !== 'function') {
            answer.textContent = content;
            return;
        }
        answer.innerHTML = Utils.renderMarkdown(content);
    },

    closePDFSelectionAIAskDialog() {
        const dialogState = this.pdfAIAskDialog;
        if (!dialogState) return;
        if (dialogState.abortController) {
            dialogState.abortController.abort();
        }
        dialogState.overlay?.remove?.();
        this.pdfAIAskDialog = null;
    },

    async writeClipboardText(text) {
        if (navigator.clipboard?.writeText) {
            await navigator.clipboard.writeText(text);
            return;
        }
        const textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.setAttribute('readonly', 'readonly');
        textarea.style.position = 'fixed';
        textarea.style.left = '-9999px';
        textarea.style.top = '0';
        document.body.appendChild(textarea);
        textarea.select();
        const copied = document.execCommand('copy');
        textarea.remove();
        if (!copied) {
            throw new Error('copy failed');
        }
    },

    hasImageTransform() {
        const state = this.viewState || this.defaultViewState();
        return Math.abs(state.scale - 1) > 0.001 || Math.abs(state.x) > 0.5 || Math.abs(state.y) > 0.5;
    },

    clampViewState(state = this.viewState || this.defaultViewState()) {
        const viewport = this.stage?.querySelector('[data-viewer-viewport]');
        const image = this.stage?.querySelector('[data-viewer-image]');
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

    applyImageTransform() {
        const viewport = this.stage?.querySelector('[data-viewer-viewport]');
        const image = this.stage?.querySelector('[data-viewer-image]');
        if (!viewport || !image) return;

        this.viewState = this.clampViewState(this.viewState);
        const state = this.viewState;
        image.style.transform = `translate(${state.x}px, ${state.y}px) scale(${state.scale})`;
        image.draggable = false;
        viewport.classList.toggle('is-zoomed', state.scale > 1);
        viewport.classList.toggle('is-dragging', Boolean(this.dragState));
        if (this.resetButton) {
            this.resetButton.disabled = !this.hasImageTransform();
        }
    },

    handleImageWheel(event, viewport) {
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
        this.applyImageTransform();
    },

    beginImageDrag(event, viewport) {
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
                // Ignore capture failures for non-primary pointers.
            }
        }
        this.applyImageTransform();
    },

    updateImageDrag(event) {
        if (!this.dragState || event.pointerId !== this.dragState.pointerID) return;
        this.viewState = {
            ...this.viewState,
            x: this.dragState.originX + (event.clientX - this.dragState.startX),
            y: this.dragState.originY + (event.clientY - this.dragState.startY)
        };
        this.applyImageTransform();
    },

    endImageDrag(event = null) {
        if (!this.dragState) return;
        if (event && event.pointerId !== this.dragState.pointerID) return;

        const viewport = this.stage?.querySelector('[data-viewer-viewport]');
        if (viewport && typeof viewport.hasPointerCapture === 'function' && viewport.hasPointerCapture(this.dragState.pointerID)) {
            try {
                viewport.releasePointerCapture(this.dragState.pointerID);
            } catch (error) {
                // Ignore release failures after cancellation.
            }
        }
        this.dragState = null;
        this.applyImageTransform();
    },

    resolveResource() {
        const params = new URLSearchParams(window.location.search);
        const kind = String(params.get('kind') || '').trim().toLowerCase();
        const src = String(params.get('src') || '').trim();

        if (!src) {
            throw new Error(t('viewer.err_missing_src', '缺少资源地址。'));
        }

        const url = new URL(src, window.location.origin);
        if (url.origin !== window.location.origin) {
            throw new Error(t('viewer.err_cross_origin', '只支持打开 CiteBox 当前实例中的资源。'));
        }

        if (kind === 'image') {
            if (!url.pathname.startsWith('/files/figures/')) {
                throw new Error(t('viewer.err_image_only', '当前仅支持打开图片库中的原图资源。'));
            }
            return {
                kind,
                href: url.href,
                label: t('viewer.label_image', '原图查看'),
                name: this.filenameFromURL(url) || t('viewer.label_image_default', '图片')
            };
        }

        if (kind === 'pdf') {
            if (!url.pathname.startsWith('/files/papers/')) {
                throw new Error(t('viewer.err_pdf_only', '当前仅支持打开文献 PDF 资源。'));
            }
            return {
                kind,
                href: url.href,
                label: t('viewer.label_pdf', 'PDF 查看'),
                name: this.filenameFromURL(url) || t('viewer.label_pdf_default', 'PDF 文档'),
                paperId: String(params.get('paper_id') || '').trim(),
                annotationId: String(params.get('annotation_id') || '').trim()
            };
        }

        throw new Error(t('viewer.err_unsupported_kind', '不支持的资源类型。'));
    },

    filenameFromURL(url) {
        const pathname = String(url?.pathname || '');
        const segments = pathname.split('/').filter(Boolean);
        if (!segments.length) return '';
        try {
            return decodeURIComponent(segments[segments.length - 1]);
        } catch (error) {
            return segments[segments.length - 1];
        }
    },

    close(options = {}) {
        if (this.closing) {
            return;
        }

        this.closing = true;
        this.endImageDrag();
        this.destroyPDFState();
        const { deferNavigation = false } = options;
        const finalizeClose = () => {
            this.navigateBack();
        };

        if (deferNavigation) {
            // Let the Escape event finish first so it does not close restored modals on the previous page.
            window.setTimeout(finalizeClose, 0);
            return;
        }

        finalizeClose();
    },

    navigateBack() {
        const params = new URLSearchParams(window.location.search);
        const back = String(params.get('back') || '').trim();
        if (back) {
            try {
                const backURL = new URL(back, window.location.origin);
                if (backURL.origin === window.location.origin) {
                    if (this.shouldUseHistoryBack(backURL)) {
                        window.history.back();
                        return;
                    }
                    window.location.replace(backURL.href);
                    return;
                }
            } catch (error) {
                // Ignore invalid back URLs and continue with other fallbacks.
            }
        }

        if (window.history.length > 1) {
            window.history.back();
            return;
        }

        const referrer = String(document.referrer || '').trim();
        if (referrer) {
            try {
                const referrerURL = new URL(referrer);
                if (referrerURL.origin === window.location.origin) {
                    window.location.replace(referrerURL.href);
                    return;
                }
            } catch (error) {
                // Ignore invalid referrer URLs and fall back to the dashboard.
            }
        }

        window.location.replace('/');
    },

    shouldUseHistoryBack(backURL) {
        if (window.history.length <= 1) {
            return false;
        }

        const referrer = String(document.referrer || '').trim();
        if (!referrer) {
            return false;
        }

        try {
            const referrerURL = new URL(referrer, window.location.origin);
            if (referrerURL.origin !== window.location.origin) {
                return false;
            }
            return this.normalizeBackComparisonURL(referrerURL) === this.normalizeBackComparisonURL(backURL);
        } catch (error) {
            return false;
        }
    },

    normalizeBackComparisonURL(url) {
        const normalized = new URL(url, window.location.origin);
        normalized.hash = '';
        normalized.searchParams.delete('restore_modal');
        return `${normalized.pathname}${normalized.search}`;
    },

    waitForI18nReady() {
        if (typeof CiteBoxI18n === 'undefined') {
            return Promise.resolve();
        }
        return new Promise((resolve) => {
            const check = () => {
                if (CiteBoxI18n._ready) {
                    window.requestAnimationFrame(resolve);
                    return;
                }
                window.setTimeout(check, 20);
            };
            check();
        });
    },

    escapeHTML(value = '') {
        return String(value)
            .replaceAll('&', '&amp;')
            .replaceAll('<', '&lt;')
            .replaceAll('>', '&gt;')
            .replaceAll('"', '&quot;')
            .replaceAll("'", '&#39;');
    }
};

document.addEventListener('DOMContentLoaded', () => {
    ResourceViewerPage.waitForI18nReady().then(() => {
        ResourceViewerPage.init();
    });
});
