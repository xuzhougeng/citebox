// ai-empty-state.js — guidance shown in an empty conversation.
//
// Renders a one-line explanation, example prompts that fill the composer, and
// a notice when the model bound to the AI assistant cannot answer yet. The
// readiness check mirrors the backend's assistant model resolution
// (assistant_master_model_id → qa_model_id → default_model_id → top-level).

(function (browserRoot, factory) {
    'use strict';

    const api = factory(browserRoot);
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    }
    if (browserRoot) {
        browserRoot.AIReader = browserRoot.AIReader || {};
        browserRoot.AIReader.emptyState = api;
    }
})(typeof window !== 'undefined' ? window : null, function (root) {
    'use strict';

    const SETTINGS_AI_HREF = '/settings#settings-ai';

    const EXAMPLE_PROMPTS = [
        {
            id: 'library',
            i18nKey: 'ai.empty_example_library',
            fallback: '在我的文献库中查找讨论【主题】的论文，并概括各自的核心结论',
            shortcutId: 'library_search',
        },
        {
            id: 'source',
            i18nKey: 'ai.empty_example_source',
            fallback: '为这句话找出处：【粘贴一句论断】',
            shortcutId: 'external_search_evidence',
        },
        {
            id: 'discovery',
            i18nKey: 'ai.empty_example_discovery',
            fallback: '最近有哪些关于【主题】的综述？请推荐几篇',
            shortcutId: 'external_search_discovery',
        },
    ];

    function translate(key, fallback) {
        if (root && root.CiteBoxI18n && typeof root.CiteBoxI18n.t === 'function') {
            return root.CiteBoxI18n.t(key, fallback);
        }
        if (root && typeof root.t === 'function') {
            return root.t(key, fallback);
        }
        return fallback || key;
    }

    function format(key, fallback, values) {
        let text = translate(key, fallback);
        Object.keys(values || {}).forEach((name) => {
            text = text.split('{' + name + '}').join(values[name]);
        });
        return text;
    }

    function escapeHtml(value) {
        return String(value == null ? '' : value)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    function trim(value) {
        return String(value == null ? '' : value).trim();
    }

    function resolveAssistantModel(settings) {
        if (!settings || typeof settings !== 'object') return null;
        const scene = settings.scene_models || {};
        const models = Array.isArray(settings.models) ? settings.models : [];
        const modelID = [scene.assistant_master_model_id, scene.qa_model_id, scene.default_model_id]
            .map(trim)
            .find(Boolean);
        if (modelID) {
            const found = models.find((item) => item && trim(item.id) === modelID);
            if (found) return found;
        }
        if (!trim(settings.provider) && !trim(settings.model) && !trim(settings.api_key)) {
            return null;
        }
        return {
            id: '',
            name: '',
            provider: settings.provider,
            api_key: settings.api_key,
            model: settings.model,
        };
    }

    // codexStatus: undefined while the status request is pending, null when it
    // failed, or the GET /api/ai/codex/status body.
    function assessModelReadiness(settings, codexStatus) {
        if (!settings || typeof settings !== 'object') return { state: 'unknown' };
        // Magpie supplies the text models at runtime; personal settings may be empty.
        if (settings.runtime_managed === true) return { state: 'ready', name: 'Magpie' };
        const model = resolveAssistantModel(settings);
        if (!model) return { state: 'missing_model' };
        const name = trim(model.name) || trim(model.model) || trim(model.id);
        if (trim(model.provider) === 'codex') {
            if (codexStatus === undefined) return { state: 'checking', name };
            if (codexStatus === null) return { state: 'unknown', name };
            const ok = codexStatus.desktop_available === true
                && codexStatus.cli_available === true
                && codexStatus.authenticated === true;
            if (!ok) return { state: 'codex_unavailable', name, message: trim(codexStatus.message) };
            if (!trim(model.model)) return { state: 'missing_model_id', name };
            return { state: 'ready', name };
        }
        if (!trim(model.api_key)) return { state: 'missing_api_key', name };
        if (!trim(model.model)) return { state: 'missing_model_id', name };
        return { state: 'ready', name };
    }

    function readinessNotice(readiness) {
        const r = readiness || {};
        const name = r.name || translate('ai.empty_notice_unnamed_model', '未命名模型');
        switch (r.state) {
        case 'missing_model':
            return translate('ai.empty_notice_missing_model', '还没有配置 AI 模型，添加模型后才能提问。');
        case 'missing_api_key':
            return format('ai.empty_notice_missing_api_key', 'AI 助手使用的模型「{name}」还没有填写 API Key，提问会失败。', { name });
        case 'missing_model_id':
            return format('ai.empty_notice_missing_model_id', 'AI 助手使用的模型「{name}」还没有选择模型 ID。', { name });
        case 'codex_unavailable': {
            const base = format('ai.empty_notice_codex_unavailable', 'AI 助手使用的 Codex 模型「{name}」当前不可用。', { name });
            return r.message ? base + ' ' + r.message : base;
        }
        default:
            return '';
        }
    }

    // Range of the first 【…】 or […] placeholder, so typing replaces it.
    function placeholderRange(text) {
        const match = /【[^】]*】|\[[^\]]*\]/.exec(String(text || ''));
        if (!match) return null;
        return [match.index, match.index + match[0].length];
    }

    function buildHTML(readiness) {
        const notice = readinessNotice(readiness);
        const noticeHTML = notice
            ? '<div class="ai-empty-notice" role="status">' +
                '<span>' + escapeHtml(notice) + '</span>' +
                '<a class="ai-empty-notice-link" href="' + SETTINGS_AI_HREF + '">' +
                    escapeHtml(translate('ai.empty_notice_link', '前往 AI 模型配置')) +
                '</a>' +
            '</div>'
            : '';
        const examplesHTML = EXAMPLE_PROMPTS.map((spec) => (
            '<button class="ai-empty-example" type="button" data-ai-empty-example="' + escapeHtml(spec.id) + '">' +
                escapeHtml(translate(spec.i18nKey, spec.fallback)) +
            '</button>'
        )).join('');
        return (
            '<p class="ai-empty-lead">' +
                escapeHtml(translate('ai.empty_lead', '基于你的文献库提问，也可以检索外部文献或为论断找出处。')) +
            '</p>' +
            noticeHTML +
            '<p class="ai-empty-examples-label">' + escapeHtml(translate('ai.empty_examples_label', '试试这样问')) + '</p>' +
            '<div class="ai-empty-examples">' + examplesHTML + '</div>'
        );
    }

    // Renders into a fresh element appended to container; returns it.
    function render(container, options) {
        if (!container || typeof document === 'undefined') return null;
        const opts = options || {};
        const el = document.createElement('div');
        el.className = 'ai-empty-state';
        el.dataset.aiEmptyState = '1';
        el.innerHTML = buildHTML(opts.readiness);
        el.addEventListener('click', (e) => {
            const button = e.target.closest('[data-ai-empty-example]');
            if (!button) return;
            const spec = EXAMPLE_PROMPTS.find((item) => item.id === button.dataset.aiEmptyExample);
            if (!spec || typeof opts.onPickExample !== 'function') return;
            opts.onPickExample({
                id: spec.id,
                text: translate(spec.i18nKey, spec.fallback),
                shortcutId: spec.shortcutId,
            });
        });
        container.appendChild(el);
        return el;
    }

    return {
        EXAMPLE_PROMPTS,
        SETTINGS_AI_HREF,
        resolveAssistantModel,
        assessModelReadiness,
        readinessNotice,
        placeholderRange,
        buildHTML,
        render,
    };
});
