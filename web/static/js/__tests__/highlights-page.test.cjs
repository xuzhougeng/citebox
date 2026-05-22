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

function loadPage() {
    const elements = {
        highlightQuery: createElement('highlightQuery'),
        highlightSort: createElement('highlightSort'),
        highlightResultMeta: createElement('highlightResultMeta'),
        highlightList: createElement('highlightList'),
        highlightPagination: createElement('highlightPagination'),
    };
    const viewerURLs = [];
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
                if (type === 'DOMContentLoaded') handler();
            },
            getElementById(id) {
                return elements[id] || null;
            },
        },
        API: {
            listPDFAnnotationsGlobal() {
                return Promise.resolve({
                    annotations: [{
                        id: 11,
                        paper_id: 42,
                        paper_title: 'Example Paper',
                        paper_pdf_url: '/files/papers/example.pdf',
                        page_start: 3,
                        page_end: 3,
                        quote_text: 'selected highlight text',
                        updated_at: '2026-05-22T10:00:00Z',
                    }],
                    pagination: { page: 1, page_size: 50, total: 1, total_pages: 1 },
                });
            },
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
    await new Promise((resolve) => setImmediate(resolve));

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
