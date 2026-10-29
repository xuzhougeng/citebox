if (typeof window.t !== 'function') window.t = function(k,f){return f||k};

const AppNav = {
    PRIMARY_ORDER: ['/', '/library', '/figures', '/notes', '/ai', '/research'],
    SECONDARY_HREFS: ['/highlights', '/palettes', '/groups', '/tags'],
    SETTINGS_ICON: '<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/></svg>',

    init() {
        const navLinks = document.querySelector('.nav-links');
        if (!navLinks) return;
        if (navLinks.dataset.navEnhanced === '1') return;
        navLinks.dataset.navEnhanced = '1';

        this.moveSettingsToActions(navLinks);
        this.reorderPrimary(navLinks);
        this.buildDropdown(navLinks);
        this.buildMobileToggle(navLinks);
        this.initOverflow(navLinks);
        this.bindGlobalDismiss();
    },

    // Settings is an app-level destination, not a library view: show it as a
    // gear next to the upload action instead of burying it under "More".
    moveSettingsToActions(navLinks) {
        const link = navLinks.querySelector('a[href="/settings"]');
        const actions = document.querySelector('.nav-actions');
        if (!link || !actions) return;
        const label = link.textContent.trim() || '设置';
        const gear = document.createElement('a');
        gear.href = '/settings';
        gear.className = 'nav-icon-link' + (link.classList.contains('active') ? ' active' : '');
        gear.setAttribute('aria-label', label);
        gear.setAttribute('title', label);
        gear.setAttribute('data-i18n-aria-label', 'nav.settings');
        gear.setAttribute('data-i18n-title', 'nav.settings');
        if (link.classList.contains('active')) gear.setAttribute('aria-current', 'page');
        gear.innerHTML = this.SETTINGS_ICON;
        link.closest('li')?.remove();
        actions.insertBefore(gear, actions.firstChild);
    },

    // The shared HTML template lists links in an older order; sorting here keeps
    // every page's <nav> in sync without touching each file.
    reorderPrimary(navLinks) {
        const items = Array.from(navLinks.children).filter((el) => el.tagName === 'LI');
        const rank = (li) => {
            const index = this.PRIMARY_ORDER.indexOf(li.querySelector('a')?.getAttribute('href'));
            return index === -1 ? this.PRIMARY_ORDER.length : index;
        };
        items.sort((a, b) => rank(a) - rank(b)).forEach((li) => navLinks.appendChild(li));
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

    initOverflow(navLinks) {
        const navbar = navLinks.closest('.navbar-content');
        if (!navbar || !this._dropdown || typeof ResizeObserver === 'undefined') return;

        this._primaryItems = Array.from(navLinks.children).filter((item) => item !== this._dropdown);
        const schedule = () => {
            if (this._overflowFrame) cancelAnimationFrame(this._overflowFrame);
            this._overflowFrame = requestAnimationFrame(() => {
                this._overflowFrame = null;
                this.fitPrimaryLinks(navbar, navLinks);
            });
        };
        this._overflowObserver = new ResizeObserver(schedule);
        this._overflowObserver.observe(navbar);
        const actions = navbar.querySelector('.nav-actions');
        if (actions) this._overflowObserver.observe(actions);
        this._overflowObserver.observe(navLinks);
        if (document.fonts?.ready) document.fonts.ready.then(schedule);
        schedule();
    },

    fitPrimaryLinks(navbar, navLinks) {
        const menu = this._dropdown.querySelector('.nav-dropdown-menu');
        const focused = document.activeElement;
        this._primaryItems.forEach((item) => {
            if (item.parentElement !== navLinks) navLinks.insertBefore(item, this._dropdown);
            item.removeAttribute('role');
            item.querySelector('a')?.removeAttribute('role');
        });
        // Restoring items in their original order also handles a wider window.
        this._primaryItems.forEach((item) => navLinks.insertBefore(item, this._dropdown));

        if (window.matchMedia('(min-width: 721px)').matches) {
            const styles = getComputedStyle(navbar);
            const available = navbar.clientWidth - parseFloat(styles.paddingLeft) - parseFloat(styles.paddingRight);
            const gap = parseFloat(styles.columnGap) || 0;
            const fits = () => {
                const children = Array.from(navbar.children).filter((item) => getComputedStyle(item).display !== 'none');
                const used = children.reduce((sum, item) => sum + item.getBoundingClientRect().width, 0);
                return used + gap * Math.max(0, children.length - 1) <= available + 1;
            };
            const priority = ['/research', '/', '/notes', '/figures', '/ai', '/library'];
            const candidates = [...this._primaryItems].sort((a, b) => {
                const rank = (item) => priority.indexOf(item.querySelector('a')?.getAttribute('href'));
                return rank(a) - rank(b);
            });
            for (const item of candidates) {
                if (fits()) break;
                item.setAttribute('role', 'none');
                item.querySelector('a')?.setAttribute('role', 'menuitem');
                menu.appendChild(item);
            }
        }

        const active = Boolean(menu.querySelector('a.active'));
        this._dropdown.classList.toggle('is-active', active);
        this._dropdown.querySelector('.nav-dropdown-toggle').classList.toggle('active', active);
        // Moving a link can clear focus in WebKit. Keep its new location reachable.
        if (focused && this._primaryItems.some((item) => item.contains(focused))) {
            if (menu.contains(focused) && !this._dropdown.classList.contains('is-open')) {
                this._dropdown.querySelector('.nav-dropdown-toggle').focus();
            } else {
                focused.focus({ preventScroll: true });
            }
        }
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

// Dropping a PDF anywhere in the app hands it to the upload page. Files cannot
// survive navigation, so the dropped File is parked in IndexedDB and the
// upload page picks it up via ?dropped=1 (see UploadPage.consumeDroppedFiles).
const AppGlobalDrop = {
    DB_NAME: 'citebox-dropped-files',
    STORE: 'files',

    init() {
        if (this.initialized || !document.querySelector('.navbar')) return;
        this.initialized = true;
        let depth = 0;

        const hasFiles = (event) => Array.from(event.dataTransfer?.types || []).includes('Files');
        const ownDropzone = (event) => Boolean(event.target.closest?.('.dropzone, .drop-zone, [data-dropzone]'));

        document.addEventListener('dragenter', (event) => {
            if (!hasFiles(event) || ownDropzone(event)) return;
            depth += 1;
            this.showOverlay();
        });
        document.addEventListener('dragleave', (event) => {
            if (!hasFiles(event)) return;
            depth = Math.max(0, depth - 1);
            if (depth === 0) this.hideOverlay();
        });
        document.addEventListener('dragover', (event) => {
            if (!hasFiles(event) || ownDropzone(event)) return;
            event.preventDefault();
            event.dataTransfer.dropEffect = 'copy';
        });
        document.addEventListener('drop', async (event) => {
            if (!hasFiles(event)) return;
            depth = 0;
            this.hideOverlay();
            if (ownDropzone(event)) return;
            event.preventDefault();
            const files = Array.from(event.dataTransfer.files || []).filter((file) =>
                file.type === 'application/pdf' || /\.pdf$/i.test(file.name));
            if (!files.length) {
                if (typeof Utils !== 'undefined') Utils.showToast(t('shared.drop.only_pdf', '只能拖入 PDF 文件'), 'error');
                return;
            }
            if (typeof UploadPage !== 'undefined' && typeof UploadPage.acceptFiles === 'function') {
                UploadPage.acceptFiles(files);
                return;
            }
            try {
                await this.park(files);
                window.location.href = '/upload?dropped=1';
            } catch (error) {
                window.location.href = '/upload';
            }
        });
    },

    showOverlay() {
        if (!this.overlay) {
            this.overlay = document.createElement('div');
            this.overlay.className = 'global-drop-overlay';
            this.overlay.setAttribute('aria-hidden', 'true');
            this.overlay.innerHTML = `<div class="global-drop-overlay-card">${t('shared.drop.release_to_upload', '松开即可上传 PDF')}</div>`;
            document.body.appendChild(this.overlay);
        }
        this.overlay.classList.add('is-visible');
    },

    hideOverlay() {
        this.overlay?.classList.remove('is-visible');
    },

    openDB() {
        return new Promise((resolve, reject) => {
            const request = indexedDB.open(this.DB_NAME, 1);
            request.onupgradeneeded = () => request.result.createObjectStore(this.STORE);
            request.onsuccess = () => resolve(request.result);
            request.onerror = () => reject(request.error);
        });
    },

    async park(files) {
        const db = await this.openDB();
        await new Promise((resolve, reject) => {
            const tx = db.transaction(this.STORE, 'readwrite');
            tx.objectStore(this.STORE).put(files, 'pending');
            tx.oncomplete = resolve;
            tx.onerror = () => reject(tx.error);
        });
        db.close();
    },

    async take() {
        const db = await this.openDB();
        const files = await new Promise((resolve, reject) => {
            const tx = db.transaction(this.STORE, 'readwrite');
            const store = tx.objectStore(this.STORE);
            const request = store.get('pending');
            request.onsuccess = () => {
                store.delete('pending');
                resolve(request.result || []);
            };
            request.onerror = () => reject(request.error);
        });
        db.close();
        return files;
    }
};

// Closes open <details class="action-menu"> overflow menus on outside click,
// Escape, or after an item is chosen. Works for menus rendered at any time.
const AppActionMenus = {
    init() {
        if (this.initialized) return;
        this.initialized = true;

        document.addEventListener('click', (event) => {
            const item = event.target.closest('.action-menu-item');
            document.querySelectorAll('details.action-menu[open]').forEach((menu) => {
                if (!menu.contains(event.target) || (item && menu.contains(item))) {
                    menu.removeAttribute('open');
                }
            });
        });

        document.addEventListener('toggle', (event) => {
            const menu = event.target;
            if (!(menu instanceof HTMLDetailsElement) || !menu.classList.contains('action-menu') || !menu.open) return;
            document.querySelectorAll('details.action-menu[open]').forEach((other) => {
                if (other !== menu) other.removeAttribute('open');
            });
        }, true);

        document.addEventListener('keydown', (event) => {
            if (event.key !== 'Escape') return;
            const open = document.querySelector('details.action-menu[open]');
            if (!open) return;
            event.stopPropagation();
            open.removeAttribute('open');
            open.querySelector('summary')?.focus();
        }, true);
    }
};

const AppNavigationHotkeys = {
    routes: [
        { key: '1', path: '/', i18n: 'nav.overview', fallback: '总览' },
        { key: '2', path: '/library', i18n: 'nav.library', fallback: '文献库' },
        { key: '3', path: '/figures', i18n: 'nav.figures', fallback: '图片库' },
        { key: '4', path: '/notes', i18n: 'nav.notes', fallback: '笔记' },
        { key: '5', path: '/ai', i18n: 'nav.ai', fallback: 'AI 助手' },
        { key: '6', path: '/research', i18n: 'nav.research', fallback: '文献搜索' },
        { key: '9', path: '/settings', i18n: 'nav.settings', fallback: '设置' }
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

document.addEventListener('DOMContentLoaded', async () => {
    const path = window.location.pathname;
    AppNav.init();
    AppActionMenus.init();
    AppGlobalDrop.init();
    if (window.CiteBoxI18n && typeof window.CiteBoxI18n.init === 'function') {
        await window.CiteBoxI18n.init();
    }
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
