if (typeof window.t !== 'function') window.t = function(k,f){return f||k};
const PaperNoteViewer = {
    init() {
        this.modal = document.getElementById('paperNoteModal');
        this.body = document.getElementById('paperNoteModalBody');
        this.closeButton = document.getElementById('closePaperNoteModal');
        if (!this.modal) {
            const shell = document.createElement('div');
            shell.id = 'paperNoteModal';
            shell.className = 'modal-shell hidden';
            shell.innerHTML = `
                <div class="modal-dialog figure-modal-dialog note-modal-dialog">
                    <button id="closePaperNoteModal" class="modal-close" type="button" aria-label="${t('shared.paper.close', '关闭')}">×</button>
                    <div id="paperNoteModalBody"></div>
                </div>
            `;
            document.body.appendChild(shell);
            this.modal = shell;
            this.body = shell.querySelector('#paperNoteModalBody');
            this.closeButton = shell.querySelector('#closePaperNoteModal');
        }
        if (!this.modal || this.initialized) return;
        this.initialized = true;

        this.handleKeydown = (event) => {
            if (!this.modal || this.modal.classList.contains('hidden')) return;
            if (event.defaultPrevented) return;
            if (typeof Utils !== 'undefined' && typeof Utils.isTopVisibleModal === 'function' && !Utils.isTopVisibleModal(this.modal)) {
                return;
            }
            const target = event.target;
            const isEditableTarget = target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName));
            if (event.key === 'Escape') {
                event.preventDefault();
                event.stopPropagation();
                this.close();
                return;
            }
            if (isEditableTarget) return;
        };

        this.closeButton.addEventListener('click', () => this.close());
        this.modal.addEventListener('click', (event) => {
            if (event.target === this.modal) {
                this.close();
            }
        });
        this.body.addEventListener('input', (event) => {
            const notesInput = event.target.closest('#paperNoteViewerInput');
            if (!notesInput) return;
            this.noteDraft = notesInput.value;
        });
        this.body.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-paper-note-action]');
            if (!button) return;

            if (button.dataset.paperNoteAction === 'set-mode') {
                this.noteMode = button.dataset.noteMode === 'preview' ? 'preview' : 'write';
                this.render();
                return;
            }
            if (button.dataset.paperNoteAction === 'save-notes') {
                await this.saveNotes();
                return;
            }
            if (button.dataset.paperNoteAction === 'save-wolai') {
                await this.saveNotesToWolai(button);
                return;
            }
            if (button.dataset.paperNoteAction === 'open-ai') {
                window.location.href = `/ai?paper_id=${this.paper?.id || ''}`;
                return;
            }
            if (button.dataset.paperNoteAction === 'open-paper') {
                this.close();
                if (typeof this.onOpenPaper === 'function' && this.paper?.id) {
                    await this.onOpenPaper(this.paper.id);
                }
            }
        });
        this.body.addEventListener('keydown', async (event) => {
            const notesInput = event.target.closest('#paperNoteViewerInput');
            if (!notesInput || event.key !== 'Enter' || (!event.metaKey && !event.ctrlKey)) return;
            event.preventDefault();
            await this.saveNotes();
        });
        document.addEventListener('keydown', this.handleKeydown);
    },

    open(options = {}) {
        this.init();
        this.paper = options.paper || null;
        this.onChanged = options.onChanged;
        this.onOpenPaper = options.onOpenPaper;
        this.noteDraft = this.paper?.paper_notes_text || '';
        this.noteMode = this.noteDraft.trim() ? 'preview' : 'write';
        this.render();
        this.modal.classList.remove('hidden');
        document.body.classList.add('modal-open');
    },

    close() {
        if (!this.modal) return;
        this.modal.classList.add('hidden');
        if (!document.querySelector('.modal-shell:not(.hidden)')) {
            document.body.classList.remove('modal-open');
        }
    },

    currentNotesDraft() {
        return this.body.querySelector('#paperNoteViewerInput')?.value ?? this.noteDraft ?? (this.paper?.paper_notes_text || '');
    },

    async saveNotes() {
        if (!this.paper) return;

        try {
            const payload = await API.updatePaper(
                this.paper.id,
                PaperViewer.buildUpdatePayload(this.paper, {
                    paper_notes_text: this.currentNotesDraft()
                })
            );
            this.paper = payload.paper;
            this.noteDraft = this.paper.paper_notes_text || '';
            if (!this.noteMode || !this.noteDraft.trim()) {
                this.noteMode = 'write';
            }
            Utils.showToast(t('shared.paper.note_saved', '文献笔记已保存'));
            this.render();
            if (typeof this.onChanged === 'function') {
                await this.onChanged(payload.paper);
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async saveNotesToWolai(button) {
        if (!this.paper) return;

        const actionButton = button instanceof HTMLElement ? button : null;
        const originalLabel = actionButton?.textContent || t('shared.paper.save_to_wolai', '保存到 Wolai');
        if (actionButton) {
            actionButton.disabled = true;
            actionButton.textContent = t('shared.paper.saving', '保存中...');
        }

        try {
            const result = await API.savePaperNoteToWolai(this.paper.id, {
                notes_text: this.currentNotesDraft()
            });
            Utils.showToast(result.message || t('shared.paper.note_saved_to_wolai', '文献笔记已保存到 Wolai'));
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            if (actionButton) {
                actionButton.disabled = false;
                actionButton.textContent = originalLabel;
            }
        }
    },

    render() {
        const paper = this.paper;
        if (!paper) {
            this.body.innerHTML = `<div class="empty-state"><h3>${t('shared.paper.no_notes_to_show', '没有可展示的文献笔记')}</h3></div>`;
            return;
        }

        const noteText = this.currentNotesDraft();
        const isPreviewMode = this.noteMode === 'preview';
        const tags = PaperViewer.renderTagChips(paper.tags || []);
        const managementNotePreview = String(paper.notes_text || '').trim();

        this.body.innerHTML = `
            <div class="note-lightbox">
                <section class="note-lightbox-main">
                    <div class="note-lightbox-editor-card">
                        <div class="note-lightbox-head">
                            <div class="note-lightbox-head-row">
                                <div>
                                    <p class="eyebrow">Paper Notes</p>
                                    <h2>${Utils.escapeHTML(paper.title)}</h2>
                                    <p class="note-lightbox-subtitle">${Utils.escapeHTML(paper.original_filename || '')}</p>
                                </div>
                                <div class="note-lightbox-mode-switch">
                                    <button class="btn ${isPreviewMode ? 'btn-outline' : 'btn-primary'}" type="button" data-paper-note-action="set-mode" data-note-mode="write">${t('shared.paper.write_mode', '编辑')}</button>
                                    <button class="btn ${isPreviewMode ? 'btn-primary' : 'btn-outline'}" type="button" data-paper-note-action="set-mode" data-note-mode="preview">${t('shared.paper.markdown_preview', 'Markdown 预览')}</button>
                                </div>
                            </div>
                        </div>

                        <div class="note-lightbox-meta-grid">
                            <div class="note-lightbox-meta-item">
                                <span>${t("shared.paper.current_group", "当前分组")}</span>
                                <strong>${Utils.escapeHTML(paper.group_name || t("shared.paper.ungrouped", "未分组"))}</strong>
                            </div>
                            <div class="note-lightbox-meta-item">
                                <span>${t("shared.paper.paper_tags", "文献标签")}</span>
                                <div class="figure-preview-tags ${paper.tags?.length ? '' : 'is-empty'}">
                                    ${paper.tags?.length ? tags : `<span class="figure-preview-empty">${t('shared.paper.no_tags', '无标签')}</span>`}
                                </div>
                            </div>
                        </div>

                        ${isPreviewMode ? `
                            <section class="note-lightbox-render-panel">
                                <span class="note-lightbox-panel-label">${t('shared.paper.markdown_preview', 'Markdown 预览')}</span>
                                <div class="markdown-preview">${Utils.renderMarkdown(noteText)}</div>
                            </section>
                        ` : `
                            <label class="field note-lightbox-field">
                                <span>${t("shared.paper.paper_notes_label", "文献笔记")}</span>
                                <textarea id="paperNoteViewerInput" class="form-textarea note-lightbox-textarea" rows="16" placeholder="${t('shared.paper.paper_notes_placeholder', '记录这篇文献的 AI 解读、阅读结论、方法摘要或后续行动')}">${Utils.escapeHTML(noteText)}</textarea>
                            </label>
                        `}

                        <div class="figure-notes-actions">
                            <span class="muted">${isPreviewMode ? t('shared.paper.preview_hint', '预览基于当前草稿渲染；切回编辑可继续修改。') : t('shared.paper.save_hint', '支持多行内容，按 Ctrl/Cmd + Enter 可快速保存。')}</span>
                            <div style="display:flex;gap:0.6rem;flex-wrap:wrap">
                                <button class="btn btn-outline" type="button" data-paper-note-action="save-wolai">${t("shared.paper.save_to_wolai", "保存到 Wolai")}</button>
                                <button class="btn btn-primary" type="button" data-paper-note-action="save-notes">${t("shared.paper.save_paper_notes", "保存文献笔记")}</button>
                            </div>
                        </div>
                    </div>
                </section>

                <aside class="note-lightbox-side">
                    <div class="note-lightbox-preview-card">
                        <div class="detail-meta-panel paper-note-meta-panel">
                            <div><span>${t("shared.paper.recent_update", "最近更新")}</span><strong>${Utils.escapeHTML(Utils.formatDate(paper.updated_at || paper.created_at))}</strong></div>
                            <div><span>${t("shared.paper.extracted_figures", "提取图片")}</span><strong>${paper.figure_count || 0}</strong></div>
                            <div><span>${t("shared.paper.abstract_label", "摘要")}</span><strong>${Utils.escapeHTML(paper.abstract_text || t('shared.paper.no_abstract', '暂无摘要'))}</strong></div>
                        </div>
                    </div>

                    <div class="note-lightbox-preview-card">
                        <div class="note-lightbox-tip">
                            <span>${t("shared.paper.management_notes", "管理笔记")}</span>
                            <p>${Utils.escapeHTML(managementNotePreview || t('shared.paper.no_management_notes', '当前没有管理笔记。'))}</p>
                        </div>
                    </div>

                    <div class="note-lightbox-actions">
                        <button class="btn btn-outline" type="button" data-paper-note-action="open-paper">${t("shared.paper.view_paper_detail", "查看文献详情")}</button>
                        <button class="btn btn-outline" type="button" data-paper-note-action="open-ai">${t("shared.paper.go_ai_reading", "去 AI伴读")}</button>
                        <a class="btn btn-outline" href="${Utils.resourceViewerURL('pdf', paper.pdf_url)}">${t('shared.paper.open_pdf', '打开 PDF')}</a>
                    </div>
                </aside>
            </div>
        `;
    }
};

const PaperPDFTextViewer = {
    init() {
        this.modal = document.getElementById('paperPdfTextModal');
        this.body = document.getElementById('paperPdfTextModalBody');
        this.closeButton = document.getElementById('closePaperPdfTextModal');
        if (!this.modal) {
            const shell = document.createElement('div');
            shell.id = 'paperPdfTextModal';
            shell.className = 'modal-shell hidden';
            shell.innerHTML = `
                <div class="modal-dialog figure-modal-dialog note-modal-dialog pdf-text-modal-dialog">
                    <button id="closePaperPdfTextModal" class="modal-close" type="button" aria-label="${t('shared.paper.close', '关闭')}">×</button>
                    <div id="paperPdfTextModalBody"></div>
                </div>
            `;
            document.body.appendChild(shell);
            this.modal = shell;
            this.body = shell.querySelector('#paperPdfTextModalBody');
            this.closeButton = shell.querySelector('#closePaperPdfTextModal');
        }
        if (!this.modal || this.initialized) return;
        this.initialized = true;

        this.handleKeydown = (event) => {
            if (!this.modal || this.modal.classList.contains('hidden')) return;
            if (event.defaultPrevented) return;
            if (typeof Utils !== 'undefined' && typeof Utils.isTopVisibleModal === 'function' && !Utils.isTopVisibleModal(this.modal)) {
                return;
            }
            const target = event.target;
            const isEditableTarget = target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName));
            if (event.key === 'Escape') {
                event.preventDefault();
                event.stopPropagation();
                this.close();
                return;
            }
            if (isEditableTarget) return;
        };

        this.closeButton.addEventListener('click', () => this.close());
        this.modal.addEventListener('click', (event) => {
            if (event.target === this.modal) {
                this.close();
            }
        });
        this.body.addEventListener('input', (event) => {
            const textInput = event.target.closest('#paperPdfTextViewerInput');
            if (!textInput) return;
            this.textDraft = textInput.value;
            this.syncStats();
        });
        this.body.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-paper-pdf-action]');
            if (!button) return;

            if (button.dataset.paperPdfAction === 'set-mode') {
                this.textMode = button.dataset.pdfMode === 'preview' ? 'preview' : 'write';
                this.render();
                if (this.textMode === 'write') {
                    requestAnimationFrame(() => {
                        this.body.querySelector('#paperPdfTextViewerInput')?.focus();
                    });
                }
                return;
            }
            if (button.dataset.paperPdfAction === 'save-text') {
                await this.saveText();
                return;
            }
            if (button.dataset.paperPdfAction === 'copy-text') {
                await this.copyText();
                return;
            }
            if (button.dataset.paperPdfAction === 'open-pdf' && this.paper?.pdf_url) {
                window.open(Utils.resourceViewerURL('pdf', this.paper.pdf_url), '_blank', 'noopener,noreferrer');
            }
        });
        this.body.addEventListener('keydown', async (event) => {
            const textInput = event.target.closest('#paperPdfTextViewerInput');
            if (!textInput || event.key !== 'Enter' || (!event.metaKey && !event.ctrlKey)) return;
            event.preventDefault();
            await this.saveText();
        });
        document.addEventListener('keydown', this.handleKeydown);
    },

    open(options = {}) {
        this.init();
        this.paper = options.paper || null;
        this.onChanged = options.onChanged;
        this.textDraft = this.paper?.pdf_text || '';
        this.textMode = 'write';
        this.render();
        this.modal.classList.remove('hidden');
        document.body.classList.add('modal-open');
        requestAnimationFrame(() => {
            if (this.textMode === 'write') {
                this.body.querySelector('#paperPdfTextViewerInput')?.focus();
            }
        });
    },

    close() {
        if (!this.modal) return;
        this.modal.classList.add('hidden');
        if (!document.querySelector('.modal-shell:not(.hidden)')) {
            document.body.classList.remove('modal-open');
        }
    },

    currentTextDraft() {
        return this.body.querySelector('#paperPdfTextViewerInput')?.value ?? this.textDraft ?? (this.paper?.pdf_text || '');
    },

    syncStats() {
        const counter = this.body.querySelector('[data-paper-pdf-text-length]');
        if (counter) {
            counter.textContent = `${this.currentTextDraft().length.toLocaleString()} ${t("shared.paper.characters", "字符")}`;
        }
    },

    async saveText() {
        if (!this.paper) return;

        try {
            const payload = await API.updatePaper(
                this.paper.id,
                PaperViewer.buildUpdatePayload(this.paper, {
                    pdf_text: this.currentTextDraft()
                })
            );
            this.paper = payload.paper;
            this.textDraft = this.paper.pdf_text || '';
            Utils.showToast(t('shared.paper.pdf_text_saved', 'PDF 原文已保存'));
            this.render();
            if (typeof this.onChanged === 'function') {
                await this.onChanged(payload.paper);
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async copyText() {
        const text = this.currentTextDraft();
        if (!text) {
            Utils.showToast(t('shared.paper.no_pdf_text_to_copy', '当前没有可复制的 PDF 原文'), 'error');
            return;
        }

        try {
            await navigator.clipboard.writeText(text);
            Utils.showToast(t('shared.paper.pdf_text_copied', 'PDF 原文已复制'));
        } catch (error) {
            Utils.showToast(t('shared.paper.copy_failed', '复制失败，请手动选择文本'), 'error');
        }
    },

    render() {
        const paper = this.paper;
        if (!paper) {
            this.body.innerHTML = `<div class="empty-state"><h3>${t('shared.paper.no_pdf_text_to_show', '没有可展示的 PDF 原文')}</h3></div>`;
            return;
        }

        const text = this.currentTextDraft();
        const tags = PaperViewer.renderTagChips(paper.tags || []);
        const abstractPreview = String(paper.abstract_text || '').trim();
        const managementNotePreview = String(paper.notes_text || '').trim();
        const isPreviewMode = this.textMode === 'preview';

        this.body.innerHTML = `
            <div class="note-lightbox pdf-text-lightbox">
                <section class="note-lightbox-main">
                    <div class="note-lightbox-editor-card">
                        <div class="note-lightbox-head">
                            <div class="note-lightbox-head-row">
                                <div>
                                    <p class="eyebrow">PDF Text</p>
                                    <h2>${Utils.escapeHTML(paper.title)}</h2>
                                    <p class="note-lightbox-subtitle">${Utils.escapeHTML(paper.original_filename || '')}</p>
                                </div>
                                <div class="pdf-text-head-tools">
                                    <div class="note-lightbox-mode-switch">
                                        <button class="btn ${isPreviewMode ? 'btn-outline' : 'btn-primary'}" type="button" data-paper-pdf-action="set-mode" data-pdf-mode="write">${t("shared.paper.write_mode", "编辑")}</button>
                                        <button class="btn ${isPreviewMode ? 'btn-primary' : 'btn-outline'}" type="button" data-paper-pdf-action="set-mode" data-pdf-mode="preview">${t("shared.paper.markdown_preview", "Markdown 预览")}</button>
                                    </div>
                                    <div class="pdf-text-head-meta">
                                        <span class="status-badge tone-${Utils.statusTone(paper.extraction_status)}">${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}</span>
                                        <span class="pdf-text-counter" data-paper-pdf-text-length>${text.length.toLocaleString()} ${t("shared.paper.characters", "字符")}</span>
                                    </div>
                                </div>
                            </div>
                        </div>

                        <div class="note-lightbox-meta-grid">
                            <div class="note-lightbox-meta-item">
                                <span>${t("shared.paper.current_group", "当前分组")}</span>
                                <strong>${Utils.escapeHTML(paper.group_name || t("shared.paper.ungrouped", "未分组"))}</strong>
                            </div>
                            <div class="note-lightbox-meta-item">
                                <span>${t("shared.paper.paper_tags", "文献标签")}</span>
                                <div class="figure-preview-tags ${paper.tags?.length ? '' : 'is-empty'}">
                                    ${paper.tags?.length ? tags : `<span class="figure-preview-empty">${t('shared.paper.no_tags', '无标签')}</span>`}
                                </div>
                            </div>
                        </div>

                        ${isPreviewMode ? `
                            <section class="note-lightbox-render-panel">
                                <span class="note-lightbox-panel-label">${t('shared.paper.markdown_preview', 'Markdown 预览')}</span>
                                <div class="markdown-preview pdf-text-markdown-preview">${Utils.renderMarkdown(text)}</div>
                            </section>
                        ` : `
                            <label class="field note-lightbox-field">
                                <span>${t("shared.paper.pdf_text_label", "PDF 原文")}</span>
                                <textarea id="paperPdfTextViewerInput" class="form-textarea note-lightbox-textarea pdf-text-editor-textarea" rows="24" data-native-context-menu="true" placeholder="${t('shared.paper.pdf_text_placeholder', '在这里补充、修正或整理整篇 PDF 的全文内容，支持使用 Markdown')}">${Utils.escapeHTML(text)}</textarea>
                            </label>
                        `}

                        <div class="figure-notes-actions">
                            <span class="muted">${isPreviewMode ? t('shared.paper.preview_hint_long', '预览基于当前草稿渲染；全文较长时首次切换可能稍慢。') : t('shared.paper.edit_hint', '支持多行编辑和 Markdown 语法，按 Ctrl/Cmd + Enter 可快速保存。')}</span>
                            <div class="pdf-text-inline-actions">
                                <button class="btn btn-outline" type="button" data-paper-pdf-action="copy-text">${t("shared.paper.copy_all_text", "复制全文")}</button>
                                <button class="btn btn-primary" type="button" data-paper-pdf-action="save-text">${t("shared.paper.save_all_text", "保存全文")}</button>
                            </div>
                        </div>
                    </div>
                </section>

                <aside class="note-lightbox-side">
                    <div class="note-lightbox-preview-card">
                        <div class="detail-meta-panel paper-note-meta-panel">
                            <div><span>${t("shared.paper.recent_update", "最近更新")}</span><strong>${Utils.escapeHTML(Utils.formatDate(paper.updated_at || paper.created_at))}</strong></div>
                            <div><span>${t("shared.paper.extracted_figures", "提取图片")}</span><strong>${paper.figure_count || 0}</strong></div>
                            <div><span>${t("shared.paper.original_file", "原始文件")}</span><strong>${Utils.escapeHTML(paper.original_filename || '')}</strong></div>
                        </div>
                    </div>

                    <div class="note-lightbox-preview-card">
                        <div class="note-lightbox-tip">
                            <span>${t("shared.paper.abstract_label", "摘要")}</span>
                            <p>${Utils.escapeHTML(abstractPreview || t('shared.paper.no_abstract_yet', '当前还没有摘要，可在文献详情中补充。'))}</p>
                        </div>
                        <div class="note-lightbox-tip">
                            <span>${t("shared.paper.management_notes", "管理笔记")}</span>
                            <p>${Utils.escapeHTML(managementNotePreview || t('shared.paper.no_management_notes', '当前没有管理笔记。'))}</p>
                        </div>
                    </div>

                    <div class="note-lightbox-actions">
                        <button class="btn btn-outline" type="button" data-paper-pdf-action="open-pdf">${t("shared.paper.open_pdf", "打开 PDF")}</button>
                    </div>
                </aside>
            </div>
        `;
    }
};

const PaperViewer = {
    init() {
        this.modal = document.getElementById('paperModal');
        this.body = document.getElementById('paperModalBody');
        this.closeButton = document.getElementById('closePaperModal');
        if (!this.modal || this.initialized) return;
        this.initialized = true;
        PaperNoteViewer.init();
        PaperPDFTextViewer.init();

        this.handleKeydown = (event) => {
            if (!this.modal || this.modal.classList.contains('hidden')) return;
            if (event.defaultPrevented) return;
            if (typeof Utils !== 'undefined' && typeof Utils.isTopVisibleModal === 'function' && !Utils.isTopVisibleModal(this.modal)) {
                return;
            }
            if (event.key === 'Escape') {
                event.preventDefault();
                event.stopPropagation();
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
            if (button.dataset.modalAction === 'view-pdf-text') {
                this.openPdfTextViewer();
            }
            if (button.dataset.modalAction === 'preview-figure') {
                await this.openFigurePreview(Number(button.dataset.figureIndex));
            }
            if (button.dataset.modalAction === 'delete-figure') {
                await this.deleteFigure(Number(button.dataset.figureId));
            }
            if (button.dataset.modalAction === 'open-paper-notes') {
                this.openPaperNotes();
            }
        });
    },

    renderTagChips(tags = []) {
        if (!tags.length) {
            return `<span class="muted">${t('shared.paper.no_tags', '无标签')}</span>`;
        }
        return tags.map((tag) => `<span class="chip" style="--chip-color:${tag.color}">${Utils.escapeHTML(tag.name)}</span>`).join('');
    },

    buildUpdatePayload(paper, overrides = {}) {
        const hasOverride = (key) => Object.prototype.hasOwnProperty.call(overrides, key);
        const tags = hasOverride('tags')
            ? overrides.tags
            : (paper.tags || []).map((tag) => (typeof tag === 'string' ? tag : tag.name || '')).filter(Boolean);

        return {
            title: hasOverride('title') ? overrides.title : (paper.title || ''),
            pdf_text: hasOverride('pdf_text') ? overrides.pdf_text : (paper.pdf_text || ''),
            abstract_text: hasOverride('abstract_text') ? overrides.abstract_text : (paper.abstract_text || ''),
            notes_text: hasOverride('notes_text') ? overrides.notes_text : (paper.notes_text || ''),
            paper_notes_text: hasOverride('paper_notes_text') ? overrides.paper_notes_text : (paper.paper_notes_text || ''),
            group_id: hasOverride('group_id') ? overrides.group_id : (paper.group_id ?? null),
            tags: Array.isArray(tags) ? tags : []
        };
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
        this.paperTagAutocomplete?.destroy?.();
        this.paperTagAutocomplete = null;
        if (!this.modal) return;
        this.modal.classList.add('hidden');
        if (!document.querySelector('.modal-shell:not(.hidden)')) {
            document.body.classList.remove('modal-open');
        }
    },

    render() {
        this.paperTagAutocomplete?.destroy?.();
        this.paperTagAutocomplete = null;

        const paper = this.paper;
        const groupOptions = [`<option value="">${t('shared.paper.ungrouped', '未分组')}</option>`]
            .concat(this.groups.map((group) => `
                <option value="${group.id}" ${String(group.id) === String(paper.group_id || '') ? 'selected' : ''}>
                    ${Utils.escapeHTML(group.name)}
                </option>
            `))
            .join('');
        const figures = (paper.figures || []).filter((figure) => !figure.parent_figure_id);
        const statusClass = Utils.statusTone(paper.extraction_status);
        const paperNotePreview = String(paper.paper_notes_text || '').replace(/\s+/g, ' ').trim();
        
        // 解析框选结果为人类可读格式
        const boxesHtml = this.renderBoxes(paper.boxes);
        const figureSection = figures.length ? figures.map((figure, index) => `
            <article class="figure-preview-card figure-detail-card">
                <div class="figure-preview-stage">
                    <button class="figure-preview-media" type="button" data-modal-action="preview-figure" data-figure-index="${index}" aria-label="${t('shared.paper.view_large', '查看大图')}">
                        <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.original_name || paper.title)}">
                    </button>
                    <div class="figure-preview-badges">
                        <span class="figure-badge figure-badge-strong">${t('shared.paper.page_n', '第 {page} 页').replace('{page}', figure.page_number || '-')}</span>
                        <span class="figure-badge">${Utils.escapeHTML(figure.display_label || `Fig ${figure.figure_index || '-'}`)}</span>
                        ${figure.parent_figure_id ? `<span class="figure-badge">${t('shared.paper.subfigure', '子图')}</span>` : ''}
                        ${figure.source === 'manual' ? `<span class="figure-badge">${t('shared.paper.manual_extraction', '人工提取')}</span>` : ''}
                    </div>
                </div>
                <div class="figure-preview-body">
                    <div class="figure-preview-head">
                        <span class="figure-preview-label">${t("shared.paper.source_paper", "来源文献")}</span>
                        <strong class="figure-preview-title">${Utils.escapeHTML(paper.title)}</strong>
                    </div>
                    <div class="figure-preview-tags ${figure.tags?.length ? '' : 'is-empty'}">
                        ${figure.tags?.length ? this.renderTagChips(figure.tags || []) : `<span class="figure-preview-empty">${t('shared.paper.no_tags', '无标签')}</span>`}
                    </div>
                    <div class="card-actions">
                        <button class="btn btn-primary" type="button" data-modal-action="preview-figure" data-figure-index="${index}">${t("shared.paper.view_large", "查看大图")}</button>
                        <button class="btn btn-outline danger" type="button" data-modal-action="delete-figure" data-figure-id="${figure.id}">${t("shared.paper.delete_figure", "删除图片")}</button>
                        <a class="btn btn-outline" href="${Utils.resourceViewerURL('image', figure.image_url)}">${t('shared.paper.original_image', '原图')}</a>
                    </div>
                </div>
            </article>
        `).join('') : `<p class="muted">${Utils.isProcessingStatus(paper.extraction_status) ? t('shared.paper.parsing_in_progress', '后台解析完成后会在这里显示提取图片。') : t('shared.paper.no_extracted_figures', '没有可展示的提取图片。')}</p>`;

        this.body.innerHTML = `
            <div class="detail-head">
                <div>
                    <p class="eyebrow">${t("shared.paper.paper_detail", "文献详情")}</p>
                    <h2>${Utils.escapeHTML(paper.title)}</h2>
                </div>
                <span class="status-pill ${statusClass}">
                    ${Utils.escapeHTML(Utils.statusLabel(paper.extraction_status))}
                </span>
            </div>

            <form id="paperViewerForm" class="detail-form">
                <div class="form-grid detail-form-grid">
                    <label class="field field-span-2">
                        <span>${t("shared.paper.title_label", "标题")}</span>
                        <input id="paperViewerTitle" class="form-input" type="text" value="${Utils.escapeHTML(paper.title)}">
                    </label>
                    <label class="field">
                        <span>${t("shared.paper.group_label", "分组")}</span>
                        <select id="paperViewerGroup" class="form-input">${groupOptions}</select>
                    </label>
                    <label class="field">
                        <span>${t("shared.paper.tags_label", "标签")}</span>
                        <div class="tag-autocomplete-field">
                            <input id="paperViewerTags" class="form-input" type="text" value="${Utils.escapeHTML(Utils.joinTags(paper.tags || []))}" placeholder="${t('shared.paper.comma_separated', '逗号分隔')}" autocomplete="off" spellcheck="false">
                            <div class="tag-autocomplete-panel hidden" data-paper-tag-suggestions></div>
                        </div>
                    </label>
                    <label class="field field-span-2">
                        <span>${t("shared.paper.abstract_label", "摘要")}</span>
                        <textarea id="paperViewerAbstract" class="form-textarea" rows="4" placeholder="${t('shared.paper.abstract_placeholder', '为这篇文献补充摘要或核心结论')}">${Utils.escapeHTML(paper.abstract_text || '')}</textarea>
                    </label>
                    <label class="field field-span-2">
                        <span>${t("shared.paper.management_notes", "管理笔记")}</span>
                        <textarea id="paperViewerNotes" class="form-textarea" rows="4" placeholder="${t('shared.paper.notes_placeholder', '记录这篇文献的整理备注、迁移说明或管理信息')}">${Utils.escapeHTML(paper.notes_text || '')}</textarea>
                    </label>
                    <div class="field field-span-2">
                        <span>${t("shared.paper.paper_notes_label", "文献笔记")}</span>
                        <button class="figure-note-inline-trigger ${paperNotePreview ? '' : 'is-empty'}" type="button" data-modal-action="open-paper-notes" aria-label="${t('shared.paper.open_notes', '打开笔记')}">
                            <span class="figure-note-inline-text">${Utils.escapeHTML(paperNotePreview || t('shared.paper.paper_notes_empty_trigger', '还没有文献笔记，点击后在独立笔记面板中查看和编辑。'))}</span>
                            <span class="figure-note-inline-action">${t("shared.paper.open_notes", "打开笔记")}</span>
                        </button>
                    </div>
                </div>
                <div class="detail-actions">
                    <button class="btn btn-primary" type="submit">${t("shared.paper.save", "保存")}</button>
                    <a class="btn btn-outline" href="/manual?paper_id=${paper.id}" target="_blank" rel="noreferrer">${t("shared.paper.manual_annotation", "手动标注")}</a>
                    ${(paper.extraction_status === 'failed' || paper.extraction_status === 'cancelled') ? '<button class="btn btn-outline" type="button" data-modal-action="reextract-paper">${t("shared.paper.reparse", "重新解析")}</button>' : ''}
                    <button class="btn btn-outline danger" type="button" data-modal-action="delete-paper">${t("shared.paper.delete_paper", "删除文献")}</button>
                    <a class="btn btn-outline" href="/ai?paper_id=${paper.id}">${t('shared.paper.ai_reading', 'AI伴读')}</a>
                    <a class="btn btn-outline" href="${Utils.resourceViewerURL('pdf', paper.pdf_url)}">${t('shared.paper.open_pdf', '打开 PDF')}</a>
                </div>
            </form>

            <div class="detail-meta-panel">
                <div><span>${t("shared.paper.original_file", "原始文件")}</span><strong>${Utils.escapeHTML(paper.original_filename)}</strong></div>
                <div><span>${t("shared.paper.pdf_size", "PDF 大小")}</span><strong>${Utils.formatFileSize(paper.file_size || 0)}</strong></div>
                <div><span>${t("shared.paper.extracted_figures", "提取图片")}</span><strong>${figures.length}</strong></div>
                <div><span>${t("shared.paper.current_tags", "当前标签")}</span><strong>${this.renderTagChips(paper.tags || [])}</strong></div>
                <div><span>${t("shared.paper.recent_update", "最近更新")}</span><strong>${Utils.formatDate(paper.updated_at || paper.created_at)}</strong></div>
            </div>

            ${paper.extractor_message ? `<p class="notice ${statusClass}">${Utils.escapeHTML(paper.extractor_message)}</p>` : ''}

            <section class="detail-section">
                <div class="section-head">
                    <h3>${t("shared.paper.extracted_figures", "提取图片")}</h3>
                    <span>${figures.length} ${t("shared.paper.n_figures", "张")}</span>
                </div>
                <div class="figure-preview-grid detail-figure-grid">
                    ${figureSection}
                </div>
            </section>

            <details class="detail-section detail-section-collapsible">
                <summary class="section-head section-head-boxes">
                    <h3>${t("shared.paper.box_results", "框选结果")}</h3>
                    <span class="boxes-count">${boxesHtml.count} ${t("shared.paper.n_box_regions", "个框选区域")}</span>
                </summary>
                <div class="boxes-content">
                    ${boxesHtml.html}
                </div>
            </details>

            <section class="detail-section">
                <div class="section-head section-head-pdf-text">
                    <h3>${t("shared.paper.pdf_text_section", "PDF 原文")}</h3>
                    <button type="button" class="btn btn-small btn-outline" data-modal-action="view-pdf-text">${t("shared.paper.view_edit_preview", "查看 / 编辑 / 预览")}</button>
                </div>
                <div class="pdf-text-preview">
                    ${paper.pdf_text ? `
                        <pre class="pdf-text-snippet" data-native-context-menu="true">${Utils.escapeHTML(paper.pdf_text.substring(0, 1000))}${paper.pdf_text.length > 1000 ? '\n...' : ''}</pre>
                        <p class="pdf-text-meta">共 ${paper.pdf_text.length.toLocaleString()} ${t("shared.paper.characters", "字符")}</p>
                    ` : `<p class="muted">${t('shared.paper.no_pdf_text_hint', '暂无 PDF 原文，点击上方按钮可补充、编辑或切换 Markdown 预览。')}</p>`}
                </div>
            </section>
        `;

        Utils.mergeScopedTagCatalog?.('paper', paper.tags || []);
        this.paperTagAutocomplete = Utils.bindCommaSeparatedTagInputAutocomplete?.({
            input: this.body.querySelector('#paperViewerTags'),
            panel: this.body.querySelector('[data-paper-tag-suggestions]'),
            scope: 'paper'
        }) || null;
    },

    paperFiguresForViewer() {
        const paper = this.paper;
        return (paper.figures || []).filter((figure) => !figure.parent_figure_id).map((figure) => ({
            ...figure,
            paper_id: paper.id,
            paper_title: paper.title,
            group_id: paper.group_id,
            group_name: paper.group_name || '',
            tags: figure.tags || []
        }));
    },

    async openFigurePreview(index) {
        if (typeof FigureViewer === 'undefined') {
            Utils.openResourceViewer('image', this.paperFiguresForViewer()?.[index]?.image_url);
            return;
        }

        await FigureViewer.open({
            figures: this.paperFiguresForViewer(),
            index,
            page: 1,
            totalPages: 1,
            onOpenPaper: async () => {
                FigureViewer.close();
            },
            onMetaChanged: async (paper) => {
                if (!paper) return;
                this.paper = paper;
                this.render();
                if (typeof this.onChanged === 'function') {
                    await this.onChanged();
                }
            }
        });
    },

    async save() {
        try {
            const payload = await API.updatePaper(
                this.paper.id,
                this.buildUpdatePayload(this.paper, {
                    title: document.getElementById('paperViewerTitle').value.trim(),
                    abstract_text: document.getElementById('paperViewerAbstract').value.trim(),
                    notes_text: document.getElementById('paperViewerNotes').value.trim(),
                    group_id: document.getElementById('paperViewerGroup').value ? Number(document.getElementById('paperViewerGroup').value) : null,
                    tags: Utils.splitTags(document.getElementById('paperViewerTags').value)
                })
            );
            this.paper = payload.paper;
            Utils.mergeScopedTagCatalog?.('paper', payload.paper?.tags || []);
            Utils.showToast(t('shared.paper.info_updated', '文献信息已更新'));
            this.render();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async remove() {
        const confirmed = await Utils.confirm(t('shared.paper.confirm_delete_paper', '删除后会移除 PDF、提取图片以及相关关联。'));
        if (!confirmed) return;

        try {
            await API.deletePaper(this.paper.id);
            Utils.showToast(t('shared.paper.paper_deleted', '文献已删除'));
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
            Utils.showToast(t('shared.paper.reparse_submitted', '文献已重新提交解析'), 'info');
            this.render();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    async deleteFigure(figureID) {
        const confirmed = await Utils.confirm(t('shared.paper.confirm_delete_figure', '删除后会移除这张图片文件，但不会删除整篇文献。'));
        if (!confirmed) return;

        try {
            const payload = await API.deleteFigure(figureID);
            this.paper = payload.paper;
            Utils.showToast(t('shared.paper.figure_deleted', '图片已删除'));
            this.render();
            if (typeof this.onChanged === 'function') {
                await this.onChanged();
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    openPaperNotes() {
        PaperNoteViewer.open({
            paper: this.paper,
            onChanged: async (paper) => {
                if (!paper) return;
                this.paper = paper;
                this.render();
                if (typeof this.onChanged === 'function') {
                    await this.onChanged();
                }
            },
            onOpenPaper: async (paperID) => {
                if (!paperID) return;
                const nextPaper = await API.getPaper(paperID);
                this.paper = nextPaper;
                this.render();
            }
        });
    },

    // 解析并渲染框选结果为人类可读格式
    renderBoxes(boxesData) {
        if (!boxesData) {
            return { html: `<p class="muted">${t('shared.paper.no_box_results', '暂无框选结果')}</p>`, count: 0 };
        }
        
        let boxes = [];
        try {
            // boxesData 可能是字符串或已解析的对象
            boxes = typeof boxesData === 'string' ? JSON.parse(boxesData) : boxesData;
            // 处理不同格式：可能是数组直接是 boxes，或 {boxes: [...]}
            if (boxes && typeof boxes === 'object' && !Array.isArray(boxes)) {
                boxes = boxes.boxes || boxes.regions || boxes.regions || [];
            }
        } catch (e) {
            return { html: `<pre class="code-block">${Utils.escapeHTML(String(boxesData))}</pre>`, count: 0 };
        }
        
        if (!Array.isArray(boxes) || boxes.length === 0) {
            return { html: `<p class="muted">${t('shared.paper.no_box_results', '暂无框选结果')}</p>`, count: 0 };
        }
        
        // 按页码分组统计
        const pageGroups = {};
        boxes.forEach((box, index) => {
            const page = box.page || box.page_number || box.pageNumber || '-';
            if (!pageGroups[page]) pageGroups[page] = [];
            pageGroups[page].push({ ...box, index: index + 1 });
        });
        
        const pages = Object.keys(pageGroups).sort((a, b) => {
            if (a === '-') return 1;
            if (b === '-') return -1;
            return parseInt(a) - parseInt(b);
        });
        
        let html = '<div class="boxes-timeline">';
        pages.forEach(page => {
            const pageBoxes = pageGroups[page];
            html += `
                <div class="boxes-page-group">
                    <div class="boxes-page-header">
                        <span class="boxes-page-number">${t('shared.paper.page_n', '第 {page} 页').replace('{page}', page)}</span>
                        <span class="boxes-page-count">${pageBoxes.length} ${t("shared.paper.n_regions", "个区域")}</span>
                    </div>
                    <div class="boxes-list">
                        ${pageBoxes.map(box => this.renderBoxItem(box)).join('')}
                    </div>
                </div>
            `;
        });
        html += '</div>';
        
        return { html, count: boxes.length };
    },
    
    // 渲染单个框选项
    renderBoxItem(box) {
        const bbox = box.bbox || box.box || box.region || [];
        const [x1, y1, x2, y2] = Array.isArray(bbox) ? bbox : [bbox.x1, bbox.y1, bbox.x2, bbox.y2];
        const width = x1 !== undefined && x2 !== undefined ? Math.round(x2 - x1) : '-';
        const height = y1 !== undefined && y2 !== undefined ? Math.round(y2 - y1) : '-';
        
        // 提取类型/标签信息
        const type = box.type || box.label || box.category || t('shared.paper.box_region', '框选区域');
        const confidence = box.confidence ? `${(box.confidence * 100).toFixed(1)}%` : null;
        
        return `
            <div class="box-item">
                <div class="box-item-header">
                    <span class="box-item-index">#${box.index}</span>
                    <span class="box-item-type">${Utils.escapeHTML(type)}</span>
                    ${confidence ? `<span class="box-item-confidence">${confidence}</span>` : ''}
                </div>
                <div class="box-item-coords">
                    <span class="coord-item" title="${t('shared.paper.top_left_coords', '左上角坐标')}">(${x1 !== undefined ? Math.round(x1) : '-'}, ${y1 !== undefined ? Math.round(y1) : '-'})</span>
                    <span class="coord-arrow">→</span>
                    <span class="coord-item" title="${t('shared.paper.bottom_right_coords', '右下角坐标')}">(${x2 !== undefined ? Math.round(x2) : '-'}, ${y2 !== undefined ? Math.round(y2) : '-'})</span>
                    <span class="coord-size">${width}×${height}</span>
                </div>
                ${box.text ? `<div class="box-item-text">${Utils.escapeHTML(box.text.substring(0, 100))}${box.text.length > 100 ? '...' : ''}</div>` : ''}
            </div>
        `;
    },
    
    // 打开 PDF 原文编辑器
    openPdfTextViewer() {
        PaperPDFTextViewer.open({
            paper: this.paper,
            onChanged: async (paper) => {
                if (!paper) return;
                this.paper = paper;
                this.render();
                if (typeof this.onChanged === 'function') {
                    await this.onChanged();
                }
            }
        });
    }
};
