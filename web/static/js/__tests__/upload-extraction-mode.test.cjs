const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');

const uploadSource = fs.readFileSync(path.resolve(__dirname, '../upload.js'), 'utf8');
const UploadPage = vm.runInNewContext(`${uploadSource}\nUploadPage;`, {});

const builtInExtractor = {
    extractor_profile: 'open_source_vision',
    pdf_text_source: 'pdfjs'
};

test('built-in extraction accepts a Codex subscription model without an API key', () => {
    const aiSettings = {
        models: [{
            id: 'codex-figure',
            provider: 'codex',
            api_key: '',
            model: 'gpt-5.2-codex'
        }],
        scene_models: { figure_model_id: 'codex-figure' }
    };

    assert.equal(UploadPage.isBuiltInLLMReady(builtInExtractor, aiSettings), true);
    assert.equal(UploadPage.isAutoExtractionReady(builtInExtractor, aiSettings), true);
});

test('built-in extraction still requires an API key for API-backed models', () => {
    const aiSettings = {
        models: [{
            id: 'openai-figure',
            provider: 'openai',
            api_key: '',
            model: 'gpt-4.1-mini'
        }],
        scene_models: { figure_model_id: 'openai-figure' }
    };

    assert.equal(UploadPage.isBuiltInLLMReady(builtInExtractor, aiSettings), false);
    aiSettings.models[0].api_key = 'test-key';
    assert.equal(UploadPage.isBuiltInLLMReady(builtInExtractor, aiSettings), true);
});
