const ResourceViewerPage = {
    init() {
        this.stage = document.getElementById('viewerStage');
        this.closeButton = document.getElementById('viewerCloseButton');
        this.kindLabel = document.getElementById('viewerKindLabel');
        this.title = document.getElementById('viewerTitle');
        this.toolbarActions = document.getElementById('viewerToolbarActions');
        this.resetButton = document.getElementById('viewerResetButton');
        this.closing = false;
        this.viewState = this.defaultViewState();
        this.dragState = null;

        this.closeButton?.addEventListener('click', () => this.close());
        this.resetButton?.addEventListener('click', () => {
            this.resetImageView();
            this.applyImageTransform();
        });
        document.addEventListener('keydown', (event) => {
            if (event.key !== 'Escape') return;
            event.preventDefault();
            event.stopPropagation();
            event.stopImmediatePropagation();
            this.close({ deferNavigation: true });
        });
        this.stage?.addEventListener('wheel', (event) => {
            const viewport = event.target.closest('[data-viewer-viewport]');
            if (!viewport) return;
            event.preventDefault();
            this.handleImageWheel(event, viewport);
        }, { passive: false });
        this.stage?.addEventListener('pointerdown', (event) => {
            const viewport = event.target.closest('[data-viewer-viewport]');
            if (!viewport) return;
            this.beginImageDrag(event, viewport);
        });
        document.addEventListener('pointermove', (event) => {
            this.updateImageDrag(event);
        });
        document.addEventListener('pointerup', (event) => {
            this.endImageDrag(event);
        });
        document.addEventListener('pointercancel', (event) => {
            this.endImageDrag(event);
        });
        window.addEventListener('resize', () => {
            this.applyImageTransform();
        });
        this.stage?.addEventListener('click', (event) => {
            if (event.target === this.stage) {
                this.close();
            }
        });

        this.render();
    },

    render() {
        try {
            const resource = this.resolveResource();
            this.endImageDrag();
            this.resetImageView();
            document.title = `${resource.label} - CiteBox`;
            this.kindLabel.textContent = resource.label;
            this.title.textContent = resource.name;

            if (resource.kind === 'image') {
                this.toggleImageToolbar(true);
                this.stage.className = 'viewer-stage image-mode';
                this.stage.innerHTML = `
                    <div class="viewer-image-viewport" data-viewer-viewport>
                        <img class="viewer-image" src="${resource.href}" alt="${this.escapeHTML(resource.name)}" data-viewer-image>
                    </div>
                `;
                const image = this.stage.querySelector('[data-viewer-image]');
                if (image) {
                    if (image.complete) {
                        this.applyImageTransform();
                    } else {
                        image.addEventListener('load', () => this.applyImageTransform(), { once: true });
                    }
                }
                return;
            }

            this.toggleImageToolbar(false);
            this.stage.className = 'viewer-stage';
            this.stage.innerHTML = `
                <iframe class="viewer-frame" src="${resource.href}" title="${this.escapeHTML(resource.name)}"></iframe>
            `;
        } catch (error) {
            this.toggleImageToolbar(false);
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

    defaultViewState() {
        return {
            scale: 1,
            x: 0,
            y: 0
        };
    },

    resetImageView() {
        this.viewState = this.defaultViewState();
        this.dragState = null;
    },

    toggleImageToolbar(visible) {
        if (!this.toolbarActions) return;
        this.toolbarActions.hidden = !visible;
        if (!visible && this.resetButton) {
            this.resetButton.disabled = true;
        }
    },

    hasImageTransform() {
        const state = this.viewState || this.defaultViewState();
        return Math.abs(state.scale - 1) > 0.001 || Math.abs(state.x) > 0.5 || Math.abs(state.y) > 0.5;
    },

    clampViewState(state = this.viewState || this.defaultViewState()) {
        const viewport = this.stage?.querySelector('[data-viewer-viewport]');
        const image = this.stage?.querySelector('[data-viewer-image]');
        const scale = Math.min(6, Math.max(1, Number(state.scale) || 1));
        if (!viewport || !image || scale <= 1) {
            return { scale: 1, x: 0, y: 0 };
        }

        const baseWidth = image.offsetWidth || image.clientWidth || 0;
        const baseHeight = image.offsetHeight || image.clientHeight || 0;
        if (!baseWidth || !baseHeight) {
            return { scale, x: state.x || 0, y: state.y || 0 };
        }

        const maxX = Math.max(0, (baseWidth * scale - viewport.clientWidth) / 2);
        const maxY = Math.max(0, (baseHeight * scale - viewport.clientHeight) / 2);
        return {
            scale,
            x: Math.min(maxX, Math.max(-maxX, Number(state.x) || 0)),
            y: Math.min(maxY, Math.max(-maxY, Number(state.y) || 0))
        };
    },

    applyImageTransform() {
        const viewport = this.stage?.querySelector('[data-viewer-viewport]');
        const image = this.stage?.querySelector('[data-viewer-image]');
        if (!viewport || !image) return;

        this.viewState = this.clampViewState(this.viewState);
        const state = this.viewState;
        image.style.transform = `translate(${state.x}px, ${state.y}px) scale(${state.scale})`;
        image.draggable = false;
        viewport.classList.toggle('is-zoomed', state.scale > 1);
        viewport.classList.toggle('is-dragging', Boolean(this.dragState));
        if (this.resetButton) {
            this.resetButton.disabled = !this.hasImageTransform();
        }
    },

    handleImageWheel(event, viewport) {
        const previous = this.viewState || this.defaultViewState();
        const zoomFactor = event.deltaY < 0 ? 1.18 : 1 / 1.18;
        const nextScale = Math.min(6, Math.max(1, previous.scale * zoomFactor));
        if (Math.abs(nextScale - previous.scale) < 0.001) return;

        const rect = viewport.getBoundingClientRect();
        const pointX = event.clientX - (rect.left + rect.width / 2);
        const pointY = event.clientY - (rect.top + rect.height / 2);
        const ratio = nextScale / previous.scale;

        this.viewState = {
            scale: nextScale,
            x: pointX - ratio * (pointX - previous.x),
            y: pointY - ratio * (pointY - previous.y)
        };
        this.applyImageTransform();
    },

    beginImageDrag(event, viewport) {
        if ((event.button !== 0 && event.button !== 1) || (this.viewState?.scale || 1) <= 1) return;
        event.preventDefault();
        this.dragState = {
            pointerID: event.pointerId,
            startX: event.clientX,
            startY: event.clientY,
            originX: this.viewState.x,
            originY: this.viewState.y
        };
        if (typeof viewport.setPointerCapture === 'function') {
            try {
                viewport.setPointerCapture(event.pointerId);
            } catch (error) {
                // Ignore capture failures for non-primary pointers.
            }
        }
        this.applyImageTransform();
    },

    updateImageDrag(event) {
        if (!this.dragState || event.pointerId !== this.dragState.pointerID) return;
        this.viewState = {
            ...this.viewState,
            x: this.dragState.originX + (event.clientX - this.dragState.startX),
            y: this.dragState.originY + (event.clientY - this.dragState.startY)
        };
        this.applyImageTransform();
    },

    endImageDrag(event = null) {
        if (!this.dragState) return;
        if (event && event.pointerId !== this.dragState.pointerID) return;

        const viewport = this.stage?.querySelector('[data-viewer-viewport]');
        if (viewport && typeof viewport.hasPointerCapture === 'function' && viewport.hasPointerCapture(this.dragState.pointerID)) {
            try {
                viewport.releasePointerCapture(this.dragState.pointerID);
            } catch (error) {
                // Ignore release failures after cancellation.
            }
        }
        this.dragState = null;
        this.applyImageTransform();
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
        this.endImageDrag();
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
        const params = new URLSearchParams(window.location.search);
        const back = String(params.get('back') || '').trim();
        if (back) {
            try {
                const backURL = new URL(back, window.location.origin);
                if (backURL.origin === window.location.origin) {
                    if (this.shouldUseHistoryBack(backURL)) {
                        window.history.back();
                        return;
                    }
                    window.location.replace(backURL.href);
                    return;
                }
            } catch (error) {
                // Ignore invalid back URLs and continue with other fallbacks.
            }
        }

        if (window.history.length > 1) {
            window.history.back();
            return;
        }

        const referrer = String(document.referrer || '').trim();
        if (referrer) {
            try {
                const referrerURL = new URL(referrer);
                if (referrerURL.origin === window.location.origin) {
                    window.location.replace(referrerURL.href);
                    return;
                }
            } catch (error) {
                // Ignore invalid referrer URLs and fall back to the dashboard.
            }
        }

        window.location.replace('/');
    },

    shouldUseHistoryBack(backURL) {
        if (window.history.length <= 1) {
            return false;
        }

        const referrer = String(document.referrer || '').trim();
        if (!referrer) {
            return false;
        }

        try {
            const referrerURL = new URL(referrer, window.location.origin);
            if (referrerURL.origin !== window.location.origin) {
                return false;
            }
            return this.normalizeBackComparisonURL(referrerURL) === this.normalizeBackComparisonURL(backURL);
        } catch (error) {
            return false;
        }
    },

    normalizeBackComparisonURL(url) {
        const normalized = new URL(url, window.location.origin);
        normalized.hash = '';
        normalized.searchParams.delete('restore_modal');
        return `${normalized.pathname}${normalized.search}`;
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
