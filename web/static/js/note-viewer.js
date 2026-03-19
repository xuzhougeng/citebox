const NoteViewer = {
    init() {
        this.modal = document.getElementById('noteModal');
        this.body = document.getElementById('noteModalBody');
        this.closeButton = document.getElementById('closeNoteModal');
        if (!this.modal) {
            const shell = document.createElement('div');
            shell.id = 'noteModal';
            shell.className = 'modal-shell hidden';
            shell.innerHTML = `
                <div class="modal-dialog figure-modal-dialog note-modal-dialog">
                    <button id="closeNoteModal" class="modal-close" type="button" aria-label="关闭">×</button>
                    <div id="noteModalBody"></div>
                </div>
            `;
            document.body.appendChild(shell);
            this.modal = shell;
            this.body = shell.querySelector('#noteModalBody');
            this.closeButton = shell.querySelector('#closeNoteModal');
        }
        if (!this.modal || this.initialized) return;
        this.initialized = true;

        this.handleKeydown = (event) => {
            if (!this.modal || this.modal.classList.contains('hidden')) return;
            const target = event.target;
            const isEditableTarget = target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName));
            if (event.key === 'Escape') {
                event.preventDefault();
                event.stopPropagation();
                void this.openPreview();
                return;
            }
            if (isEditableTarget) {
                return;
            }
            if (event.key === 'ArrowLeft') {
                void this.previous();
            }
            if (event.key === 'ArrowRight') {
                void this.next();
            }
        };

        this.closeButton.addEventListener('click', () => this.close());
        this.modal.addEventListener('click', (event) => {
            if (event.target === this.modal) {
                this.close();
            }
        });
        this.body.addEventListener('input', (event) => {
            const notesInput = event.target.closest('#noteViewerInput');
            if (!notesInput) return;
            this.noteDraft = notesInput.value;
        });
        this.body.addEventListener('click', async (event) => {
            const button = event.target.closest('[data-note-action]');
            if (!button) return;

            if (button.dataset.noteAction === 'prev') {
                await this.previous();
                return;
            }
            if (button.dataset.noteAction === 'next') {
                await this.next();
                return;
            }
            if (button.dataset.noteAction === 'set-mode') {
                this.noteMode = button.dataset.noteMode === 'preview' ? 'preview' : 'write';
                this.render();
                return;
            }
            if (button.dataset.noteAction === 'save-notes') {
                await this.saveNotesFromInput();
                return;
            }
            if (button.dataset.noteAction === 'open-preview') {
                await this.openPreview();
                return;
            }
            if (button.dataset.noteAction === 'open-paper') {
                await this.openPaper();
            }
        });
        this.body.addEventListener('keydown', async (event) => {
            const notesInput = event.target.closest('#noteViewerInput');
            if (!notesInput || event.key !== 'Enter' || (!event.metaKey && !event.ctrlKey)) return;
            event.preventDefault();
            await this.saveNotesFromInput();
        });
        document.addEventListener('keydown', this.handleKeydown);
    },

    async open(options = {}) {
        this.init();
        this.figures = Array.isArray(options.figures) ? options.figures : [];
        this.index = Math.max(0, Math.min(Number(options.index) || 0, this.figures.length - 1));
        this.page = Math.max(1, Number(options.page) || 1);
        this.totalPages = Math.max(1, Number(options.totalPages) || 1);
        this.loadPage = typeof options.loadPage === 'function' ? options.loadPage : null;
        this.onOpenPaper = options.onOpenPaper;
        this.onMetaChanged = options.onMetaChanged;
        this.loadingPage = false;
        this.noteDraft = '';
        this.noteMode = 'write';
        this.syncCurrentFigureState({ resetMode: true });

        try {
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

    canMovePrevious() {
        return Boolean(this.figures?.length) && (this.index > 0 || this.page > 1);
    },

    canMoveNext() {
        return Boolean(this.figures?.length) && (this.index < this.figures.length - 1 || this.page < this.totalPages);
    },

    async previous() {
        if (!this.canMovePrevious() || this.loadingPage) return;
        if (this.index > 0) {
            this.index -= 1;
            this.syncCurrentFigureState({ resetMode: true });
            this.render();
            return;
        }
        await this.loadAdjacentPage(this.page - 1, 'last');
    },

    async next() {
        if (!this.canMoveNext() || this.loadingPage) return;
        if (this.index < this.figures.length - 1) {
            this.index += 1;
            this.syncCurrentFigureState({ resetMode: true });
            this.render();
            return;
        }
        await this.loadAdjacentPage(this.page + 1, 'first');
    },

    async loadAdjacentPage(targetPage, targetIndex) {
        if (!this.loadPage || targetPage < 1 || targetPage > this.totalPages) return;

        this.loadingPage = true;
        this.render();

        try {
            const payload = await this.loadPage(targetPage);
            const figures = payload?.figures || [];
            if (!figures.length) return;

            this.figures = figures;
            this.page = targetPage;
            this.totalPages = Math.max(1, Number(payload.total_pages) || this.totalPages);
            this.index = targetIndex === 'last' ? figures.length - 1 : 0;
            this.syncCurrentFigureState({ resetMode: true });
        } catch (error) {
            Utils.showToast(error.message, 'error');
        } finally {
            this.loadingPage = false;
            this.render();
        }
    },

    currentFigureNotesDraft() {
        return this.body.querySelector('#noteViewerInput')?.value ?? this.noteDraft ?? (this.currentFigure?.notes_text || '');
    },

    async saveNotesFromInput() {
        await this.updateCurrentFigureNotes(this.currentFigureNotesDraft(), '图片笔记已保存');
    },

    async updateCurrentFigureNotes(notesText, successMessage) {
        if (!this.currentFigure) return;

        try {
            const payload = await API.updateFigure(this.currentFigure.id, {
                notes_text: notesText
            });
            this.syncPaperMetadata(payload.paper);
            Utils.showToast(successMessage);
            this.render();
            if (typeof this.onMetaChanged === 'function') {
                await this.onMetaChanged(payload.paper);
            }
        } catch (error) {
            Utils.showToast(error.message, 'error');
        }
    },

    syncPaperMetadata(paper) {
        this.figures = mergeFigureCollectionWithPaper(this.figures, paper);
        this.syncCurrentFigureState({ resetMode: false, forceDraftFromFigure: true });
    },

    syncCurrentFigureState(options = {}) {
        const { resetMode = false, forceDraftFromFigure = false } = options;
        this.currentFigure = this.figures?.[this.index];
        const figureNotes = this.currentFigure?.notes_text || '';

        if (forceDraftFromFigure || typeof this.noteDraft !== 'string' || resetMode) {
            this.noteDraft = figureNotes;
        }
        if (resetMode || !this.noteMode) {
            this.noteMode = figureNotes.trim() ? 'preview' : 'write';
        }
    },

    async openPreview() {
        if (!this.currentFigure) return;

        this.close();
        await FigureViewer.open({
            figures: this.figures || [],
            index: this.index,
            page: this.page,
            totalPages: this.totalPages,
            loadPage: this.loadPage,
            onOpenPaper: this.onOpenPaper,
            onMetaChanged: this.onMetaChanged
        });
    },

    async openPaper() {
        if (!this.currentFigure) return;

        this.close();
        if (typeof this.onOpenPaper === 'function') {
            await this.onOpenPaper(this.currentFigure.paper_id);
        }
    },

    render() {
        this.currentFigure = this.figures?.[this.index];
        if (!this.currentFigure) {
            this.body.innerHTML = '<div class="empty-state"><h3>没有可展示的笔记</h3></div>';
            return;
        }

        const figure = this.currentFigure;
        const total = this.figures.length;
        const canPrev = this.canMovePrevious();
        const canNext = this.canMoveNext();
        const noteText = this.currentFigureNotesDraft();
        const isPreviewMode = this.noteMode === 'preview';

        this.body.innerHTML = `
            <div class="note-lightbox">
                <section class="note-lightbox-main">
                    <div class="figure-lightbox-toolbar">
                        <div class="figure-lightbox-counter">第 ${this.index + 1} / ${total} 张 · 第 ${this.page} / ${this.totalPages} 页</div>
                        <div class="figure-lightbox-nav">
                            <button class="btn btn-outline" type="button" data-note-action="prev" ${!canPrev || this.loadingPage ? 'disabled' : ''}>上一条</button>
                            <button class="btn btn-outline" type="button" data-note-action="next" ${!canNext || this.loadingPage ? 'disabled' : ''}>下一条</button>
                        </div>
                    </div>

                    <div class="note-lightbox-editor-card">
                        <div class="note-lightbox-head">
                            <div class="note-lightbox-head-row">
                                <div>
                                    <p class="eyebrow">Figure Notes</p>
                                    <h2>${Utils.escapeHTML(figure.paper_title)}</h2>
                                    <p class="note-lightbox-subtitle">第 ${figure.page_number || '-'} 页 · #${figure.figure_index || '-'}${figure.source === 'manual' ? ' · 人工提取' : ''}</p>
                                </div>
                                <div class="note-lightbox-mode-switch">
                                    <button class="btn ${isPreviewMode ? 'btn-outline' : 'btn-primary'}" type="button" data-note-action="set-mode" data-note-mode="write">编辑</button>
                                    <button class="btn ${isPreviewMode ? 'btn-primary' : 'btn-outline'}" type="button" data-note-action="set-mode" data-note-mode="preview">Markdown 预览</button>
                                </div>
                            </div>
                        </div>

                        <div class="note-lightbox-meta-grid">
                            <div class="note-lightbox-meta-item">
                                <span>图片标签</span>
                                <div class="figure-preview-tags ${figure.tags?.length ? '' : 'is-empty'}">
                                    ${figure.tags?.length ? BrowserUI.renderTagChips(figure.tags || []) : '<span class="figure-preview-empty">无标签</span>'}
                                </div>
                            </div>
                            <div class="note-lightbox-meta-item">
                                <span>来源分组</span>
                                <strong>${Utils.escapeHTML(figure.group_name || '未分组')}</strong>
                            </div>
                        </div>

                        ${isPreviewMode ? `
                            <section class="note-lightbox-render-panel">
                                <span class="note-lightbox-panel-label">Markdown 预览</span>
                                <div class="markdown-preview">${Utils.renderMarkdown(noteText)}</div>
                            </section>
                        ` : `
                            <label class="field note-lightbox-field">
                                <span>图片笔记</span>
                                <textarea id="noteViewerInput" class="form-textarea note-lightbox-textarea" rows="16" placeholder="记录这张图的观察、AI 解读摘要、方法要点或后续检索关键词">${Utils.escapeHTML(noteText)}</textarea>
                            </label>
                        `}
                        <div class="figure-notes-actions">
                            <span class="muted">${isPreviewMode ? '预览基于当前草稿渲染；切回编辑可继续修改。' : '支持多行内容，按 Ctrl/Cmd + Enter 可快速保存。'}</span>
                            <button class="btn btn-primary" type="button" data-note-action="save-notes">保存笔记</button>
                        </div>
                    </div>
                </section>

                <aside class="note-lightbox-side">
                    <div class="note-lightbox-preview-card">
                        <div class="note-lightbox-preview-media">
                            <img src="${figure.image_url}" alt="${Utils.escapeHTML(figure.caption || figure.paper_title)}">
                        </div>
                        ${figure.caption ? `
                            <div class="figure-lightbox-caption">
                                ${Utils.escapeHTML(figure.caption)}
                            </div>
                        ` : ''}
                    </div>

                    <div class="note-lightbox-actions">
                        <button class="btn btn-outline" type="button" data-note-action="open-preview">查看大图</button>
                        <button class="btn btn-outline" type="button" data-note-action="open-paper">查看文献</button>
                        <a class="btn btn-outline" href="${Utils.resourceViewerURL('image', figure.image_url)}">原图</a>
                    </div>

                    <div class="note-lightbox-tip">
                        <span>整理建议</span>
                        <p>先把可检索的关键词和 AI 解读写进笔记，再决定是否回到大图页补标签。</p>
                    </div>
                </aside>
            </div>
        `;
    }
};
