const UploadPage = {
    file: null,
    pollTimer: null,
    activePaperId: null,
    pollInFlight: false,
    lastStatus: '',

    async init() {
        this.form = document.getElementById('paperUploadForm');
        if (!this.form) return;

        this.pdfInput = document.getElementById('pdfInput');
        this.dropArea = document.getElementById('dropArea');
        this.selectedFile = document.getElementById('selectedFile');
        this.titleInput = document.getElementById('titleInput');
        this.groupSelect = document.getElementById('groupSelect');
        this.tagsInput = document.getElementById('tagsInput');
        this.submitButton = document.getElementById('submitUploadBtn');
        this.resultCard = document.getElementById('uploadResult');

        this.bindEvents();
        await this.loadGroups();
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

        const formData = new FormData();
        formData.append('pdf', this.file);
        formData.append('title', this.titleInput.value.trim());
        formData.append('tags', this.tagsInput.value.trim());
        if (this.groupSelect.value) {
            formData.append('group_id', this.groupSelect.value);
        }

        this.submitButton.disabled = true;
        this.submitButton.textContent = '上传中...';

        try {
            const payload = await API.uploadPaper(formData);
            const paper = payload.paper;

            this.renderResult(paper);
            this.startPolling(paper);

            Utils.showToast(
                Utils.isProcessingStatus(paper.extraction_status)
                    ? '文献已入库，后台开始解析'
                    : (paper.extraction_status === 'manual_pending' ? '文献已入库，请继续手动标注' : '文献已入库'),
                Utils.statusTone(paper.extraction_status)
            );

            this.form.reset();
            this.file = null;
            this.selectedFile.classList.add('empty');
            this.selectedFile.innerHTML = '<span>尚未选择 PDF</span>';
            await this.loadGroups();
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

    renderResult(paper) {
        const figures = paper.figures || [];
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
        } else if (paper.extraction_status === 'manual_pending') {
            figureContent = `
                <div class="empty-state">
                    <h3>当前走手动标注流程</h3>
                    <p>系统没有启动自动解析，你可以直接打开人工框选提取页，把需要的图片录入到文献里。</p>
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
                <span>PDF：${Utils.escapeHTML(paper.original_filename || '')}</span>
            </div>

            ${paper.extractor_message ? `<p class="notice ${statusTone}">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}

            <div class="result-actions">
                <a class="btn btn-primary" href="/library">查看文献库</a>
                <a class="btn btn-outline" href="${paper.pdf_url}" target="_blank" rel="noreferrer">打开 PDF</a>
                <a class="btn btn-outline" href="/manual?paper_id=${paper.id}">人工框选提取</a>
                ${(paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') ? '<button class="btn btn-outline" type="button" data-action="reextract">重新解析</button>' : ''}
            </div>

            ${figureContent}
        `;
    }
};
