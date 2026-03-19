const ResourceViewerPage = {
    init() {
        this.stage = document.getElementById('viewerStage');
        this.closeButton = document.getElementById('viewerCloseButton');
        this.kindLabel = document.getElementById('viewerKindLabel');
        this.title = document.getElementById('viewerTitle');
        this.closing = false;

        this.closeButton?.addEventListener('click', () => this.close());
        document.addEventListener('keydown', (event) => {
            if (event.key !== 'Escape') return;
            event.preventDefault();
            event.stopPropagation();
            event.stopImmediatePropagation();
            this.close({ deferNavigation: true });
        });

        this.render();
    },

    render() {
        try {
            const resource = this.resolveResource();
            document.title = `${resource.label} - CiteBox`;
            this.kindLabel.textContent = resource.label;
            this.title.textContent = resource.name;

            if (resource.kind === 'image') {
                this.stage.className = 'viewer-stage image-mode';
                this.stage.innerHTML = `
                    <img class="viewer-image" src="${resource.href}" alt="${this.escapeHTML(resource.name)}">
                `;
                this.stage.addEventListener('click', (event) => {
                    if (event.target === this.stage) {
                        this.close();
                    }
                });
                return;
            }

            this.stage.className = 'viewer-stage';
            this.stage.innerHTML = `
                <iframe class="viewer-frame" src="${resource.href}" title="${this.escapeHTML(resource.name)}"></iframe>
            `;
        } catch (error) {
            document.title = '文件查看失败 - CiteBox';
            this.kindLabel.textContent = '文件查看';
            this.title.textContent = '无法打开资源';
            this.stage.className = 'viewer-stage';
            this.stage.innerHTML = `
                <div class="viewer-empty">
                    <h1>无法打开这个资源</h1>
                    <p>${this.escapeHTML(error.message || '资源地址无效或不受支持。')}</p>
                </div>
            `;
        }
    },

    resolveResource() {
        const params = new URLSearchParams(window.location.search);
        const kind = String(params.get('kind') || '').trim().toLowerCase();
        const src = String(params.get('src') || '').trim();

        if (!src) {
            throw new Error('缺少资源地址。');
        }

        const url = new URL(src, window.location.origin);
        if (url.origin !== window.location.origin) {
            throw new Error('只支持打开 CiteBox 当前实例中的资源。');
        }

        if (kind === 'image') {
            if (!url.pathname.startsWith('/files/figures/')) {
                throw new Error('当前仅支持打开图片库中的原图资源。');
            }
            return {
                kind,
                href: url.href,
                label: '原图查看',
                name: this.filenameFromURL(url) || '图片'
            };
        }

        if (kind === 'pdf') {
            if (!url.pathname.startsWith('/files/papers/')) {
                throw new Error('当前仅支持打开文献 PDF 资源。');
            }
            return {
                kind,
                href: url.href,
                label: 'PDF 查看',
                name: this.filenameFromURL(url) || 'PDF 文档'
            };
        }

        throw new Error('不支持的资源类型。');
    },

    filenameFromURL(url) {
        const pathname = String(url?.pathname || '');
        const segments = pathname.split('/').filter(Boolean);
        if (!segments.length) return '';
        try {
            return decodeURIComponent(segments[segments.length - 1]);
        } catch (error) {
            return segments[segments.length - 1];
        }
    },

    close(options = {}) {
        if (this.closing) {
            return;
        }

        this.closing = true;
        const { deferNavigation = false } = options;
        const finalizeClose = () => {
            this.navigateBack();
        };

        if (deferNavigation) {
            // Let the Escape event finish first so it does not close restored modals on the previous page.
            window.setTimeout(finalizeClose, 0);
            return;
        }

        finalizeClose();
    },

    navigateBack() {
        if (window.history.length > 1) {
            window.history.back();
            return;
        }

        const params = new URLSearchParams(window.location.search);
        const back = String(params.get('back') || '').trim();
        if (back) {
            try {
                const backURL = new URL(back, window.location.origin);
                if (backURL.origin === window.location.origin) {
                    window.location.assign(backURL.href);
                    return;
                }
            } catch (error) {
                // Ignore invalid back URLs and continue with other fallbacks.
            }
        }

        const referrer = String(document.referrer || '').trim();
        if (referrer) {
            try {
                const referrerURL = new URL(referrer);
                if (referrerURL.origin === window.location.origin) {
                    window.location.assign(referrerURL.href);
                    return;
                }
            } catch (error) {
                // Ignore invalid referrer URLs and fall back to the dashboard.
            }
        }

        window.location.assign('/');
    },

    escapeHTML(value = '') {
        return String(value)
            .replaceAll('&', '&amp;')
            .replaceAll('<', '&lt;')
            .replaceAll('>', '&gt;')
            .replaceAll('"', '&quot;')
            .replaceAll("'", '&#39;');
    }
};

document.addEventListener('DOMContentLoaded', () => {
    ResourceViewerPage.init();
});
