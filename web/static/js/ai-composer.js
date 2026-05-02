(function (browserRoot, factory) {
    'use strict';

    const api = factory(browserRoot);
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    }
    if (browserRoot) {
        browserRoot.AIReader = browserRoot.AIReader || {};
        browserRoot.AIReader.composer = api.Composer;
    }
})(typeof window !== 'undefined' ? window : null, function (root) {
    'use strict';

    const SHORTCUT_SPECS = [
        {
            id: 'library_search',
            intentHint: 'library_search',
            i18nKey: 'ai.intent_library_search',
            fallbackLabel: '查全库',
        },
        {
            id: 'external_search_discovery',
            intentHint: 'external_search',
            searchGoalHint: 'discovery',
            i18nKey: 'ai.intent_external_search',
            fallbackLabel: '查外部',
        },
        {
            id: 'external_search_evidence',
            intentHint: 'external_search',
            searchGoalHint: 'evidence',
            i18nKey: 'ai.intent_external_evidence',
            fallbackLabel: '找出处',
        },
        {
            id: 'paper_read',
            intentHint: 'paper_read',
            i18nKey: 'ai.intent_paper_read',
            fallbackLabel: '读文献',
        },
        {
            id: 'figure_lookup',
            intentHint: 'figure_lookup',
            i18nKey: 'ai.intent_figure_lookup',
            fallbackLabel: '看图/图文',
        },
    ];

    const shortcutSpecById = SHORTCUT_SPECS.reduce((acc, spec) => {
        acc[spec.id] = spec;
        return acc;
    }, {});

    function translate(key, fallback) {
        if (root && root.CiteBoxI18n && typeof root.CiteBoxI18n.t === 'function') {
            return root.CiteBoxI18n.t(key, fallback);
        }
        if (root && typeof root.t === 'function') {
            return root.t(key, fallback);
        }
        return fallback || key;
    }

    function escapeHtml(value) {
        return String(value == null ? '' : value)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    function payloadForShortcut(shortcutId, content) {
        const payload = { content: content };
        const spec = shortcutSpecById[shortcutId];
        if (!spec) {
            return payload;
        }
        payload.intent_hint = spec.intentHint;
        if (spec.searchGoalHint) {
            payload.search_goal_hint = spec.searchGoalHint;
        }
        return payload;
    }

    const Composer = {
        init(opts) {
            opts = opts || {};
            this.input = opts.input || null;
            this.sendBtn = opts.sendBtn || null;
            this.stopBtn = opts.stopBtn || null;
            this.shortcutRoot = opts.shortcutRoot || null;
            this.onSend = opts.onSend || function () {};
            this.activeShortcutId = '';
            this._renderShortcuts();
            this._bind();
        },

        _bind() {
            if (this.sendBtn) {
                this.sendBtn.addEventListener('click', () => this.submit());
            }
        },

        _renderShortcuts() {
            if (!this.shortcutRoot) return;
            this.shortcutRoot.innerHTML = SHORTCUT_SPECS.map((spec) => (
                '<button class="ai-intent-shortcut" type="button" data-shortcut-id="' + escapeHtml(spec.id) + '" data-i18n="' + escapeHtml(spec.i18nKey) + '">' +
                    escapeHtml(translate(spec.i18nKey, spec.fallbackLabel)) +
                '</button>'
            )).join('');
            this.shortcutRoot.querySelectorAll('[data-shortcut-id]').forEach((button) => {
                button.addEventListener('click', () => {
                    const nextShortcutId = button.dataset.shortcutId || '';
                    const alreadyActive = this.activeShortcutId === nextShortcutId && button.classList.contains('is-active');
                    this.activeShortcutId = alreadyActive ? '' : nextShortcutId;
                    this.shortcutRoot.querySelectorAll('.is-active').forEach((el) => el.classList.remove('is-active'));
                    if (!alreadyActive) {
                        button.classList.add('is-active');
                    }
                    if (this.input) {
                        this.input.focus();
                    }
                });
            });
        },

        submit() {
            const content = ((this.input && this.input.value) || '').trim();
            let payload;
            if (!content) return;
            if (this.activeShortcutId) {
                payload = payloadForShortcut(this.activeShortcutId, content);
            } else {
                payload = { content: content };
            }
            this.activeShortcutId = '';
            if (this.shortcutRoot) {
                this.shortcutRoot.querySelectorAll('.is-active').forEach((el) => el.classList.remove('is-active'));
            }
            this.onSend(payload);
        },
    };

    return {
        Composer: Composer,
        SHORTCUT_SPECS: SHORTCUT_SPECS,
        payloadForShortcut: payloadForShortcut,
    };
});
