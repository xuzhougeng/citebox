const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

function loadView() {
    const source = fs.readFileSync(path.join(__dirname, '../../web/static/js/ai-conversation-view.js'), 'utf8');
    const context = {
        console,
        Event: function Event() {},
        localStorage: {
            getItem() { return null; },
            setItem() {},
        },
        window: {
            AIReader: {},
            location: { href: 'http://localhost/ai' },
        },
    };
    vm.createContext(context);
    vm.runInContext(source, context, { filename: 'ai-conversation-view.js' });
    return { context, view: context.window.AIReader.view, window: context.window };
}

function fakeInput(value) {
    const handlers = {};
    return {
        handlers,
        input: {
            value,
            addEventListener(name, handler) {
                handlers[name] = handler;
            },
            dispatchEvent() {},
        },
    };
}

test('keyboard send delegates to composer so selected intent is preserved', () => {
    const loaded = loadView();
    const view = loaded.view;
    const { input, handlers } = fakeInput('帮我查找 ATAC 文章');
    let submitCalled = false;
    let directSendCalled = false;

    view._state.els = null;
    view.sendCurrentInput = () => {
        directSendCalled = true;
    };
    loaded.window.AIReader.composer = {
        submit() {
            submitCalled = true;
        },
    };

    view.init({ questionInput: input });
    let prevented = false;
    handlers.keydown({
        ctrlKey: true,
        metaKey: false,
        key: 'Enter',
        preventDefault() {
            prevented = true;
        },
    });

    assert.equal(prevented, true);
    assert.equal(submitCalled, true);
    assert.equal(directSendCalled, false);
});

test('existing conversation context ignores stale draft paper id', () => {
    const loaded = loadView();
    const view = loaded.view;

    view._state.conversationId = 99;
    view._state._draftPaperId = 123;
    view._state.pinnedPapers = [];

    const context = view._currentContext();
    assert.equal(context.source, 'ai');
    assert.equal(Object.hasOwn(context, 'paper_id'), false);
});

test('loading an existing conversation clears draft paper id', async () => {
    const loaded = loadView();
    const view = loaded.view;
    view._state._draftPaperId = 123;
    view._renderAll = () => {};
    loaded.context.fetch = async () => ({
        ok: true,
        async json() {
            return {
                pinned_papers: [],
                recent_messages: [],
                turn_runs: [],
            };
        },
    });

    await view.load(99);

    assert.equal(view._state.conversationId, 99);
    assert.equal(view._state._draftPaperId, 0);
});
