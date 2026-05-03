'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'ai-result-cards.js');

function loadCards() {
    const code = fs.readFileSync(modulePath, 'utf8');
    const context = {
        console: console,
        document: {
            __citeboxAIResultCardsBound: false,
            addEventListener() {},
        },
        navigator: {},
        window: {
            location: { origin: 'http://localhost' },
        },
        t(key, fallback) {
            return fallback || key;
        },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: modulePath });
    return context.window.AIReader.resultCards.render;
}

const render = loadCards();

test('renderCard - generated_image: renders thumbnail trigger, prompt, download and copy buttons', () => {
    const html = render([{
        card_type: 'generated_image',
        payload: {
            image_id: 7,
            file_url: '/api/ai-generated-images/7/file',
            prompt: 'a graphical abstract of CRISPR',
            model: 'gpt-image-2',
            size: '1024x1024',
            quality: 'high',
            cost_estimate_usd: 0.19,
        },
    }]);
    assert.ok(html.includes('/api/ai-generated-images/7/file'), 'should contain file URL');
    assert.ok(html.includes('data-ai-image-preview='), 'should contain image preview trigger');
    assert.ok(html.includes('ai-generated-image-thumb'), 'should contain thumbnail class');
    assert.ok(html.includes('a graphical abstract of CRISPR'), 'should contain prompt text');
    assert.ok(html.includes('gpt-image-2 · 1024x1024 · high · $0.19'), 'should contain meta line');
    assert.ok(html.includes('data-ai-image-gen-copy='), 'should contain copy button attribute');
    assert.ok(html.includes('download'), 'should contain download attribute');
});

test('renderCard - generated_image: handles missing file_url gracefully', () => {
    const html = render([{
        card_type: 'generated_image',
        payload: {
            prompt: 'test prompt',
            model: 'gpt-image-2',
        },
    }]);
    assert.ok(html.includes('ai-figure-missing'), 'should show missing image placeholder');
    assert.ok(!html.includes('<img'), 'should not render img tag when URL is missing');
});

test('renderCard - generated_image: handles null cost_estimate_usd', () => {
    const html = render([{
        card_type: 'generated_image',
        payload: {
            file_url: '/api/ai-generated-images/1/file',
            model: 'gpt-image-2',
            size: '1024x1024',
        },
    }]);
    assert.ok(html.includes('gpt-image-2 · 1024x1024'), 'should include model and size');
    assert.ok(!html.includes('$'), 'should not include cost when null');
});
