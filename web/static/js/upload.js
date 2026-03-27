const UploadPage = {
    file: null,
    sourceMode: 'file',
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
        if (typeof t !== 'function') { window.t = function(k, f) { return f || k; }; }
        this.form = document.getElementById('paperUploadForm');
        if (!this.form) return;

        this.pdfInput = document.getElementById('pdfInput');
        this.dropArea = document.getElementById('dropArea');
        this.fileSourceSection = document.getElementById('fileSourceSection');
        this.doiSourceSection = document.getElementById('doiSourceSection');
        this.sourceModeHint = document.getElementById('sourceModeHint');
        this.sourceModeButtons = Array.from(document.querySelectorAll('[data-upload-source-mode]'));
        this.selectedFile = document.getElementById('selectedFile');
        this.doiInput = document.getElementById('doiInput');
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
        this.setSourceMode(this.sourceMode);
        await Promise.all([
            this.loadGroups(),
            this.loadExtractionModeOptions()
        ]);
    },

    bindEvents() {
        this.sourceModeButtons.forEach((button) => {
            button.addEventListener('click', () => this.setSourceMode(button.dataset.uploadSourceMode));
        });

        this.dropArea?.addEventListener('click', () => this.pdfInput.click());
        this.dropArea?.addEventListener('keydown', (event) => {
            if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                this.pdfInput.click();
            }
        });

        ['dragenter', 'dragover', 'dragleave', 'drop'].forEach((eventName) => {
            this.dropArea?.addEventListener(eventName, (event) => {
                event.preventDefault();
                event.stopPropagation();
            });
        });

        ['dragenter', 'dragover'].forEach((eventName) => {
            this.dropArea?.addEventListener(eventName, () => {
                this.dropArea.classList.add('dragover');
            });
        });

        ['dragleave', 'drop'].forEach((eventName) => {
            this.dropArea?.addEventListener(eventName, () => {
                this.dropArea.classList.remove('dragover');
            });
        });

        this.dropArea?.addEventListener('drop', (event) => {
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

    setSourceMode(mode) {
        this.sourceMode = mode === 'doi' ? 'doi' : 'file';
        this.fileSourceSection?.classList.toggle('hidden', this.sourceMode !== 'file');
        this.doiSourceSection?.classList.toggle('hidden', this.sourceMode !== 'doi');

        this.sourceModeButtons.forEach((button) => {
            const active = button.dataset.uploadSourceMode === this.sourceMode;
            button.classList.toggle('btn-primary', active);
            button.classList.toggle('btn-outline', !active);
            button.setAttribute('aria-pressed', active ? 'true' : 'false');
        });

        if (this.sourceModeHint) {
            this.sourceModeHint.textContent = this.sourceMode === 'doi'
                ? t('upload.source_mode_hint_doi', '输入 DOI 后，系统会优先从 Open Access 来源自动检索并导入 PDF。')
                : t('upload.source_mode_hint_file', '从本地选择 PDF 文件，上传后按当前解析配置入库。');
        }

        if (this.submitButton) {
            this.submitButton.textContent = this.sourceMode === 'doi'
                ? t('upload.btn_import_doi', '查找并导入 DOI')
                : t('upload.btn_upload', '上传文献');
        }
    },

    async loadGroups() {
        try {
            const payload = await API.listGroups();
            const groups = payload.groups || [];
            this.groupSelect.innerHTML = `<option value="">${t('upload.no_group', '暂不分组')}</option>`;
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
            Utils.showToast(t('upload.err_load_extractor', '加载自动解析配置失败，当前仅允许手工标注'), 'error');
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
            this.extractionModeSelect.innerHTML = `<option value="manual">${t('upload.mode_manual', '手工')}</option>`;
            this.extractionModeSelect.value = 'manual';
            this.extractionModeSelect.disabled = true;
        } else {
            this.extractionModeSelect.innerHTML = `
                <option value="auto" ${this.extractorReady ? '' : 'disabled'}>${t('upload.mode_auto', '自动标注')}</option>
                <option value="manual">${t('upload.mode_manual', '手工标注')}</option>
            `;
            this.extractionModeSelect.value = this.extractorReady ? 'auto' : 'manual';
            this.extractionModeSelect.disabled = false;
        }

        if (this.extractionModeHint) {
            if (usesManualProfile) {
                this.extractionModeHint.textContent = t('upload.mode_hint_manual_profile', '当前 PDF 提取方案为手工：上传后不会自动提图，但会自动提取并保存全文；微信上传也同样如此。');
            } else if (this.usesBuiltInLLMExtraction(this.extractorSettings) && this.extractorReady) {
                this.extractionModeHint.textContent = t('upload.mode_hint_builtin_ready', '默认使用自动标注；上传后后台会用内置 AI 解析图片坐标，全文也会自动保存。');
            } else if (this.usesBuiltInLLMExtraction(this.extractorSettings)) {
                this.extractionModeHint.textContent = t('upload.mode_hint_builtin_not_ready', '当前已选择内置 AI 坐标提取，但图片场景模型或 API Key 还没配好，只能使用手工标注；上传后会自动保存全文。');
            } else if (this.extractorReady && this.usesBrowserPDFText(this.extractorSettings)) {
                this.extractionModeHint.textContent = t('upload.mode_hint_browser_text', '默认自动标注，系统会自动提取图片并保存全文。');
            } else if (this.extractorReady) {
                this.extractionModeHint.textContent = t('upload.mode_hint_default', '默认使用自动标注；也可以切到手工标注自行框选图片。');
            } else {
                this.extractionModeHint.textContent = t('upload.mode_hint_not_configured', '尚未配置自动解析，仅支持手工标注。上传后会自动保存全文。');
            }
        }
    },

    setFile(file) {
        if (!file) return;
        const isPDF = file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf');
        if (!isPDF) {
            Utils.showToast(t('upload.err_not_pdf', '请选择 PDF 文件'), 'error');
            return;
        }

        this.setSourceMode('file');
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
        if (this.sourceMode === 'doi') {
            await this.submitDOIImport();
            return;
        }
        await this.submitFileUpload();
    },

    async submitFileUpload() {
        if (!this.file) {
            Utils.showToast(t('upload.err_no_file', '请先选择 PDF'), 'error');
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
        this.submitButton.textContent = t('upload.btn_uploading', '上传中...');

        try {
            const payload = await API.uploadPaper(formData);
            const paper = payload.paper;
            this.lastUploadedFile = sourceFile;
            this.tagAutocomplete?.mergeTags(paper?.tags || Utils.splitTags(this.tagsInput.value.trim()));

            this.renderResult(paper);
            this.startPolling(paper);
            void this.runPostUploadEnrichment(paper, sourceFile, extractorSettings, extractionMode);

            let toastMessage = t('upload.toast_uploaded', '文献已入库');
            if (Utils.isProcessingStatus(paper.extraction_status)) {
                toastMessage = t('upload.toast_uploaded_processing', '文献已入库，后台开始解析');
            }
            Utils.showToast(toastMessage, Utils.statusTone(paper.extraction_status));

            this.form.reset();
            this.file = null;
            this.selectedFile.classList.add('empty');
            this.selectedFile.innerHTML = `<span>${t('upload.no_file_selected', '尚未选择 PDF')}</span>`;
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
            this.submitButton.textContent = this.sourceMode === 'doi'
                ? t('upload.btn_import_doi', '查找并导入 DOI')
                : t('upload.btn_upload', '上传文献');
        }
    },

    async submitDOIImport() {
        const doi = this.doiInput?.value?.trim() || '';
        if (!doi) {
            Utils.showToast(t('upload.err_no_doi', '请先输入 DOI'), 'error');
            return;
        }

        const extractorSettings = { ...(this.extractorSettings || {}) };
        const extractionMode = this.extractionModeSelect?.value || 'manual';
        const payload = {
            doi,
            title: this.titleInput.value.trim(),
            tags: Utils.splitTags(this.tagsInput.value.trim()),
            extraction_mode: extractionMode
        };
        if (this.groupSelect.value) {
            payload.group_id = Number(this.groupSelect.value);
        }

        this.submitButton.disabled = true;
        this.submitButton.textContent = t('upload.btn_importing_doi', '检索中...');

        try {
            const response = await API.importPaperByDOI(payload);
            const paper = response.paper;
            this.lastUploadedFile = null;
            this.tagAutocomplete?.mergeTags(paper?.tags || Utils.splitTags(this.tagsInput.value.trim()));

            this.renderResult(paper);
            this.startPolling(paper);

            let toastMessage = t('upload.toast_imported_doi', 'DOI 文献已入库');
            if (Utils.isProcessingStatus(paper.extraction_status)) {
                toastMessage = t('upload.toast_imported_doi_processing', 'DOI 文献已入库，后台开始解析');
            }
            Utils.showToast(toastMessage, Utils.statusTone(paper.extraction_status));

            this.form.reset();
            this.file = null;
            this.selectedFile.classList.add('empty');
            this.selectedFile.innerHTML = `<span>${t('upload.no_file_selected', '尚未选择 PDF')}</span>`;
            this.tagAutocomplete?.refresh?.();
            await Promise.all([
                this.loadGroups(),
                this.loadExtractionModeOptions()
            ]);
            this.setSourceMode('doi');
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
            this.submitButton.textContent = this.sourceMode === 'doi'
                ? t('upload.btn_import_doi', '查找并导入 DOI')
                : t('upload.btn_upload', '上传文献');
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
                    const message = this.lastStatus === 'completed' ? t('upload.toast_parse_done', '文献解析完成') : t('upload.toast_parse_ended', '文献解析已结束');
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
            Utils.showToast(t('upload.toast_reextract', '文献已重新提交解析'), 'info');
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

        this.setPDFTextSyncState(paper.id, 'running', t('upload.sync_extracting_text', '正在提取全文，完成后会自动保存到当前文献。'));

        try {
            const pdfText = await this.extractFullTextFromFile(file);
            if (!pdfText) {
                throw new Error(t('upload.sync_no_text', '没有从当前 PDF 中提取到可用全文'));
            }

            const payload = await API.updatePaperPDFText(paper.id, {
                pdf_text: pdfText
            });

            this.currentPaper = payload.paper;
            this.setPDFTextSyncState(paper.id, 'success', t('upload.sync_text_saved', '已保存全文（{0} 字）。').replace('{0}', pdfText.length.toLocaleString()));
            this.renderResult(payload.paper);
            Utils.showToast(t('upload.sync_text_toast', '已保存全文（{0} 字）').replace('{0}', pdfText.length.toLocaleString()));
            return payload.paper;
        } catch (error) {
            this.setPDFTextSyncState(paper.id, 'error', error.message || t('upload.sync_text_failed', '全文提取失败'));
            Utils.showToast(error.message || t('upload.sync_text_failed', '全文提取失败'), 'error');
            return paper;
        }
    },

    async maybeDetectAndSaveFiguresWithLLM(paper, file) {
        if (!paper?.id || !file) {
            return paper;
        }

        this.setFigureSyncState(paper.id, 'running', t('upload.sync_figure_rendering', '浏览器正在逐页渲染 PDF，并调用内置 AI 识别图片坐标。'));

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
                    t('upload.sync_figure_page', '内置 AI 正在识别第 {0} / {1} 页的图片坐标。').replace('{0}', pageNumber).replace('{1}', pdfDocument.numPages)
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
                this.setFigureSyncState(paper.id, 'success', t('upload.sync_figure_done', '内置 AI 已自动录入 {0} 张图片。').replace('{0}', detectedCount));
                Utils.showToast(t('upload.sync_figure_toast', 'AI 已自动录入 {0} 张图片').replace('{0}', detectedCount));
            } else {
                this.setFigureSyncState(paper.id, 'success', t('upload.sync_figure_none', '内置 AI 已完成坐标识别，但没有找到可保存的主图；你仍可继续手工标注。'));
            }
            return currentPaper;
        } catch (error) {
            this.setFigureSyncState(paper.id, 'error', error.message || t('upload.sync_figure_error', '内置 AI 图片坐标提取失败'));
            Utils.showToast(error.message || t('upload.sync_figure_error', '内置 AI 图片坐标提取失败'), 'error');
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
                                <strong>${t('upload.result_page', '第 {0} 页').replace('{0}', figure.page_number || '-')}${figure.source === 'manual' ? t('upload.result_manual_source', ' · 人工提取') : ''}</strong>
                                <span>${Utils.escapeHTML(figure.caption || figure.original_name || t('upload.result_unnamed_figure', '未命名图片'))}</span>
                            </figcaption>
                        </figure>
                    `).join('')}
                </div>
            `;
        } else if (Utils.isProcessingStatus(paper.extraction_status)) {
            figureContent = `
                <div class="empty-state">
                    <h3>${t('upload.result_processing_title', '后台正在解析这篇文献')}</h3>
                    <p>${t('upload.result_processing_text', 'PDF 原文、框选结果和提取图片会在解析完成后自动出现在这里。')}</p>
                </div>
            `;
        } else if (paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') {
            figureContent = `
                <div class="empty-state">
                    <h3>${t('upload.result_failed_title', '没有生成可展示的图片')}</h3>
                    <p>${t('upload.result_failed_text', '这篇文献的后台解析没有成功完成，可以先回到文献库查看错误信息。')}</p>
                </div>
            `;
        } else if (paper.extraction_status === 'completed' && !paper.pdf_text && !figures.length) {
            figureContent = `
                <div class="empty-state">
                    <h3>${t('upload.result_completed_no_fig_title', '文献已入库，可按需补录图片')}</h3>
                    <p>${Utils.escapeHTML(paper.extractor_message || t('upload.result_completed_no_fig_text', '你可以随时打开人工框选提取页，把需要的图片录入到当前文献。'))}</p>
                </div>
            `;
        } else {
            figureContent = `
                <div class="empty-state">
                    <h3>${t('upload.result_empty_title', '暂时没有可展示的图片')}</h3>
                    <p>${t('upload.result_empty_text', '上传结果已经保存，但当前还没有提取图片。')}</p>
                </div>
            `;
        }

        this.resultCard.classList.remove('hidden');
        this.resultCard.innerHTML = `
            <div class="result-head">
                <div>
                    <p class="eyebrow">${t('upload.result_eyebrow', '上传结果')}</p>
                    <h2>${Utils.escapeHTML(paper.title)}</h2>
                </div>
                <span class="status-pill ${statusTone}">${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}</span>
            </div>

            <div class="result-meta">
                <span>${t('upload.result_group', '分组：')}${Utils.escapeHTML(paper.group_name || t('upload.result_no_group', '未分组'))}</span>
                <span>${t('upload.result_tags', '标签：')}${tags || t('upload.result_no_tags', '无')}</span>
                <span>${t('upload.result_figures', '提取图片：')}${figures.length}${t('upload.result_figures_unit', ' 张')}</span>
                <span>${t('upload.result_fulltext', '全文：')}${paper.pdf_text ? `${paper.pdf_text.length.toLocaleString()}${t('upload.result_fulltext_chars', ' 字')}` : t('upload.result_fulltext_none', '未保存')}</span>
                ${paper.doi ? `<span>${t('upload.result_doi', 'DOI：')}${Utils.escapeHTML(paper.doi)}</span>` : ''}
                <span>${t('upload.result_pdf', 'PDF：')}${Utils.escapeHTML(paper.original_filename || '')}</span>
            </div>

            ${paper.extractor_message ? `<p class="notice ${statusTone}">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}
            ${this.renderFigureSyncNotice(paper)}
            ${this.renderPDFTextSyncNotice(paper)}

            <div class="result-actions">
                <a class="btn btn-primary" href="/library">${t('upload.result_view_library', '查看文献库')}</a>
                <a class="btn btn-outline" href="${Utils.resourceViewerURL('pdf', paper.pdf_url)}">${t('upload.result_open_pdf', '打开 PDF')}</a>
                <a class="btn btn-outline" href="/manual?paper_id=${paper.id}">${t('upload.result_manual_extract', '人工框选提取')}</a>
                ${(paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') ? `<button class="btn btn-outline" type="button" data-action="reextract">${t('upload.result_reextract', '重新解析')}</button>` : ''}
            </div>

            ${figureContent}
        `;
    }
};
