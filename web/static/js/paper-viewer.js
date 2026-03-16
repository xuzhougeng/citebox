const PaperViewer = {
    init() {
        this.modal = document.getElementById('paperModal');
        this.body = document.getElementById('paperModalBody');
        this.closeButton = document.getElementById('closePaperModal');
        if (!this.modal || this.initialized) return;
        this.initialized = true;

        this.handleKeydown = (event) => {
            if (!this.modal || this.modal.classList.contains('hidden')) return;
            if (event.key === 'Escape') {
                this.close();
            }
        };

        this.closeButton.addEventListener('click', () => this.close());
        this.modal.addEventListener('click', (event) => {
            if (event.target === this.modal) {
                this.close();
            }
        });
        document.addEventListener('keydown', this.handleKeydown);

        this.body.addEventListener('submit', async (event) => {
            const form = event.target.closest('#paperViewerForm');
            if (!form) return;
            event.preventDefault();
            await this.save();
        });

        this.body.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-modal-action]');
            if (!button) return;
            if (button.dataset.modalAction === 'reextract-paper') {
                await this.reextract();
            }
            if (button.dataset.modalAction === 'delete-paper') {
                await this.remove();
            }
        });
    },

    renderTagChips(tags = []) {
        if (!tags.length) {
            return '<span class="muted">无标签</span>';
        }
        return tags.map((tag) => `<span class="chip" style="--chip-color:${tag.color}">${Utils.escapeHTML(tag.name)}</span>`).join('');
    },

    async open(id, onChanged) {
        this.init();
        this.onChanged = onChanged;
        try {
            const [paper, groupsPayload] = await Promise.all([API.getPaper(id), API.listGroups()]);
            this.paper = paper;
            this.groups = groupsPayload.groups || [];
            this.render();
            this.modal.classList.remove('hidden');
            document.body.classList.add('modal-open');
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    close() {
        if (!this.modal) return;
        this.modal.classList.add('hidden');
        document.body.classList.remove('modal-open');
    },

    render() {
        const paper = this.paper;
        const groupOptions = ['<option value="">未分组</option>']
            .concat(this.groups.map((group) => `
                <option value="${group.id}" ${String(group.id) === String(paper.group_id || '') ? 'selected' : ''}>
                    ${Utils.escapeHTML(group.name)}
                </option>
            `))
            .join('');
        const figures = paper.figures || [];
        const boxes = paper.boxes ? JSON.stringify(paper.boxes, null, 2) : '';
        const statusClass = Utils.statusTone(paper.extraction_status);
        const figureSection = figures.length ? figures.map((figure) => `
            <article class="figure-preview-card figure-detail-card">
                <div class="figure-preview-stage">
                    <a class="figure-preview-media" href="${figure.image_url}" target="_blank" rel="noreferrer" aria-label="打开原图">
                        <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.original_name || paper.title)}">
                    </a>
                    <div class="figure-preview-badges">
                        <span class="figure-badge figure-badge-strong">第 ${figure.page_number || '-'} 页</span>
                        <span class="figure-badge">#${figure.figure_index || '-'}</span>
                    </div>
                </div>
                <div class="figure-preview-body">
                    <div class="figure-preview-head">
                        <span class="figure-preview-label">来源文献</span>
                        <strong class="figure-preview-title">${Utils.escapeHTML(paper.title)}</strong>
                    </div>
                    <div class="figure-preview-tags ${paper.tags?.length ? '' : 'is-empty'}">
                        ${paper.tags?.length ? this.renderTagChips(paper.tags || []) : '<span class="figure-preview-empty">无标签</span>'}
                    </div>
                    <div class="card-actions">
                        <a class="btn btn-outline" href="${figure.image_url}" target="_blank" rel="noreferrer">原图</a>
                    </div>
                </div>
            </article>
        `).join('') : `<p class="muted">${Utils.isProcessingStatus(paper.extraction_status) ? '后台解析完成后会在这里显示提取图片。' : '没有可展示的提取图片。'}</p>`;

        this.body.innerHTML = `
            <div class="detail-head">
                <div>
                    <p class="eyebrow">文献详情</p>
                    <h2>${Utils.escapeHTML(paper.title)}</h2>
                </div>
                <span class="status-pill ${statusClass}">
                    ${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}
                </span>
            </div>

            <form id="paperViewerForm" class="detail-form">
                <div class="form-grid detail-form-grid">
                    <label class="field field-span-2">
                        <span>标题</span>
                        <input id="paperViewerTitle" class="form-input" type="text" value="${Utils.escapeHTML(paper.title)}">
                    </label>
                    <label class="field">
                        <span>分组</span>
                        <select id="paperViewerGroup" class="form-input">${groupOptions}</select>
                    </label>
                    <label class="field">
                        <span>标签</span>
                        <input id="paperViewerTags" class="form-input" type="text" value="${Utils.escapeHTML(Utils.joinTags(paper.tags || []))}" placeholder="逗号分隔">
                    </label>
                    <label class="field field-span-2">
                        <span>摘要</span>
                        <textarea id="paperViewerAbstract" class="form-textarea" rows="4" placeholder="为这篇文献补充摘要或核心结论">${Utils.escapeHTML(paper.abstract_text || '')}</textarea>
                    </label>
                    <label class="field field-span-2">
                        <span>备注</span>
                        <textarea id="paperViewerNotes" class="form-textarea" rows="5" placeholder="记录你的整理备注、阅读结论或迁移补充信息">${Utils.escapeHTML(paper.notes_text || '')}</textarea>
                    </label>
                </div>
                <div class="detail-actions">
                    <button class="btn btn-primary" type="submit">保存</button>
                    ${(paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') ? '<button class="btn btn-outline" type="button" data-modal-action="reextract-paper">重新解析</button>' : ''}
                    <button class="btn btn-outline danger" type="button" data-modal-action="delete-paper">删除文献</button>
                    <a class="btn btn-outline" href="/ai?paper_id=${paper.id}" target="_blank" rel="noreferrer">AI 阅读</a>
                    <a class="btn btn-outline" href="${paper.pdf_url}" target="_blank" rel="noreferrer">打开 PDF</a>
                </div>
            </form>

            <div class="detail-meta-panel">
                <div><span>原始文件</span><strong>${Utils.escapeHTML(paper.original_filename)}</strong></div>
                <div><span>PDF 大小</span><strong>${Utils.formatFileSize(paper.file_size || 0)}</strong></div>
                <div><span>提取图片</span><strong>${figures.length}</strong></div>
                <div><span>当前标签</span><strong>${this.renderTagChips(paper.tags || [])}</strong></div>
                <div><span>最近更新</span><strong>${Utils.formatDate(paper.updated_at || paper.created_at)}</strong></div>
            </div>

            ${paper.extractor_message ? `<p class="notice ${statusClass}">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}

            <section class="detail-section">
                <div class="section-head">
                    <h3>提取图片</h3>
                    <span>${figures.length} 张</span>
                </div>
                <div class="figure-preview-grid detail-figure-grid">
                    ${figureSection}
                </div>
            </section>

            <section class="detail-section">
                <div class="section-head">
                    <h3>框选结果</h3>
                </div>
                <pre class="code-block">${Utils.escapeHTML(boxes || '暂无框选结果')}</pre>
            </section>

            <section class="detail-section">
                <div class="section-head">
                    <h3>PDF 原文</h3>
                </div>
                <pre class="text-block">${Utils.escapeHTML(paper.pdf_text || '暂无 PDF 原文')}</pre>
            </section>
        `;
    },

    async save() {
        try {
            const payload = await API.updatePaper(this.paper.id, {
                title: document.getElementById('paperViewerTitle').value.trim(),
                abstract_text: document.getElementById('paperViewerAbstract').value.trim(),
                notes_text: document.getElementById('paperViewerNotes').value.trim(),
                group_id: document.getElementById('paperViewerGroup').value ? Number(document.getElementById('paperViewerGroup').value) : null,
                tags: Utils.splitTags(document.getElementById('paperViewerTags').value)
            });
            this.paper = payload.paper;
            Utils.showToast('文献信息已更新');
            this.render();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async remove() {
        const confirmed = await Utils.confirm('删除后会移除 PDF、提取图片以及相关关联。');
        if (!confirmed) return;

        try {
            await API.deletePaper(this.paper.id);
            Utils.showToast('文献已删除');
            this.close();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async reextract() {
        try {
            const payload = await API.reextractPaper(this.paper.id);
            this.paper = payload.paper;
            Utils.showToast('文献已重新提交解析', 'info');
            this.render();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    }
};
