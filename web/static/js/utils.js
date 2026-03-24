// 工具函数模块
const Utils = {
    modalRestoreParam: 'restore_modal',
    modalRestoreStoragePrefix: 'citebox.modalRestore.',
    defaultFigureTagPresets: Object.freeze(['图 1', '图 2', '图 3', '图 4', '图 5', '图 6', '图 7', '附图', '图片摘要']),
    tagCatalogs: new Map(),
    tagCatalogPromises: new Map(),

    formatFileSize(bytes) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    },

    formatDate(dateString) {
        const date = new Date(dateString);
        return date.toLocaleDateString('zh-CN', {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit'
        });
    },

    showToast(message, type = 'success') {
        const toast = document.createElement('div');
        toast.className = `toast toast-${type}`;
        toast.textContent = message;
        document.body.appendChild(toast);
        setTimeout(() => {
            toast.remove();
        }, 3000);
    },

    supportsDesktopSave() {
        return typeof window.citeboxDesktopSaveFile === 'function';
    },

    isDesktopApp() {
        return window.__CITEBOX_DESKTOP__ === true;
    },

    canOpenExternalURL() {
        return typeof window.citeboxDesktopOpenExternal === 'function';
    },

    async openExternalURL(value, options = {}) {
        const normalized = String(value || '').trim();
        if (!normalized) {
            return false;
        }

        let resolvedURL = normalized;
        try {
            resolvedURL = new URL(normalized, window.location.href).href;
        } catch (error) {
            resolvedURL = normalized;
        }

        if (this.canOpenExternalURL()) {
            try {
                await window.citeboxDesktopOpenExternal(resolvedURL);
                return true;
            } catch (error) {
                // Fall back to browser navigation below.
            }
        }

        const target = String(options.target || '_blank').trim().toLowerCase();
        if (!target || target === '_blank') {
            window.open(resolvedURL, '_blank', 'noopener,noreferrer');
            return true;
        }

        window.location.href = resolvedURL;
        return true;
    },

    resourceViewerURL(kind, src, back = window.location.href) {
        const params = new URLSearchParams();
        params.set('kind', String(kind || ''));
        params.set('src', String(src || ''));
        const normalizedBack = String(back || '').trim();
        if (normalizedBack) {
            params.set('back', normalizedBack);
        }
        return `/viewer?${params.toString()}`;
    },

    buildResourceViewerBackURL(back = window.location.href, options = {}) {
        const { replaceCurrentHistory = false } = options;
        const normalizedBack = String(back || '').trim();
        if (!normalizedBack) {
            return '';
        }

        const restoreState = this.captureModalRestoreState();
        if (!restoreState) {
            return normalizedBack;
        }

        const token = this.storeModalRestoreState(restoreState);
        if (!token) {
            return normalizedBack;
        }

        const resolvedBack = this.applyModalRestoreToken(normalizedBack, token);
        if (!resolvedBack) {
            return normalizedBack;
        }

        if (replaceCurrentHistory) {
            this.replaceCurrentHistoryURL(resolvedBack);
        }

        return resolvedBack;
    },

    applyModalRestoreToken(back = '', token = '') {
        const normalizedBack = String(back || '').trim();
        const normalizedToken = String(token || '').trim();
        if (!normalizedBack || !normalizedToken) {
            return '';
        }

        try {
            const backURL = new URL(normalizedBack, window.location.origin);
            if (backURL.origin !== window.location.origin) {
                return normalizedBack;
            }
            backURL.searchParams.set(this.modalRestoreParam, normalizedToken);
            return `${backURL.pathname}${backURL.search}${backURL.hash}`;
        } catch (error) {
            return '';
        }
    },

    replaceCurrentHistoryURL(nextURL = '') {
        const normalizedURL = String(nextURL || '').trim();
        if (!normalizedURL) {
            return false;
        }

        try {
            const resolvedURL = new URL(normalizedURL, window.location.origin);
            if (resolvedURL.origin !== window.location.origin) {
                return false;
            }
            window.history.replaceState(window.history.state, '', `${resolvedURL.pathname}${resolvedURL.search}${resolvedURL.hash}`);
            return true;
        } catch (error) {
            return false;
        }
    },

    buildResourceViewerNavigationURL(kind, src, back = window.location.href, options = {}) {
        const resolvedBack = this.buildResourceViewerBackURL(back, options);
        return this.resourceViewerURL(kind, src, resolvedBack || back);
    },

    openResourceViewer(kind, src, back = window.location.href, options = {}) {
        const targetURL = this.buildResourceViewerNavigationURL(kind, src, back, {
            replaceCurrentHistory: true,
            ...options
        });
        window.location.href = targetURL;
        return targetURL;
    },

    parseResourceViewerNavigationURL(href = '') {
        try {
            const url = new URL(String(href || ''), window.location.origin);
            if (url.origin !== window.location.origin) {
                return null;
            }

            const pathname = (url.pathname || '').replace(/\/+$/, '') || '/';
            if (pathname !== '/viewer') {
                return null;
            }

            const kind = String(url.searchParams.get('kind') || '').trim();
            const src = String(url.searchParams.get('src') || '').trim();
            if (!kind || !src) {
                return null;
            }

            return {
                kind,
                src,
                back: String(url.searchParams.get('back') || window.location.href).trim()
            };
        } catch (error) {
            return null;
        }
    },

    bindResourceViewerLinks() {
        if (this.resourceViewerLinksBound) {
            return;
        }

        this.resourceViewerLinksBound = true;
        document.addEventListener('click', (event) => {
            if (event.defaultPrevented || event.button !== 0) {
                return;
            }
            if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
                return;
            }

            const targetElement = event.target instanceof Element ? event.target : null;
            const link = targetElement?.closest('a[href]');
            if (!link || link.hasAttribute('download')) {
                return;
            }

            const target = String(link.getAttribute('target') || '').trim().toLowerCase();
            if (target && target !== '_self') {
                return;
            }

            const resource = this.parseResourceViewerNavigationURL(link.href);
            if (!resource) {
                return;
            }

            event.preventDefault();
            this.openResourceViewer(resource.kind, resource.src, resource.back);
        });
    },

    captureModalRestoreState() {
        if (typeof NoteViewer !== 'undefined' && this.isVisibleModal(NoteViewer.modal) && NoteViewer.currentFigure?.id && NoteViewer.currentFigure?.paper_id) {
            return {
                modal: 'figure-note',
                paperId: Number(NoteViewer.currentFigure.paper_id),
                figureId: Number(NoteViewer.currentFigure.id)
            };
        }

        if (typeof FigureViewer !== 'undefined' && this.isVisibleModal(FigureViewer.modal) && FigureViewer.currentFigure?.id && FigureViewer.currentFigure?.paper_id) {
            return {
                modal: 'figure',
                paperId: Number(FigureViewer.currentFigure.paper_id),
                figureId: Number(FigureViewer.currentFigure.id)
            };
        }

        if (typeof PaperNoteViewer !== 'undefined' && this.isVisibleModal(PaperNoteViewer.modal) && PaperNoteViewer.paper?.id) {
            return {
                modal: 'paper-note',
                paperId: Number(PaperNoteViewer.paper.id)
            };
        }

        if (typeof PaperViewer !== 'undefined' && this.isVisibleModal(PaperViewer.modal) && PaperViewer.paper?.id) {
            return {
                modal: 'paper',
                paperId: Number(PaperViewer.paper.id)
            };
        }

        return null;
    },

    isVisibleModal(modal) {
        return Boolean(modal && !modal.classList.contains('hidden'));
    },

    isTopVisibleModal(modal) {
        if (!this.isVisibleModal(modal)) {
            return false;
        }

        const visibleModals = Array.from(document.querySelectorAll('.modal-shell:not(.hidden)'));
        return visibleModals.length > 0 && visibleModals[visibleModals.length - 1] === modal;
    },

    storeModalRestoreState(state) {
        try {
            const token = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`;
            window.sessionStorage.setItem(`${this.modalRestoreStoragePrefix}${token}`, JSON.stringify(state));
            return token;
        } catch (error) {
            return '';
        }
    },

    consumeModalRestoreState() {
        const url = new URL(window.location.href);
        const token = String(url.searchParams.get(this.modalRestoreParam) || '').trim();
        if (!token) {
            return null;
        }

        url.searchParams.delete(this.modalRestoreParam);
        window.history.replaceState({}, '', `${url.pathname}${url.search}${url.hash}`);

        try {
            const storageKey = `${this.modalRestoreStoragePrefix}${token}`;
            const raw = window.sessionStorage.getItem(storageKey);
            window.sessionStorage.removeItem(storageKey);
            if (!raw) {
                return null;
            }
            return JSON.parse(raw);
        } catch (error) {
            return null;
        }
    },

    isModalRestoreStateActive(state) {
        if (!state) {
            return false;
        }

        const modal = String(state.modal || '').trim();
        const paperId = Number(state.paperId);
        const figureId = Number(state.figureId);

        if (modal === 'figure-note') {
            return typeof NoteViewer !== 'undefined'
                && this.isVisibleModal(NoteViewer.modal)
                && Number(NoteViewer.currentFigure?.id) === figureId
                && Number(NoteViewer.currentFigure?.paper_id) === paperId;
        }

        if (modal === 'figure') {
            return typeof FigureViewer !== 'undefined'
                && this.isVisibleModal(FigureViewer.modal)
                && Number(FigureViewer.currentFigure?.id) === figureId
                && Number(FigureViewer.currentFigure?.paper_id) === paperId;
        }

        if (modal === 'paper-note') {
            return typeof PaperNoteViewer !== 'undefined'
                && this.isVisibleModal(PaperNoteViewer.modal)
                && Number(PaperNoteViewer.paper?.id) === paperId;
        }

        if (modal === 'paper') {
            return typeof PaperViewer !== 'undefined'
                && this.isVisibleModal(PaperViewer.modal)
                && Number(PaperViewer.paper?.id) === paperId;
        }

        return false;
    },

    async restoreModalState(state) {
        if (!state || typeof PaperViewer === 'undefined') {
            return false;
        }

        if (this.isModalRestoreStateActive(state)) {
            return true;
        }

        const paperId = Number(state.paperId);
        if (!paperId || !document.getElementById('paperModal')) {
            return false;
        }

        try {
            await PaperViewer.open(paperId);

            if (state.modal === 'paper-note' && typeof PaperViewer.openPaperNotes === 'function') {
                PaperViewer.openPaperNotes();
                return true;
            }

            if (state.modal !== 'figure' && state.modal !== 'figure-note') {
                return true;
            }

            if (typeof FigureViewer === 'undefined') {
                return true;
            }

            const figureId = Number(state.figureId);
            const figures = PaperViewer.paper?.figures || [];
            const figureIndex = figures.findIndex((figure) => Number(figure.id) === figureId);
            if (figureIndex < 0) {
                return true;
            }

            await PaperViewer.openFigurePreview(figureIndex);

            if (state.modal === 'figure-note' && typeof FigureViewer.openNotes === 'function') {
                await FigureViewer.openNotes();
            }
            return true;
        } catch (error) {
            return false;
        }
    },

    blobToBase64(blob) {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = () => {
                const result = String(reader.result || '');
                const commaIndex = result.indexOf(',');
                resolve(commaIndex >= 0 ? result.slice(commaIndex + 1) : result);
            };
            reader.onerror = () => reject(reader.error || new Error('读取文件失败'));
            reader.readAsDataURL(blob);
        });
    },

    triggerBlobDownload(blob, filename = 'download.bin') {
        const objectURL = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = objectURL;
        link.download = filename || 'download.bin';
        document.body.appendChild(link);
        link.click();
        link.remove();
        setTimeout(() => URL.revokeObjectURL(objectURL), 1000);
    },

    async saveBlobDownload(blob, filename = 'download.bin') {
        if (Utils.supportsDesktopSave()) {
            const base64 = await Utils.blobToBase64(blob);
            const result = await window.citeboxDesktopSaveFile(filename || 'download.bin', base64);
            return Boolean(result && result.saved);
        }

        Utils.triggerBlobDownload(blob, filename);
        return true;
    },

    debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    },

    confirm(message, title = '确认') {
        return new Promise((resolve) => {
            const overlay = document.createElement('div');
            overlay.className = 'dialog-overlay';
            overlay.innerHTML = `
                <div class="dialog-box">
                    <div class="dialog-header">
                        <h3>${title}</h3>
                    </div>
                    <div class="dialog-body">
                        <p>${message}</p>
                    </div>
                    <div class="dialog-footer">
                        <button class="btn btn-outline dialog-cancel">取消</button>
                        <button class="btn btn-danger dialog-confirm">确定</button>
                    </div>
                </div>
            `;
            document.body.appendChild(overlay);
            
            // 动画显示
            requestAnimationFrame(() => overlay.classList.add('active'));

            const close = (result) => {
                overlay.classList.remove('active');
                setTimeout(() => overlay.remove(), 200);
                resolve(result);
            };

            overlay.querySelector('.dialog-cancel').onclick = () => close(false);
            overlay.querySelector('.dialog-confirm').onclick = () => close(true);
            overlay.onclick = (e) => { if (e.target === overlay) close(false); };
        });
    },

    confirmTypedAction(options = {}) {
        const {
            title = '危险操作确认',
            message = '',
            keyword = 'CLEAR',
            confirmLabel = '确认继续',
            hint = `请输入 ${keyword} 继续`,
            badge = 'Danger Zone'
        } = options;

        return new Promise((resolve) => {
            const normalizedKeyword = String(keyword || '').trim();
            const overlay = document.createElement('div');
            overlay.className = 'dialog-overlay';
            overlay.innerHTML = `
                <div class="dialog-box dialog-box-danger">
                    <div class="dialog-danger-head">
                        <span class="dialog-danger-badge">${Utils.escapeHTML(badge)}</span>
                        <div class="dialog-header">
                            <h3>${Utils.escapeHTML(title)}</h3>
                        </div>
                    </div>
                    <div class="dialog-body dialog-danger-body">
                        <p class="dialog-danger-message">${Utils.escapeHTML(message)}</p>
                        <div class="dialog-danger-instruction">
                            <span>确认口令</span>
                            <strong>${Utils.escapeHTML(normalizedKeyword)}</strong>
                        </div>
                        <label class="dialog-danger-field">
                            <span>${Utils.escapeHTML(hint)}</span>
                            <input class="form-input dialog-confirm-input" type="text" autocomplete="off" spellcheck="false" placeholder="${Utils.escapeHTML(normalizedKeyword)}">
                        </label>
                    </div>
                    <div class="dialog-footer">
                        <button class="btn btn-outline dialog-cancel">取消</button>
                        <button class="btn btn-outline danger dialog-confirm" type="button" disabled>${Utils.escapeHTML(confirmLabel)}</button>
                    </div>
                </div>
            `;
            document.body.appendChild(overlay);
            requestAnimationFrame(() => overlay.classList.add('active'));

            const input = overlay.querySelector('.dialog-confirm-input');
            const confirmButton = overlay.querySelector('.dialog-confirm');

            const close = (result) => {
                document.removeEventListener('keydown', onKeydown);
                overlay.classList.remove('active');
                setTimeout(() => overlay.remove(), 200);
                resolve(result);
            };

            const syncState = () => {
                confirmButton.disabled = input.value.trim() !== normalizedKeyword;
            };

            const onKeydown = (event) => {
                if (event.key === 'Escape') {
                    event.preventDefault();
                    event.stopPropagation();
                    close(false);
                }
            };

            input.addEventListener('input', syncState);
            input.addEventListener('keydown', (event) => {
                if (event.key === 'Enter' && !confirmButton.disabled) {
                    event.preventDefault();
                    close(true);
                }
            });

            overlay.querySelector('.dialog-cancel').onclick = () => close(false);
            confirmButton.onclick = () => close(true);
            overlay.onclick = (event) => {
                if (event.target === overlay) close(false);
            };
            document.addEventListener('keydown', onKeydown);
            setTimeout(() => input.focus(), 0);
        });
    },

    alert(message, title = '提示') {
        return new Promise((resolve) => {
            const overlay = document.createElement('div');
            overlay.className = 'dialog-overlay';
            overlay.innerHTML = `
                <div class="dialog-box">
                    <div class="dialog-header">
                        <h3>${title}</h3>
                    </div>
                    <div class="dialog-body">
                        <p>${message}</p>
                    </div>
                    <div class="dialog-footer">
                        <button class="btn btn-primary dialog-ok">确定</button>
                    </div>
                </div>
            `;
            document.body.appendChild(overlay);
            
            requestAnimationFrame(() => overlay.classList.add('active'));

            const close = () => {
                overlay.classList.remove('active');
                setTimeout(() => overlay.remove(), 200);
                resolve();
            };

            overlay.querySelector('.dialog-ok').onclick = close;
            overlay.onclick = (e) => { if (e.target === overlay) close(); };
        });
    },

    promptFields(options = {}) {
        const {
            title = '编辑',
            description = '',
            confirmLabel = '保存',
            fields = []
        } = options;

        return new Promise((resolve) => {
            const normalizedFields = Array.isArray(fields) ? fields : [];
            const overlay = document.createElement('div');
            overlay.className = 'dialog-overlay';
            overlay.innerHTML = `
                <div class="dialog-box dialog-box-form">
                    <div class="dialog-header">
                        <h3>${Utils.escapeHTML(title)}</h3>
                    </div>
                    <form class="dialog-form">
                        ${description ? `<p class="dialog-form-description">${Utils.escapeHTML(description)}</p>` : ''}
                        <div class="dialog-form-fields">
                            ${normalizedFields.map((field) => {
                                const type = field.type === 'textarea' ? 'textarea' : (field.type || 'text');
                                const name = Utils.escapeHTML(field.name || '');
                                const label = Utils.escapeHTML(field.label || field.name || '');
                                const value = Utils.escapeHTML(field.value ?? '');
                                const placeholder = Utils.escapeHTML(field.placeholder || '');
                                const required = field.required ? ' required' : '';
                                const autocomplete = field.autocomplete ? ` autocomplete="${Utils.escapeHTML(field.autocomplete)}"` : ' autocomplete="off"';
                                if (type === 'textarea') {
                                    return `
                                        <label class="dialog-form-field">
                                            <span>${label}</span>
                                            <textarea class="form-textarea" name="${name}" rows="${Number(field.rows) || 4}" placeholder="${placeholder}"${required}${autocomplete}>${value}</textarea>
                                        </label>
                                    `;
                                }
                                const inputClass = type === 'color' ? 'color-input dialog-form-color-input' : 'form-input';
                                return `
                                    <label class="dialog-form-field">
                                        <span>${label}</span>
                                        <input class="${inputClass}" type="${Utils.escapeHTML(type)}" name="${name}" value="${value}" placeholder="${placeholder}"${required}${autocomplete}>
                                    </label>
                                `;
                            }).join('')}
                        </div>
                        <div class="dialog-footer">
                            <button class="btn btn-outline dialog-cancel" type="button">取消</button>
                            <button class="btn btn-primary dialog-confirm" type="submit">${Utils.escapeHTML(confirmLabel)}</button>
                        </div>
                    </form>
                </div>
            `;
            document.body.appendChild(overlay);
            requestAnimationFrame(() => overlay.classList.add('active'));

            const form = overlay.querySelector('.dialog-form');
            const confirmButton = overlay.querySelector('.dialog-confirm');
            const inputs = Array.from(form.querySelectorAll('[name]'));

            const collectValues = () => Object.fromEntries(inputs.map((input) => [input.name, input.value]));
            const validate = () => {
                const invalid = inputs.some((input) => input.required && !input.value.trim());
                confirmButton.disabled = invalid;
            };

            const close = (result) => {
                document.removeEventListener('keydown', onKeydown);
                overlay.classList.remove('active');
                setTimeout(() => overlay.remove(), 200);
                resolve(result);
            };

            const onKeydown = (event) => {
                if (event.key === 'Escape') {
                    event.preventDefault();
                    event.stopPropagation();
                    close(null);
                }
            };

            form.addEventListener('submit', (event) => {
                event.preventDefault();
                if (confirmButton.disabled) return;
                close(collectValues());
            });

            inputs.forEach((input) => {
                input.addEventListener('input', validate);
            });

            overlay.querySelector('.dialog-cancel').onclick = () => close(null);
            overlay.onclick = (event) => {
                if (event.target === overlay) close(null);
            };

            document.addEventListener('keydown', onKeydown);
            validate();
            setTimeout(() => inputs[0]?.focus(), 0);
        });
    },

    escapeHTML(value = '') {
        return String(value)
            .replaceAll('&', '&amp;')
            .replaceAll('<', '&lt;')
            .replaceAll('>', '&gt;')
            .replaceAll('"', '&quot;')
            .replaceAll("'", '&#39;');
    },

    renderMarkdown(value = '', options = {}) {
        const source = String(value || '').replace(/\r\n?/g, '\n').trim();
        if (!source) {
            return '<div class="markdown-empty">暂无笔记内容</div>';
        }

        const placeholders = [];
        const stash = (html, tokenPrefix = 'MDTOKEN') => {
            const token = `%%${tokenPrefix}${placeholders.length}%%`;
            placeholders.push({ token, html });
            return token;
        };

        let text = Utils.escapeHTML(source);
        text = text.replace(/```([a-zA-Z0-9_-]+)?\n?([\s\S]*?)```/g, (_, language = '', code = '') => {
            const normalizedLanguage = String(language || '').trim();
            const languageBadge = normalizedLanguage
                ? `<span class="markdown-code-label">${Utils.escapeHTML(normalizedLanguage)}</span>`
                : '';
            const codeClass = normalizedLanguage ? ` class="language-${Utils.escapeHTML(normalizedLanguage)}"` : '';
            const normalizedCode = String(code || '').replace(/^\n+|\n+$/g, '');
            return stash(`
                <div class="markdown-code-shell">
                    ${languageBadge}
                    <pre class="markdown-code-block"><code${codeClass}>${normalizedCode}</code></pre>
                </div>
            `, 'MDBLOCKTOKEN');
        });
        text = text.replace(/`([^`\n]+)`/g, (_, code) => stash(`<code class="markdown-inline-code">${code}</code>`, 'MDINLINEROOTTOKEN'));

        const lines = text.split('\n').map((line) => line.trimEnd());
        let html = Utils.renderMarkdownBlocks(lines, options);
        placeholders.forEach(({ token, html: fragment }) => {
            html = html.replaceAll(token, fragment);
        });
        return html;
    },

    renderMarkdownBlocks(lines = [], options = {}) {
        const blocks = [];
        let index = 0;

        while (index < lines.length) {
            const line = lines[index];
            if (Utils.isMarkdownBlankLine(line)) {
                index += 1;
                continue;
            }

            const placeholder = Utils.renderMarkdownPlaceholderBlock(line);
            if (placeholder) {
                blocks.push(placeholder);
                index += 1;
                continue;
            }

            const imageBlock = Utils.renderMarkdownImageBlock(line, options);
            if (imageBlock) {
                blocks.push(imageBlock);
                index += 1;
                continue;
            }

            const divider = Utils.renderMarkdownHorizontalRule(line);
            if (divider) {
                blocks.push(divider);
                index += 1;
                continue;
            }

            const heading = Utils.renderMarkdownHeadingBlock(line, options);
            if (heading) {
                blocks.push(heading);
                index += 1;
                continue;
            }

            if (Utils.isMarkdownBlockquoteLine(line)) {
                const blockquote = Utils.consumeMarkdownBlockquote(lines, index, options);
                blocks.push(blockquote.html);
                index = blockquote.nextIndex;
                continue;
            }

            const listMarker = Utils.getMarkdownListMarker(line);
            if (listMarker) {
                const list = Utils.consumeMarkdownList(lines, index, options, listMarker);
                blocks.push(list.html);
                index = list.nextIndex;
                continue;
            }

            const paragraph = Utils.consumeMarkdownParagraph(lines, index, options);
            blocks.push(paragraph.html);
            index = paragraph.nextIndex;
        }

        return blocks.join('');
    },

    consumeMarkdownParagraph(lines = [], startIndex = 0, options = {}) {
        const paragraphLines = [];
        let index = startIndex;

        while (index < lines.length) {
            const line = lines[index];
            if (Utils.isMarkdownBlankLine(line)) {
                break;
            }
            if (index > startIndex && Utils.isMarkdownBlockBoundaryLine(line)) {
                break;
            }
            paragraphLines.push(line);
            index += 1;
        }

        return {
            html: Utils.renderMarkdownFlowBlock(paragraphLines, options),
            nextIndex: index
        };
    },

    consumeMarkdownBlockquote(lines = [], startIndex = 0, options = {}) {
        const quoteLines = [];
        let index = startIndex;

        while (index < lines.length && Utils.isMarkdownBlockquoteLine(lines[index])) {
            quoteLines.push(lines[index].replace(/^\s*&gt;\s?/, ''));
            index += 1;
        }

        return {
            html: `<blockquote class="markdown-blockquote">${Utils.renderMarkdownBlocks(quoteLines, options)}</blockquote>`,
            nextIndex: index
        };
    },

    consumeMarkdownList(lines = [], startIndex = 0, options = {}, firstMarker = null) {
        const marker = firstMarker || Utils.getMarkdownListMarker(lines[startIndex]);
        if (!marker) {
            return { html: '', nextIndex: startIndex };
        }

        const items = [];
        let index = startIndex;

        while (index < lines.length) {
            const currentMarker = Utils.getMarkdownListMarker(lines[index]);
            if (!currentMarker || currentMarker.type !== marker.type || currentMarker.indent !== marker.indent) {
                break;
            }

            const item = Utils.consumeMarkdownListItem(lines, index, options, currentMarker);
            items.push(item.html);
            index = item.nextIndex;
        }

        const tagName = marker.type === 'ordered' ? 'ol' : 'ul';
        const orderedClass = marker.type === 'ordered' ? ' markdown-list-ordered' : '';
        const startAttr = marker.type === 'ordered' && marker.start > 1 ? ` start="${marker.start}"` : '';

        return {
            html: `
                <${tagName} class="markdown-list${orderedClass}"${startAttr}>
                    ${items.join('')}
                </${tagName}>
            `,
            nextIndex: index
        };
    },

    consumeMarkdownListItem(lines = [], startIndex = 0, options = {}, marker = null) {
        const currentMarker = marker || Utils.getMarkdownListMarker(lines[startIndex]);
        if (!currentMarker) {
            return { html: '', nextIndex: startIndex };
        }

        const itemLines = [currentMarker.content];
        let index = startIndex + 1;

        while (index < lines.length) {
            const line = lines[index];

            if (Utils.isMarkdownBlankLine(line)) {
                const nextIndex = Utils.findNextMarkdownNonBlankLine(lines, index + 1);
                if (nextIndex < 0) {
                    index = lines.length;
                    break;
                }

                const nextMarker = Utils.getMarkdownListMarker(lines[nextIndex]);
                if (nextMarker && nextMarker.type === currentMarker.type && nextMarker.indent === currentMarker.indent) {
                    index = nextIndex;
                    break;
                }
                if (Utils.isMarkdownBlockBoundaryLine(lines[nextIndex]) && Utils.getMarkdownLineIndent(lines[nextIndex]) <= currentMarker.indent) {
                    index = nextIndex;
                    break;
                }

                itemLines.push('');
                index += 1;
                continue;
            }

            const siblingMarker = Utils.getMarkdownListMarker(line);
            if (siblingMarker && siblingMarker.type === currentMarker.type && siblingMarker.indent === currentMarker.indent) {
                break;
            }
            if (Utils.getMarkdownLineIndent(line) < currentMarker.indent) {
                break;
            }

            const stripLength = Math.min(Utils.getMarkdownLineIndent(line), currentMarker.contentOffset);
            itemLines.push(line.slice(stripLength));
            index += 1;
        }

        return {
            html: `<li>${Utils.renderMarkdownBlocks(itemLines, options)}</li>`,
            nextIndex: index
        };
    },

    findNextMarkdownNonBlankLine(lines = [], startIndex = 0) {
        for (let index = startIndex; index < lines.length; index += 1) {
            if (!Utils.isMarkdownBlankLine(lines[index])) {
                return index;
            }
        }
        return -1;
    },

    getMarkdownListMarker(value = '') {
        const line = String(value || '');
        const match = line.match(/^(\s*)([-*+]|\d+\.)\s+(.*)$/);
        if (!match) {
            return null;
        }

        const indent = match[1].length;
        const markerText = match[2];
        const content = match[3];
        const contentOffset = match[0].length - content.length;
        const ordered = /\d+\./.test(markerText);

        return {
            indent,
            markerText,
            content,
            contentOffset,
            type: ordered ? 'ordered' : 'unordered',
            start: ordered ? parseInt(markerText, 10) : 1
        };
    },

    getMarkdownLineIndent(value = '') {
        const match = String(value || '').match(/^[ \t]*/);
        return match ? match[0].length : 0;
    },

    isMarkdownBlankLine(value = '') {
        return !String(value || '').trim();
    },

    isMarkdownPlaceholderLine(value = '') {
        return /^%%MDBLOCKTOKEN\d+%%$/.test(String(value || '').trim());
    },

    isMarkdownHorizontalRuleLine(value = '') {
        return /^(-{3,}|\*{3,}|_{3,})$/.test(String(value || '').trim());
    },

    isMarkdownImageOnlyLine(value = '') {
        return /^!\[([^\]]*)\]\(([^)\s]+)\)$/.test(String(value || '').trim());
    },

    isMarkdownHeadingLine(value = '') {
        return /^(#{1,6})\s+(.+)$/.test(String(value || '').trim());
    },

    isMarkdownBlockquoteLine(value = '') {
        return /^\s*&gt;(?:\s|$)/.test(String(value || ''));
    },

    isMarkdownBlockBoundaryLine(value = '') {
        return Utils.isMarkdownPlaceholderLine(value)
            || Utils.isMarkdownHorizontalRuleLine(value)
            || Utils.isMarkdownImageOnlyLine(value)
            || Utils.isMarkdownHeadingLine(value)
            || Utils.isMarkdownBlockquoteLine(value)
            || Boolean(Utils.getMarkdownListMarker(value));
    },

    renderMarkdownPlaceholderBlock(value = '') {
        return Utils.isMarkdownPlaceholderLine(value) ? String(value || '').trim() : '';
    },

    renderMarkdownHorizontalRule(value = '') {
        return Utils.isMarkdownHorizontalRuleLine(value) ? '<hr class="markdown-divider">' : '';
    },

    renderMarkdownHeadingBlock(value = '', options = {}) {
        const heading = String(value || '').trim().match(/^(#{1,6})\s+(.+)$/);
        if (!heading) {
            return '';
        }

        const level = heading[1].length;
        return `<h${level} class="markdown-heading markdown-heading-${level}">${Utils.renderMarkdownInline(heading[2], options)}</h${level}>`;
    },

    renderMarkdownInline(value = '', options = {}) {
        let text = String(value || '');
        const placeholders = [];
        const stash = (html) => {
            const token = `%%MDINLINETOKEN${placeholders.length}%%`;
            placeholders.push({ token, html });
            return token;
        };

        text = text.replace(/!\[([^\]]*)\]\(([^)\s]+)\)/g, (_, altText, src) => {
            return stash(
                Utils.renderMarkdownImageHTML(altText, src, options, 'markdown-image markdown-inline-image')
                    || altText
                    || '[图片不可用]'
            );
        });
        text = text.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (_, label, href) => {
            const safeHref = Utils.safeMarkdownHref(href);
            if (!safeHref) {
                return label;
            }
            return stash(`<a class="markdown-link" href="${safeHref}" target="_blank" rel="noreferrer">${label}</a>`);
        });
        text = text.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
        text = text.replace(/__([^_]+)__/g, '<strong>$1</strong>');
        text = text.replace(/(^|[^*])\*([^*]+)\*(?!\*)/g, '$1<em>$2</em>');
        text = text.replace(/(^|[^_])_([^_]+)_(?!_)/g, '$1<em>$2</em>');
        text = text.replace(/~~([^~]+)~~/g, '<del>$1</del>');
        placeholders.forEach(({ token, html }) => {
            text = text.replaceAll(token, html);
        });

        return text;
    },

    renderMarkdownImageBlock(value = '', options = {}) {
        const match = String(value || '').trim().match(/^!\[([^\]]*)\]\(([^)\s]+)\)$/);
        if (!match) return '';

        const [, altText = '', src = ''] = match;
        const figureHTML = Utils.renderMarkdownImageFigure(altText, src, options);
        if (!figureHTML) {
            const fallback = altText || '图片不可用';
            return `<p class="markdown-paragraph">${fallback}</p>`;
        }

        return figureHTML;
    },

    renderMarkdownImageHTML(altText = '', src = '', options = {}, className = 'markdown-image') {
        const safeSrc = Utils.resolveMarkdownImageSrc(src, options);
        if (!safeSrc) return '';

        return `<img class="${className}" src="${safeSrc}" alt="${altText}" loading="lazy" decoding="async">`;
    },

    renderMarkdownImageFigure(altText = '', src = '', options = {}) {
        const imageHTML = Utils.renderMarkdownImageHTML(altText, src, options, 'markdown-image');
        if (!imageHTML) return '';

        const caption = String(altText || '').trim();
        return `
            <figure class="markdown-figure">
                ${imageHTML}
                ${caption ? `<figcaption class="markdown-figcaption">${caption}</figcaption>` : ''}
            </figure>
        `;
    },

    renderMarkdownFlowBlock(lines = [], options = {}, wrapperTag = 'p') {
        const segments = [];
        let textBuffer = '';

        const flushText = () => {
            if (!textBuffer) return;
            segments.push(`<${wrapperTag} class="markdown-paragraph">${textBuffer}</${wrapperTag}>`);
            textBuffer = '';
        };

        lines.forEach((line, lineIndex) => {
            const pieces = Utils.splitMarkdownFigureSegments(line, options);
            pieces.forEach((piece, pieceIndex) => {
                if (piece.type === 'text') {
                    const html = Utils.renderMarkdownInline(piece.value, options);
                    if (!html) return;
                    if (textBuffer && pieceIndex === 0 && lineIndex > 0) {
                        textBuffer += '<br>';
                    }
                    textBuffer += html;
                    return;
                }

                flushText();
                segments.push(piece.html);
            });
        });

        flushText();
        return segments.join('');
    },

    splitMarkdownFigureSegments(value = '', options = {}) {
        const source = String(value || '');
        const regex = /!\[([^\]]*)\]\(([^)\s]+)\)/g;
        const segments = [];
        let cursor = 0;
        let match;

        while ((match = regex.exec(source)) !== null) {
            const [token, altText = '', src = ''] = match;
            const textBefore = source.slice(cursor, match.index);
            const [suffix, consumedLength] = Utils.extractMarkdownFigureSuffix(source.slice(match.index + token.length));

            if (textBefore || altText || suffix) {
                segments.push({
                    type: 'text',
                    value: `${textBefore}${altText}${suffix}`
                });
            }

            const figureHTML = Utils.renderMarkdownImageFigure(altText, src, options);
            if (figureHTML) {
                segments.push({
                    type: 'figure',
                    html: figureHTML
                });
            }

            cursor = match.index + token.length + consumedLength;
        }

        const tail = source.slice(cursor);
        if (tail || !segments.length) {
            segments.push({
                type: 'text',
                value: tail
            });
        }

        return segments;
    },

    extractMarkdownFigureSuffix(value = '') {
        const text = String(value || '');
        const match = text.match(/^[\s]*[)）\]】》」』"'’”]*[、，,。：；;.!！？?]*/);
        return match ? [match[0], match[0].length] : ['', 0];
    },

    renderMarkdownListItem(value = '', options = {}) {
        return `<li>${Utils.renderMarkdownFlowBlock([value], options, 'div')}</li>`;
    },

    resolveMarkdownImageSrc(value = '', options = {}) {
        const raw = String(value || '').trim();
        if (!raw) return '';

        const figureMatch = raw.match(/^figure:\/\/(\d+)$/i);
        if (figureMatch) {
            if (typeof options.resolveFigureSrc !== 'function') {
                return '';
            }
            return Utils.safeMarkdownImageSrc(options.resolveFigureSrc(Number(figureMatch[1])) || '');
        }

        return Utils.safeMarkdownImageSrc(raw);
    },

    safeMarkdownHref(value = '') {
        const href = String(value || '').trim();
        if (!href) return '';
        if (/^(https?:|mailto:)/i.test(href)) return href;
        if (href.startsWith('/')) return href;
        return '';
    },

    safeMarkdownImageSrc(value = '') {
        const src = String(value || '').trim();
        if (!src) return '';
        if (src.startsWith('/files/figures/')) return src;
        return '';
    },

    buildPaginationItems(currentPage = 1, totalPages = 0) {
        const total = Math.max(0, Number(totalPages) || 0);
        const current = Math.min(Math.max(1, Number(currentPage) || 1), Math.max(total, 1));

        if (!total) {
            return [];
        }

        if (total <= 7) {
            return Array.from({ length: total }, (_, index) => index + 1);
        }

        const items = [1];
        let start = current <= 4 ? 2 : current - 1;
        let end = current >= total - 3 ? total - 1 : current + 1;

        if (current <= 4) {
            end = 5;
        }

        if (current >= total - 3) {
            start = total - 4;
        }

        start = Math.max(2, start);
        end = Math.min(total - 1, end);

        if (start > 2) {
            items.push('ellipsis');
        }

        for (let page = start; page <= end; page += 1) {
            items.push(page);
        }

        if (end < total - 1) {
            items.push('ellipsis');
        }

        items.push(total);
        return items;
    },

    normalizePaginationPage(value, totalPages = 0) {
        const total = Math.max(0, Number(totalPages) || 0);
        const page = Number(String(value ?? '').trim());

        if (!total || !Number.isInteger(page) || page < 1 || page > total) {
            return null;
        }

        return page;
    },

    renderPagination(container, currentPage = 1, totalPages = 0) {
        if (!container) return;

        const total = Math.max(0, Number(totalPages) || 0);
        const current = Math.min(Math.max(1, Number(currentPage) || 1), Math.max(total, 1));
        container.dataset.currentPage = String(current);
        container.dataset.totalPages = String(total);

        if (total <= 1) {
            container.innerHTML = '';
            return;
        }

        const jumpInputId = `${container.id || 'pagination'}JumpInput`;
        const pageButtons = Utils.buildPaginationItems(current, total).map((item) => {
            if (item === 'ellipsis') {
                return '<span class="pagination-ellipsis" aria-hidden="true">...</span>';
            }

            return `
                <button class="${item === current ? 'active' : ''}" type="button" data-page="${item}" ${item === current ? 'aria-current="page"' : ''}>
                    ${item}
                </button>
            `;
        }).join('');

        container.innerHTML = `
            <button class="pagination-nav" type="button" data-page="${current - 1}" ${current <= 1 ? 'disabled' : ''}>上一页</button>
            ${pageButtons}
            <button class="pagination-nav" type="button" data-page="${current + 1}" ${current >= total ? 'disabled' : ''}>下一页</button>
            <span class="pagination-meta">第 ${current} / ${total} 页</span>
            <form class="pagination-jump" data-pagination-jump-form>
                <label class="pagination-jump-label" for="${jumpInputId}">跳至</label>
                <input id="${jumpInputId}" class="form-input pagination-jump-input" type="number" min="1" max="${total}" step="1" value="${current}" inputmode="numeric" data-pagination-input>
                <button class="pagination-jump-button" type="submit" data-pagination-jump>跳转</button>
            </form>
        `;
    },

    bindPagination(container, onPageChange) {
        if (!container || typeof onPageChange !== 'function' || container.dataset.paginationBound === 'true') {
            return;
        }

        const navigate = async (value, input) => {
            const totalPages = Number(container.dataset.totalPages || 0);
            const targetPage = Utils.normalizePaginationPage(value, totalPages);

            if (targetPage === null) {
                if (totalPages > 0) {
                    Utils.showToast(`请输入 1 - ${totalPages} 的页码`, 'error');
                }
                if (input) {
                    input.focus();
                    if (typeof input.select === 'function') {
                        input.select();
                    }
                }
                return;
            }

            const currentPage = Number(container.dataset.currentPage || 0);
            if (targetPage === currentPage) {
                return;
            }

            await onPageChange(targetPage);
        };

        container.addEventListener('click', async (event) => {
            const pageButton = event.target.closest('button[data-page]');
            if (pageButton) {
                if (pageButton.disabled) return;
                await navigate(pageButton.dataset.page);
            }
        });

        container.addEventListener('submit', async (event) => {
            const form = event.target.closest('form[data-pagination-jump-form]');
            if (!form) return;
            event.preventDefault();

            const input = form.querySelector('input[data-pagination-input]');
            if (!input) return;

            await navigate(input.value, input);
        }, true);

        container.dataset.paginationBound = 'true';
    },

    splitTags(value = '') {
        return value
            .split(',')
            .map((item) => item.trim())
            .filter(Boolean);
    },

    joinTags(tags = []) {
        return tags.map((tag) => tag.name || tag).join(', ');
    },

    normalizeTagScope(scope = '') {
        return String(scope || '').trim().toLowerCase() === 'figure' ? 'figure' : 'paper';
    },

    tagCountKey(scope = '') {
        return this.normalizeTagScope(scope) === 'figure' ? 'figure_count' : 'paper_count';
    },

    tagCatalog(scope = '') {
        const normalizedScope = this.normalizeTagScope(scope);
        if (!this.tagCatalogs.has(normalizedScope)) {
            this.tagCatalogs.set(normalizedScope, new Map());
        }
        return this.tagCatalogs.get(normalizedScope);
    },

    mergeScopedTagCatalog(scope = '', tags = []) {
        const normalizedScope = this.normalizeTagScope(scope);
        const catalog = this.tagCatalog(normalizedScope);
        const countKey = this.tagCountKey(normalizedScope);

        (Array.isArray(tags) ? tags : []).forEach((rawTag) => {
            const name = String(typeof rawTag === 'string' ? rawTag : rawTag?.name || '').trim();
            if (!name) return;

            const key = name.toLowerCase();
            const existing = catalog.get(key) || {};
            const rawCount = typeof rawTag === 'string'
                ? 1
                : Number(rawTag?.[countKey] ?? existing[countKey] ?? 0);

            catalog.set(key, {
                name,
                [countKey]: Math.max(Number(existing[countKey] || 0), rawCount)
            });
        });

        return catalog;
    },

    ensureScopedTagCatalogLoaded(scope = '') {
        const normalizedScope = this.normalizeTagScope(scope);
        const catalog = this.tagCatalog(normalizedScope);
        if (catalog.size > 0) {
            return Promise.resolve(catalog);
        }
        if (this.tagCatalogPromises.has(normalizedScope)) {
            return this.tagCatalogPromises.get(normalizedScope);
        }
        if (typeof API === 'undefined' || typeof API.listTags !== 'function') {
            return Promise.resolve(catalog);
        }

        const promise = API.listTags({ scope: normalizedScope })
            .then((payload) => {
                this.mergeScopedTagCatalog(normalizedScope, payload?.tags || []);
                return this.tagCatalog(normalizedScope);
            })
            .catch(() => this.tagCatalog(normalizedScope))
            .finally(() => {
                this.tagCatalogPromises.delete(normalizedScope);
            });

        this.tagCatalogPromises.set(normalizedScope, promise);
        return promise;
    },

    parseCommaSeparatedTagInput(value = '') {
        const raw = String(value || '');
        const segments = raw.split(',');
        const currentSegment = segments.pop() ?? '';
        return {
            committedTags: segments.map((segment) => segment.trim()).filter(Boolean),
            query: currentSegment.trim()
        };
    },

    filteredScopedTagSuggestions(scope = '', query = '', excludedTags = []) {
        const normalizedScope = this.normalizeTagScope(scope);
        const normalizedQuery = String(query || '').trim().toLowerCase();
        if (!normalizedQuery) {
            return [];
        }

        const catalog = this.tagCatalog(normalizedScope);
        const countKey = this.tagCountKey(normalizedScope);
        const excluded = new Set(
            (Array.isArray(excludedTags) ? excludedTags : [])
                .map((tag) => String(typeof tag === 'string' ? tag : tag?.name || '').trim().toLowerCase())
                .filter(Boolean)
        );

        return Array.from(catalog.values())
            .filter((tag) => tag?.name)
            .filter((tag) => !excluded.has(tag.name.toLowerCase()))
            .filter((tag) => tag.name.toLowerCase().includes(normalizedQuery))
            .sort((first, second) => {
                const firstName = String(first.name || '');
                const secondName = String(second.name || '');
                const firstLower = firstName.toLowerCase();
                const secondLower = secondName.toLowerCase();
                const firstStarts = firstLower.startsWith(normalizedQuery) ? 0 : 1;
                const secondStarts = secondLower.startsWith(normalizedQuery) ? 0 : 1;
                if (firstStarts !== secondStarts) {
                    return firstStarts - secondStarts;
                }

                const firstIndex = firstLower.indexOf(normalizedQuery);
                const secondIndex = secondLower.indexOf(normalizedQuery);
                if (firstIndex !== secondIndex) {
                    return firstIndex - secondIndex;
                }

                const usageDiff = Number(second[countKey] || 0) - Number(first[countKey] || 0);
                if (usageDiff !== 0) {
                    return usageDiff;
                }

                if (firstName.length !== secondName.length) {
                    return firstName.length - secondName.length;
                }
                return firstName.localeCompare(secondName, 'zh-CN', { sensitivity: 'base' });
            })
            .slice(0, 8);
    },

    renderScopedTagSuggestions(scope = '', query = '', excludedTags = []) {
        const normalizedScope = this.normalizeTagScope(scope);
        const countKey = this.tagCountKey(normalizedScope);
        const suggestions = this.filteredScopedTagSuggestions(normalizedScope, query, excludedTags);
        if (!suggestions.length) {
            return '';
        }

        return suggestions.map((tag) => `
            <button
                class="tag-autocomplete-suggestion"
                type="button"
                data-tag-autocomplete-suggestion="${this.escapeHTML(tag.name)}"
            >
                <span>${this.escapeHTML(tag.name)}</span>
                ${Number(tag[countKey] || 0) > 0 ? `<small>已用 ${Number(tag[countKey])} 次</small>` : ''}
            </button>
        `).join('');
    },

    applyCommaSeparatedTagSuggestion(value = '', tagName = '') {
        const normalizedTag = String(tagName || '').trim();
        if (!normalizedTag) {
            return String(value || '');
        }

        const { committedTags } = this.parseCommaSeparatedTagInput(value);
        const existing = new Set(committedTags.map((tag) => tag.toLowerCase()));
        if (!existing.has(normalizedTag.toLowerCase())) {
            committedTags.push(normalizedTag);
        }
        return `${committedTags.join(', ')}, `;
    },

    bindCommaSeparatedTagInputAutocomplete(options = {}) {
        const input = options.input instanceof HTMLInputElement ? options.input : null;
        const panel = options.panel instanceof HTMLElement ? options.panel : null;
        if (!input || !panel) {
            return null;
        }

        const scope = this.normalizeTagScope(options.scope);
        const wrapper = options.wrapper instanceof HTMLElement
            ? options.wrapper
            : (input.closest('.tag-autocomplete-field') || panel.parentElement);

        const refresh = () => {
            const { committedTags, query } = this.parseCommaSeparatedTagInput(input.value);
            const markup = this.renderScopedTagSuggestions(scope, query, committedTags);
            panel.innerHTML = markup;
            panel.classList.toggle('hidden', !markup);
        };

        const hide = () => {
            panel.classList.add('hidden');
        };

        const applySuggestion = (tagName = '') => {
            const normalizedTag = String(tagName || '').trim();
            if (!normalizedTag) {
                return;
            }

            input.value = this.applyCommaSeparatedTagSuggestion(input.value, normalizedTag);
            if (typeof input.setSelectionRange === 'function') {
                const caret = input.value.length;
                input.setSelectionRange(caret, caret);
            }
            input.dispatchEvent(new Event('input', { bubbles: true }));
            input.focus();
            hide();
        };

        const handleInput = () => {
            refresh();
        };
        const handleFocus = () => {
            refresh();
        };
        const handleKeydown = (event) => {
            if (event.key === 'Escape') {
                hide();
                return;
            }
            if (event.key !== 'Enter' && event.key !== 'Tab') {
                return;
            }

            const firstSuggestion = panel.querySelector('[data-tag-autocomplete-suggestion]');
            if (!firstSuggestion || panel.classList.contains('hidden')) {
                return;
            }

            event.preventDefault();
            applySuggestion(firstSuggestion.dataset.tagAutocompleteSuggestion || '');
        };
        const handlePanelMouseDown = (event) => {
            event.preventDefault();
        };
        const handlePanelClick = (event) => {
            const button = event.target.closest('[data-tag-autocomplete-suggestion]');
            if (!button) {
                return;
            }
            applySuggestion(button.dataset.tagAutocompleteSuggestion || '');
        };
        const handleDocumentClick = (event) => {
            if (!(event.target instanceof Element)) {
                hide();
                return;
            }
            if (wrapper?.contains(event.target)) {
                return;
            }
            hide();
        };

        input.addEventListener('input', handleInput);
        input.addEventListener('focus', handleFocus);
        input.addEventListener('keydown', handleKeydown);
        panel.addEventListener('mousedown', handlePanelMouseDown);
        panel.addEventListener('click', handlePanelClick);
        document.addEventListener('click', handleDocumentClick);

        void this.ensureScopedTagCatalogLoaded(scope).then(() => {
            if (document.contains(input)) {
                refresh();
            }
        });

        return {
            refresh,
            hide,
            mergeTags: (tags = []) => {
                Utils.mergeScopedTagCatalog(scope, tags);
                refresh();
            },
            destroy: () => {
                input.removeEventListener('input', handleInput);
                input.removeEventListener('focus', handleFocus);
                input.removeEventListener('keydown', handleKeydown);
                panel.removeEventListener('mousedown', handlePanelMouseDown);
                panel.removeEventListener('click', handlePanelClick);
                document.removeEventListener('click', handleDocumentClick);
            }
        };
    },

    isProcessingStatus(status = '') {
        return status === 'queued' || status === 'running';
    },

    statusTone(status = '') {
        if (status === 'completed') return 'success';
        if (status === 'failed' || status === 'cancelled') return 'error';
        if (status === 'queued' || status === 'running' || status === 'manual_pending') return 'info';
        return 'info';
    },

    statusLabel(status = '') {
        if (status === 'queued') return '等待解析';
        if (status === 'running') return '解析中';
        if (status === 'manual_pending') return '待手动标注';
        if (status === 'completed') return '已完成';
        if (status === 'failed') return '解析失败';
        if (status === 'cancelled') return '已取消';
        return status || '未知状态';
    }
};
