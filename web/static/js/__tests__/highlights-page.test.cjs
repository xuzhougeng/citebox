'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'highlights.js');

function createElement(id) {
    return {
        id,
        value: '',
        innerHTML: '',
        textContent: '',
        dataset: {},
        listeners: {},
        addEventListener(type, handler) { this.listeners[type] = handler; },
        closest() { return null; },
    };
}

function flushAsync(times = 1) {
    let chain = Promise.resolve();
    for (let i = 0; i < times; i += 1) {
        chain = chain.then(() => new Promise((resolve) => setImmediate(resolve)));
    }
    return chain;
}

function createDeferred() {
    let resolve;
    let reject;
    const promise = new Promise((promiseResolve, promiseReject) => {
        resolve = promiseResolve;
        reject = promiseReject;
    });
    return { promise, resolve, reject };
}

function annotation(overrides = {}) {
    return {
        id: 11,
        paper_id: 42,
        paper_title: 'Example Paper',
        paper_pdf_url: '/files/papers/example.pdf',
        page_start: 3,
        page_end: 3,
        quote_text: 'selected highlight text',
        updated_at: '2026-05-22T10:00:00Z',
        ...overrides,
    };
}

function loadPage(options = {}) {
    const elements = {
        highlightQuery: createElement('highlightQuery'),
        highlightSort: createElement('highlightSort'),
        highlightResultMeta: createElement('highlightResultMeta'),
        highlightList: createElement('highlightList'),
        highlightPagination: createElement('highlightPagination'),
    };
    const viewerURLs = [];
    const listPDFAnnotationsGlobal = options.listPDFAnnotationsGlobal || (() => Promise.resolve({
        annotations: [annotation()],
        pagination: { page: 1, page_size: 50, total: 1, total_pages: 1 },
    }));
    const code = fs.readFileSync(modulePath, 'utf8') + '\nglobalThis.__TEST_HIGHLIGHTS__ = HighlightLibraryPage;';
    const context = {
        console,
        URL,
        URLSearchParams,
        window: {
            location: { href: 'http://localhost/highlights' },
        },
        document: {
            addEventListener(type, handler) {
                if (type === 'DOMContentLoaded') {
                    if (options.autoStart !== false) handler();
                }
            },
            getElementById(id) {
                return elements[id] || null;
            },
        },
        API: {
            listPDFAnnotationsGlobal,
            deletePDFAnnotation() {
                return Promise.resolve({ success: true });
            },
        },
        Utils: {
            escapeHTML(value) { return String(value || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;'); },
            formatDate() { return '2026/05/22 10:00'; },
            renderPagination() {},
            bindPagination() {},
            showToast() {},
            confirm() { return Promise.resolve(true); },
            resourceViewerURL(kind, src, back, options) {
                viewerURLs.push({ kind, src, back, options: JSON.parse(JSON.stringify(options)) });
                return `/viewer?kind=${kind}&src=${encodeURIComponent(src)}&paper_id=${options.paperId}&page=${options.page}&annotation_id=${options.annotationId}`;
            },
        },
        t(key, fallback) { return fallback || key; },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: modulePath });
    return { page: context.__TEST_HIGHLIGHTS__, elements, context, viewerURLs };
}

test('highlight library renders annotations and opens target PDF URL', async () => {
    const { page, elements, context, viewerURLs } = loadPage();
    await flushAsync();

    assert.match(elements.highlightList.innerHTML, /selected highlight text/);
    assert.match(elements.highlightList.innerHTML, /Example Paper/);
    page.openHighlight(11);

    assert.equal(context.window.location.href, '/viewer?kind=pdf&src=%2Ffiles%2Fpapers%2Fexample.pdf&paper_id=42&page=3&annotation_id=11');
    assert.deepEqual(viewerURLs[0].options, {
        paperId: 42,
        annotationId: 11,
        page: 3,
    });
});

test('highlight library opens unsafe integer-sized string IDs without precision loss', async () => {
    const { page, context, viewerURLs } = loadPage({
        listPDFAnnotationsGlobal() {
            return Promise.resolve({
                annotations: [annotation({
                    id: '9007199254740993',
                    paper_id: '9007199254740995',
                    quote_text: 'unsafe id highlight',
                })],
                pagination: { page: 1, page_size: 50, total: 1, total_pages: 1 },
            });
        },
    });
    await flushAsync();

    page.openHighlight('9007199254740993');

    assert.equal(context.window.location.href, '/viewer?kind=pdf&src=%2Ffiles%2Fpapers%2Fexample.pdf&paper_id=9007199254740995&page=3&annotation_id=9007199254740993');
    assert.deepEqual(viewerURLs[0].options, {
        paperId: '9007199254740995',
        annotationId: '9007199254740993',
        page: 3,
    });
});

test('out-of-range empty page reloads the last valid page', async () => {
    const requests = [];
    const { page, elements } = loadPage({
        autoStart: false,
        listPDFAnnotationsGlobal(params) {
            requests.push(JSON.parse(JSON.stringify(params)));
            if (params.page === 3) {
                return Promise.resolve({
                    annotations: [],
                    pagination: { page: 3, page_size: 50, total: 51, total_pages: 2 },
                });
            }
            return Promise.resolve({
                annotations: [annotation({ id: 21, quote_text: 'last valid page highlight' })],
                pagination: { page: 2, page_size: 50, total: 51, total_pages: 2 },
            });
        },
    });

    page.state.page = 3;
    page.init();
    await flushAsync(3);

    assert.deepEqual(requests.map((request) => request.page), [3, 2]);
    assert.equal(page.state.page, 2);
    assert.match(elements.highlightList.innerHTML, /last valid page highlight/);
});

test('stale older load response does not overwrite newer load response', async () => {
    const first = createDeferred();
    const second = createDeferred();
    const requests = [];
    const { page, elements } = loadPage({
        autoStart: false,
        listPDFAnnotationsGlobal(params) {
            requests.push(JSON.parse(JSON.stringify(params)));
            return requests.length === 1 ? first.promise : second.promise;
        },
    });

    page.init();
    page.state.query = 'newer';
    page.load();
    await flushAsync();

    second.resolve({
        annotations: [annotation({ id: 31, quote_text: 'newer highlight' })],
        pagination: { page: 1, page_size: 50, total: 1, total_pages: 1 },
    });
    await flushAsync();

    first.resolve({
        annotations: [annotation({ id: 30, quote_text: 'older highlight' })],
        pagination: { page: 1, page_size: 50, total: 1, total_pages: 1 },
    });
    await flushAsync();

    assert.deepEqual(requests.map((request) => request.query), ['', 'newer']);
    assert.match(elements.highlightList.innerHTML, /newer highlight/);
    assert.doesNotMatch(elements.highlightList.innerHTML, /older highlight/);
});
