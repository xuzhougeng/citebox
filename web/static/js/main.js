if (typeof window.t !== 'function') window.t = function(k,f){return f||k};

const AppNav = {
    SECONDARY_HREFS: ['/palettes', '/groups', '/tags', '/settings'],

    init() {
        const navLinks = document.querySelector('.nav-links');
        if (!navLinks) return;
        if (navLinks.dataset.navEnhanced === '1') return;
        navLinks.dataset.navEnhanced = '1';

        this.reorderPrimary(navLinks);
        this.buildDropdown(navLinks);
        this.buildMobileToggle(navLinks);
        this.bindGlobalDismiss();
    },

    // Place "文献搜索" (/research) immediately after "AI 助手" (/ai). The shared HTML
    // template lists them in the opposite order; doing the move here keeps every
    // page's <nav> in sync without touching 13 separate files.
    reorderPrimary(navLinks) {
        const researchLink = navLinks.querySelector('a[href="/research"]');
        const aiLink = navLinks.querySelector('a[href="/ai"]');
        if (!researchLink || !aiLink) return;
        const researchLi = researchLink.closest('li');
        const aiLi = aiLink.closest('li');
        if (!researchLi || !aiLi) return;
        if (aiLi.nextElementSibling !== researchLi) {
            aiLi.after(researchLi);
        }
    },

    buildDropdown(navLinks) {
        const items = Array.from(navLinks.children).filter((el) => el.tagName === 'LI');
        const secondary = [];

        items.forEach((li) => {
            const a = li.querySelector('a[href]');
            if (!a) return;
            const href = a.getAttribute('href');
            if (this.SECONDARY_HREFS.includes(href)) {
                secondary.push(li);
            }
        });

        if (secondary.length === 0) return;

        const isActive = secondary.some((li) => li.querySelector('a.active'));

        const dropdown = document.createElement('li');
        dropdown.className = 'nav-dropdown' + (isActive ? ' is-active' : '');

        const toggle = document.createElement('button');
        toggle.type = 'button';
        toggle.className = 'nav-dropdown-toggle' + (isActive ? ' active' : '');
        toggle.setAttribute('aria-haspopup', 'true');
        toggle.setAttribute('aria-expanded', 'false');
        toggle.setAttribute('data-i18n-aria-label', 'nav.more_aria');
        toggle.setAttribute('aria-label', '更多');

        const labelSpan = document.createElement('span');
        labelSpan.className = 'nav-dropdown-label';
        labelSpan.setAttribute('data-i18n', 'nav.more');
        labelSpan.textContent = '更多';
        toggle.appendChild(labelSpan);

        const caret = document.createElement('span');
        caret.className = 'nav-dropdown-caret';
        caret.setAttribute('aria-hidden', 'true');
        caret.textContent = '▾';
        toggle.appendChild(caret);

        const menu = document.createElement('ul');
        menu.className = 'nav-dropdown-menu';
        menu.setAttribute('role', 'menu');

        navLinks.appendChild(dropdown);
        secondary.forEach((li) => {
            li.setAttribute('role', 'none');
            const link = li.querySelector('a');
            if (link) link.setAttribute('role', 'menuitem');
            menu.appendChild(li);
        });

        dropdown.appendChild(toggle);
        dropdown.appendChild(menu);

        toggle.addEventListener('click', (event) => {
            event.stopPropagation();
            this.toggleDropdown(dropdown);
        });

        menu.addEventListener('click', (event) => {
            if (event.target.closest('a[href]')) {
                this.closeDropdown(dropdown);
            }
        });

        this._dropdown = dropdown;
    },

    buildMobileToggle(navLinks) {
        const navbar = document.querySelector('.navbar-content');
        if (!navbar) return;
        const logo = navbar.querySelector('.logo');
        if (!logo) return;
        if (navbar.querySelector('.nav-toggle')) return;

        const toggle = document.createElement('button');
        toggle.type = 'button';
        toggle.className = 'nav-toggle';
        toggle.setAttribute('aria-controls', 'primary-nav');
        toggle.setAttribute('aria-expanded', 'false');
        toggle.setAttribute('aria-label', '菜单');
        toggle.setAttribute('data-i18n-aria-label', 'nav.toggle_menu');
        toggle.innerHTML = '<span class="nav-toggle-bar" aria-hidden="true"></span>';

        navLinks.id = navLinks.id || 'primary-nav';

        navbar.insertBefore(toggle, logo.nextSibling);

        toggle.addEventListener('click', (event) => {
            event.stopPropagation();
            this.toggleMobile(toggle, navLinks);
        });

        navLinks.addEventListener('click', (event) => {
            if (event.target.closest('a[href]')) {
                this.closeMobile(toggle, navLinks);
            }
        });

        this._mobileToggle = toggle;
        this._mobileNav = navLinks;
    },

    bindGlobalDismiss() {
        document.addEventListener('click', (event) => {
            if (this._dropdown && this._dropdown.classList.contains('is-open')
                && !this._dropdown.contains(event.target)) {
                this.closeDropdown(this._dropdown);
            }
            if (this._mobileToggle && this._mobileToggle.getAttribute('aria-expanded') === 'true'
                && !this._mobileNav.contains(event.target)
                && !this._mobileToggle.contains(event.target)) {
                this.closeMobile(this._mobileToggle, this._mobileNav);
            }
        });

        document.addEventListener('keydown', (event) => {
            if (event.key !== 'Escape') return;
            if (this._dropdown && this._dropdown.classList.contains('is-open')) {
                this.closeDropdown(this._dropdown);
                const t = this._dropdown.querySelector('.nav-dropdown-toggle');
                if (t) t.focus();
            }
            if (this._mobileToggle && this._mobileToggle.getAttribute('aria-expanded') === 'true') {
                this.closeMobile(this._mobileToggle, this._mobileNav);
                this._mobileToggle.focus();
            }
        });
    },

    toggleDropdown(dropdown) {
        const open = dropdown.classList.toggle('is-open');
        const t = dropdown.querySelector('.nav-dropdown-toggle');
        if (t) t.setAttribute('aria-expanded', open ? 'true' : 'false');
    },

    closeDropdown(dropdown) {
        dropdown.classList.remove('is-open');
        const t = dropdown.querySelector('.nav-dropdown-toggle');
        if (t) t.setAttribute('aria-expanded', 'false');
    },

    toggleMobile(toggle, navLinks) {
        const open = toggle.getAttribute('aria-expanded') !== 'true';
        toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
        navLinks.classList.toggle('is-open', open);
        document.body.classList.toggle('nav-open', open);
    },

    closeMobile(toggle, navLinks) {
        toggle.setAttribute('aria-expanded', 'false');
        navLinks.classList.remove('is-open');
        document.body.classList.remove('nav-open');
    }
};

const AppNavigationHotkeys = {
    routes: [
        { key: '1', path: '/', i18n: 'nav.overview', fallback: '总览' },
        { key: '2', path: '/library', i18n: 'nav.library', fallback: '文献库' },
        { key: '3', path: '/figures', i18n: 'nav.figures', fallback: '图片库' },
        { key: '4', path: '/palettes', i18n: 'nav.palettes', fallback: '配色库' },
        { key: '5', path: '/groups', i18n: 'nav.groups', fallback: '分组' },
        { key: '6', path: '/tags', i18n: 'nav.tags', fallback: '标签' },
        { key: '7', path: '/notes', i18n: 'nav.notes', fallback: '笔记' },
        { key: '8', path: '/ai', i18n: 'nav.ai', fallback: 'AI 助手' },
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
    AppNav.init();
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
