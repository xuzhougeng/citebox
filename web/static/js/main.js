const AppNavigationHotkeys = {
    routes: [
        { key: '1', path: '/', label: '总览' },
        { key: '2', path: '/library', label: '文献库' },
        { key: '3', path: '/figures', label: '图片库' },
        { key: '4', path: '/palettes', label: '配色库' },
        { key: '5', path: '/groups', label: '分组' },
        { key: '6', path: '/tags', label: '标签' },
        { key: '7', path: '/notes', label: '笔记' },
        { key: '8', path: '/ai', label: 'AI伴读' },
        { key: '9', path: '/settings', label: '配置' }
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
        if (/^Digit[1-9]$/.test(code)) {
            return code.slice(5);
        }
        if (/^Numpad[1-9]$/.test(code)) {
            return code.slice(6);
        }

        const key = String(event.key || '');
        return /^[1-9]$/.test(key) ? key : '';
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

const AppUpdateNotice = {
    storageKey: 'citebox.dismissedUpdateVersion',

    init() {
        if (this.initialized) return;
        this.initialized = true;

        window.setTimeout(() => {
            void this.checkAndPrompt();
        }, 400);
    },

    async checkAndPrompt() {
        if (typeof API === 'undefined' || typeof API.getVersionStatus !== 'function') {
            return;
        }
        if (this.isPromptVisible()) {
            return;
        }

        try {
            const status = await API.getVersionStatus(false);
            if (!status?.has_update || !status.latest_version || !status.latest_release_url) {
                this.clearDismissedVersion();
                return;
            }
            if (this.dismissedVersion() === String(status.latest_version || '').trim()) {
                return;
            }
            this.showPrompt(status);
        } catch (error) {
            // Ignore version prompt failures on page load.
        }
    },

    dismissedVersion() {
        try {
            return String(window.localStorage.getItem(this.storageKey) || '').trim();
        } catch (error) {
            return '';
        }
    },

    clearDismissedVersion() {
        try {
            window.localStorage.removeItem(this.storageKey);
        } catch (error) {
            // Ignore storage failures.
        }
    },

    rememberDismissedVersion(version = '') {
        const normalized = String(version || '').trim();
        if (!normalized) return;
        try {
            window.localStorage.setItem(this.storageKey, normalized);
        } catch (error) {
            // Ignore storage failures.
        }
    },

    isPromptVisible() {
        return Boolean(this.overlay && document.body.contains(this.overlay));
    },

    closePrompt() {
        if (!this.overlay) return;
        this.overlay.remove();
        this.overlay = null;
    },

    showPrompt(status = {}) {
        this.closePrompt();

        const currentVersion = String(status.current_version || '当前版本').trim();
        const latestVersion = String(status.latest_version || '').trim();
        const releaseURL = String(status.latest_release_url || '').trim();
        const publishedAt = status.published_at
            ? `发布时间：${Utils.escapeHTML(Utils.formatDate(status.published_at))}`
            : '已有新的正式版本可用';

        const overlay = document.createElement('div');
        overlay.className = 'dialog-overlay';
        overlay.innerHTML = `
            <div class="dialog-box dialog-box-update">
                <div class="dialog-update-head">
                    <span class="dialog-update-badge">发现更新</span>
                    <h3>检测到新版本 ${Utils.escapeHTML(latestVersion)}</h3>
                </div>
                <div class="dialog-body dialog-update-body">
                    <p>当前版本是 <strong>${Utils.escapeHTML(currentVersion)}</strong>，建议更新到 <strong>${Utils.escapeHTML(latestVersion)}</strong>。</p>
                    <p>${publishedAt}</p>
                </div>
                <div class="dialog-footer">
                    <button class="btn btn-outline" type="button" data-update-action="later">暂不更新</button>
                    <button class="btn btn-primary" type="button" data-update-action="now">立刻更新</button>
                </div>
            </div>
        `;

        overlay.addEventListener('click', (event) => {
            const button = event.target.closest('[data-update-action]');
            if (!button) return;

            if (button.dataset.updateAction === 'later') {
                this.rememberDismissedVersion(latestVersion);
                this.closePrompt();
                return;
            }
            if (button.dataset.updateAction === 'now' && releaseURL) {
                this.closePrompt();
                if (typeof Utils !== 'undefined' && typeof Utils.openExternalURL === 'function') {
                    void Utils.openExternalURL(releaseURL);
                    return;
                }
                window.location.href = releaseURL;
            }
        });

        document.body.appendChild(overlay);
        this.overlay = overlay;
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
    if (typeof DesktopTranslate !== 'undefined' && typeof DesktopTranslate.init === 'function') {
        DesktopTranslate.init();
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

    if (path === '/palettes' || path === '/palettes.html') {
        PalettesPage.init();
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
    AppUpdateNotice.init();
});

window.addEventListener('pageshow', () => {
    restorePendingModalState();
});
