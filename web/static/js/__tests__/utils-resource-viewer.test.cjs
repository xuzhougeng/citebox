'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'utils.js');

function loadUtilsContext(options = {}) {
    const code = fs.readFileSync(modulePath, 'utf8') + '\nglobalThis.__TEST_UTILS__ = Utils;';
    const storage = new Map();
    const replacedURLs = [];
    const scrollCalls = [];
    const context = {
        console: console,
        setTimeout,
        clearTimeout,
        requestAnimationFrame(callback) {
            if (typeof callback === 'function') callback();
        },
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
            scrollX: options.scrollX ?? 0,
            scrollY: options.scrollY ?? 0,
            requestAnimationFrame(callback) {
                if (typeof callback === 'function') callback();
            },
            scrollTo(x, y) {
                scrollCalls.push({ x, y });
            },
            history: {
                replaceState(_state, _title, url) {
                    replacedURLs.push(String(url || ''));
                },
            },
            open() {},
            sessionStorage: {
                getItem(key) { return storage.get(key) || null; },
                setItem(key, value) { storage.set(key, String(value)); },
                removeItem(key) { storage.delete(key); },
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
    return { Utils: context.__TEST_UTILS__, context, storage, replacedURLs, scrollCalls };
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

test('resource viewer navigation URL stores scroll restore state for returning pages', () => {
    const { Utils, storage, replacedURLs } = loadUtilsContext({ scrollX: 8, scrollY: 1840 });

    const href = Utils.buildResourceViewerNavigationURL(
        'pdf',
        '/files/papers/a.pdf',
        'http://localhost/manual?paper_id=42',
        { replaceCurrentHistory: true, paperId: 42 }
    );

    const viewerURL = new URL(href, 'http://localhost');
    const backURL = new URL(viewerURL.searchParams.get('back'), 'http://localhost');
    const token = backURL.searchParams.get('restore_modal');

    assert.equal(viewerURL.pathname, '/viewer');
    assert.ok(token);
    assert.equal(new URL(replacedURLs[0], 'http://localhost').searchParams.get('restore_modal'), token);

    const stored = JSON.parse(storage.get(`citebox.modalRestore.${token}`));
    assert.equal(stored.scrollX, 8);
    assert.equal(stored.scrollY, 1840);
});

test('restoreModalState restores scroll-only viewer return state', async () => {
    const { Utils, scrollCalls } = loadUtilsContext();

    const restored = await Utils.restoreModalState({ scrollX: 12, scrollY: 2200 });

    assert.equal(restored, true);
    assert.deepEqual(scrollCalls.at(-1), { x: 12, y: 2200 });
});
