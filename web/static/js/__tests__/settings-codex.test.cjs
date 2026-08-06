const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const assert = require('node:assert/strict');
const CodexModels = require('../codex-model-capabilities.js');

const root = path.resolve(__dirname, '../../..');

test('settings page exposes Codex subscription as a distinct provider', () => {
    const html = fs.readFileSync(path.join(root, 'settings.html'), 'utf8');
    assert.match(html, /<option value="codex" data-i18n="settings\.ai\.provider_codex">Codex Subscription<\/option>/);
    assert.match(html, /id="codexModelOptions"/);
    assert.match(html, /codex-model-capabilities\.js/);
});

test('API client wires desktop Codex status and model discovery endpoints', () => {
    const source = fs.readFileSync(path.join(root, 'static/js/api.js'), 'utf8');
    assert.match(source, /\/ai\/codex\/status/);
    assert.match(source, /\/ai\/codex\/models/);
});

test('Codex settings copy stays aligned in Chinese and English', () => {
    const zh = JSON.parse(fs.readFileSync(path.join(root, 'static/locales/zh-CN/settings.json'), 'utf8'));
    const en = JSON.parse(fs.readFileSync(path.join(root, 'static/locales/en/settings.json'), 'utf8'));
    assert.ok(zh['settings.ai.provider_note_codex']);
    assert.ok(en['settings.ai.provider_note_codex']);
    assert.ok(zh['settings.ai.provider_codex']);
    assert.ok(en['settings.ai.provider_codex']);
});

test('Codex model capabilities follow advertised image modalities and reasoning efforts', () => {
    const catalog = [{
        id: 'text-only',
        input_modalities: ['text'],
        default_reasoning_effort: 'high',
        supported_reasoning_efforts: ['low', 'high', 'future-value']
    }];

    assert.deepEqual(CodexModels.resolveCapabilities(catalog, 'text-only'), {
        supportsImages: false,
        supportedReasoningEfforts: ['low', 'high'],
        defaultReasoningEffort: 'high'
    });
});

test('Codex model capabilities fall back safely when optional catalog fields are absent', () => {
    assert.deepEqual(CodexModels.resolveCapabilities([{ id: 'default' }], 'default'), {
        supportsImages: true,
        supportedReasoningEfforts: [],
        defaultReasoningEffort: ''
    });
    assert.equal(CodexModels.resolveCapabilities([], 'missing'), null);
});
