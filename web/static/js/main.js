if (typeof window.t !== 'function') window.t = function(k,f){return f||k};
const AppNavigationHotkeys = {
    routes: [
        { key: '1', path: '/', i18n: 'nav.overview', fallback: '总览' },
        { key: '2', path: '/library', i18n: 'nav.library', fallback: '文献库' },
        { key: '3', path: '/figures', i18n: 'nav.figures', fallback: '图片库' },
        { key: '4', path: '/palettes', i18n: 'nav.palettes', fallback: '配色库' },
        { key: '5', path: '/groups', i18n: 'nav.groups', fallback: '分组' },
        { key: '6', path: '/tags', i18n: 'nav.tags', fallback: '标签' },
        { key: '7', path: '/notes', i18n: 'nav.notes', fallback: '笔记' },
        { key: '8', path: '/ai', i18n: 'nav.ai', fallback: 'AI伴读' },
        { key: '9', path: '/settings', i18n: 'nav.settings', fallback: '配置' }
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
            const label = t(route.i18n, route.fallback);
            link.title = `${label}${t('hotkey.shortcut_hint', '（快捷键 {key}）').replace('{key}', route.key)}`;
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

        const currentVersion = String(status.current_version || t('update.current_version', '当前版本')).trim();
        const latestVersion = String(status.latest_version || '').trim();
        const releaseURL = String(status.latest_release_url || '').trim();
        const publishedAt = status.published_at
            ? `${t('update.published_at', '发布时间：')}${Utils.escapeHTML(Utils.formatDate(status.published_at))}`
            : t('update.new_version_available', '已有新的正式版本可用');

        const overlay = document.createElement('div');
        overlay.className = 'dialog-overlay';
        overlay.innerHTML = `
            <div class="dialog-box dialog-box-update">
                <div class="dialog-update-head">
                    <span class="dialog-update-badge">${t('update.badge', '发现更新')}</span>
                    <h3>${t('update.new_version_detected', '检测到新版本 {version}').replace('{version}', Utils.escapeHTML(latestVersion))}</h3>
                </div>
                <div class="dialog-body dialog-update-body">
                    <p>${t('update.version_message', '当前版本是 <strong>{current}</strong>，建议更新到 <strong>{latest}</strong>。').replace('{current}', Utils.escapeHTML(currentVersion)).replace('{latest}', Utils.escapeHTML(latestVersion))}</p>
                    <p>${publishedAt}</p>
                </div>
                <div class="dialog-footer">
                    <button class="btn btn-outline" type="button" data-update-action="later">${t('update.later', '暂不更新')}</button>
                    <button class="btn btn-primary" type="button" data-update-action="now">${t('update.now', '立刻更新')}</button>
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
