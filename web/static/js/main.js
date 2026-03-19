const AppNavigationHotkeys = {
    routes: [
        { key: '1', path: '/', label: '总览' },
        { key: '2', path: '/library', label: '文献库' },
        { key: '3', path: '/figures', label: '图片库' },
        { key: '4', path: '/groups', label: '分组' },
        { key: '5', path: '/tags', label: '标签' },
        { key: '6', path: '/notes', label: '笔记' },
        { key: '7', path: '/ai', label: 'AI伴读' },
        { key: '8', path: '/settings', label: '配置' }
    ],

    init() {
        if (this.initialized) return;
        this.initialized = true;
        this.decorateNavLinks();

        document.addEventListener('keydown', (event) => {
            if (event.defaultPrevented) return;
            if (event.ctrlKey || event.metaKey || event.altKey) return;
            if (this.shouldIgnoreKeydown(event)) return;

            const shortcut = this.resolveShortcut(event);
            if (!shortcut) return;

            const route = this.routes.find((item) => item.key === shortcut);
            if (!route) return;

            const currentPath = this.normalizePath(window.location.pathname);
            const targetPath = this.normalizePath(route.path);
            if (currentPath === targetPath) return;

            event.preventDefault();
            window.location.href = route.path;
        });
    },

    decorateNavLinks() {
        document.querySelectorAll('.nav-links a[href]').forEach((link) => {
            const route = this.routes.find((item) => this.normalizePath(item.path) === this.normalizePath(link.getAttribute('href') || ''));
            if (!route) return;
            link.title = `${route.label}（快捷键 ${route.key}）`;
        });
    },

    resolveShortcut(event) {
        const code = String(event.code || '');
        if (/^Digit[1-8]$/.test(code)) {
            return code.slice(5);
        }
        if (/^Numpad[1-8]$/.test(code)) {
            return code.slice(6);
        }

        const key = String(event.key || '');
        return /^[1-8]$/.test(key) ? key : '';
    },

    shouldIgnoreKeydown(event) {
        const target = event.target;
        if (target instanceof HTMLElement) {
            if (target.isContentEditable) {
                return true;
            }
            if (['INPUT', 'TEXTAREA', 'SELECT', 'BUTTON'].includes(target.tagName)) {
                return true;
            }
        }

        return Boolean(document.querySelector('.dialog-overlay, .modal-shell:not(.hidden)'));
    },

    normalizePath(path = '') {
        const normalized = String(path || '').trim();
        if (!normalized || normalized === '/index.html') {
            return '/';
        }
        return normalized.replace(/\.html$/, '');
    }
};

function restorePendingModalState() {
    const modalRestoreState = typeof Utils !== 'undefined' && typeof Utils.consumeModalRestoreState === 'function'
        ? Utils.consumeModalRestoreState()
        : null;

    if (!modalRestoreState || typeof Utils === 'undefined' || typeof Utils.restoreModalState !== 'function') {
        return;
    }

    window.setTimeout(() => {
        void Utils.restoreModalState(modalRestoreState);
    }, 0);
}

document.addEventListener('DOMContentLoaded', () => {
    const path = window.location.pathname;
    AppNavigationHotkeys.init();
    if (typeof Utils !== 'undefined' && typeof Utils.bindResourceViewerLinks === 'function') {
        Utils.bindResourceViewerLinks();
    }

    if (path === '/' || path === '/index.html') {
        DashboardPage.init();
    }

    if (path === '/library' || path === '/library.html') {
        LibraryPage.init();
    }

    if (path === '/upload' || path === '/upload.html') {
        UploadPage.init();
    }

    if (path === '/manual' || path === '/manual.html') {
        ManualPage.init();
    }

    if (path === '/figures' || path === '/figures.html') {
        FiguresPage.init();
    }

    if (path === '/groups' || path === '/groups.html') {
        GroupsPage.init();
    }

    if (path === '/tags' || path === '/tags.html') {
        TagsPage.init();
    }

    if (path === '/notes' || path === '/notes.html') {
        NotesPage.init();
    }

    if (path === '/ai' || path === '/ai.html') {
        AIReaderPage.init();
    }

    if (path === '/settings' || path === '/settings.html') {
        SettingsPage.init();
    }

    restorePendingModalState();
});

window.addEventListener('pageshow', () => {
    restorePendingModalState();
});
