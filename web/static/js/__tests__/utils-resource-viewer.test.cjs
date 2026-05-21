'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'utils.js');

function loadUtilsContext() {
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
            getElementById() {
                return null;
            },
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
    return { Utils: context.__TEST_UTILS__, context };
}

function loadUtils() {
    return loadUtilsContext().Utils;
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
        paperId: '42',
    });
});

test('resource viewer URLs preserve large paper identifiers as strings', () => {
    const Utils = loadUtils();
    const paperId = '1777519479295165603';
    const href = Utils.resourceViewerURL('pdf', '/files/papers/a.pdf', '/library', { paperId });
    const parsed = Utils.parseResourceViewerNavigationURL(href);

    assert.equal(new URL(href, 'http://localhost').searchParams.get('paper_id'), paperId);
    assert.equal(parsed.paperId, paperId);
});

test('modal restore state preserves large paper identifiers as strings', async () => {
    const { Utils, context } = loadUtilsContext();
    const paperId = '1777519479295165603';
    const visibleModal = {
        classList: {
            contains() { return false; },
        },
    };
    const hiddenModal = {
        classList: {
            contains() { return true; },
        },
    };
    let openedPaperId = '';

    context.PaperViewer = {
        modal: visibleModal,
        paper: { id: paperId, figures: [] },
        open(id) {
            openedPaperId = id;
            return Promise.resolve();
        },
    };
    context.document.getElementById = (id) => (id === 'paperModal' ? {} : null);

    const state = Utils.captureModalRestoreState();

    assert.equal(state.paperId, paperId);
    assert.equal(typeof state.paperId, 'string');
    assert.equal(Utils.isModalRestoreStateActive({ modal: 'paper', paperId }), true);

    context.PaperViewer.modal = hiddenModal;
    assert.equal(await Utils.restoreModalState({ modal: 'paper', paperId }), true);
    assert.equal(openedPaperId, paperId);
    assert.equal(typeof openedPaperId, 'string');
});
