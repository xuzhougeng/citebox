'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');

const modulePath = path.resolve(__dirname, '..', 'ai-pdf-panel.js');

function loadModule() {
    delete require.cache[modulePath];
    return require(modulePath);
}

test('mergePanelContext returns body untouched without panel context', () => {
    const { mergePanelContext } = loadModule();
    const body = { content: 'q', context: { source: 'ai' } };
    const out = mergePanelContext(body, null);
    assert.equal(out, body);
    assert.deepEqual(body.context, { source: 'ai' });
});

test('mergePanelContext unions figure ids with @figure mentions, deduplicated', () => {
    const { mergePanelContext } = loadModule();
    const body = { content: 'q', context: { source: 'ai', figure_ids: [12, 13] } };
    mergePanelContext(body, {
        figure_ids: [13, 14, '15', -1, 0, 'abc'],
        excerpts: [],
    });
    assert.deepEqual(body.context.figure_ids, [12, 13, 14, 15]);
});

test('mergePanelContext creates context when missing', () => {
    const { mergePanelContext } = loadModule();
    const body = { content: 'q' };
    mergePanelContext(body, { figure_ids: [7], excerpts: [] });
    assert.deepEqual(body.context.figure_ids, [7]);
});

test('mergePanelContext does not set figure_ids when empty', () => {
    const { mergePanelContext } = loadModule();
    const body = { content: 'q', context: { source: 'ai' } };
    mergePanelContext(body, { figure_ids: [], excerpts: [] });
    assert.equal('figure_ids' in body.context, false);
    assert.equal('excerpts' in body.context, false);
});

test('mergePanelContext cleans excerpts: trim, caps, drops blanks, keeps page/paper', () => {
    const { mergePanelContext, MAX_EXCERPTS, MAX_EXCERPT_CHARS } = loadModule();
    const excerpts = [
        { paper_id: 3, page: 2, text: '  hello world  ' },
        { text: '   ' },
        { paper_id: 'not-a-number', page: 'x', text: 'no ids' },
    ];
    for (let i = 0; i < MAX_EXCERPTS + 2; i += 1) {
        excerpts.push({ text: 'filler ' + i });
    }
    excerpts.push({ text: 'x'.repeat(MAX_EXCERPT_CHARS + 50) });

    const body = { content: 'q' };
    mergePanelContext(body, { figure_ids: [], excerpts });

    assert.equal(body.context.excerpts.length, MAX_EXCERPTS);
    assert.deepEqual(body.context.excerpts[0], { paper_id: 3, page: 2, text: 'hello world' });
    assert.deepEqual(body.context.excerpts[1], { text: 'no ids' });
    assert.equal(body.context.excerpts.some((e) => e.text.startsWith('x'.repeat(100))), false,
        'overlong excerpt beyond the cap must be dropped, not included');
});

test('mergePanelContext keeps existing excerpts-free context fields', () => {
    const { mergePanelContext } = loadModule();
    const body = { content: 'q', context: { source: 'ai', paper_id: 5 } };
    mergePanelContext(body, {
        figure_ids: [9],
        excerpts: [{ paper_id: 5, page: 1, text: 'quote' }],
    });
    assert.equal(body.context.paper_id, 5);
    assert.deepEqual(body.context.figure_ids, [9]);
    assert.deepEqual(body.context.excerpts, [{ paper_id: 5, page: 1, text: 'quote' }]);
});
