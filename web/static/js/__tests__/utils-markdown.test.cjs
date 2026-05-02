'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'utils.js');

function loadUtils() {
    const code = fs.readFileSync(modulePath, 'utf8') + '\nglobalThis.__TEST_UTILS__ = Utils;';
    const context = {
        console: console,
        setTimeout,
        clearTimeout,
        URL,
        URLSearchParams,
        MutationObserver: class MutationObserver {
            observe() {}
            disconnect() {}
        },
        window: {
            location: {
                href: 'http://localhost/ai',
                origin: 'http://localhost',
            },
            history: {
                replaceState() {},
            },
            open() {},
        },
        document: {
            readyState: 'complete',
            body: {},
            addEventListener() {},
            querySelectorAll() {
                return [];
            },
        },
        t(key, fallback) {
            return fallback || key;
        },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: modulePath });
    return context.__TEST_UTILS__;
}

test('renderMarkdown renders pipe tables through the shared markdown entrypoint', () => {
    const Utils = loadUtils();
    const html = Utils.renderMarkdown([
        '| Name | Value |',
        '| --- | --- |',
        '| code | `x` |',
    ].join('\n'));

    assert.match(html, /<table/);
    assert.match(html, /<thead>/);
    assert.match(html, /<tbody>/);
    assert.match(html, /<code class="markdown-inline-code">x<\/code>/);
});
