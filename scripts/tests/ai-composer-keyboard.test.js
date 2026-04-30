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

class FakeElement {
    constructor(tagName) {
        this.tagName = tagName;
        this.children = [];
        this.parentNode = null;
        this.dataset = {};
        this._className = '';
        this._textContent = '';
        this._innerHTML = '';
        this.scrollTop = 0;
        this.scrollHeight = 0;
        this.classList = {
            add: (...names) => {
                const current = new Set(this._className.split(/\s+/).filter(Boolean));
                names.forEach((name) => current.add(name));
                this._className = Array.from(current).join(' ');
            },
            remove: (...names) => {
                const remove = new Set(names);
                this._className = this._className.split(/\s+/).filter((name) => name && !remove.has(name)).join(' ');
            },
            contains: (name) => this._className.split(/\s+/).includes(name),
        };
    }

    get className() {
        return this._className;
    }

    set className(value) {
        this._className = String(value || '');
    }

    get textContent() {
        if (this.children.length === 0) return this._textContent;
        return this.children.map((child) => child.textContent).join('');
    }

    set textContent(value) {
        this._textContent = String(value || '');
        this._innerHTML = '';
        this.children = [];
    }

    get innerHTML() {
        return this._innerHTML;
    }

    set innerHTML(value) {
        this._innerHTML = String(value || '');
        this._textContent = '';
        this.children = [];
    }

    get firstChild() {
        return this.children[0] || null;
    }

    appendChild(child) {
        child.parentNode = this;
        this.children.push(child);
        this.scrollHeight = this.children.length;
        return child;
    }

    querySelector(selector) {
        const className = selector.includes('.ai-message-text') ? 'ai-message-text'
            : selector.includes('.ai-message-artifacts') ? 'ai-message-artifacts'
                : '';
        if (!className) return null;
        return this.children.find((child) => child.classList && child.classList.contains(className)) || null;
    }
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

test('streaming assistant bubble shows thinking state until first delta', () => {
    const loaded = loadView();
    const view = loaded.view;
    const conversation = new FakeElement('div');
    loaded.context.document = {
        createElement(tagName) {
            return new FakeElement(tagName);
        },
    };
    view._state.els = { conversation };

    const bubble = view._appendMessageBubble({ role: 'assistant', content: '', streaming: true });
    const text = bubble.querySelector(':scope > .ai-message-text');

    assert.equal(text.textContent, '思考中…');
    assert.equal(bubble.classList.contains('has-streaming-status'), true);

    view._setAssistantText(bubble, '真实回答', false);

    assert.equal(text.textContent, '真实回答');
    assert.equal(bubble.classList.contains('has-streaming-status'), false);
});
