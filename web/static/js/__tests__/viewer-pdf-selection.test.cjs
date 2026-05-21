'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'viewer.js');

function createElement(name) {
    return {
        name,
        children: new Set(),
        classList: {
            classes: new Set(),
            add(value) { this.classes.add(value); },
            remove(value) { this.classes.delete(value); },
            contains(value) { return this.classes.has(value); },
        },
        style: {},
        contains(node) {
            if (node === this) return true;
            return this.children.has(node) || this.children.has(node?.parentElement);
        },
        appendChild(child) {
            child.parentElement = this;
            this.children.add(child);
        },
        querySelector(selector) {
            if (selector === '[data-pdf-scroll]') return this.pdfScroll || null;
            return null;
        },
        getBoundingClientRect() {
            return { left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 };
        },
    };
}

function loadViewerPage(selection) {
    const code = `${fs.readFileSync(modulePath, 'utf8')}\nglobalThis.__RESOURCE_VIEWER_PAGE__ = ResourceViewerPage;`;
    const context = {
        console,
        URL,
        URLSearchParams,
        Node: { TEXT_NODE: 3 },
        navigator: {},
        window: {
            location: {
                href: 'http://localhost/viewer?kind=pdf&src=%2Fpaper.pdf',
                origin: 'http://localhost',
                search: '?kind=pdf&src=%2Fpaper.pdf',
            },
            history: { length: 1 },
            innerWidth: 1280,
            innerHeight: 800,
            setTimeout,
            clearTimeout,
            getSelection() {
                return selection;
            },
        },
        document: {
            referrer: '',
            addEventListener() {},
            createElement(tagName) {
                return createElement(tagName);
            },
            head: createElement('head'),
        },
        t(key, fallback) {
            return fallback || key;
        },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: modulePath });
    return context.__RESOURCE_VIEWER_PAGE__;
}

function rect(left, top, right, bottom) {
    return {
        left,
        top,
        right,
        bottom,
        width: right - left,
        height: bottom - top,
    };
}

test('currentPDFSelectionText reads the browser native selection text', () => {
    const scroll = createElement('scroll');
    const textLayer = createElement('textLayer');
    const textNode = { nodeType: 3, parentElement: textLayer };
    scroll.appendChild(textLayer);
    const selection = {
        anchorNode: textNode,
        focusNode: textNode,
        toString() {
            return '  Among the genomic alterations observed in ER+ breast cancer,\nARID1A being  ';
        },
    };
    const viewer = loadViewerPage(selection);
    viewer.stage = createElement('stage');
    viewer.stage.pdfScroll = scroll;

    assert.equal(
        viewer.currentPDFSelectionText(),
        'Among the genomic alterations observed in ER+ breast cancer,\nARID1A being'
    );
});

test('defaultPDFState uses official PDF.js viewer state fields', () => {
    const viewer = loadViewerPage(null);
    const state = viewer.defaultPDFState();

    assert.equal(state.pdfjsLib, null);
    assert.equal(state.pdfjsViewerLib, null);
    assert.equal(state.eventBus, null);
    assert.equal(state.linkService, null);
    assert.equal(state.pdfViewer, null);
    assert.equal(state.loadToken, 0);
    assert.equal(Object.hasOwn(state, 'renderTask'), false);
    assert.equal(Object.hasOwn(state, 'textLayer'), false);
    assert.equal(Object.hasOwn(state, 'selectionDrag'), false);
});

test('isCurrentPDFLoad checks load token and viewer identity', () => {
    const viewer = loadViewerPage(null);
    const pdfViewer = {};
    viewer.pdfState = {
        ...viewer.defaultPDFState(),
        loadToken: 4,
        pdfViewer,
    };

    assert.equal(viewer.isCurrentPDFLoad(4), true);
    assert.equal(viewer.isCurrentPDFLoad(4, pdfViewer), true);
    assert.equal(viewer.isCurrentPDFLoad(5), false);
    assert.equal(viewer.isCurrentPDFLoad(4, {}), false);
});

test('currentPDFSelectionText ignores browser native selection outside the PDF viewer', () => {
    const scroll = createElement('scroll');
    const outside = createElement('outside');
    const selection = {
        anchorNode: outside,
        focusNode: outside,
        toString() {
            return 'outside page selection';
        },
    };
    const viewer = loadViewerPage(selection);
    viewer.stage = createElement('stage');
    viewer.stage.pdfScroll = scroll;
    viewer.pdfState = viewer.defaultPDFState();
    viewer.pdfState.selectionText = '  stored PDF selection  ';

    assert.equal(viewer.currentPDFSelectionText(), 'stored PDF selection');
});

test('selectionBelongsToPDFViewer accepts selections inside the PDF scroll area', () => {
    const scroll = createElement('scroll');
    const textLayer = createElement('textLayer');
    const textNode = { nodeType: 3, parentElement: textLayer };
    scroll.appendChild(textLayer);
    const selection = {
        anchorNode: textNode,
        focusNode: textNode,
        toString() { return 'ER+ breast cancer'; },
    };
    const viewer = loadViewerPage(selection);
    viewer.stage = createElement('stage');
    viewer.stage.pdfScroll = scroll;

    assert.equal(viewer.selectionBelongsToPDFViewer(selection), true);
});

test('selectionBelongsToPDFViewer rejects selections outside the PDF scroll area', () => {
    const scroll = createElement('scroll');
    const outside = createElement('outside');
    const selection = {
        anchorNode: outside,
        focusNode: outside,
        toString() { return 'outside text'; },
    };
    const viewer = loadViewerPage(selection);
    viewer.stage = createElement('stage');
    viewer.stage.pdfScroll = scroll;

    assert.equal(viewer.selectionBelongsToPDFViewer(selection), false);
});

test('pdfSelectionClientRect unions visible native selection rects', () => {
    const selection = {
        rangeCount: 1,
        getRangeAt() {
            return {
                getClientRects() {
                    return [
                        rect(40, 80, 140, 104),
                        rect(38, 112, 220, 136),
                    ];
                },
                getBoundingClientRect() {
                    return rect(40, 80, 220, 136);
                },
            };
        },
        toString() { return 'two selected lines'; },
    };
    const viewer = loadViewerPage(selection);
    const actual = viewer.pdfSelectionClientRect(selection);

    for (const [field, value] of Object.entries(rect(38, 80, 220, 136))) {
        assert.equal(actual[field], value);
    }
});
