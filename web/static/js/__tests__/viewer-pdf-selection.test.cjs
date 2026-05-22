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
        listeners: {},
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
        addEventListener(type, handler) {
            this.listeners[type] = handler;
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
    const translateDialog = options.translateDialog || null;
    const context = {
        console,
        AbortController,
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
            requestAnimationFrame(callback) { return callback(); },
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
            querySelector(selector) {
                if (selector === '.translate-dialog-overlay:not(.hidden)') {
                    return translateDialog;
                }
                if (selector === '.viewer-ai-ask-overlay:not(.hidden)') {
                    return options.aiAskDialog || null;
                }
                return null;
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

test('PDF toolbar paper action opens AI with the current paper id', async () => {
    const { viewer } = loadViewerPage(null);
    viewer.stage = { className: '', innerHTML: '' };
    viewer.pdfDetailLink = { href: '', hidden: true };
    viewer.loadPDFDocument = async () => false;

    await viewer.renderPDFResource({
        href: 'http://localhost/files/papers/paper.pdf',
        paperId: '42',
    });

    assert.equal(viewer.pdfDetailLink.href, '/ai?paper_id=42');
    assert.equal(viewer.pdfDetailLink.hidden, false);
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

test('Escape clears an open PDF selection menu before closing the viewer', () => {
    const { viewer } = loadViewerPage(null);
    let closed = false;
    let cleared = false;
    const event = {
        key: 'Escape',
        preventDefault() {},
        stopPropagation() {},
        stopImmediatePropagation() {},
    };
    viewer.selectionMenu = createElement('menu');
    viewer.close = () => {
        closed = true;
    };
    viewer.clearPDFSelection = () => {
        cleared = true;
        viewer.selectionMenu.classList.add('hidden');
    };

    viewer.handleEscapeKey(event);

    assert.equal(cleared, true);
    assert.equal(closed, false);
});

test('Escape does not close the PDF reader while translation dialog is open', () => {
    const translateDialog = createElement('translateDialog');
    const { viewer } = loadViewerPage(null, { translateDialog });
    let closed = false;
    const event = {
        key: 'Escape',
        preventDefault() {},
        stopPropagation() {},
        stopImmediatePropagation() {},
    };
    viewer.selectionMenu = createElement('menu');
    viewer.selectionMenu.classList.add('hidden');
    viewer.close = () => {
        closed = true;
    };

    viewer.handleEscapeKey(event);

    assert.equal(closed, false);
});

test('Escape does not close the PDF reader while PDF AI ask dialog is open', () => {
    const aiAskDialog = createElement('aiAskDialog');
    const { viewer } = loadViewerPage(null, { aiAskDialog });
    let closed = false;
    const event = {
        key: 'Escape',
        preventDefault() {},
        stopPropagation() {},
        stopImmediatePropagation() {},
    };
    viewer.selectionMenu = createElement('menu');
    viewer.selectionMenu.classList.add('hidden');
    viewer.close = () => {
        closed = true;
    };

    viewer.handleEscapeKey(event);

    assert.equal(closed, false);
});

test('PDF selection menu routes ask-ai action to the AI ask dialog', async () => {
    const { viewer } = loadViewerPage(null);
    const menu = createElement('menu');
    let asked = false;
    viewer.selectionMenu = menu;
    viewer.openPDFSelectionAIAsk = async () => {
        asked = true;
    };

    viewer.bindPDFSelectionMenu();
    await menu.listeners.click({
        target: {
            closest(selector) {
                return selector === '[data-pdf-selection-action]'
                    ? { dataset: { pdfSelectionAction: 'ask-ai' } }
                    : null;
            },
        },
        preventDefault() {},
        stopPropagation() {},
    });

    assert.equal(asked, true);
});

test('PDF selection AI question includes selected passage and user question', () => {
    const { viewer } = loadViewerPage(null);
    const prompt = viewer.buildPDFSelectionAIQuestion(
        'Among the genomic alterations observed in ER+ breast cancer',
        '为什么这里强调 ARID1A？'
    );

    assert.match(prompt, /请只围绕下面这段 PDF 划选内容回答/);
    assert.match(prompt, /Among the genomic alterations observed in ER\+ breast cancer/);
    assert.match(prompt, /为什么这里强调 ARID1A？/);
});

test('PDF selection AI ask uses transient read stream API', async () => {
    const { context, viewer } = loadViewerPage(null);
    let request = null;
    context.API = {
        async readPaperWithAIStream(data, options) {
            request = data;
            options.onEvent({ type: 'delta', delta: 'answer' });
        },
    };
    const questionInput = { value: '为什么这里强调 ARID1A？' };
    const submitButton = { disabled: false, textContent: '' };
    const dialogState = {
        overlay: {
            querySelector(selector) {
                if (selector === '.viewer-ai-ask-question') return questionInput;
                if (selector === '[data-pdf-ai-ask-action="submit"]') return submitButton;
                return null;
            },
        },
        selectionText: 'Among the genomic alterations observed in ER+ breast cancer',
        paperId: 42,
        loading: false,
        answer: '',
    };
    viewer.pdfAIAskDialog = dialogState;
    viewer.renderPDFSelectionAIAnswer = () => {};

    await viewer.submitPDFSelectionAIAsk(dialogState);

    assert.equal(request.paper_id, 42);
    assert.equal(request.action, 'paper_qa');
    assert.equal(request.include_figures, false);
    assert.match(request.question, /Among the genomic alterations observed in ER\+ breast cancer/);
    assert.match(request.question, /为什么这里强调 ARID1A？/);
    assert.equal(dialogState.answer, 'answer');
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
