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
            descI18nKey: 'ai.intent_library_search_desc',
            fallbackDescription: '开启后，下一条消息只在你的文献库中检索并回答（再次点击取消）',
        },
        {
            id: 'external_search_discovery',
            intentHint: 'external_search',
            searchGoalHint: 'discovery',
            i18nKey: 'ai.intent_external_search',
            fallbackLabel: '查外部',
            descI18nKey: 'ai.intent_external_search_desc',
            fallbackDescription: '开启后，下一条消息到 PubMed、Semantic Scholar 等外部来源检索相关文献',
        },
        {
            id: 'external_search_evidence',
            intentHint: 'external_search',
            searchGoalHint: 'evidence',
            i18nKey: 'ai.intent_external_evidence',
            fallbackLabel: '找出处',
            descI18nKey: 'ai.intent_external_evidence_desc',
            fallbackDescription: '开启后，下一条消息为你给出的论断查找可引用的原始出处',
        },
        {
            id: 'paper_read',
            intentHint: 'paper_read',
            i18nKey: 'ai.intent_paper_read',
            fallbackLabel: '读文献',
            descI18nKey: 'ai.intent_paper_read_desc',
            fallbackDescription: '开启后，下一条消息精读已钉住的文献，回答其中的细节',
        },
        {
            id: 'figure_lookup',
            intentHint: 'figure_lookup',
            i18nKey: 'ai.intent_figure_lookup',
            fallbackLabel: '看图/图文',
            descI18nKey: 'ai.intent_figure_lookup_desc',
            fallbackDescription: '开启后，下一条消息检索并解读文献中的图片和图注',
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

        // Shortcuts are one-turn toggles: the pressed one routes the next
        // message, then resets. The title is both tooltip and accessible
        // description, so the short label is not the only cue.
        _renderShortcuts() {
            if (!this.shortcutRoot) return;
            this.shortcutRoot.innerHTML = SHORTCUT_SPECS.map((spec) => (
                '<button class="ai-intent-shortcut" type="button" aria-pressed="false"' +
                    ' data-shortcut-id="' + escapeHtml(spec.id) + '"' +
                    ' data-i18n="' + escapeHtml(spec.i18nKey) + '"' +
                    ' data-i18n-title="' + escapeHtml(spec.descI18nKey) + '"' +
                    ' title="' + escapeHtml(translate(spec.descI18nKey, spec.fallbackDescription)) + '">' +
                    escapeHtml(translate(spec.i18nKey, spec.fallbackLabel)) +
                '</button>'
            )).join('');
            this.shortcutRoot.querySelectorAll('[data-shortcut-id]').forEach((button) => {
                button.addEventListener('click', () => {
                    const nextShortcutId = button.dataset.shortcutId || '';
                    const alreadyActive = this.activeShortcutId === nextShortcutId && button.classList.contains('is-active');
                    this.setActiveShortcut(alreadyActive ? '' : nextShortcutId);
                    if (this.input) {
                        this.input.focus();
                    }
                });
            });
        },

        setActiveShortcut(shortcutId) {
            const nextId = shortcutSpecById[shortcutId] ? shortcutId : '';
            this.activeShortcutId = nextId;
            if (!this.shortcutRoot) return;
            this.shortcutRoot.querySelectorAll('[data-shortcut-id]').forEach((button) => {
                const pressed = !!nextId && button.dataset.shortcutId === nextId;
                if (pressed) {
                    button.classList.add('is-active');
                } else {
                    button.classList.remove('is-active');
                }
                if (typeof button.setAttribute === 'function') {
                    button.setAttribute('aria-pressed', pressed ? 'true' : 'false');
                }
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
            this.setActiveShortcut('');
            this.onSend(payload);
        },
    };

    return {
        Composer: Composer,
        SHORTCUT_SPECS: SHORTCUT_SPECS,
        payloadForShortcut: payloadForShortcut,
    };
});
