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

function loadViewerPage(selection, options = {}) {
    const code = `${fs.readFileSync(modulePath, 'utf8')}\nglobalThis.__RESOURCE_VIEWER_PAGE__ = ResourceViewerPage;`;
    const context = {
        console,
        Map: options.Map || Map,
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
    return {
        context,
        viewer: context.__RESOURCE_VIEWER_PAGE__
    };
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
    const { viewer } = loadViewerPage(selection);
    viewer.stage = createElement('stage');
    viewer.stage.pdfScroll = scroll;

    assert.equal(
        viewer.currentPDFSelectionText(),
        'Among the genomic alterations observed in ER+ breast cancer,\nARID1A being'
    );
});

test('defaultPDFState uses official PDF.js viewer state fields', () => {
    const { viewer } = loadViewerPage(null);
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
    const { viewer } = loadViewerPage(null);
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

test('ensurePDFViewerCompatibility adds Map getOrInsertComputed for bundled PDF.js viewer', () => {
    class TestMap extends Map {}
    const { context, viewer } = loadViewerPage(null, { Map: TestMap });
    assert.equal(typeof context.Map.prototype.getOrInsertComputed, 'undefined');

    viewer.ensurePDFViewerCompatibility();
    const map = new context.Map();
    let calls = 0;
    const first = map.getOrInsertComputed('key', (key) => {
        calls += 1;
        return `${key}-value`;
    });
    const second = map.getOrInsertComputed('key', () => {
        calls += 1;
        return 'ignored';
    });

    assert.equal(first, 'key-value');
    assert.equal(second, 'key-value');
    assert.equal(calls, 1);
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
    const { viewer } = loadViewerPage(selection);
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
    const { viewer } = loadViewerPage(selection);
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
    const { viewer } = loadViewerPage(selection);
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
    const { viewer } = loadViewerPage(selection);
    const actual = viewer.pdfSelectionClientRect(selection);

    for (const [field, value] of Object.entries(rect(38, 80, 220, 136))) {
        assert.equal(actual[field], value);
    }
});

test('refreshPDFSelectionMenu stores native selection text and bounds and shows the menu', () => {
    const scroll = createElement('scroll');
    const textLayer = createElement('textLayer');
    const textNode = { nodeType: 3, parentElement: textLayer };
    const selectionRect = rect(24, 48, 180, 72);
    scroll.appendChild(textLayer);
    const selection = {
        anchorNode: textNode,
        focusNode: textNode,
        rangeCount: 1,
        getRangeAt() {
            return {
                getClientRects() {
                    return [selectionRect];
                },
                getBoundingClientRect() {
                    return selectionRect;
                },
            };
        },
        toString() {
            return '  selected PDF text  ';
        },
    };
    const { viewer } = loadViewerPage(selection);
    viewer.stage = createElement('stage');
    viewer.stage.pdfScroll = scroll;
    viewer.pdfState = {
        ...viewer.defaultPDFState(),
        pdfDocument: {},
    };
    let shownRect = null;
    viewer.showPDFSelectionMenu = (menuRect) => {
        shownRect = menuRect;
    };

    viewer.refreshPDFSelectionMenu();

    assert.equal(viewer.pdfState.selectionText, 'selected PDF text');
    for (const [field, value] of Object.entries(selectionRect)) {
        assert.equal(viewer.pdfState.selectionClientRect[field], value);
        assert.equal(shownRect[field], value);
    }
});

test('refreshPDFSelectionMenu hides and clears state for selections outside the PDF viewer', () => {
    const scroll = createElement('scroll');
    const outside = createElement('outside');
    const selection = {
        anchorNode: outside,
        focusNode: outside,
        rangeCount: 1,
        toString() {
            return 'outside page selection';
        },
    };
    const { viewer } = loadViewerPage(selection);
    viewer.stage = createElement('stage');
    viewer.stage.pdfScroll = scroll;
    viewer.selectionMenu = createElement('menu');
    viewer.pdfState = {
        ...viewer.defaultPDFState(),
        pdfDocument: {},
        selectionText: 'stored PDF selection',
        selectionClientRect: rect(10, 20, 50, 40),
    };

    viewer.refreshPDFSelectionMenu();

    assert.equal(viewer.selectionMenu.classList.contains('hidden'), true);
    assert.equal(viewer.pdfState.selectionText, '');
    assert.equal(viewer.pdfState.selectionClientRect, null);
});

test('refreshPDFSelectionMenu hides and clears state for empty PDF selections', () => {
    const scroll = createElement('scroll');
    const textLayer = createElement('textLayer');
    const textNode = { nodeType: 3, parentElement: textLayer };
    scroll.appendChild(textLayer);
    const selection = {
        anchorNode: textNode,
        focusNode: textNode,
        rangeCount: 0,
        toString() {
            return '';
        },
    };
    const { viewer } = loadViewerPage(selection);
    viewer.stage = createElement('stage');
    viewer.stage.pdfScroll = scroll;
    viewer.selectionMenu = createElement('menu');
    viewer.pdfState = {
        ...viewer.defaultPDFState(),
        pdfDocument: {},
        selectionText: 'stored PDF selection',
        selectionClientRect: rect(10, 20, 50, 40),
    };

    viewer.refreshPDFSelectionMenu();

    assert.equal(viewer.selectionMenu.classList.contains('hidden'), true);
    assert.equal(viewer.pdfState.selectionText, '');
    assert.equal(viewer.pdfState.selectionClientRect, null);
});

test('clearPDFSelection removes native browser ranges', () => {
    let removeAllRangesCount = 0;
    const selection = {
        removeAllRanges() {
            removeAllRangesCount += 1;
        },
    };
    const { viewer } = loadViewerPage(selection);
    viewer.pdfState = {
        ...viewer.defaultPDFState(),
        selectionText: 'selected text',
        selectionClientRect: rect(10, 20, 50, 40),
    };

    viewer.clearPDFSelection();

    assert.equal(removeAllRangesCount, 1);
    assert.equal(viewer.pdfState.selectionText, '');
    assert.equal(viewer.pdfState.selectionClientRect, null);
});

test('clearPDFSelection tolerates selections without removeAllRanges', () => {
    const { viewer } = loadViewerPage({});
    viewer.pdfState = {
        ...viewer.defaultPDFState(),
        selectionText: 'selected text',
        selectionClientRect: rect(10, 20, 50, 40),
    };

    assert.doesNotThrow(() => viewer.clearPDFSelection());
    assert.equal(viewer.pdfState.selectionText, '');
    assert.equal(viewer.pdfState.selectionClientRect, null);
});

test('clearPDFSelection tolerates null browser selections', () => {
    const { viewer } = loadViewerPage(null);
    viewer.pdfState = {
        ...viewer.defaultPDFState(),
        selectionText: 'selected text',
        selectionClientRect: rect(10, 20, 50, 40),
    };

    assert.doesNotThrow(() => viewer.clearPDFSelection());
    assert.equal(viewer.pdfState.selectionText, '');
    assert.equal(viewer.pdfState.selectionClientRect, null);
});
