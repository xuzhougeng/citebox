var CiteBoxI18n = {
    STORAGE_KEY: 'citebox_lang',
    DEFAULT_LANG: 'zh-CN',
    SUPPORTED: [
        { code: 'zh-CN', label: '中文' },
        { code: 'en', label: 'EN' }
    ],

    _common: {},
    _page: {},
    _shared: {},
    _ready: false,

    get: function() {
        try { return localStorage.getItem(this.STORAGE_KEY) || this.DEFAULT_LANG; }
        catch(e) { return this.DEFAULT_LANG; }
    },

    set: function(lang) {
        try { localStorage.setItem(this.STORAGE_KEY, lang); } catch(e) {}
    },

    t: function(key, fallback) {
        return this._page[key] || this._shared[key] || this._common[key] || fallback || key;
    },

    detectPage: function() {
        var p = window.location.pathname.replace(/\.html$/, '').replace(/^\//, '');
        return p || 'index';
    },

    _fetchJSON: function(url) {
        return fetch(url).then(function(r) {
            return r.ok ? r.json() : {};
        }).catch(function() { return {}; });
    },

    loadLocale: function() {
        var lang = this.get();
        var page = this.detectPage();
        var base = '/static/locales/' + lang;
        var self = this;

        return Promise.all([
            this._fetchJSON(base + '/common.json'),
            this._fetchJSON(base + '/' + page + '.json'),
            this._fetchJSON(base + '/shared.json')
        ]).then(function(results) {
            self._common = results[0];
            self._page = results[1];
            self._shared = results[2];
            self._ready = true;
        });
    },

    applyDOM: function() {
        var self = this;
        document.documentElement.lang = this.get();

        var titleKey = this._page['_title'] || this._common['_title.' + this.detectPage()];
        if (titleKey) document.title = titleKey;

        document.querySelectorAll('[data-i18n]').forEach(function(el) {
            el.textContent = self.t(el.dataset.i18n);
        });
        document.querySelectorAll('[data-i18n-placeholder]').forEach(function(el) {
            el.placeholder = self.t(el.dataset.i18nPlaceholder);
        });
        document.querySelectorAll('[data-i18n-title]').forEach(function(el) {
            el.title = self.t(el.dataset.i18nTitle);
        });
        document.querySelectorAll('[data-i18n-html]').forEach(function(el) {
            el.innerHTML = self.t(el.dataset.i18nHtml);
        });
        document.querySelectorAll('[data-i18n-aria-label]').forEach(function(el) {
            el.setAttribute('aria-label', self.t(el.dataset.i18nAriaLabel));
        });
    },

    _injectStyles: function() {
        if (document.getElementById('citebox-i18n-styles')) return;
        var style = document.createElement('style');
        style.id = 'citebox-i18n-styles';
        style.textContent = [
            '.lang-switcher { display:inline-flex; align-items:center; gap:0; margin-right:8px; }',
            '.lang-btn { padding:2px 7px; font-size:12px; font-weight:500; border:1px solid var(--border,#d5cdc4); background:transparent; color:var(--text-muted,#8b7e75); cursor:pointer; transition:all .2s; line-height:1.5; font-family:inherit; }',
            '.lang-btn:first-child { border-radius:4px 0 0 4px; }',
            '.lang-btn:last-child { border-radius:0 4px 4px 0; border-left:none; }',
            '.lang-btn.active { background:var(--accent,#c17f59); color:#fff; border-color:var(--accent,#c17f59); }',
            '.lang-btn:not(.active):hover { border-color:var(--accent,#c17f59); color:var(--accent,#c17f59); }'
        ].join('\n');
        document.head.appendChild(style);
    },

    injectSwitcher: function() {
        var target = document.querySelector('.nav-actions');
        if (!target) target = document.querySelector('.login-footer');
        if (!target || document.querySelector('.lang-switcher')) return;

        this._injectStyles();

        var current = this.get();
        var self = this;
        var switcher = document.createElement('div');
        switcher.className = 'lang-switcher';

        this.SUPPORTED.forEach(function(item) {
            var btn = document.createElement('button');
            btn.className = 'lang-btn' + (item.code === current ? ' active' : '');
            btn.textContent = item.label;
            btn.type = 'button';
            btn.setAttribute('aria-label', item.label);
            btn.addEventListener('click', function() {
                if (item.code !== current) {
                    self.set(item.code);
                    window.location.reload();
                }
            });
            switcher.appendChild(btn);
        });

        target.insertBefore(switcher, target.firstChild);
    },

    init: function() {
        var self = this;
        return this.loadLocale().catch(function() {
            self._ready = true;
        }).then(function() {
            window.t = self.t.bind(self);
            self.applyDOM();
            self.injectSwitcher();
        }).finally(function() {
            document.documentElement.removeAttribute('data-lang-loading');
        });
    }
};

if (typeof window.t !== 'function') {
    window.t = function(k, f) { return f || k; };
}

document.addEventListener('DOMContentLoaded', function() {
    CiteBoxI18n.init();
});
