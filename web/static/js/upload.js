const UploadPage = {
    file: null,
    pollTimer: null,
    activePaperId: null,
    pollInFlight: false,
    lastStatus: '',
    currentPaper: null,
    extractorReady: false,
    extractorSettings: null,
    aiSettings: null,
    lastUploadedFile: null,
    pdfjsLib: null,
    figureSyncState: {
        paperId: 0,
        status: 'idle',
        message: ''
    },
    pdfTextSyncState: {
        paperId: 0,
        status: 'idle',
        message: ''
    },

    async init() {
        this.form = document.getElementById('paperUploadForm');
        if (!this.form) return;

        this.pdfInput = document.getElementById('pdfInput');
        this.dropArea = document.getElementById('dropArea');
        this.selectedFile = document.getElementById('selectedFile');
        this.titleInput = document.getElementById('titleInput');
        this.groupSelect = document.getElementById('groupSelect');
        this.tagsInput = document.getElementById('tagsInput');
        this.tagsSuggestions = document.getElementById('tagsInputSuggestions');
        this.extractionModeSelect = document.getElementById('extractionModeSelect');
        this.extractionModeHint = document.getElementById('extractionModeHint');
        this.submitButton = document.getElementById('submitUploadBtn');
        this.resultCard = document.getElementById('uploadResult');

        this.tagAutocomplete = Utils.bindCommaSeparatedTagInputAutocomplete?.({
            input: this.tagsInput,
            panel: this.tagsSuggestions,
            scope: 'paper'
        }) || null;

        this.bindEvents();
        await Promise.all([
            this.loadGroups(),
            this.loadExtractionModeOptions()
        ]);
    },

    bindEvents() {
        this.dropArea.addEventListener('click', () => this.pdfInput.click());
        this.dropArea.addEventListener('keydown', (event) => {
            if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                this.pdfInput.click();
            }
        });

        ['dragenter', 'dragover', 'dragleave', 'drop'].forEach((eventName) => {
            this.dropArea.addEventListener(eventName, (event) => {
                event.preventDefault();
                event.stopPropagation();
            });
        });

        ['dragenter', 'dragover'].forEach((eventName) => {
            this.dropArea.addEventListener(eventName, () => {
                this.dropArea.classList.add('dragover');
            });
        });

        ['dragleave', 'drop'].forEach((eventName) => {
            this.dropArea.addEventListener(eventName, () => {
                this.dropArea.classList.remove('dragover');
            });
        });

        this.dropArea.addEventListener('drop', (event) => {
            const file = event.dataTransfer.files[0];
            this.setFile(file);
        });

        this.pdfInput.addEventListener('change', (event) => {
            this.setFile(event.target.files[0]);
        });

        this.form.addEventListener('submit', async (event) => {
            event.preventDefault();
            await this.submit();
        });

        this.resultCard.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-action]');
            if (!button) return;
            if (button.dataset.action === 'reextract' && this.activePaperId) {
                await this.reextractCurrentPaper();
            }
        });

        window.addEventListener('beforeunload', () => this.stopPolling());
    },

    async loadGroups() {
        try {
            const payload = await API.listGroups();
            const groups = payload.groups || [];
            this.groupSelect.innerHTML = '<option value="">暂不分组</option>';
            groups.forEach((group) => {
                this.groupSelect.insertAdjacentHTML(
                    'beforeend',
                    `<option value="${group.id}">${Utils.escapeHTML(group.name)}</option>`
                );
            });
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async loadExtractionModeOptions() {
        let extractorSettings = {
            extractor_profile: 'pdffigx_v1',
            pdf_text_source: 'extractor'
        };
        try {
            extractorSettings = await API.getExtractorSettings();
        } catch (error) {
            Utils.showToast('加载自动解析配置失败，当前仅允许手工标注', 'error');
        }
        this.extractorSettings = extractorSettings;

        try {
            this.aiSettings = await API.getAISettings();
        } catch (error) {
            this.aiSettings = null;
        }

        this.extractorReady = this.isAutoExtractionReady(this.extractorSettings, this.aiSettings);
        const usesManualProfile = this.usesManualExtractionProfile(this.extractorSettings);

        if (!this.extractionModeSelect) return;

        if (usesManualProfile) {
            this.extractionModeSelect.innerHTML = '<option value="manual">手工</option>';
            this.extractionModeSelect.value = 'manual';
            this.extractionModeSelect.disabled = true;
        } else {
            this.extractionModeSelect.innerHTML = `
                <option value="auto" ${this.extractorReady ? '' : 'disabled'}>自动标注</option>
                <option value="manual">手工标注</option>
            `;
            this.extractionModeSelect.value = this.extractorReady ? 'auto' : 'manual';
            this.extractionModeSelect.disabled = false;
        }

        if (this.extractionModeHint) {
            if (usesManualProfile) {
                this.extractionModeHint.textContent = '当前 PDF 提取方案为手工：上传后不会自动提图，但会自动提取并保存全文；微信上传也同样如此。';
            } else if (this.usesBuiltInLLMExtraction(this.extractorSettings) && this.extractorReady) {
                this.extractionModeHint.textContent = '默认使用自动标注；上传后后台会用内置 AI 解析图片坐标，全文也会自动保存。';
            } else if (this.usesBuiltInLLMExtraction(this.extractorSettings)) {
                this.extractionModeHint.textContent = '当前已选择内置 AI 坐标提取，但图片场景模型或 API Key 还没配好，只能使用手工标注；上传后会自动保存全文。';
            } else if (this.extractorReady && this.usesBrowserPDFText(this.extractorSettings)) {
                this.extractionModeHint.textContent = '默认自动标注，系统会自动提取图片并保存全文。';
            } else if (this.extractorReady) {
                this.extractionModeHint.textContent = '默认使用自动标注；也可以切到手工标注自行框选图片。';
            } else {
                this.extractionModeHint.textContent = '尚未配置自动解析，仅支持手工标注。上传后会自动保存全文。';
            }
        }
    },

    setFile(file) {
        if (!file) return;
        const isPDF = file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf');
        if (!isPDF) {
            Utils.showToast('请选择 PDF 文件', 'error');
            return;
        }

        this.file = file;
        this.selectedFile.classList.remove('empty');
        this.selectedFile.innerHTML = `
            <strong>${Utils.escapeHTML(file.name)}</strong>
            <span>${Utils.formatFileSize(file.size)}</span>
        `;

        if (!this.titleInput.value.trim()) {
            this.titleInput.value = file.name.replace(/\.pdf$/i, '').replaceAll(/[_-]+/g, ' ').trim();
        }
    },

    async submit() {
        if (!this.file) {
            Utils.showToast('请先选择 PDF', 'error');
            return;
        }

        const sourceFile = this.file;
        const extractorSettings = { ...(this.extractorSettings || {}) };
        const extractionMode = this.extractionModeSelect?.value || 'manual';
        const formData = new FormData();
        formData.append('pdf', sourceFile);
        formData.append('title', this.titleInput.value.trim());
        formData.append('tags', this.tagsInput.value.trim());
        if (this.extractionModeSelect?.value) {
            formData.append('extraction_mode', extractionMode);
        }
        if (this.groupSelect.value) {
            formData.append('group_id', this.groupSelect.value);
        }

        this.submitButton.disabled = true;
        this.submitButton.textContent = '上传中...';

        try {
            const payload = await API.uploadPaper(formData);
            const paper = payload.paper;
            this.lastUploadedFile = sourceFile;
            this.tagAutocomplete?.mergeTags(paper?.tags || Utils.splitTags(this.tagsInput.value.trim()));

            this.renderResult(paper);
            this.startPolling(paper);
            void this.runPostUploadEnrichment(paper, sourceFile, extractorSettings, extractionMode);

            let toastMessage = '文献已入库';
            if (Utils.isProcessingStatus(paper.extraction_status)) {
                toastMessage = '文献已入库，后台开始解析';
            }
            Utils.showToast(toastMessage, Utils.statusTone(paper.extraction_status));

            this.form.reset();
            this.file = null;
            this.selectedFile.classList.add('empty');
            this.selectedFile.innerHTML = '<span>尚未选择 PDF</span>';
            this.tagAutocomplete?.refresh?.();
            await Promise.all([
                this.loadGroups(),
                this.loadExtractionModeOptions()
            ]);
        } catch (error) {
            const duplicatePaperId = Number(error?.payload?.paper?.id || 0);
            if (error?.code === 'CONFLICT' && duplicatePaperId > 0) {
                Utils.showToast(error.message, 'info');
                window.location.href = `/library?paper_id=${encodeURIComponent(duplicatePaperId)}&from=duplicate`;
                return;
            }
            Utils.showToast(error.message, 'error');
        } finally {
            this.submitButton.disabled = false;
            this.submitButton.textContent = '上传文献';
        }
    },

    startPolling(paper) {
        this.stopPolling();
        if (!paper || !paper.id) return;

        this.activePaperId = Number(paper.id);
        this.lastStatus = paper.extraction_status || '';

        if (!Utils.isProcessingStatus(this.lastStatus)) {
            return;
        }

        this.pollTimer = window.setInterval(() => {
            this.pollPaper();
        }, 2000);
    },

    stopPolling() {
        if (this.pollTimer) {
            window.clearInterval(this.pollTimer);
            this.pollTimer = null;
        }
        this.pollInFlight = false;
    },

    async pollPaper() {
        if (!this.activePaperId || this.pollInFlight) return;
        this.pollInFlight = true;

        try {
            const paper = await API.getPaper(this.activePaperId);
            const previousStatus = this.lastStatus;
            this.lastStatus = paper.extraction_status || '';
            this.renderResult(paper);

            if (!Utils.isProcessingStatus(this.lastStatus)) {
                this.stopPolling();
                if (Utils.isProcessingStatus(previousStatus)) {
                    const tone = Utils.statusTone(this.lastStatus);
                    const message = this.lastStatus === 'completed' ? '文献解析完成' : '文献解析已结束';
                    Utils.showToast(message, tone);
                }
            }
        } catch (error) {
            this.stopPolling();
            Utils.showToast(error.message, 'error');
        } finally {
            this.pollInFlight = false;
        }
    },

    async reextractCurrentPaper() {
        if (!this.activePaperId) return;
        try {
            const payload = await API.reextractPaper(this.activePaperId);
            const paper = payload.paper;
            this.renderResult(paper);
            this.startPolling(paper);
            Utils.showToast('文献已重新提交解析', 'info');
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    usesBuiltInLLMExtraction(settings) {
        return String(settings?.extractor_profile || '').trim() === 'open_source_vision';
    },

    usesManualExtractionProfile(settings) {
        return String(settings?.extractor_profile || '').trim() === 'manual';
    },

    usesBrowserPDFText(settings) {
        return String(settings?.pdf_text_source || '').trim() === 'pdfjs';
    },

    resolveAIModelConfig(aiSettings, preferredModelId) {
        const models = Array.isArray(aiSettings?.models) ? aiSettings.models : [];
        if (preferredModelId) {
            const matched = models.find((item) => String(item?.id || '').trim() === String(preferredModelId).trim());
            if (matched) {
                return matched;
            }
        }
        return models[0] || null;
    },

    resolveFigureDetectionModel(aiSettings) {
        const sceneModels = aiSettings?.scene_models || {};
        return this.resolveAIModelConfig(
            aiSettings,
            sceneModels.figure_model_id || sceneModels.default_model_id || ''
        );
    },

    isBuiltInLLMReady(settings, aiSettings) {
        if (!this.usesBuiltInLLMExtraction(settings)) {
            return false;
        }
        const modelConfig = this.resolveFigureDetectionModel(aiSettings);
        return Boolean(
            modelConfig &&
            String(modelConfig.api_key || '').trim() &&
            String(modelConfig.model || '').trim() &&
            String(modelConfig.provider || '').trim()
        );
    },

    isAutoExtractionReady(settings, aiSettings) {
        if (this.usesManualExtractionProfile(settings)) {
            return false;
        }
        if (this.usesBuiltInLLMExtraction(settings)) {
            return this.isBuiltInLLMReady(settings, aiSettings);
        }
        return Boolean(settings?.effective_extractor_url);
    },

    shouldRunBuiltInLLMExtraction(settings, extractionMode) {
        return String(extractionMode || '').trim() === 'auto' && this.isBuiltInLLMReady(settings, this.aiSettings);
    },

    shouldAutoExtractPDFText(settings, extractionMode) {
        return this.usesBrowserPDFText(settings) || String(extractionMode || '').trim() === 'manual';
    },

    setFigureSyncState(paperId, status, message) {
        this.figureSyncState = {
            paperId: Number(paperId || 0),
            status: String(status || 'idle'),
            message: String(message || '')
        };
        if (this.currentPaper && Number(this.currentPaper.id) === Number(this.figureSyncState.paperId)) {
            this.renderResult(this.currentPaper);
        }
    },

    setPDFTextSyncState(paperId, status, message) {
        this.pdfTextSyncState = {
            paperId: Number(paperId || 0),
            status: String(status || 'idle'),
            message: String(message || '')
        };
        if (this.currentPaper && Number(this.currentPaper.id) === Number(this.pdfTextSyncState.paperId)) {
            this.renderResult(this.currentPaper);
        }
    },

    renderPDFTextSyncNotice(paper) {
        const state = this.pdfTextSyncState || {};
        if (!paper?.id || Number(state.paperId) !== Number(paper.id) || !state.message || state.status === 'idle') {
            return '';
        }

        let tone = 'info';
        if (state.status === 'success') {
            tone = 'success';
        } else if (state.status === 'error') {
            tone = 'error';
        }
        return `<p class="notice ${tone}">${Utils.escapeHTML(state.message)}</p>`;
    },

    renderFigureSyncNotice(paper) {
        const state = this.figureSyncState || {};
        if (!paper?.id || Number(state.paperId) !== Number(paper.id) || !state.message || state.status === 'idle') {
            return '';
        }

        let tone = 'info';
        if (state.status === 'success') {
            tone = 'success';
        } else if (state.status === 'error') {
            tone = 'error';
        }
        return `<p class="notice ${tone}">${Utils.escapeHTML(state.message)}</p>`;
    },

    async runPostUploadEnrichment(paper, file, settings, extractionMode) {
        return this.maybeExtractAndSavePDFText(paper, file, settings, extractionMode);
    },

    async maybeExtractAndSavePDFText(paper, file, settings, extractionMode) {
        if (!paper?.id || !file || !this.shouldAutoExtractPDFText(settings, extractionMode)) {
            return paper;
        }
        if (paper.pdf_text) {
            return paper;
        }

        this.setPDFTextSyncState(paper.id, 'running', '正在提取全文，完成后会自动保存到当前文献。');

        try {
            const pdfText = await this.extractFullTextFromFile(file);
            if (!pdfText) {
                throw new Error('没有从当前 PDF 中提取到可用全文');
            }

            const payload = await API.updatePaperPDFText(paper.id, {
                pdf_text: pdfText
            });

            this.currentPaper = payload.paper;
            this.setPDFTextSyncState(paper.id, 'success', `已保存全文（${pdfText.length.toLocaleString()} 字）。`);
            this.renderResult(payload.paper);
            Utils.showToast(`已保存全文（${pdfText.length.toLocaleString()} 字）`);
            return payload.paper;
        } catch (error) {
            this.setPDFTextSyncState(paper.id, 'error', error.message || '全文提取失败');
            Utils.showToast(error.message || '全文提取失败', 'error');
            return paper;
        }
    },

    async maybeDetectAndSaveFiguresWithLLM(paper, file) {
        if (!paper?.id || !file) {
            return paper;
        }

        this.setFigureSyncState(paper.id, 'running', '浏览器正在逐页渲染 PDF，并调用内置 AI 识别图片坐标。');

        const pdfjsLib = await this.ensurePDFJSReady();
        const objectURL = URL.createObjectURL(file);
        let detectedCount = 0;
        let currentPaper = paper;

        try {
            const loadingTask = pdfjsLib.getDocument({
                url: objectURL,
                cMapUrl: '/static/vendor/pdfjs/cmaps/',
                cMapPacked: true,
                standardFontDataUrl: '/static/vendor/pdfjs/standard_fonts/',
                wasmUrl: '/static/vendor/pdfjs/wasm/'
            });
            const pdfDocument = await loadingTask.promise;

            for (let pageNumber = 1; pageNumber <= (pdfDocument.numPages || 0); pageNumber += 1) {
                this.setFigureSyncState(
                    paper.id,
                    'running',
                    `内置 AI 正在识别第 ${pageNumber} / ${pdfDocument.numPages} 页的图片坐标。`
                );

                const page = await pdfDocument.getPage(pageNumber);
                const renderedPage = await this.renderPDFPageForDetection(page);
                const detection = await API.detectAIFigureRegions({
                    paper_id: paper.id,
                    page_number: pageNumber,
                    page_width: renderedPage.canvas.width,
                    page_height: renderedPage.canvas.height,
                    image_data: renderedPage.canvas.toDataURL('image/jpeg', 0.92)
                });

                const regions = Array.isArray(detection?.regions) ? detection.regions : [];
                if (!regions.length) {
                    continue;
                }

                const manualRegions = [];
                for (const region of regions) {
                    const imageData = this.buildRegionImageData(renderedPage.canvas, region);
                    if (!imageData) {
                        continue;
                    }
                    manualRegions.push({
                        page_number: pageNumber,
                        x: region.x,
                        y: region.y,
                        width: region.width,
                        height: region.height,
                        source: 'llm',
                        image_data: imageData,
                        caption: ''
                    });
                }

                if (!manualRegions.length) {
                    continue;
                }

                const payload = await API.manualExtractFigures(paper.id, {
                    regions: manualRegions
                });
                detectedCount += Number(payload?.added_count || manualRegions.length);
                currentPaper = payload.paper || currentPaper;
                this.currentPaper = currentPaper;
                this.renderResult(currentPaper);
            }

            if (detectedCount > 0) {
                this.setFigureSyncState(paper.id, 'success', `内置 AI 已自动录入 ${detectedCount} 张图片。`);
                Utils.showToast(`AI 已自动录入 ${detectedCount} 张图片`);
            } else {
                this.setFigureSyncState(paper.id, 'success', '内置 AI 已完成坐标识别，但没有找到可保存的主图；你仍可继续手工标注。');
            }
            return currentPaper;
        } catch (error) {
            this.setFigureSyncState(paper.id, 'error', error.message || '内置 AI 图片坐标提取失败');
            Utils.showToast(error.message || '内置 AI 图片坐标提取失败', 'error');
            return currentPaper;
        } finally {
            URL.revokeObjectURL(objectURL);
        }
    },

    async renderPDFPageForDetection(page) {
        const baseViewport = page.getViewport({ scale: 1 });
        const targetMaxDimension = 1680;
        const baseMaxDimension = Math.max(baseViewport.width, baseViewport.height, 1);
        const scale = Math.min(2.4, Math.max(1.35, targetMaxDimension / baseMaxDimension));
        const viewport = page.getViewport({ scale });
        const canvas = document.createElement('canvas');
        canvas.width = Math.max(1, Math.round(viewport.width));
        canvas.height = Math.max(1, Math.round(viewport.height));

        const context = canvas.getContext('2d', { alpha: false });
        context.fillStyle = '#ffffff';
        context.fillRect(0, 0, canvas.width, canvas.height);

        await page.render({
            canvasContext: context,
            viewport
        }).promise;

        return { canvas, viewport };
    },

    buildRegionImageData(canvas, region) {
        const left = Math.max(0, Math.floor(Number(region?.x || 0) * canvas.width));
        const top = Math.max(0, Math.floor(Number(region?.y || 0) * canvas.height));
        const right = Math.min(canvas.width, Math.ceil((Number(region?.x || 0) + Number(region?.width || 0)) * canvas.width));
        const bottom = Math.min(canvas.height, Math.ceil((Number(region?.y || 0) + Number(region?.height || 0)) * canvas.height));
        const width = right - left;
        const height = bottom - top;

        if (width < 2 || height < 2) {
            return '';
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

    async ensurePDFJSReady() {
        if (this.pdfjsLib) {
            return this.pdfjsLib;
        }

        const pdfjsLib = await import('/static/vendor/pdfjs/legacy/build/pdf.min.mjs');
        pdfjsLib.GlobalWorkerOptions.workerSrc = '/static/vendor/pdfjs/legacy/build/pdf.worker.min.mjs';
        this.pdfjsLib = pdfjsLib;
        return pdfjsLib;
    },

    async extractFullTextFromFile(file) {
        const pdfjsLib = await this.ensurePDFJSReady();
        const objectURL = URL.createObjectURL(file);

        try {
            const loadingTask = pdfjsLib.getDocument({
                url: objectURL,
                cMapUrl: '/static/vendor/pdfjs/cmaps/',
                cMapPacked: true,
                standardFontDataUrl: '/static/vendor/pdfjs/standard_fonts/',
                wasmUrl: '/static/vendor/pdfjs/wasm/'
            });
            const pdfDocument = await loadingTask.promise;
            const pages = [];

            for (let pageNumber = 1; pageNumber <= (pdfDocument.numPages || 0); pageNumber += 1) {
                const page = await pdfDocument.getPage(pageNumber);
                const textContent = await page.getTextContent();
                const pageText = this.extractPageText(textContent);
                if (pageText) {
                    pages.push(pageText);
                }
            }

            return pages.join('\n\n').trim();
        } finally {
            URL.revokeObjectURL(objectURL);
        }
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
    },

    renderResult(paper) {
        this.currentPaper = paper;
        const figures = (paper.figures || []).filter((figure) => !figure.parent_figure_id);
        const statusTone = Utils.statusTone(paper.extraction_status);
        const tags = (paper.tags || [])
            .map((tag) => `<span class="chip" style="--chip-color:${tag.color}">${Utils.escapeHTML(tag.name)}</span>`)
            .join('');

        let figureContent = '';
        if (figures.length) {
            figureContent = `
                <div class="figure-grid compact-grid">
                    ${figures.map((figure) => `
                        <figure class="figure-card">
                            <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.original_name || paper.title)}">
                            <figcaption>
                                <strong>第 ${figure.page_number || '-'} 页${figure.source === 'manual' ? ' · 人工提取' : ''}</strong>
                                <span>${Utils.escapeHTML(figure.caption || figure.original_name || '未命名图片')}</span>
                            </figcaption>
                        </figure>
                    `).join('')}
                </div>
            `;
        } else if (Utils.isProcessingStatus(paper.extraction_status)) {
            figureContent = `
                <div class="empty-state">
                    <h3>后台正在解析这篇文献</h3>
                    <p>PDF 原文、框选结果和提取图片会在解析完成后自动出现在这里。</p>
                </div>
            `;
        } else if (paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') {
            figureContent = `
                <div class="empty-state">
                    <h3>没有生成可展示的图片</h3>
                    <p>这篇文献的后台解析没有成功完成，可以先回到文献库查看错误信息。</p>
                </div>
            `;
        } else if (paper.extraction_status === 'completed' && !paper.pdf_text && !figures.length) {
            figureContent = `
                <div class="empty-state">
                    <h3>文献已入库，可按需补录图片</h3>
                    <p>${Utils.escapeHTML(paper.extractor_message || '你可以随时打开人工框选提取页，把需要的图片录入到当前文献。')}</p>
                </div>
            `;
        } else {
            figureContent = `
                <div class="empty-state">
                    <h3>暂时没有可展示的图片</h3>
                    <p>上传结果已经保存，但当前还没有提取图片。</p>
                </div>
            `;
        }

        this.resultCard.classList.remove('hidden');
        this.resultCard.innerHTML = `
            <div class="result-head">
                <div>
                    <p class="eyebrow">上传结果</p>
                    <h2>${Utils.escapeHTML(paper.title)}</h2>
                </div>
                <span class="status-pill ${statusTone}">${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}</span>
            </div>

            <div class="result-meta">
                <span>分组：${Utils.escapeHTML(paper.group_name || '未分组')}</span>
                <span>标签：${tags || '无'}</span>
                <span>提取图片：${figures.length} 张</span>
                <span>全文：${paper.pdf_text ? `${paper.pdf_text.length.toLocaleString()} 字` : '未保存'}</span>
                <span>PDF：${Utils.escapeHTML(paper.original_filename || '')}</span>
            </div>

            ${paper.extractor_message ? `<p class="notice ${statusTone}">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}
            ${this.renderFigureSyncNotice(paper)}
            ${this.renderPDFTextSyncNotice(paper)}

            <div class="result-actions">
                <a class="btn btn-primary" href="/library">查看文献库</a>
                <a class="btn btn-outline" href="${Utils.resourceViewerURL('pdf', paper.pdf_url)}">打开 PDF</a>
                <a class="btn btn-outline" href="/manual?paper_id=${paper.id}">人工框选提取</a>
                ${(paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') ? '<button class="btn btn-outline" type="button" data-action="reextract">重新解析</button>' : ''}
            </div>

            ${figureContent}
        `;
    }
};
