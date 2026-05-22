'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const settingsModulePath = path.resolve(__dirname, '..', 'settings.js');
const settingsHTMLPath = path.resolve(__dirname, '..', '..', '..', 'settings.html');

function loadSettingsPage() {
    const code = fs.readFileSync(settingsModulePath, 'utf8') + '\nmodule.exports = SettingsPage;';
    const context = {
        console,
        module: { exports: {} },
        exports: {},
        t(_key, fallback) {
            return fallback || _key;
        },
        window: {
            t(_key, fallback) {
                return fallback || _key;
            },
        },
        document: {},
        API: {},
        Utils: {},
        setTimeout() {
            return 1;
        },
        clearTimeout() {},
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: settingsModulePath });
    return context.module.exports;
}

function makeSettingsSubject() {
    const SettingsPage = loadSettingsPage();
    const subject = Object.create(SettingsPage);
    subject.currentAIModelsForSave = () => [];
    subject.readSceneModelSelections = () => ({});
    subject.getAIPayloadModels = (models) => models;
    subject.createAIModelDraft = () => ({ id: 'model_1' });
    subject.renderAIModels = () => {};
    subject.renderSceneModelSelectors = () => {};
    subject.renderRolePromptList = () => {};
    subject.extractPromptSettings = () => ({});
    subject.systemPromptInput = { value: '' };
    subject.qaPromptInput = { value: '' };
    subject.figurePromptInput = { value: '' };
    subject.tagPromptInput = { value: '' };
    subject.groupPromptInput = { value: '' };
    subject.translatePromptInput = { value: '' };
    subject.ttsPromptInput = { value: '' };
    subject.temperatureInput = { value: '0.4' };
    subject.maxFiguresInput = { value: '3' };
    subject.pinPapersLimitInput = { value: '7' };
    subject.contextBudgetTokensInput = { value: '64000' };
    subject.translationPrimaryLanguageInput = { value: '中文' };
    subject.translationTargetLanguageInput = { value: 'English' };
    subject.imageGenEnabledInput = { checked: false };
    subject.imageGenAPIKeyInput = { value: '' };
    subject.imageGenBaseURLInput = { value: '' };
    subject.imageGenModelInput = { value: '' };
    subject.imageGenSizeSelect = { value: '' };
    subject.imageGenQualitySelect = { value: '' };
    return subject;
}

test('settings page exposes image generation controls in HTML', () => {
    const html = fs.readFileSync(settingsHTMLPath, 'utf8');

    for (const id of [
        'aiImageGenEnabledInput',
        'aiImageGenAPIKeyInput',
        'aiImageGenBaseURLInput',
        'aiImageGenModelInput',
        'aiImageGenSizeSelect',
        'aiImageGenQualitySelect',
    ]) {
        assert.match(html, new RegExp(`id="${id}"`), `expected ${id} control in settings.html`);
    }
});

test('settings page exposes per-model image input capability control in HTML', () => {
    const html = fs.readFileSync(settingsHTMLPath, 'utf8');

    assert.match(html, /id="aiModelSupportsImagesInput"/);
    assert.match(html, /data-i18n="settings\.ai\.supports_images"/);
});

test('getAIPayloadModels preserves per-model image input capability', () => {
    const SettingsPage = loadSettingsPage();
    const subject = Object.create(SettingsPage);

    const payload = subject.getAIPayloadModels([
        {
            id: 'qa',
            name: 'QA',
            provider: 'openai',
            model: 'text-only',
            max_output_tokens: 1200,
            supports_images: false,
        },
    ]);

    assert.equal(payload[0].supports_images, false);
});

test('buildAIModelSettingsPayload includes image generation settings', () => {
    const subject = makeSettingsSubject();
    subject.imageGenEnabledInput.checked = true;
    subject.imageGenAPIKeyInput.value = ' sk-image ';
    subject.imageGenBaseURLInput.value = ' https://images.example.com/v1/ ';
    subject.imageGenModelInput.value = ' gpt-image-2 ';
    subject.imageGenSizeSelect.value = '1536x1024';
    subject.imageGenQualitySelect.value = 'medium';

    const payload = subject.buildAIModelSettingsPayload();

    assert.deepEqual(JSON.parse(JSON.stringify(payload.image_gen)), {
        enabled: true,
        api_key: 'sk-image',
        base_url: 'https://images.example.com/v1/',
        model: 'gpt-image-2',
        size: '1536x1024',
        quality: 'medium',
    });
});

test('applyAISettings hydrates image generation controls', () => {
    const subject = makeSettingsSubject();

    subject.applyAISettings({
        models: [],
        image_gen: {
            enabled: true,
            api_key: 'sk-hydrated',
            base_url: 'https://images.example.com',
            model: 'gpt-image-2',
            size: '1024x1536',
            quality: 'low',
        },
        translation: {
            primary_language: '中文',
            target_language: 'English',
        },
    }, {
        overwritePromptInputs: false,
        overwriteRolePrompts: false,
    });

    assert.equal(subject.imageGenEnabledInput.checked, true);
    assert.equal(subject.imageGenAPIKeyInput.value, 'sk-hydrated');
    assert.equal(subject.imageGenBaseURLInput.value, 'https://images.example.com');
    assert.equal(subject.imageGenModelInput.value, 'gpt-image-2');
    assert.equal(subject.imageGenSizeSelect.value, '1024x1536');
    assert.equal(subject.imageGenQualitySelect.value, 'low');
});
