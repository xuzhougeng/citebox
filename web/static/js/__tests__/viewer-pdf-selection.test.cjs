'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'viewer.js');

function loadViewerPage() {
    const code = `${fs.readFileSync(modulePath, 'utf8')}\nglobalThis.__RESOURCE_VIEWER_PAGE__ = ResourceViewerPage;`;
    const context = {
        console,
        URL,
        URLSearchParams,
        window: {
            location: {
                href: 'http://localhost/viewer?kind=pdf&src=%2Fpaper.pdf',
                origin: 'http://localhost',
                search: '?kind=pdf&src=%2Fpaper.pdf',
            },
            history: { length: 1 },
            setTimeout,
            clearTimeout,
            requestAnimationFrame(callback) { return callback(); },
            getSelection() {
                return { removeAllRanges() {} };
            },
        },
        document: {
            referrer: '',
            addEventListener() {},
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

function span(text, bounds) {
    return {
        textContent: text,
        getBoundingClientRect() {
            return bounds;
        },
    };
}

test('PDF drag selection follows text flow and renders one highlight per selected line', () => {
    const viewer = loadViewerPage();
    const pageRect = rect(0, 0, 800, 1000);
    const textLayer = {
        closest() {
            return { getBoundingClientRect: () => pageRect };
        },
        querySelectorAll() {
            return [
                span('based', rect(120, 100, 210, 126)),
                span('on', rect(222, 100, 252, 126)),
                span('the', rect(264, 100, 308, 126)),
                span('the', rect(62, 140, 104, 166)),
                span('amplification', rect(116, 140, 282, 166)),
                span('of', rect(294, 140, 322, 166)),
                span('ERBB2', rect(334, 140, 420, 166)),
            ];
        },
    };
    const drag = {
        startX: 118,
        startY: 112,
        currentX: 430,
        currentY: 152,
    };
    const selection = viewer.collectPDFSelectionFromClientRect(
        viewer.normalizedClientRect(drag.startX, drag.startY, drag.currentX, drag.currentY),
        textLayer,
        drag
    );

    assert.equal(selection.text, 'based on the\nthe amplification of ERBB2');
    assert.equal(selection.rects.length, 2);
    assert.deepEqual(Array.from(selection.rects, (item) => Math.round(item.left)), [120, 62]);
    assert.deepEqual(Array.from(selection.rects, (item) => Math.round(item.width)), [188, 358]);
});

test('PDF text line grouping keeps wide intra-line spaces but splits columns', () => {
    const viewer = loadViewerPage();
    const segments = viewer.buildPDFTextLineSegments([
        span('the amplification of', rect(189, 1237, 315, 1253)),
        span('ERBB2', rect(390, 1237, 440, 1253)),
        span('(also known as HER2) that dic-', rect(465, 1237, 668, 1253)),
        span('SWI/SNF chromatin remodeling complexes, with', rect(811, 1237, 1128, 1253)),
    ].map((item) => ({
        text: item.textContent,
        rect: item.getBoundingClientRect(),
    })));

    assert.equal(segments.length, 2);
    assert.deepEqual(Array.from(segments, (item) => Math.round(item.left)), [189, 811]);
    assert.deepEqual(Array.from(segments, (item) => Math.round(item.right)), [668, 1128]);
});
