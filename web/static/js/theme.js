if (typeof window.t !== 'function') window.t = function(k,f){return f||k};
const CiteBoxTheme = {
    STORAGE_KEY: 'citebox_theme',
    APPEARANCE_API: '/api/settings/appearance',
    THEMES: ['warm', 'light', 'dark'],
    LABELS: { warm: '暖色', light: '明亮', dark: '暗黑' },
    DOTS: {
        warm:  { bg: '#efe6d7', accent: '#a45c40' },
        light: { bg: '#f2f4f8', accent: '#3868a8' },
        dark:  { bg: '#181a1e', accent: '#d4a060' },
    },

    get() {
        return localStorage.getItem(this.STORAGE_KEY) || 'warm';
    },

    apply(theme, options) {
        if (!this.THEMES.includes(theme)) theme = 'warm';
        var persist = !options || options.persist !== false;

        document.documentElement.classList.add('theme-transition');

        if (theme === 'warm') {
            document.documentElement.removeAttribute('data-theme');
        } else {
            document.documentElement.setAttribute('data-theme', theme);
        }

        localStorage.setItem(this.STORAGE_KEY, theme);

        if (persist) this.persistServer(theme);

        document.querySelectorAll('.theme-dot').forEach(function(dot) {
            dot.classList.toggle('active', dot.dataset.theme === theme);
        });

        setTimeout(function() {
            document.documentElement.classList.remove('theme-transition');
        }, 400);
    },

    // The desktop app starts its embedded server on a random loopback port,
    // so every launch is a new localStorage origin and the saved theme would
    // be lost. The server-side appearance setting is the durable store;
    // localStorage stays as the pre-paint cache and offline fallback.
    persistServer(theme) {
        try {
            fetch(this.APPEARANCE_API, {
                method: 'PUT',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ theme: theme })
            }).catch(function() {});
        } catch (e) {}
    },

    syncFromServer() {
        var self = this;
        var controller = typeof AbortController === 'function' ? new AbortController() : null;
        var timer = controller ? setTimeout(function() { controller.abort(); }, 2000) : null;
        return fetch(this.APPEARANCE_API, {
            credentials: 'same-origin',
            signal: controller ? controller.signal : undefined
        }).then(function(r) {
            if (timer) clearTimeout(timer);
            return r.ok ? r.json() : null;
        }).then(function(data) {
            if (data && data.theme && self.THEMES.includes(data.theme) && data.theme !== self.get()) {
                self.apply(data.theme, { persist: false });
            }
        }).catch(function() {});
    },

    injectSwitcher() {
        var navbar = document.querySelector('.nav-actions');
        if (!navbar || document.querySelector('.theme-switcher')) return;

        var switcher = document.createElement('div');
        switcher.className = 'theme-switcher';

        var current = this.get();
        var self = this;

        this.THEMES.forEach(function(theme) {
            var dot = document.createElement('button');
            dot.className = 'theme-dot' + (theme === current ? ' active' : '');
            dot.dataset.theme = theme;
            dot.title = t('shared.theme.' + theme, self.LABELS[theme]);
            dot.setAttribute('aria-label', t('shared.theme.' + theme, self.LABELS[theme]));
            dot.setAttribute('data-i18n-title', 'shared.theme.' + theme);
            dot.setAttribute('data-i18n-aria-label', 'shared.theme.' + theme);
            dot.style.setProperty('--dot-bg', self.DOTS[theme].bg);
            dot.style.setProperty('--dot-accent', self.DOTS[theme].accent);
            dot.addEventListener('click', function() { self.apply(theme); });
            switcher.appendChild(dot);
        });

        navbar.insertBefore(switcher, navbar.firstChild);
    },

    init() {
        this.injectSwitcher();
        this.syncFromServer();
    }
};

document.addEventListener('DOMContentLoaded', function() {
    CiteBoxTheme.init();
});
