const ManualPage = {
    state: {
        paperId: null,
        paper: null,
        pageCount: 0,
        currentPage: 1,
        selections: [],
        nextSelectionId: 1,
        drawing: null,
        loadingPreview: false,
        submitting: false
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
        this.previewImage = document.getElementById('manualPreviewImage');
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

        this.previewImage.addEventListener('load', () => {
            this.state.loadingPreview = false;
            this.previewFrame.classList.remove('is-loading');
            this.workspaceHint.textContent = `当前是第 ${this.state.currentPage} 页。拖拽可新增框选，右侧可补充 caption 并提交。`;
            this.renderSelections();
        });

        this.previewImage.addEventListener('error', () => {
            this.state.loadingPreview = false;
            this.previewFrame.classList.remove('is-loading');
            this.workspaceHint.textContent = '页面预览加载失败，请稍后重试。';
            Utils.showToast('PDF 页面预览加载失败', 'error');
        });
    },

    async loadWorkspace() {
        try {
            const payload = await API.getManualExtractionWorkspace(this.state.paperId);
            const workspace = payload.workspace;
            this.state.paper = workspace.paper;
            this.state.pageCount = workspace.page_count || 0;

            this.renderWorkspace();

            const initialPage = Number(new URLSearchParams(window.location.search).get('page')) || 1;
            await this.loadPage(Math.min(Math.max(initialPage, 1), Math.max(this.state.pageCount, 1)));
        } catch (error) {
            Utils.showToast(error.message, 'error');
            this.renderMissingPaper(error.message);
        }
    },

    renderWorkspace() {
        const paper = this.state.paper;
        const manualCount = (paper.figures || []).filter((figure) => figure.source === 'manual').length;

        this.pageTitle.textContent = `${paper.title} 的人工框选提取`;
        this.pageSubtitle.textContent = paper.extractor_message
            ? `自动处理状态：${Utils.statusLabel(paper.extraction_status)}。${paper.extractor_message}`
            : '可直接在页图上框选图片区域，并把裁剪结果追加到当前文献。';

        this.openPDFLink.href = paper.pdf_url || '/';
        this.backLibraryLink.href = '/';

        this.summaryStrip.innerHTML = `
            <div class="stat-card"><span>PDF 页数</span><strong>${this.state.pageCount || 0}</strong></div>
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
                    <a class="btn btn-primary" href="/">返回文献库</a>
                </div>
            `;
        }
    },

    async loadPage(page) {
        if (!this.state.pageCount) return;
        this.state.currentPage = page;
        this.state.loadingPreview = true;
        this.previewFrame.classList.add('is-loading');
        this.workspaceHint.textContent = `正在加载第 ${page} 页预览...`;
        this.pageIndicator.textContent = `第 ${page} / ${this.state.pageCount} 页`;
        this.prevPageBtn.disabled = page <= 1;
        this.nextPageBtn.disabled = page >= this.state.pageCount;
        this.previewImage.src = `${API.manualPreviewURL(this.state.paperId, page)}&t=${Date.now()}`;
        this.renderSelections();
    },

    startDrawing(event) {
        if (this.state.loadingPreview || !this.previewImage.src) return;
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
            const payload = await API.manualExtractFigures(this.state.paperId, {
                regions: this.state.selections.map((selection) => ({
                    page_number: selection.page_number,
                    x: selection.x,
                    y: selection.y,
                    width: selection.width,
                    height: selection.height,
                    caption: selection.caption.trim(),
                    replace_figure_id: selection.replace_figure_id || null
                }))
            });

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
