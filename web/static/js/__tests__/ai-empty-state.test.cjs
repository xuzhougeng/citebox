'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');

const modulePath = path.resolve(__dirname, '..', 'ai-empty-state.js');

function loadModule() {
    delete require.cache[modulePath];
    return require(modulePath);
}

function settingsWith(model, scene) {
    return {
        provider: 'openai',
        api_key: '',
        model: 'top-level-model',
        models: [model],
        scene_models: scene || { default_model_id: model.id },
    };
}

test('resolveAssistantModel follows master, then qa, then default binding', () => {
    const { resolveAssistantModel } = loadModule();
    const settings = {
        models: [
            { id: 'a', name: 'A' },
            { id: 'b', name: 'B' },
            { id: 'c', name: 'C' },
        ],
        scene_models: { assistant_master_model_id: '', qa_model_id: 'b', default_model_id: 'c' },
    };
    assert.equal(resolveAssistantModel(settings).id, 'b');
    settings.scene_models.assistant_master_model_id = 'a';
    assert.equal(resolveAssistantModel(settings).id, 'a');
});

test('resolveAssistantModel falls back to top-level settings for an unknown binding', () => {
    const { resolveAssistantModel } = loadModule();
    const model = resolveAssistantModel({
        provider: 'openai',
        api_key: 'key',
        model: 'gpt-x',
        models: [],
        scene_models: { default_model_id: 'missing' },
    });
    assert.equal(model.model, 'gpt-x');
    assert.equal(model.api_key, 'key');
    assert.equal(resolveAssistantModel({ models: [], scene_models: {} }), null);
});

test('assessModelReadiness reports a missing API key with the model name', () => {
    const { assessModelReadiness, readinessNotice } = loadModule();
    const readiness = assessModelReadiness(settingsWith({
        id: 'default-openai', name: 'OpenAI Default', provider: 'openai', api_key: ' ', model: 'gpt-4.1-mini',
    }));
    assert.equal(readiness.state, 'missing_api_key');
    assert.equal(readiness.name, 'OpenAI Default');
    assert.match(readinessNotice(readiness), /OpenAI Default/);
});

test('assessModelReadiness is ready with an API key and model id', () => {
    const { assessModelReadiness, readinessNotice } = loadModule();
    const readiness = assessModelReadiness(settingsWith({
        id: 'm', name: 'Mine', provider: 'anthropic', api_key: 'k', model: 'claude-x',
    }));
    assert.equal(readiness.state, 'ready');
    assert.equal(readinessNotice(readiness), '');
});

test('assessModelReadiness treats Codex as configured only when its status says so', () => {
    const { assessModelReadiness } = loadModule();
    const settings = settingsWith({ id: 'codex', name: 'Codex', provider: 'codex', api_key: '', model: 'gpt-5' });
    assert.equal(assessModelReadiness(settings, undefined).state, 'checking');
    assert.equal(assessModelReadiness(settings, null).state, 'unknown');
    assert.equal(assessModelReadiness(settings, {
        desktop_available: true, cli_available: true, authenticated: true,
    }).state, 'ready');
    const unavailable = assessModelReadiness(settings, {
        desktop_available: false, cli_available: true, authenticated: true, message: 'desktop only',
    });
    assert.equal(unavailable.state, 'codex_unavailable');
    assert.equal(unavailable.message, 'desktop only');
});

test('assessModelReadiness stays quiet when settings could not be loaded', () => {
    const { assessModelReadiness, readinessNotice } = loadModule();
    const readiness = assessModelReadiness(null);
    assert.equal(readiness.state, 'unknown');
    assert.equal(readinessNotice(readiness), '');
});

test('buildHTML links to AI settings only when the model is not ready', () => {
    const { buildHTML, SETTINGS_AI_HREF, EXAMPLE_PROMPTS } = loadModule();
    const notReady = buildHTML({ state: 'missing_api_key', name: 'OpenAI Default' });
    assert.ok(notReady.includes(SETTINGS_AI_HREF));
    assert.equal((notReady.match(/data-ai-empty-example=/g) || []).length, EXAMPLE_PROMPTS.length);
    assert.ok(!buildHTML({ state: 'ready', name: 'X' }).includes(SETTINGS_AI_HREF));
});

test('example prompts map to existing composer shortcuts', () => {
    const { EXAMPLE_PROMPTS } = loadModule();
    const composerPath = path.resolve(__dirname, '..', 'ai-composer.js');
    delete require.cache[composerPath];
    const { SHORTCUT_SPECS } = require(composerPath);
    const ids = new Set(SHORTCUT_SPECS.map((spec) => spec.id));
    EXAMPLE_PROMPTS.forEach((spec) => assert.ok(ids.has(spec.shortcutId), spec.shortcutId));
});

test('placeholderRange selects the bracketed slot in either locale', () => {
    const { placeholderRange } = loadModule();
    const zh = '为这句话找出处：【粘贴一句论断】';
    const [start, end] = placeholderRange(zh);
    assert.equal(zh.slice(start, end), '【粘贴一句论断】');
    const en = 'Reviews on [topic] please';
    const range = placeholderRange(en);
    assert.equal(en.slice(range[0], range[1]), '[topic]');
    assert.equal(placeholderRange('no slot'), null);
});

test('assessModelReadiness treats Magpie-managed settings as ready without a personal key', () => {
    const { assessModelReadiness } = loadModule();
    const settings = settingsWith({ id: 'm1', name: 'Personal', provider: 'openai', api_key: '', model: '' });
    assert.equal(assessModelReadiness(settings).state, 'missing_api_key');
    settings.runtime_managed = true;
    assert.equal(assessModelReadiness(settings).state, 'ready');
});
