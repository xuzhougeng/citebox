if (typeof window.t !== 'function') window.t = function(k,f){return f||k};
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
            <button type="button" data-translate-action="copy">${t('shared.translate.copy', '复制')}</button>
            <button type="button" data-translate-action="translate">${t('shared.translate.translate', '翻译')}</button>
        `;
        document.body.appendChild(this.menu);
    },

    bind() {
        document.addEventListener('contextmenu', (event) => {
            if (!this.enabled()) {
                return;
            }
            if (this.shouldKeepNativeMenu(event)) {
                this.hideMenu();
                return;
            }

            const selectedText = this.currentSelectionText();
            if (!String(selectedText || '').trim()) {
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
            if (actionButton?.dataset.translateAction === 'copy') {
                event.preventDefault();
                event.stopPropagation();
                await this.copySelection();
                return;
            }
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
        return String(window.getSelection?.().toString() || '');
    },

    shouldKeepNativeMenu(event) {
        return this.isIgnoredTarget(event.target, event) || this.hasFocusedEditableElement();
    },

    isIgnoredTarget(target, event) {
        const elements = this.contextElements(target, event);
        for (const element of elements) {
            if (element.closest('#desktopTranslateMenu, .translate-dialog-overlay, [data-native-context-menu="true"]')) {
                return true;
            }
            if (this.isEditableElement(element)) {
                return true;
            }
        }
        return false;
    },

    contextElements(target, event) {
        const elements = [];
        const pushElement = (candidate) => {
            if (!(candidate instanceof Element)) return;
            if (!elements.includes(candidate)) {
                elements.push(candidate);
            }
        };

        pushElement(target);
        if (typeof event?.composedPath === 'function') {
            event.composedPath().forEach(pushElement);
        }
        pushElement(document.activeElement);
        return elements;
    },

    hasFocusedEditableElement() {
        const active = document.activeElement;
        return active instanceof Element && this.isEditableElement(active);
    },

    isEditableElement(element) {
        return Boolean(element.closest('input, textarea, select, [contenteditable=""], [contenteditable="true"], [contenteditable="plaintext-only"]'));
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
        await this.translateText(text, {
            title: t('shared.translate.word_translate', '划词翻译')
        });
    },

    async copySelection() {
        const text = String(this.pendingText || '');
        this.pendingText = '';
        this.hideMenu();
        if (!text.trim()) {
            Utils.showToast(t('shared.translate.no_content_to_copy', '没有可复制的内容'), 'error');
            return;
        }
        await this.copyText(text, {
            successMessage: t('shared.translate.copied_selection', '已复制所选内容'),
            errorMessage: t('shared.translate.copy_failed', '复制失败，请手动选择文本')
        });
    },

    async translateText(text, options = {}) {
        const content = String(text || '').trim();
        const title = String(options.title || '').trim() || t('shared.translate.ai_translate', 'AI 翻译');
        const emptyMessage = String(options.emptyMessage || '').trim() || t('shared.translate.no_content_to_translate', '没有可翻译的内容');
        if (!content) {
            Utils.showToast(emptyMessage, 'error');
            return;
        }

        this.renderResultDialog({
            title,
            loading: true,
            sourceLanguage: '',
            targetLanguage: '',
            translation: ''
        });

        try {
            const result = await API.translateWithAI({ text: content });
            this.renderResultDialog({
                title,
                loading: false,
                sourceLanguage: result.source_language || '',
                targetLanguage: result.target_language || '',
                translation: result.translation || ''
            });
        } catch (error) {
            this.renderResultDialog({
                title,
                loading: false,
                error: error.message || t('shared.translate.failed', '翻译失败'),
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
                        <h3>${Utils.escapeHTML(state.title || t('shared.translate.ai_translate', 'AI 翻译'))}</h3>
                        <p class="translate-dialog-subtitle">${this.renderSubtitle(state)}</p>
                    </div>
                    <button class="modal-close translate-dialog-close" type="button" data-translate-dialog-action="close" aria-label="${t('shared.translate.close', '关闭')}">×</button>
                </div>
                <div class="translate-dialog-body">
                    ${this.renderDialogBody(state)}
                </div>
                <div class="dialog-footer">
                    <button class="btn btn-outline" type="button" data-translate-dialog-action="close">${t('shared.translate.close', '关闭')}</button>
                    ${state.loading || state.error ? '' : `<button class="btn btn-primary" type="button" data-translate-dialog-action="copy">${t('shared.translate.copy_translation', '复制译文')}</button>`}
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
            return t('shared.translate.requesting', '正在请求翻译结果...');
        }
        if (state.error) {
            return t('shared.translate.failed', '翻译失败');
        }
        if (state.sourceLanguage && state.targetLanguage) {
            return `${Utils.escapeHTML(state.sourceLanguage)} -> ${Utils.escapeHTML(state.targetLanguage)}`;
        }
        return t('shared.translate.result', '翻译结果');
    },

    renderDialogBody(state = {}) {
        if (state.loading) {
            return `<div class="translate-result-loading">${t('shared.translate.loading_model', '正在调用翻译模型，请稍候。')}</div>`;
        }
        if (state.error) {
            return `<div class="translate-result-error">${Utils.escapeHTML(state.error)}</div>`;
        }
        return `<pre class="translate-result-text">${Utils.escapeHTML(state.translation || '')}</pre>`;
    },

    async copyTranslation(text = '') {
        const content = String(text || '').trim();
        if (!content) {
            Utils.showToast(t('shared.translate.no_translation_to_copy', '没有可复制的译文'), 'error');
            return;
        }

        await this.copyText(content, {
            successMessage: t('shared.translate.translation_copied', '译文已复制'),
            errorMessage: t('shared.translate.copy_failed', '复制失败，请手动选择文本')
        });
    },

    async copyText(text = '', messages = {}) {
        const content = String(text || '');
        if (!content.trim()) {
            return false;
        }

        try {
            if (typeof window.citeboxDesktopWriteClipboardText === 'function') {
                await window.citeboxDesktopWriteClipboardText(content);
            } else {
                await navigator.clipboard.writeText(content);
            }
            Utils.showToast(messages.successMessage || t('shared.translate.copied', '已复制'));
            return true;
        } catch (error) {
            Utils.showToast(messages.errorMessage || t('shared.translate.copy_failed', '复制失败，请手动选择文本'), 'error');
            return false;
        }
    },

    closeResultDialog() {
        if (!this.resultDialog) return;
        this.resultDialog.remove();
        this.resultDialog = null;
    }
};
