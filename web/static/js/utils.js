// 工具函数模块
const Utils = {
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
        const stash = (html) => {
            const token = `%%MDTOKEN${placeholders.length}%%`;
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
            `);
        });
        text = text.replace(/`([^`\n]+)`/g, (_, code) => stash(`<code class="markdown-inline-code">${code}</code>`));

        const blocks = text.split(/\n{2,}/).map((block) => block.trim()).filter(Boolean);
        let html = blocks.map((block) => Utils.renderMarkdownBlock(block, options)).join('');
        placeholders.forEach(({ token, html: fragment }) => {
            html = html.replaceAll(token, fragment);
        });
        return html;
    },

    renderMarkdownBlock(block, options = {}) {
        const lines = block.split('\n').map((line) => line.trimEnd());

        if (lines.length === 1 && /^(-{3,}|\*{3,}|_{3,})$/.test(lines[0].trim())) {
            return '<hr class="markdown-divider">';
        }

        if (lines.length === 1) {
            const image = Utils.renderMarkdownImageBlock(lines[0], options);
            if (image) {
                return image;
            }
            const heading = lines[0].match(/^(#{1,6})\s+(.+)$/);
            if (heading) {
                const level = heading[1].length;
                return `<h${level} class="markdown-heading markdown-heading-${level}">${Utils.renderMarkdownInline(heading[2], options)}</h${level}>`;
            }
        }

        const headingWithList = Utils.renderMarkdownHeadingListBlock(lines, options);
        if (headingWithList) {
            return headingWithList;
        }

        if (lines.every((line) => /^&gt;\s?/.test(line))) {
            const content = lines
                .map((line) => line.replace(/^&gt;\s?/, ''))
                .map((line) => Utils.renderMarkdownInline(line, options))
                .join('<br>');
            return `<blockquote class="markdown-blockquote">${content}</blockquote>`;
        }

        if (lines.every((line) => /^[-*+]\s+/.test(line))) {
            return `
                <ul class="markdown-list">
                    ${lines.map((line) => Utils.renderMarkdownListItem(line.replace(/^[-*+]\s+/, ''), options)).join('')}
                </ul>
            `;
        }

        if (lines.every((line) => /^\d+\.\s+/.test(line))) {
            return Utils.renderMarkdownOrderedList(lines, options);
        }

        const orderedItemWithNestedList = Utils.renderMarkdownSingleOrderedItem(lines, options);
        if (orderedItemWithNestedList) {
            return orderedItemWithNestedList;
        }

        return Utils.renderMarkdownFlowBlock(lines, options);
    },

    renderMarkdownHeadingListBlock(lines, options = {}) {
        if (lines.length < 2) {
            return '';
        }

        const heading = lines[0].match(/^(#{1,6})\s+(.+)$/);
        if (!heading) {
            return '';
        }

        const rest = lines.slice(1).filter((line) => line.trim());
        if (!rest.length) {
            return '';
        }

        const level = heading[1].length;
        const headingHTML = `<h${level} class="markdown-heading markdown-heading-${level}">${Utils.renderMarkdownInline(heading[2], options)}</h${level}>`;

        if (rest.every((line) => /^[-*+]\s+/.test(line))) {
            return `${headingHTML}
                <ul class="markdown-list">
                    ${rest.map((line) => Utils.renderMarkdownListItem(line.replace(/^[-*+]\s+/, ''), options)).join('')}
                </ul>
            `;
        }

        if (rest.every((line) => /^\d+\.\s+/.test(line))) {
            return `${headingHTML}${Utils.renderMarkdownOrderedList(rest, options)}`;
        }

        return '';
    },

    renderMarkdownOrderedList(lines, options = {}) {
        const firstMatch = lines[0].match(/^(\d+)\.\s+(.+)$/);
        const start = firstMatch ? parseInt(firstMatch[1], 10) : 1;
        const startAttr = start > 1 ? ` start="${start}"` : '';

        return `
                <ol class="markdown-list markdown-list-ordered"${startAttr}>
                    ${lines.map((line) => Utils.renderMarkdownListItem(line.replace(/^\d+\.\s+/, ''), options)).join('')}
                </ol>
            `;
    },

    renderMarkdownSingleOrderedItem(lines, options = {}) {
        if (!lines.length) {
            return '';
        }

        const firstMatch = lines[0].match(/^(\d+)\.\s+(.+)$/);
        if (!firstMatch || lines.length === 1) {
            return '';
        }

        const start = parseInt(firstMatch[1], 10);
        const startAttr = start > 1 ? ` start="${start}"` : '';
        const bodyLines = [];
        const nestedItems = [];

        for (const line of lines.slice(1)) {
            if (/^\s*[-*+]\s+/.test(line)) {
                nestedItems.push(line.replace(/^\s*[-*+]\s+/, ''));
                continue;
            }

            if (/^\s{2,}\S/.test(line) || /^\t+\S/.test(line)) {
                bodyLines.push(line.trim());
                continue;
            }

            return '';
        }

        if (!bodyLines.length && !nestedItems.length) {
            return '';
        }

        const itemParts = [Utils.renderMarkdownFlowBlock([firstMatch[2]], options, 'div')];
        if (bodyLines.length) {
            itemParts.push(Utils.renderMarkdownFlowBlock(bodyLines, options, 'div'));
        }
        if (nestedItems.length) {
            itemParts.push(`
                <ul class="markdown-list markdown-list-nested">
                    ${nestedItems.map((item) => Utils.renderMarkdownListItem(item, options)).join('')}
                </ul>
            `);
        }

        return `
                <ol class="markdown-list markdown-list-ordered"${startAttr}>
                    <li>${itemParts.join('')}</li>
                </ol>
            `;
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
        if (status === 'completed') return '解析完成';
        if (status === 'failed') return '解析失败';
        if (status === 'cancelled') return '已取消';
        return status || '未知状态';
    }
};
