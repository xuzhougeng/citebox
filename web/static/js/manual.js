const ManualPage = {
    previewScale: 1.25,
    extractScale: 2.8,

    state: {
        paperId: null,
        paper: null,
        pageCount: 0,
        currentPage: 1,
        selections: [],
        nextSelectionId: 1,
        drawing: null,
        loadingPreview: false,
        submitting: false,
        pdfjsLib: null,
        pdfDocument: null,
        renderCache: new Map(),
        previewRenderToken: 0
    },

    async init() {
        this.cacheElements();
        if (!this.previewFrame) return;

        const paperId = Number(new URLSearchParams(window.location.search).get('paper_id'));
        if (!paperId) {
            Utils.showToast('缺少 paper_id 参数', 'error');
            this.renderMissingPaper();
            return;
        }

        this.state.paperId = paperId;
        this.bindEvents();
        await this.loadWorkspace();
    },

    cacheElements() {
        this.pageTitle = document.getElementById('manualPageTitle');
        this.pageSubtitle = document.getElementById('manualPageSubtitle');
        this.summaryStrip = document.getElementById('manualSummaryStrip');
        this.openPDFLink = document.getElementById('manualOpenPDFLink');
        this.backLibraryLink = document.getElementById('manualBackLibraryLink');
        this.previewFrame = document.getElementById('manualPreviewFrame');
        this.previewCanvas = document.getElementById('manualPreviewCanvas');
        this.overlay = document.getElementById('manualOverlay');
        this.draftBox = document.getElementById('manualDraftBox');
        this.pageIndicator = document.getElementById('manualPageIndicator');
        this.prevPageBtn = document.getElementById('manualPrevPageBtn');
        this.nextPageBtn = document.getElementById('manualNextPageBtn');
        this.clearPageBtn = document.getElementById('manualClearPageBtn');
        this.clearAllBtn = document.getElementById('manualClearAllBtn');
        this.submitBtn = document.getElementById('manualSubmitBtn');
        this.selectionList = document.getElementById('manualSelectionList');
        this.workspaceHint = document.getElementById('manualWorkspaceHint');
        this.notice = document.getElementById('manualExtractionNotice');
    },

    bindEvents() {
        this.prevPageBtn.addEventListener('click', async () => {
            if (this.state.currentPage <= 1) return;
            await this.loadPage(this.state.currentPage - 1);
        });

        this.nextPageBtn.addEventListener('click', async () => {
            if (this.state.currentPage >= this.state.pageCount) return;
            await this.loadPage(this.state.currentPage + 1);
        });

        this.clearPageBtn.addEventListener('click', async () => {
            const pageSelections = this.currentPageSelections();
            if (!pageSelections.length) return;
            const confirmed = await Utils.confirm(`将删除第 ${this.state.currentPage} 页上的 ${pageSelections.length} 个框选区域。`);
            if (!confirmed) return;
            this.state.selections = this.state.selections.filter((item) => item.page_number !== this.state.currentPage);
            this.renderSelections();
        });

        this.clearAllBtn.addEventListener('click', async () => {
            if (!this.state.selections.length) return;
            const confirmed = await Utils.confirm('将清空当前文献的全部待提交框选区域。');
            if (!confirmed) return;
            this.state.selections = [];
            this.renderSelections();
        });

        this.submitBtn.addEventListener('click', async () => {
            await this.submitSelections();
        });

        this.selectionList.addEventListener('input', (event) => {
            const input = event.target.closest('[data-selection-field]');
            if (!input) return;
            const selection = this.findSelection(Number(input.dataset.selectionId));
            if (!selection) return;
            if (input.dataset.selectionField === 'caption') {
                selection.caption = input.value;
            }
        });

        this.selectionList.addEventListener('change', (event) => {
            const input = event.target.closest('[data-selection-field="replace_figure_id"]');
            if (!input) return;
            const selection = this.findSelection(Number(input.dataset.selectionId));
            if (!selection) return;
            selection.replace_figure_id = input.value ? Number(input.value) : null;
            this.renderSelections();
        });

        this.selectionList.addEventListener('click', async (event) => {
            const action = event.target.closest('[data-selection-action]');
            if (!action) return;

            const selectionId = Number(action.dataset.selectionId);
            const selection = this.findSelection(selectionId);
            if (!selection) return;

            if (action.dataset.selectionAction === 'remove') {
                this.state.selections = this.state.selections.filter((item) => item.id !== selectionId);
                this.renderSelections();
                return;
            }

            if (action.dataset.selectionAction === 'locate') {
                await this.loadPage(selection.page_number);
                this.scrollPreviewIntoView();
            }
        });

        this.overlay.addEventListener('pointerdown', (event) => this.startDrawing(event));
        this.overlay.addEventListener('pointermove', (event) => this.updateDrawing(event));
        this.overlay.addEventListener('pointerup', (event) => this.finishDrawing(event));
        this.overlay.addEventListener('pointerleave', (event) => this.finishDrawing(event));
    },

    async loadWorkspace() {
        try {
            const payload = await API.getManualExtractionWorkspace(this.state.paperId);
            const workspace = payload.workspace;
            this.state.paper = workspace.paper;
            this.renderWorkspace();

            await this.loadPDFDocument();
            this.renderWorkspace();

            const initialPage = Number(new URLSearchParams(window.location.search).get('page')) || 1;
            await this.loadPage(Math.min(Math.max(initialPage, 1), Math.max(this.state.pageCount, 1)));
        } catch (error) {
            Utils.showToast(error.message, 'error');
            this.renderMissingPaper(error.message);
        }
    },

    async ensurePDFJSReady() {
        if (this.state.pdfjsLib) {
            return this.state.pdfjsLib;
        }

        const pdfjsLib = await import('/static/vendor/pdfjs/legacy/build/pdf.min.mjs');
        pdfjsLib.GlobalWorkerOptions.workerSrc = '/static/vendor/pdfjs/legacy/build/pdf.worker.min.mjs';
        this.state.pdfjsLib = pdfjsLib;
        return pdfjsLib;
    },

    async loadPDFDocument() {
        if (!this.state.paper?.pdf_url) {
            throw new Error('当前文献缺少 PDF 文件地址');
        }

        this.previewFrame.classList.add('is-loading');
        this.workspaceHint.textContent = '正在加载 PDF 文档...';

        const pdfjsLib = await this.ensurePDFJSReady();
        const loadingTask = pdfjsLib.getDocument({
            url: this.state.paper.pdf_url,
            cMapUrl: '/static/vendor/pdfjs/cmaps/',
            cMapPacked: true,
            standardFontDataUrl: '/static/vendor/pdfjs/standard_fonts/',
            wasmUrl: '/static/vendor/pdfjs/wasm/'
        });

        const pdfDocument = await loadingTask.promise;
        this.state.pdfDocument = pdfDocument;
        this.state.pageCount = pdfDocument.numPages || 0;
        this.state.renderCache = new Map();
    },

    renderWorkspace() {
        const paper = this.state.paper;
        if (!paper) return;

        const manualCount = (paper.figures || []).filter((figure) => figure.source === 'manual').length;

        this.pageTitle.textContent = `${paper.title} 的人工框选提取`;
        this.pageSubtitle.textContent = paper.extractor_message
            ? `自动处理状态：${Utils.statusLabel(paper.extraction_status)}。${paper.extractor_message}`
            : 'PDF 预览和裁图现在直接在浏览器里完成，提交后会把结果追加到当前文献。';

        this.openPDFLink.href = paper.pdf_url || '/';
        this.backLibraryLink.href = '/';
        this.pageIndicator.textContent = this.state.pageCount
            ? `第 ${this.state.currentPage} / ${this.state.pageCount} 页`
            : '正在读取 PDF...';
        this.prevPageBtn.disabled = this.state.currentPage <= 1;
        this.nextPageBtn.disabled = !this.state.pageCount || this.state.currentPage >= this.state.pageCount;

        this.summaryStrip.innerHTML = `
            <div class="stat-card"><span>PDF 页数</span><strong>${this.state.pageCount || '...'}</strong></div>
            <div class="stat-card"><span>已有图片</span><strong>${paper.figure_count || (paper.figures || []).length || 0}</strong></div>
            <div class="stat-card"><span>人工补录</span><strong>${manualCount}</strong></div>
            <div class="stat-card"><span>自动状态</span><strong>${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status || ''))}</strong></div>
        `;

        if (paper.extractor_message) {
            this.notice.classList.remove('hidden', 'error', 'info', 'success');
            this.notice.classList.add(Utils.statusTone(paper.extraction_status || ''));
            this.notice.textContent = paper.extractor_message;
        } else {
            this.notice.classList.add('hidden');
            this.notice.textContent = '';
        }
    },

    renderMissingPaper(message = '无法加载当前文献，请返回文献库重新进入。') {
        if (this.pageTitle) this.pageTitle.textContent = '人工框选提取';
        if (this.pageSubtitle) this.pageSubtitle.textContent = message;
        if (this.summaryStrip) {
            this.summaryStrip.innerHTML = `
                <div class="empty-state">
                    <h3>无法进入人工处理工作台</h3>
                    <p>${Utils.escapeHTML(message)}</p>
                    <a class="btn btn-primary" href="/library">返回文献库</a>
                </div>
            `;
        }
    },

    async loadPage(page) {
        if (!this.state.pdfDocument || !this.state.pageCount) return;

        this.state.currentPage = page;
        this.state.loadingPreview = true;
        this.previewFrame.classList.add('is-loading');
        this.workspaceHint.textContent = `正在渲染第 ${page} 页...`;
        this.pageIndicator.textContent = `第 ${page} / ${this.state.pageCount} 页`;
        this.prevPageBtn.disabled = page <= 1;
        this.nextPageBtn.disabled = page >= this.state.pageCount;
        this.renderSelections();

        const renderToken = ++this.state.previewRenderToken;
        try {
            await this.renderPDFPageToCanvas(page, this.previewScale, this.previewCanvas, window.devicePixelRatio || 1);
            if (renderToken !== this.state.previewRenderToken) {
                return;
            }
            this.state.loadingPreview = false;
            this.previewFrame.classList.remove('is-loading');
            this.workspaceHint.textContent = `当前是第 ${page} 页。拖拽可新增框选，右侧可补充 caption 并提交。`;
            this.renderSelections();
        } catch (error) {
            if (renderToken !== this.state.previewRenderToken) {
                return;
            }
            this.state.loadingPreview = false;
            this.previewFrame.classList.remove('is-loading');
            this.workspaceHint.textContent = 'PDF 页面渲染失败，请稍后重试。';
            Utils.showToast(error.message || 'PDF 页面渲染失败', 'error');
        }
    },

    async renderPDFPageToCanvas(pageNumber, scale, canvas, outputScale = 1) {
        const page = await this.state.pdfDocument.getPage(pageNumber);
        const viewport = page.getViewport({ scale });
        const context = canvas.getContext('2d', { alpha: false });

        const width = Math.max(1, Math.ceil(viewport.width));
        const height = Math.max(1, Math.ceil(viewport.height));
        const pixelRatio = Math.max(1, outputScale);

        canvas.width = Math.ceil(width * pixelRatio);
        canvas.height = Math.ceil(height * pixelRatio);
        canvas.style.width = `${width}px`;
        canvas.style.height = `${height}px`;

        context.save();
        context.setTransform(1, 0, 0, 1, 0, 0);
        context.clearRect(0, 0, canvas.width, canvas.height);
        context.fillStyle = '#ffffff';
        context.fillRect(0, 0, canvas.width, canvas.height);
        context.restore();

        await page.render({
            canvasContext: context,
            viewport,
            transform: pixelRatio === 1 ? null : [pixelRatio, 0, 0, pixelRatio, 0, 0],
            background: 'rgba(255,255,255,1)'
        }).promise;

        return canvas;
    },

    async getExtractionCanvas(pageNumber) {
        if (this.state.renderCache.has(pageNumber)) {
            return this.state.renderCache.get(pageNumber);
        }

        const renderPromise = (async () => {
            const canvas = document.createElement('canvas');
            await this.renderPDFPageToCanvas(pageNumber, this.extractScale, canvas, 1);
            return canvas;
        })();

        this.state.renderCache.set(pageNumber, renderPromise);
        return renderPromise;
    },

    startDrawing(event) {
        if (this.state.loadingPreview || !this.previewCanvas.width) return;
        if (event.button !== 0 && event.pointerType !== 'touch') return;

        const rect = this.previewFrame.getBoundingClientRect();
        if (!rect.width || !rect.height) return;

        const point = this.pointWithinFrame(event, rect);
        this.state.drawing = {
            pointerId: event.pointerId,
            startX: point.x,
            startY: point.y,
            currentX: point.x,
            currentY: point.y
        };

        this.overlay.setPointerCapture?.(event.pointerId);
        this.renderDraft();
    },

    updateDrawing(event) {
        if (!this.state.drawing || this.state.drawing.pointerId !== event.pointerId) return;
        const rect = this.previewFrame.getBoundingClientRect();
        const point = this.pointWithinFrame(event, rect);
        this.state.drawing.currentX = point.x;
        this.state.drawing.currentY = point.y;
        this.renderDraft();
    },

    finishDrawing(event) {
        if (!this.state.drawing || this.state.drawing.pointerId !== event.pointerId) return;

        const rect = this.previewFrame.getBoundingClientRect();
        const point = this.pointWithinFrame(event, rect);
        this.state.drawing.currentX = point.x;
        this.state.drawing.currentY = point.y;

        const selection = this.buildSelectionFromDraft(rect);
        this.state.drawing = null;
        this.overlay.releasePointerCapture?.(event.pointerId);
        this.draftBox.classList.add('hidden');

        if (!selection) {
            return;
        }

        this.state.selections.push(selection);
        this.renderSelections();
    },

    buildSelectionFromDraft(rect) {
        if (!this.state.drawing || !rect.width || !rect.height) return null;

        const x1 = Math.min(this.state.drawing.startX, this.state.drawing.currentX);
        const y1 = Math.min(this.state.drawing.startY, this.state.drawing.currentY);
        const x2 = Math.max(this.state.drawing.startX, this.state.drawing.currentX);
        const y2 = Math.max(this.state.drawing.startY, this.state.drawing.currentY);

        const width = (x2 - x1) / rect.width;
        const height = (y2 - y1) / rect.height;

        if (width < 0.01 || height < 0.01) {
            return null;
        }

        return {
            id: this.state.nextSelectionId++,
            page_number: this.state.currentPage,
            x: x1 / rect.width,
            y: y1 / rect.height,
            width,
            height,
            caption: '',
            replace_figure_id: null
        };
    },

    pointWithinFrame(event, rect) {
        return {
            x: Math.max(0, Math.min(event.clientX - rect.left, rect.width)),
            y: Math.max(0, Math.min(event.clientY - rect.top, rect.height))
        };
    },

    renderDraft() {
        if (!this.state.drawing) {
            this.draftBox.classList.add('hidden');
            return;
        }

        const rect = this.previewFrame.getBoundingClientRect();
        const x1 = Math.min(this.state.drawing.startX, this.state.drawing.currentX);
        const y1 = Math.min(this.state.drawing.startY, this.state.drawing.currentY);
        const x2 = Math.max(this.state.drawing.startX, this.state.drawing.currentX);
        const y2 = Math.max(this.state.drawing.startY, this.state.drawing.currentY);

        this.draftBox.classList.remove('hidden');
        this.draftBox.style.left = `${(x1 / rect.width) * 100}%`;
        this.draftBox.style.top = `${(y1 / rect.height) * 100}%`;
        this.draftBox.style.width = `${((x2 - x1) / rect.width) * 100}%`;
        this.draftBox.style.height = `${((y2 - y1) / rect.height) * 100}%`;
    },

    renderSelections() {
        this.renderOverlay();

        if (!this.state.selections.length) {
            this.selectionList.innerHTML = `
                <div class="empty-state manual-empty-state">
                    <h3>还没有待提交的框选</h3>
                    <p>切到需要的页面后直接拖拽，右侧会同步生成待录入项。</p>
                </div>
            `;
            return;
        }

        this.selectionList.innerHTML = this.state.selections.map((selection, index) => `
            <article class="manual-selection-item ${selection.page_number === this.state.currentPage ? 'is-current-page' : ''}">
                <div class="manual-selection-head">
                    <div>
                        <strong>区域 ${index + 1}</strong>
                        <span>第 ${selection.page_number} 页</span>
                    </div>
                    <div class="manual-selection-actions">
                        <button class="btn btn-outline btn-small" type="button" data-selection-action="locate" data-selection-id="${selection.id}">定位</button>
                        <button class="btn btn-outline btn-small" type="button" data-selection-action="remove" data-selection-id="${selection.id}">删除</button>
                    </div>
                </div>
                <div class="manual-selection-meta">
                    <span>x ${(selection.x * 100).toFixed(1)}%</span>
                    <span>y ${(selection.y * 100).toFixed(1)}%</span>
                    <span>w ${(selection.width * 100).toFixed(1)}%</span>
                    <span>h ${(selection.height * 100).toFixed(1)}%</span>
                </div>
                <label class="field">
                    <span>Caption / 备注</span>
                    <textarea
                        class="form-textarea manual-selection-caption"
                        rows="3"
                        placeholder="可选，提交后作为图片说明保存"
                        data-selection-field="caption"
                        data-selection-id="${selection.id}"
                    >${Utils.escapeHTML(selection.caption || '')}</textarea>
                </label>
                <label class="field">
                    <span>替换已有图片</span>
                    <select
                        class="form-input"
                        data-selection-field="replace_figure_id"
                        data-selection-id="${selection.id}"
                    >
                        ${this.renderReplaceOptions(selection)}
                    </select>
                </label>
            </article>
        `).join('');
    },

    renderOverlay() {
        const currentPageSelections = this.currentPageSelections();
        this.overlay.innerHTML = currentPageSelections.map((selection, index) => `
            <div
                class="manual-selection-box"
                style="left:${selection.x * 100}%;top:${selection.y * 100}%;width:${selection.width * 100}%;height:${selection.height * 100}%"
            >
                <span>${index + 1}</span>
            </div>
        `).join('');
    },

    currentPageSelections() {
        return this.state.selections.filter((selection) => selection.page_number === this.state.currentPage);
    },

    findSelection(id) {
        return this.state.selections.find((selection) => selection.id === id);
    },

    renderReplaceOptions(selection) {
        const figures = this.state.paper?.figures || [];
        const selectedElsewhere = new Set(
            this.state.selections
                .filter((item) => item.id !== selection.id && item.replace_figure_id)
                .map((item) => Number(item.replace_figure_id))
        );

        const options = ['<option value="">作为新图片追加</option>'];
        figures.forEach((figure) => {
            const label = [
                `#${figure.figure_index || '-'}`,
                `第 ${figure.page_number || '-'} 页`,
                figure.source === 'manual' ? '人工' : '自动',
                figure.caption || figure.original_name || '未命名图片'
            ].join(' · ');

            const disabled = selectedElsewhere.has(Number(figure.id));
            const selected = Number(selection.replace_figure_id) === Number(figure.id);
            options.push(`
                <option value="${figure.id}" ${selected ? 'selected' : ''} ${disabled ? 'disabled' : ''}>
                    ${Utils.escapeHTML(label)}
                </option>
            `);
        });
        return options.join('');
    },

    scrollPreviewIntoView() {
        this.previewFrame.scrollIntoView({ behavior: 'smooth', block: 'center' });
    },

    async buildSelectionImageData(selection) {
        const canvas = await this.getExtractionCanvas(selection.page_number);
        const left = Math.max(0, Math.floor(selection.x * canvas.width));
        const top = Math.max(0, Math.floor(selection.y * canvas.height));
        const right = Math.min(canvas.width, Math.ceil((selection.x + selection.width) * canvas.width));
        const bottom = Math.min(canvas.height, Math.ceil((selection.y + selection.height) * canvas.height));
        const width = right - left;
        const height = bottom - top;

        if (width < 2 || height < 2) {
            throw new Error('框选区域过小，请重新选择');
        }

        const cropCanvas = document.createElement('canvas');
        cropCanvas.width = width;
        cropCanvas.height = height;
        const context = cropCanvas.getContext('2d', { alpha: false });
        context.fillStyle = '#ffffff';
        context.fillRect(0, 0, width, height);
        context.drawImage(canvas, left, top, width, height, 0, 0, width, height);
        return cropCanvas.toDataURL('image/png');
    },

    async submitSelections() {
        if (this.state.submitting) return;
        if (!this.state.selections.length) {
            Utils.showToast('请先框选至少一个区域', 'error');
            return;
        }

        this.state.submitting = true;
        this.submitBtn.disabled = true;
        this.submitBtn.textContent = '提取中...';

        try {
            const regions = [];
            for (const selection of this.state.selections) {
                regions.push({
                    page_number: selection.page_number,
                    x: selection.x,
                    y: selection.y,
                    width: selection.width,
                    height: selection.height,
                    image_data: await this.buildSelectionImageData(selection),
                    caption: selection.caption.trim(),
                    replace_figure_id: selection.replace_figure_id || null
                });
            }

            const payload = await API.manualExtractFigures(this.state.paperId, { regions });

            this.state.paper = payload.paper;
            this.state.selections = [];
            this.renderWorkspace();
            this.renderSelections();
            Utils.showToast(`已录入 ${payload.added_count || 0} 张图片`);
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            this.state.submitting = false;
            this.submitBtn.disabled = false;
            this.submitBtn.textContent = '提取并录入图片';
        }
    }
};
