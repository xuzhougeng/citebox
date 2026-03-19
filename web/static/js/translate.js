const DesktopTranslate = {
    init() {
        if (this.initialized) return;
        if (!this.enabled()) return;
        this.initialized = true;
        this.pendingText = '';
        this.ensureMenu();
        this.bind();
    },

    enabled() {
        return typeof Utils !== 'undefined'
            && typeof Utils.isDesktopApp === 'function'
            && Utils.isDesktopApp();
    },

    ensureMenu() {
        this.menu = document.getElementById('desktopTranslateMenu');
        if (this.menu) return;

        this.menu = document.createElement('div');
        this.menu.id = 'desktopTranslateMenu';
        this.menu.className = 'translate-context-menu hidden';
        this.menu.innerHTML = `
            <button type="button" data-translate-action="translate">翻译</button>
        `;
        document.body.appendChild(this.menu);
    },

    bind() {
        document.addEventListener('contextmenu', (event) => {
            if (!this.enabled()) {
                return;
            }
            if (this.isIgnoredTarget(event.target)) {
                this.hideMenu();
                return;
            }

            const selectedText = this.currentSelectionText();
            if (!selectedText) {
                this.hideMenu();
                return;
            }

            event.preventDefault();
            this.pendingText = selectedText;
            this.showMenu(event.clientX, event.clientY);
        }, true);

        document.addEventListener('click', async (event) => {
            const target = event.target instanceof Element ? event.target : null;
            const actionButton = target?.closest('[data-translate-action]');
            if (actionButton?.dataset.translateAction === 'translate') {
                event.preventDefault();
                event.stopPropagation();
                await this.translateSelection();
                return;
            }

            if (target?.closest('#desktopTranslateMenu')) {
                return;
            }
            this.hideMenu();
        });

        document.addEventListener('keydown', (event) => {
            if (event.key === 'Escape') {
                this.hideMenu();
                this.closeResultDialog();
            }
        });

        document.addEventListener('scroll', () => {
            this.hideMenu();
        }, true);
    },

    currentSelectionText() {
        return String(window.getSelection?.().toString() || '').trim();
    },

    isIgnoredTarget(target) {
        const element = target instanceof Element ? target : null;
        if (!element) return false;
        if (element.closest('#desktopTranslateMenu, .translate-dialog-overlay')) {
            return true;
        }
        const editable = element.closest('input, textarea, select, [contenteditable=""], [contenteditable="true"], [contenteditable="plaintext-only"]');
        return Boolean(editable);
    },

    showMenu(clientX, clientY) {
        if (!this.menu) return;

        this.menu.classList.remove('hidden');
        this.menu.style.left = '0px';
        this.menu.style.top = '0px';

        const rect = this.menu.getBoundingClientRect();
        const left = Math.max(12, Math.min(clientX, window.innerWidth - rect.width - 12));
        const top = Math.max(12, Math.min(clientY, window.innerHeight - rect.height - 12));
        this.menu.style.left = `${left}px`;
        this.menu.style.top = `${top}px`;
    },

    hideMenu() {
        if (!this.menu) return;
        this.menu.classList.add('hidden');
    },

    async translateSelection() {
        const text = String(this.pendingText || '').trim();
        this.pendingText = '';
        this.hideMenu();
        if (!text) {
            return;
        }

        this.renderResultDialog({
            loading: true,
            sourceLanguage: '',
            targetLanguage: '',
            translation: ''
        });

        try {
            const result = await API.translateWithAI({ text });
            this.renderResultDialog({
                loading: false,
                sourceLanguage: result.source_language || '',
                targetLanguage: result.target_language || '',
                translation: result.translation || ''
            });
        } catch (error) {
            this.renderResultDialog({
                loading: false,
                error: error.message || '翻译失败',
                sourceLanguage: '',
                targetLanguage: '',
                translation: ''
            });
        }
    },

    renderResultDialog(state = {}) {
        this.closeResultDialog();

        const overlay = document.createElement('div');
        overlay.className = 'dialog-overlay translate-dialog-overlay';
        overlay.innerHTML = `
            <div class="dialog-box translate-dialog-box">
                <div class="dialog-header translate-dialog-header">
                    <div>
                        <h3>划词翻译</h3>
                        <p class="translate-dialog-subtitle">${this.renderSubtitle(state)}</p>
                    </div>
                    <button class="modal-close translate-dialog-close" type="button" data-translate-dialog-action="close" aria-label="关闭">×</button>
                </div>
                <div class="translate-dialog-body">
                    ${this.renderDialogBody(state)}
                </div>
                <div class="dialog-footer">
                    <button class="btn btn-outline" type="button" data-translate-dialog-action="close">关闭</button>
                    ${state.loading || state.error ? '' : '<button class="btn btn-primary" type="button" data-translate-dialog-action="copy">复制译文</button>'}
                </div>
            </div>
        `;

        overlay.addEventListener('click', async (event) => {
            if (event.target === overlay) {
                this.closeResultDialog();
                return;
            }

            const target = event.target instanceof Element ? event.target : null;
            const button = target?.closest('[data-translate-dialog-action]');
            if (!button) return;

            if (button.dataset.translateDialogAction === 'close') {
                this.closeResultDialog();
                return;
            }
            if (button.dataset.translateDialogAction === 'copy') {
                await this.copyTranslation(state.translation || '');
            }
        });

        document.body.appendChild(overlay);
        this.resultDialog = overlay;
    },

    renderSubtitle(state = {}) {
        if (state.loading) {
            return '正在请求翻译结果...';
        }
        if (state.error) {
            return '翻译失败';
        }
        if (state.sourceLanguage && state.targetLanguage) {
            return `${Utils.escapeHTML(state.sourceLanguage)} -> ${Utils.escapeHTML(state.targetLanguage)}`;
        }
        return '翻译结果';
    },

    renderDialogBody(state = {}) {
        if (state.loading) {
            return '<div class="translate-result-loading">正在调用翻译模型，请稍候。</div>';
        }
        if (state.error) {
            return `<div class="translate-result-error">${Utils.escapeHTML(state.error)}</div>`;
        }
        return `<pre class="translate-result-text">${Utils.escapeHTML(state.translation || '')}</pre>`;
    },

    async copyTranslation(text = '') {
        const content = String(text || '').trim();
        if (!content) {
            Utils.showToast('没有可复制的译文', 'error');
            return;
        }

        try {
            await navigator.clipboard.writeText(content);
            Utils.showToast('译文已复制');
        } catch (error) {
            Utils.showToast('复制失败，请手动选择文本', 'error');
        }
    },

    closeResultDialog() {
        if (!this.resultDialog) return;
        this.resultDialog.remove();
        this.resultDialog = null;
    }
};
