(function () {
    'use strict';

    const intents = ['library_search', 'external_search', 'paper_read', 'figure_lookup'];

    function translate(key, fallback) {
        if (window.CiteBoxI18n && typeof window.CiteBoxI18n.t === 'function') {
            return window.CiteBoxI18n.t(key, fallback);
        }
        if (typeof window.t === 'function') {
            return window.t(key, fallback);
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

    const Composer = {
        init(opts) {
            opts = opts || {};
            this.input = opts.input || null;
            this.sendBtn = opts.sendBtn || null;
            this.stopBtn = opts.stopBtn || null;
            this.shortcutRoot = opts.shortcutRoot || null;
            this.onSend = opts.onSend || function () {};
            this.intentHint = '';
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
            const labels = {
                library_search: translate('ai.intent_library_search', '查全库'),
                external_search: translate('ai.intent_external_search', '查外部'),
                paper_read: translate('ai.intent_paper_read', '读文献'),
                figure_lookup: translate('ai.intent_figure_lookup', '看图/图文'),
            };
            this.shortcutRoot.innerHTML = intents.map((intent) => (
                '<button class="ai-intent-shortcut" type="button" data-intent="' + escapeHtml(intent) + '">' +
                    escapeHtml(labels[intent]) +
                '</button>'
            )).join('');
            this.shortcutRoot.querySelectorAll('[data-intent]').forEach((button) => {
                button.addEventListener('click', () => {
                    const nextIntent = button.dataset.intent || '';
                    const alreadyActive = this.intentHint === nextIntent && button.classList.contains('is-active');
                    this.intentHint = alreadyActive ? '' : nextIntent;
                    this.shortcutRoot.querySelectorAll('.is-active').forEach((el) => el.classList.remove('is-active'));
                    if (!alreadyActive) button.classList.add('is-active');
                    if (this.input) this.input.focus();
                });
            });
        },

        submit() {
            const content = (this.input && this.input.value || '').trim();
            if (!content) return;
            const payload = { content: content };
            if (this.intentHint) payload.intent_hint = this.intentHint;
            this.intentHint = '';
            if (this.shortcutRoot) {
                this.shortcutRoot.querySelectorAll('.is-active').forEach((el) => el.classList.remove('is-active'));
            }
            this.onSend(payload);
        },
    };

    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.composer = Composer;
    }
})();
