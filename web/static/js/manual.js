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
        extractingText: false,
        extractingTextPage: 0,
        pdfjsLib: null,
        pdfDocument: null,
        renderCache: new Map(),
        previewRenderToken: 0
    },

    async init() {
        if (typeof t !== 'function') window.t = function(k, f) { return f || k; };
        this.cacheElements();
        if (!this.previewFrame) return;

        const paperId = Number(new URLSearchParams(window.location.search).get('paper_id'));
        if (!paperId) {
            Utils.showToast(t('manual.err_missing_paper_id', '缺少 paper_id 参数'), 'error');
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
        this.extractTextBtn = document.getElementById('manualExtractTextBtn');
        this.selectionList = document.getElementById('manualSelectionList');
        this.workspaceHint = document.getElementById('manualWorkspaceHint');
        this.notice = document.getElementById('manualExtractionNotice');
        this.pageFigureTitle = document.getElementById('manualPageFigureTitle');
        this.pageFigureList = document.getElementById('manualPageFigureList');
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
            const confirmed = await Utils.confirm(t('manual.msg_confirm_clear_page', '将删除第 {page} 页上的 {count} 个框选区域。').replace('{page}', this.state.currentPage).replace('{count}', pageSelections.length));
            if (!confirmed) return;
            this.state.selections = this.state.selections.filter((item) => item.page_number !== this.state.currentPage);
            this.renderSelections();
        });

        this.clearAllBtn.addEventListener('click', async () => {
            if (!this.state.selections.length) return;
            const confirmed = await Utils.confirm(t('manual.msg_confirm_clear_all', '将清空当前文献的全部待提交框选区域。'));
            if (!confirmed) return;
            this.state.selections = [];
            this.renderSelections();
        });

        this.submitBtn.addEventListener('click', async () => {
            await this.submitSelections();
        });

        if (this.extractTextBtn) {
            this.extractTextBtn.addEventListener('click', async () => {
                await this.extractAndSaveFullText();
            });
        }

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
            throw new Error(t('manual.err_no_pdf_url', '当前文献缺少 PDF 文件地址'));
        }

        this.previewFrame.classList.add('is-loading');
        this.workspaceHint.textContent = t('manual.msg_loading_pdf', '正在加载 PDF 文档...');

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
        const status = paper.extraction_status || '';

        this.pageTitle.textContent = t('manual.msg_title_format', '{title} 的人工框选提取').replace('{title}', paper.title);
        this.pageSubtitle.textContent = paper.extractor_message
            ? ((status === 'queued' || status === 'running' || status === 'failed' || status === 'cancelled')
                ? t('manual.msg_auto_status', '自动解析状态：{status}。{message}').replace('{status}', Utils.statusLabel(status)).replace('{message}', paper.extractor_message)
                : paper.extractor_message)
            : t('manual.msg_default_subtitle', '在页面上框选图片区域，提交后会追加到当前文献。');

        this.openPDFLink.href = paper.pdf_url ? Utils.resourceViewerURL('pdf', paper.pdf_url) : '/';
        this.backLibraryLink.href = `/library?paper_id=${encodeURIComponent(paper.id)}`;
        this.pageIndicator.textContent = this.state.pageCount
            ? t('manual.page_indicator', '第 {current} / {total} 页').replace('{current}', this.state.currentPage).replace('{total}', this.state.pageCount)
            : t('manual.msg_reading_pdf', '正在读取 PDF...');
        this.prevPageBtn.disabled = this.state.currentPage <= 1;
        this.nextPageBtn.disabled = !this.state.pageCount || this.state.currentPage >= this.state.pageCount;

        const fullTextDisplay = paper.pdf_text
            ? t('manual.stat_fulltext_chars', '{count} 字').replace('{count}', paper.pdf_text.length.toLocaleString())
            : t('manual.stat_fulltext_none', '未保存');

        this.summaryStrip.innerHTML = `
            <div class="stat-card"><span>${t('manual.stat_pdf_pages', 'PDF 页数')}</span><strong>${this.state.pageCount || '...'}</strong></div>
            <div class="stat-card"><span>${t('manual.stat_figures', '已有图片')}</span><strong>${paper.figure_count || (paper.figures || []).length || 0}</strong></div>
            <div class="stat-card"><span>${t('manual.stat_manual', '人工补录')}</span><strong>${manualCount}</strong></div>
            <div class="stat-card"><span>${t('manual.stat_fulltext', 'PDF 全文')}</span><strong>${fullTextDisplay}</strong></div>
            <div class="stat-card"><span>${t('manual.stat_status', '当前状态')}</span><strong>${Utils.escapeHTML(Utils.statusLabel(status))}</strong></div>
        `;

        if (paper.extractor_message) {
            this.notice.classList.remove('hidden', 'error', 'info', 'success');
            this.notice.classList.add(Utils.statusTone(paper.extraction_status || ''));
            this.notice.textContent = paper.extractor_message;
        } else {
            this.notice.classList.add('hidden');
            this.notice.textContent = '';
        }

        this.renderActionButtons();
    },

    renderMissingPaper(message) {
        const defaultMessage = t('manual.err_load_failed', '无法加载当前文献，请返回文献库重新进入。');
        message = message || defaultMessage;
        if (this.pageTitle) this.pageTitle.textContent = t('manual.err_workspace_title', '人工框选提取');
        if (this.pageSubtitle) this.pageSubtitle.textContent = message;
        if (this.summaryStrip) {
            this.summaryStrip.innerHTML = `
                <div class="empty-state">
                    <h3>${t('manual.err_workspace_empty_title', '无法进入手动标注工作台')}</h3>
                    <p>${Utils.escapeHTML(message)}</p>
                    <a class="btn btn-primary" href="/library">${t('manual.back_library', '返回文献库')}</a>
                </div>
            `;
        }
    },

    async loadPage(page) {
        if (!this.state.pdfDocument || !this.state.pageCount) return;

        this.state.currentPage = page;
        this.syncPageParam(page);
        this.state.loadingPreview = true;
        this.previewFrame.classList.add('is-loading');
        this.workspaceHint.textContent = t('manual.msg_rendering_page', '正在渲染第 {page} 页...').replace('{page}', page);
        this.pageIndicator.textContent = t('manual.page_indicator', '第 {current} / {total} 页').replace('{current}', page).replace('{total}', this.state.pageCount);
        this.prevPageBtn.disabled = page <= 1;
        this.nextPageBtn.disabled = page >= this.state.pageCount;
        this.renderCurrentPageFigures();
        this.renderSelections();

        const renderToken = ++this.state.previewRenderToken;
        try {
            await this.renderPDFPageToCanvas(page, this.previewScale, this.previewCanvas, window.devicePixelRatio || 1);
            if (renderToken !== this.state.previewRenderToken) {
                return;
            }
            this.state.loadingPreview = false;
            this.previewFrame.classList.remove('is-loading');
            this.workspaceHint.textContent = this.pageWorkspaceHint(page, this.currentPageFigures().length);
            this.renderSelections();
        } catch (error) {
            if (renderToken !== this.state.previewRenderToken) {
                return;
            }
            this.state.loadingPreview = false;
            this.previewFrame.classList.remove('is-loading');
            this.workspaceHint.textContent = t('manual.err_render_failed', 'PDF 页面渲染失败，请稍后重试。');
            Utils.showToast(error.message || t('manual.err_render_failed_short', 'PDF 页面渲染失败'), 'error');
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
        // Evict oldest entries to prevent unbounded memory growth
        if (this.state.renderCache.size > 20) {
            const oldest = this.state.renderCache.keys().next().value;
            this.state.renderCache.delete(oldest);
        }
        return renderPromise;
    },

    syncPageParam(page) {
        try {
            const url = new URL(window.location.href);
            if (page > 1) {
                url.searchParams.set('page', String(page));
            } else {
                url.searchParams.delete('page');
            }
            window.history.replaceState(window.history.state, '', `${url.pathname}${url.search}${url.hash}`);
        } catch (error) {
            // Ignore URL sync failures and keep the manual workspace usable.
        }
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
                    <h3>${t('manual.empty_title', '还没有待提交的框选')}</h3>
                    <p>${t('manual.empty_desc', '切到需要的页面后直接拖拽，右侧会同步生成待录入项。')}</p>
                </div>
            `;
            return;
        }

        this.selectionList.innerHTML = this.state.selections.map((selection, index) => `
            <article class="manual-selection-item ${selection.page_number === this.state.currentPage ? 'is-current-page' : ''}">
                <div class="manual-selection-head">
                    <div>
                        <strong>${t('manual.selection_region', '区域 {index}').replace('{index}', index + 1)}</strong>
                        <span>${t('manual.selection_page', '第 {page} 页').replace('{page}', selection.page_number)}</span>
                    </div>
                    <div class="manual-selection-actions">
                        <button class="btn btn-outline btn-small" type="button" data-selection-action="locate" data-selection-id="${selection.id}">${t('manual.selection_locate', '定位')}</button>
                        <button class="btn btn-outline btn-small" type="button" data-selection-action="remove" data-selection-id="${selection.id}">${t('manual.selection_remove', '删除')}</button>
                    </div>
                </div>
                <div class="manual-selection-meta">
                    <span>x ${(selection.x * 100).toFixed(1)}%</span>
                    <span>y ${(selection.y * 100).toFixed(1)}%</span>
                    <span>w ${(selection.width * 100).toFixed(1)}%</span>
                    <span>h ${(selection.height * 100).toFixed(1)}%</span>
                </div>
                <label class="field">
                    <span>${t('manual.selection_caption_label', 'Caption / 备注')}</span>
                    <textarea
                        class="form-textarea manual-selection-caption"
                        rows="3"
                        placeholder="${t('manual.selection_caption_placeholder', '可选，提交后作为图片说明保存')}"
                        data-selection-field="caption"
                        data-selection-id="${selection.id}"
                    >${Utils.escapeHTML(selection.caption || '')}</textarea>
                </label>
                <label class="field">
                    <span>${t('manual.selection_replace_label', '替换已有图片')}</span>
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

    currentPageFigures() {
        const page = Number(this.state.currentPage || 0);
        return [...(this.state.paper?.figures || [])]
            .filter((figure) => Number(figure.page_number) === page && String(figure.image_url || '').trim())
            .sort((left, right) => {
                const figureDelta = Number(left.figure_index || 0) - Number(right.figure_index || 0);
                if (figureDelta) return figureDelta;

                const parentDelta = Number(Boolean(left.parent_figure_id)) - Number(Boolean(right.parent_figure_id));
                if (parentDelta) return parentDelta;

                const labelDelta = String(left.subfigure_label || '').localeCompare(String(right.subfigure_label || ''), undefined, {
                    sensitivity: 'base'
                });
                if (labelDelta) return labelDelta;

                return Number(left.id || 0) - Number(right.id || 0);
            });
    },

    pageWorkspaceHint(page, figureCount) {
        if (figureCount > 0) {
            return t('manual.msg_page_hint_with_figures', '当前是第 {page} 页，已显示 {count} 张已有图片，可直接对照后继续补框。')
                .replace('{page}', page)
                .replace('{count}', figureCount);
        }

        return t('manual.msg_page_hint', '当前是第 {page} 页。拖拽可新增框选，右侧可补充 caption 并提交。')
            .replace('{page}', page);
    },

    renderCurrentPageFigures() {
        if (!this.pageFigureList || !this.pageFigureTitle) return;

        const page = Number(this.state.currentPage || 1);
        const figures = this.currentPageFigures();

        if (!figures.length) {
            this.pageFigureTitle.textContent = t('manual.page_figures_title', '当前页已有图片');
            this.pageFigureList.innerHTML = `<div class="manual-page-figure-empty">${t('manual.page_figures_empty', '第 {page} 页还没有已提取图片。').replace('{page}', page)}</div>`;
            return;
        }

        this.pageFigureTitle.textContent = t('manual.page_figures_title_count', '当前页已有图片（{count}）')
            .replace('{page}', page)
            .replace('{count}', figures.length);

        this.pageFigureList.innerHTML = figures.map((figure) => {
            const title = String(figure.display_label || '').trim() || t('manual.page_figure_fallback', '图片');
            const imageURL = Utils.escapeHTML(String(figure.image_url || ''));
            const openURL = Utils.escapeHTML(Utils.resourceViewerURL('image', figure.image_url));
            const altText = Utils.escapeHTML(String(figure.caption || title));

            return `
                <article class="manual-page-figure-card">
                    <a class="manual-page-figure-media" href="${openURL}" aria-label="${Utils.escapeHTML(title)}">
                        <img src="${imageURL}" alt="${altText}" loading="lazy">
                    </a>
                </article>
            `;
        }).join('');
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

        const options = [`<option value="">${t('manual.selection_replace_new', '作为新图片追加')}</option>`];
        figures.forEach((figure) => {
            const figureTitle = String(figure.display_label || '').trim() || t('manual.page_figure_fallback', '图片');
            const label = `${figureTitle} · ${t('manual.selection_page', '第 {page} 页').replace('{page}', figure.page_number || '-')}`;

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

    renderActionButtons() {
        if (this.submitBtn) {
            this.submitBtn.disabled = this.state.submitting || this.state.extractingText;
            this.submitBtn.textContent = this.state.submitting ? t('manual.msg_submitting', '提取中...') : t('manual.submit_btn', '提取并录入图片');
        }

        this.renderFullTextStatus();
    },

    renderFullTextStatus() {
        if (!this.extractTextBtn) return;

        const paper = this.state.paper;
        const hasText = Boolean(paper?.pdf_text);
        const extracting = this.state.extractingText;
        const pageCount = Math.max(this.state.pageCount || 0, 0);
        const extractingPage = Math.min(this.state.extractingTextPage || 0, pageCount || Number.MAX_SAFE_INTEGER);

        this.extractTextBtn.disabled = extracting || this.state.submitting || !this.state.pdfDocument;

        let statusText = '';
        if (extracting) {
            const progress = pageCount ? `${extractingPage}/${pageCount}` : '';
            this.extractTextBtn.textContent = t('manual.extract_text_progress', '提取全文中 {progress}').replace('{progress}', progress).trim();
            statusText = pageCount
                ? t('manual.fulltext_extracting_page', '正在读取第 {current} / {total} 页文本并保存到当前文献。').replace('{current}', extractingPage).replace('{total}', pageCount)
                : t('manual.fulltext_extracting', '正在提取当前 PDF 的全文内容。');
        } else if (hasText) {
            this.extractTextBtn.textContent = t('manual.extract_text_re', '重新提取全文');
            statusText = t('manual.fulltext_status_has_text', '当前已保存 {count} 字全文，可重新提取覆盖，供 AI 伴读、检索和后续整理使用。')
                .replace('{count}', paper.pdf_text.length.toLocaleString());
        } else {
            this.extractTextBtn.textContent = t('manual.extract_text_btn', '提取全文并保存');
            statusText = t('manual.fulltext_status_empty', '当前还没有保存全文。点击右侧按钮即可提取并保存，供 AI 伴读和检索使用。');
        }

        this.extractTextBtn.title = statusText;
        this.extractTextBtn.setAttribute('aria-label', statusText);
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
            throw new Error(t('manual.err_selection_too_small', '框选区域过小，请重新选择'));
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
            Utils.showToast(t('manual.err_no_selection', '请先框选至少一个区域'), 'error');
            return;
        }

        this.state.submitting = true;
        this.renderActionButtons();

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
            this.renderCurrentPageFigures();
            this.renderSelections();
            Utils.showToast(t('manual.msg_added_figures', '已录入 {count} 张图片').replace('{count}', payload.added_count || 0));
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            this.state.submitting = false;
            this.renderActionButtons();
        }
    },

    async extractAndSaveFullText() {
        if (this.state.extractingText) return;
        if (!this.state.pdfDocument) {
            Utils.showToast(t('manual.err_pdf_not_loaded', 'PDF 还没有加载完成'), 'error');
            return;
        }

        this.state.extractingText = true;
        this.state.extractingTextPage = 0;
        this.renderActionButtons();

        try {
            const pdfText = await this.extractFullTextFromPDF();
            if (!pdfText) {
                throw new Error(t('manual.err_no_text_extracted', '没有从当前 PDF 中提取到可用全文'));
            }

            const payload = await API.updatePaperPDFText(this.state.paperId, {
                pdf_text: pdfText
            });

            this.state.paper = payload.paper;
            this.renderWorkspace();
            Utils.showToast(t('manual.msg_saved_text', '已保存全文（{count} 字）').replace('{count}', pdfText.length.toLocaleString()));
        } catch (error) {
            Utils.showToast(error.message || t('manual.err_extract_text_failed', '提取全文失败'), 'error');
        } finally {
            this.state.extractingText = false;
            this.state.extractingTextPage = 0;
            this.renderActionButtons();
        }
    },

    async extractFullTextFromPDF() {
        const pages = [];

        for (let pageNumber = 1; pageNumber <= this.state.pageCount; pageNumber += 1) {
            this.state.extractingTextPage = pageNumber;
            this.renderFullTextStatus();

            const page = await this.state.pdfDocument.getPage(pageNumber);
            const textContent = await page.getTextContent();
            const pageText = this.extractPageText(textContent);
            if (pageText) {
                pages.push(pageText);
            }
        }

        return pages.join('\n\n').trim();
    },

    extractPageText(textContent) {
        const items = Array.isArray(textContent?.items) ? textContent.items : [];
        const lines = [];
        let currentLine = '';
        let previousItem = null;

        const flushLine = () => {
            const normalized = currentLine.trimEnd();
            if (normalized) {
                lines.push(normalized);
            }
            currentLine = '';
        };

        items.forEach((item) => {
            const text = String(item?.str || '').replace(/\u00a0/g, ' ');
            const x = Number(item?.transform?.[4] || 0);
            const y = Number(item?.transform?.[5] || 0);
            const width = Number(item?.width || 0);

            if (!text) {
                if (item?.hasEOL) {
                    flushLine();
                    previousItem = null;
                }
                return;
            }

            if (previousItem && Math.abs(y - previousItem.y) > 2) {
                flushLine();
                previousItem = null;
            }

            if (previousItem && this.shouldInsertSpaceBetween(previousItem, { text, x })) {
                currentLine += ' ';
            }

            currentLine += text;

            if (item?.hasEOL) {
                flushLine();
                previousItem = null;
                return;
            }

            previousItem = { text, x, y, width };
        });

        flushLine();
        return lines.join('\n').trim();
    },

    shouldInsertSpaceBetween(previousItem, currentItem) {
        if (!previousItem || !currentItem) return false;
        if (!previousItem.text || !currentItem.text) return false;
        if (/\s$/.test(previousItem.text) || /^\s/.test(currentItem.text)) return false;
        if (/^[,.;:!?%)\]}]/.test(currentItem.text)) return false;
        if (/[([{]$/.test(previousItem.text)) return false;
        if (this.isCJKBoundary(previousItem.text, currentItem.text)) return false;
        return currentItem.x - (previousItem.x + previousItem.width) > 1;
    },

    isCJKBoundary(previousText, currentText) {
        return /[\u3400-\u9fff]$/.test(previousText) && /^[\u3400-\u9fff]/.test(currentText);
    }
};
