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
        this.closing = false;
        this.viewState = this.defaultViewState();
        this.pdfLoadToken = 0;
        this.pdfState = this.defaultPDFState();
        this.dragState = null;
        this.resizeTimer = null;
        this.pdfSelectionTimer = null;

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
        this.close({ deferNavigation: true });
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
            selectionClientRect: null
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
            fitMode: !this.isNarrowPDFViewport()
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
            this.pdfDetailLink.href = `/library?paper_id=${encodeURIComponent(paperId)}`;
            this.pdfDetailLink.hidden = false;
        }

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
        });
        eventBus.on('pagesloaded', () => {
            if (!this.isCurrentPDFLoad(loadToken, pdfViewer)) return;
            this.pdfState.pageCount = pdfViewer.pagesCount || this.pdfState.pageCount;
            this.syncPDFToolbar();
        });
        eventBus.on('pagechanging', (event) => {
            if (!this.isCurrentPDFLoad(loadToken, pdfViewer)) return;
            this.pdfState.pageNumber = event.pageNumber || pdfViewer.currentPageNumber || 1;
            this.clearPDFSelection();
            this.syncPDFToolbar();
        });
        eventBus.on('scalechanging', () => {
            if (!this.isCurrentPDFLoad(loadToken, pdfViewer)) return;
            this.pdfState.scale = pdfViewer.currentScale || this.pdfState.scale || 1;
            this.clearPDFSelection();
            this.syncPDFToolbar();
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

    pdfSelectionClientRect(selection = window.getSelection?.()) {
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
                paperId: String(params.get('paper_id') || '').trim()
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
