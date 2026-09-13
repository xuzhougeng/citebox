'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

// Model bubbling through the real modal hierarchy, including the sibling
// figure and batch dialogs. A handler on figureModalBody must not see batch clicks.
class Element {
    constructor(parent = null, dataset = {}) {
        this.parent = parent;
        this.dataset = dataset;
        this.listeners = {};
        this.classList = { add() {}, remove() {}, contains() { return false; } };
    }
    addEventListener(type, handler) { (this.listeners[type] ||= []).push(handler); }
    querySelector() { return null; }
    closest(selector) {
        if (selector === '[data-batch-interpretation-action]' && this.dataset.batchInterpretationAction) return this;
        if (selector === '[data-batch-interpretation-option]' && this.dataset.batchInterpretationOption) return this;
        return null;
    }
    async dispatch(type) {
        for (let node = this; node; node = node.parent) {
            for (const handler of node.listeners[type] || []) await handler({ target: this });
        }
    }
}
function setup(api = {}) {
    const root = new Element();
    const nodes = {};
    for (const prefix of ['figure', 'figureInterpretation', 'figureBatchInterpretation']) {
        nodes[`${prefix}Modal`] = new Element(root);
        nodes[`${prefix}ModalBody`] = new Element(nodes[`${prefix}Modal`]);
        const close = `close${prefix[0].toUpperCase()}${prefix.slice(1)}Modal`;
        nodes[close] = new Element(nodes[`${prefix}Modal`]);
    }
    const context = {
        window: {}, setTimeout: () => 1, clearTimeout() {}, document: { documentElement: { lang: 'en' }, getElementById: id => nodes[id], body: root, addEventListener() {} },
        AbortController, HTMLElement: Element, console,
        t: (key, fallback) => fallback || key,
        Utils: { showToast() {}, escapeHTML: value => String(value) }, API: { async latestFigureAIJob() { return { job: null }; }, async getPaper() { return { paper: { id: 11, figures: [] } }; }, ...api }
    };
    vm.runInNewContext(fs.readFileSync(path.join(__dirname, '..', 'browser-pages.js'), 'utf8'), context);
    vm.runInNewContext(fs.readFileSync(path.join(__dirname, '..', 'figure-viewer.js'), 'utf8') + '\nglobalThis.viewer = FigureViewer;', context);
    const viewer = context.viewer;
    viewer.init();
    viewer.currentFigure = { id: 1, paper_id: 11 };
    const figures = [
        { id: 1, figure_index: 1, notes_text: '' },
        { id: 2, figure_index: 2, notes_text: 'Human note' },
        { id: 3, figure_index: 3, notes_text: '' },
        { id: 4, figure_index: 3, parent_figure_id: 3, notes_text: '' }
    ];
    viewer.figures = figures;
    viewer.index = 0;
    viewer.paperDetails.set(11, { id: 11, figures });
    viewer.render = () => {};
    const ready = viewer.openBatchInterpretationModal();
    const action = name => new Element(viewer.batchBody, { batchInterpretationAction: name });
    const option = (name, value) => Object.assign(new Element(viewer.batchBody, { batchInterpretationOption: name }), { value });
    return { viewer, figures, action, option, ready };
}


function job(status, items = [{ figure_id: 1, status: 'completed' }, { figure_id: 3, status: 'failed', error: 'Provider failed' }]) {
    return { id: 9, paper_id: 11, scope: 'missing', mode: 'append', language: 'en', status, items };
}

test('batch sibling dialog starts a durable task and renders server progress', async () => {
    const calls = [];
    const { viewer, action, ready } = setup({
        async startFigureAIJob(request) { calls.push(request); return { job: job('running') }; },
        async getFigureAIJob() { return { job: job('failed') }; },
        async readPaperWithAI() { assert.fail('Inference must run on the server'); },
        async updateFigure() { assert.fail('The server atomically writes notes'); }
    });
    await ready;
    await action('start').dispatch('click');
    assert.equal(calls.length, 1);
    assert.equal(calls[0].paper_id, 11);
    assert.equal(calls[0].scope, 'missing');
    assert.equal(viewer.batchState.succeeded, 1);
    assert.equal(viewer.batchState.skipped, 1);
    assert.equal(viewer.batchState.failures.length, 1);
    assert.equal(viewer.batchState.done, true);
    assert.equal(viewer.batchState.pollError, false);
    assert.match(viewer.batchBody.innerHTML, /继续未完成项/);
});

test('batch scope and note mode are sent to the durable task', async () => {
    let request;
    const { action, option, ready } = setup({
        async startFigureAIJob(value) { request = value; return { job: job('completed') }; },
        async getFigureAIJob() { return { job: job('completed') }; }
    });
    await ready;
    await option('scope', 'all').dispatch('change');
    await option('mode', 'overwrite').dispatch('change');
    await action('start').dispatch('click');
    assert.equal(request.scope, 'all');
    assert.equal(request.mode, 'overwrite');
});

test('reopening restores an interrupted task and resumes the same job', async () => {
    const controls = [];
    const { viewer, action, ready } = setup({
        async latestFigureAIJob() { return { job: job('stopped') }; },
        async controlFigureAIJob(id, action) { controls.push([id, action]); return { job: job('running') }; },
        async getFigureAIJob() { return { job: job('completed') }; },
        async startFigureAIJob() { assert.fail('Resume must not create a replacement job'); }
    });
    await ready;
    assert.equal(viewer.batchState.jobId, 9);
    assert.equal(viewer.batchState.abort, true);
    await action('resume').dispatch('click');
    assert.deepEqual(controls, [[9, 'resume']]);
    assert.equal(viewer.batchState.done, true);
    assert.equal(viewer.batchState.pollError, false);
});

test('interrupt stops the server task while closing only detaches progress', async () => {
    const controls = [];
    const { viewer, action, ready } = setup({
        async latestFigureAIJob() { return { job: job('running', [{ figure_id: 1, status: 'running' }, { figure_id: 3, status: 'pending' }]) }; },
        async getFigureAIJob() { return { job: job('running', [{ figure_id: 1, status: 'running' }, { figure_id: 3, status: 'pending' }]) }; },
        async controlFigureAIJob(id, action) { controls.push([id, action]); return { job: job('stopped', [{ figure_id: 1, status: 'pending' }, { figure_id: 3, status: 'pending' }]) }; }
    });
    await ready;
    await action('stop').dispatch('click');
    assert.deepEqual(controls, [[9, 'stop']]);
    assert.equal(viewer.batchState.running, false);
    assert.match(viewer.batchBody.innerHTML, /尚有 2 张未处理/);
    await viewer.openBatchInterpretationModal();
    viewer.closeBatchInterpretationModal();
    assert.equal(viewer.batchState, null);
    assert.equal(controls.length, 1);
});

test('polling failure retains task identity and does not restart inference', async () => {
    let starts = 0;
    const { viewer, action, ready } = setup({
        async startFigureAIJob() { starts++; return { job: job('running') }; },
        async getFigureAIJob() { throw new Error('Disconnected'); }
    });
    await ready;
    await action('start').dispatch('click');
    assert.equal(starts, 1);
    assert.equal(viewer.batchState.jobId, 9);
    assert.equal(viewer.batchState.pollError, true);
    await action('start').dispatch('click');
    assert.equal(starts, 1);
});
