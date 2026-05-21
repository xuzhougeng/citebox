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
                href: 'http://localhost/library',
                origin: 'http://localhost',
            },
            history: {
                replaceState() {},
            },
            open() {},
            sessionStorage: {
                getItem() { return null; },
                setItem() {},
                removeItem() {},
            },
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

test('resourceViewerURL includes paper metadata for PDF reader links', () => {
    const Utils = loadUtils();
    const href = Utils.resourceViewerURL('pdf', '/files/papers/a.pdf', '/library', { paperId: 42 });
    const url = new URL(href, 'http://localhost');

    assert.equal(url.pathname, '/viewer');
    assert.equal(url.searchParams.get('kind'), 'pdf');
    assert.equal(url.searchParams.get('src'), '/files/papers/a.pdf');
    assert.equal(url.searchParams.get('back'), '/library');
    assert.equal(url.searchParams.get('paper_id'), '42');
});

test('parseResourceViewerNavigationURL returns paper metadata', () => {
    const Utils = loadUtils();
    const parsed = Utils.parseResourceViewerNavigationURL('/viewer?kind=pdf&src=%2Ffiles%2Fpapers%2Fa.pdf&back=%2Flibrary&paper_id=42');

    assert.deepEqual(JSON.parse(JSON.stringify(parsed)), {
        kind: 'pdf',
        src: '/files/papers/a.pdf',
        back: '/library',
        paperId: 42,
    });
});
