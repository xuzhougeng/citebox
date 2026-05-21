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
        this.pdfState = this.defaultPDFState();
        this.dragState = null;
        this.resizeTimer = null;

        this.closeButton?.addEventListener('click', () => this.close());
        this.resetButton?.addEventListener('click', () => {
            this.resetImageView();
            this.applyImageTransform();
        });
        document.addEventListener('keydown', (event) => {
            if (event.key !== 'Escape') return;
            event.preventDefault();
            event.stopPropagation();
            event.stopImmediatePropagation();
            this.close({ deferNavigation: true });
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
            await this.renderPDFPage();
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

        document.addEventListener('selectionchange', () => {
            window.setTimeout(() => this.syncPDFSelectionMenu(), 0);
        });
        document.addEventListener('pointerup', () => {
            window.setTimeout(() => this.syncPDFSelectionMenu(), 0);
        });
        document.addEventListener('keyup', () => {
            window.setTimeout(() => this.syncPDFSelectionMenu(), 0);
        });
        document.addEventListener('scroll', () => {
            this.hidePDFSelectionMenu();
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
            loadingTask: null,
            pdfDocument: null,
            resource: null,
            pageNumber: 1,
            pageCount: 0,
            scale: 1,
            fitMode: true,
            renderToken: 0,
            renderTask: null,
            textLayer: null
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

    async ensurePDFJSReady() {
        if (this.pdfState.pdfjsLib) {
            return this.pdfState.pdfjsLib;
        }
        const pdfjsLib = await import('/static/vendor/pdfjs/legacy/build/pdf.min.mjs');
        pdfjsLib.GlobalWorkerOptions.workerSrc = '/static/vendor/pdfjs/legacy/build/pdf.worker.min.mjs';
        this.pdfState.pdfjsLib = pdfjsLib;
        return pdfjsLib;
    },

    async renderPDFResource(resource) {
        this.togglePDFToolbar(true);
        this.pdfState = {
            ...this.defaultPDFState(),
            pdfjsLib: this.pdfState.pdfjsLib,
            resource,
            pageNumber: this.initialPDFPageNumber(),
            fitMode: true
        };
        this.stage.className = 'viewer-stage pdf-mode';
        this.stage.innerHTML = `
            <div class="viewer-pdf-scroll" data-pdf-scroll>
                <div class="viewer-pdf-page" data-pdf-page>
                    <canvas class="viewer-pdf-canvas" data-pdf-canvas></canvas>
                    <div class="viewer-pdf-text-layer" data-pdf-text-layer></div>
                    <div class="viewer-pdf-loading" data-pdf-loading>${t('viewer.pdf_loading', '正在加载 PDF...')}</div>
                </div>
            </div>
        `;

        if (this.pdfDetailLink && resource.paperId > 0) {
            this.pdfDetailLink.href = `/library?paper_id=${encodeURIComponent(resource.paperId)}`;
            this.pdfDetailLink.hidden = false;
        }

        await this.loadPDFDocument(resource.href);
        this.pdfState.pageNumber = Math.min(Math.max(this.pdfState.pageNumber, 1), Math.max(this.pdfState.pageCount, 1));
        await this.renderPDFPage();
    },

    initialPDFPageNumber() {
        const page = Number(new URLSearchParams(window.location.search).get('page') || 1);
        return page > 0 ? Math.floor(page) : 1;
    },

    async loadPDFDocument(href) {
        const pdfjsLib = await this.ensurePDFJSReady();
        const loadingTask = pdfjsLib.getDocument({
            url: href,
            cMapUrl: '/static/vendor/pdfjs/cmaps/',
            cMapPacked: true,
            standardFontDataUrl: '/static/vendor/pdfjs/standard_fonts/',
            wasmUrl: '/static/vendor/pdfjs/wasm/'
        });
        this.pdfState.loadingTask = loadingTask;
        const pdfDocument = await loadingTask.promise;
        this.pdfState.pdfDocument = pdfDocument;
        this.pdfState.pageCount = pdfDocument.numPages || 0;
        this.syncPDFToolbar();
    },

    async renderPDFPage() {
        const pdfDocument = this.pdfState.pdfDocument;
        if (!pdfDocument || !this.stage) return;

        const token = ++this.pdfState.renderToken;
        this.cancelPDFRenderTask();
        this.hidePDFSelectionMenu();
        const pageNumber = Math.min(Math.max(Number(this.pdfState.pageNumber) || 1, 1), Math.max(this.pdfState.pageCount, 1));
        this.pdfState.pageNumber = pageNumber;
        this.syncPDFToolbar();

        const pageElement = this.stage.querySelector('[data-pdf-page]');
        const canvas = this.stage.querySelector('[data-pdf-canvas]');
        const textLayerElement = this.stage.querySelector('[data-pdf-text-layer]');
        const loadingElement = this.stage.querySelector('[data-pdf-loading]');
        if (!pageElement || !canvas || !textLayerElement) return;

        if (loadingElement) {
            loadingElement.hidden = false;
            loadingElement.textContent = t('viewer.pdf_rendering_page', '正在渲染第 {page} 页...').replace('{page}', pageNumber);
        }
        textLayerElement.innerHTML = '';

        try {
            const page = await pdfDocument.getPage(pageNumber);
            if (token !== this.pdfState.renderToken) return;

            if (this.pdfState.fitMode) {
                this.pdfState.scale = this.calculatePDFPageFitScale(page);
            }
            const scale = this.clampPDFScale(this.pdfState.scale);
            this.pdfState.scale = scale;
            const viewport = page.getViewport({ scale });
            const outputScale = Math.max(1, window.devicePixelRatio || 1);
            const context = canvas.getContext('2d', { alpha: false });

            canvas.width = Math.ceil(viewport.width * outputScale);
            canvas.height = Math.ceil(viewport.height * outputScale);
            canvas.style.width = `${viewport.width}px`;
            canvas.style.height = `${viewport.height}px`;
            pageElement.style.width = `${viewport.width}px`;
            pageElement.style.height = `${viewport.height}px`;
            textLayerElement.style.setProperty('--total-scale-factor', scale);
            textLayerElement.style.width = `${viewport.width}px`;
            textLayerElement.style.height = `${viewport.height}px`;

            const renderTask = page.render({
                canvasContext: context,
                viewport,
                transform: outputScale !== 1 ? [outputScale, 0, 0, outputScale, 0, 0] : null
            });
            this.pdfState.renderTask = renderTask;
            await renderTask.promise;
            if (token !== this.pdfState.renderToken) return;

            const textContent = await page.getTextContent();
            if (token !== this.pdfState.renderToken) return;
            const textLayer = new this.pdfState.pdfjsLib.TextLayer({
                textContentSource: textContent,
                container: textLayerElement,
                viewport
            });
            this.pdfState.textLayer = textLayer;
            await textLayer.render();
            if (loadingElement) {
                loadingElement.hidden = true;
            }
            this.syncPDFToolbar();
        } catch (error) {
            if (this.isPDFRenderCancelledError(error) || token !== this.pdfState.renderToken) {
                return;
            }
            if (loadingElement) {
                loadingElement.hidden = false;
                loadingElement.textContent = t('viewer.pdf_render_failed', 'PDF 页面渲染失败');
            }
            throw error;
        } finally {
            if (this.pdfState.renderTask) {
                this.pdfState.renderTask = null;
            }
        }
    },

    calculatePDFPageFitScale(page) {
        const scroll = this.stage?.querySelector('[data-pdf-scroll]');
        const viewport = page.getViewport({ scale: 1 });
        const availableWidth = Math.max(320, (scroll?.clientWidth || window.innerWidth || 960) - 48);
        return this.clampPDFScale(availableWidth / Math.max(1, viewport.width));
    },

    clampPDFScale(scale) {
        return Math.min(4, Math.max(0.35, Number(scale) || 1));
    },

    async goToPDFPage(pageNumber) {
        if (!this.pdfState.pdfDocument) return;
        const nextPage = Math.min(Math.max(Math.floor(Number(pageNumber) || 1), 1), Math.max(this.pdfState.pageCount, 1));
        if (nextPage === this.pdfState.pageNumber) {
            this.syncPDFToolbar();
            return;
        }
        this.pdfState.pageNumber = nextPage;
        await this.renderPDFPage();
    },

    async setPDFScale(scale) {
        if (!this.pdfState.pdfDocument) return;
        this.pdfState.fitMode = false;
        this.pdfState.scale = this.clampPDFScale(scale);
        await this.renderPDFPage();
    },

    syncPDFToolbar() {
        if (!this.pdfControls || this.pdfControls.hidden) return;
        const pageCount = this.pdfState.pageCount || 0;
        const pageNumber = Math.min(Math.max(this.pdfState.pageNumber || 1, 1), Math.max(pageCount, 1));
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
        if (!this.pdfState?.pdfDocument || !this.pdfState.fitMode) return;
        window.clearTimeout(this.resizeTimer);
        this.resizeTimer = window.setTimeout(() => {
            this.renderPDFPage();
        }, 120);
    },

    cancelPDFRenderTask() {
        if (this.pdfState.textLayer && typeof this.pdfState.textLayer.cancel === 'function') {
            try {
                this.pdfState.textLayer.cancel();
            } catch (error) {
                // Ignore cancellation races.
            }
            this.pdfState.textLayer = null;
        }
        if (this.pdfState.renderTask && typeof this.pdfState.renderTask.cancel === 'function') {
            try {
                this.pdfState.renderTask.cancel();
            } catch (error) {
                // Ignore cancellation races.
            }
        }
    },

    destroyPDFState() {
        this.cancelPDFRenderTask();
        if (this.pdfState.loadingTask && typeof this.pdfState.loadingTask.destroy === 'function') {
            this.pdfState.loadingTask.destroy().catch(() => {});
        }
        const pdfjsLib = this.pdfState.pdfjsLib;
        this.pdfState = {
            ...this.defaultPDFState(),
            pdfjsLib
        };
    },

    isPDFRenderCancelledError(error) {
        const name = String(error?.name || '');
        const message = String(error?.message || '');
        return name === 'RenderingCancelledException'
            || name === 'AbortException'
            || message.toLowerCase().includes('cancelled');
    },

    currentPDFSelectionText() {
        const selection = window.getSelection?.();
        if (!selection || selection.rangeCount < 1) return '';
        if (!this.selectionBelongsToPDF(selection)) return '';
        return String(selection.toString() || '').trim();
    },

    selectionBelongsToPDF(selection) {
        const textLayer = this.stage?.querySelector('[data-pdf-text-layer]');
        if (!textLayer) return false;
        return this.nodeBelongsToElement(selection.anchorNode, textLayer)
            || this.nodeBelongsToElement(selection.focusNode, textLayer);
    },

    nodeBelongsToElement(node, element) {
        if (!node || !element) return false;
        const candidate = node instanceof Element ? node : node.parentElement;
        return Boolean(candidate && element.contains(candidate));
    },

    syncPDFSelectionMenu() {
        const text = this.currentPDFSelectionText();
        if (!text) {
            this.hidePDFSelectionMenu();
            return;
        }

        const rect = this.currentPDFSelectionRect();
        if (!rect) {
            this.hidePDFSelectionMenu();
            return;
        }
        this.showPDFSelectionMenu(rect);
    },

    currentPDFSelectionRect() {
        const selection = window.getSelection?.();
        if (!selection || selection.rangeCount < 1) return null;
        const range = selection.getRangeAt(0);
        const rect = range.getBoundingClientRect();
        if (rect.width || rect.height) return rect;
        const firstRect = Array.from(range.getClientRects())[0];
        return firstRect || null;
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

    async copyPDFSelection() {
        const text = this.currentPDFSelectionText();
        this.hidePDFSelectionMenu();
        window.getSelection?.().removeAllRanges();
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
        this.hidePDFSelectionMenu();
        window.getSelection?.().removeAllRanges();
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
                paperId: Number(params.get('paper_id') || 0)
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
