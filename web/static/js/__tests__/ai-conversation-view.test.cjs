'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'ai-conversation-view.js');

function loadView(parseToolTags, utils, renderMentionHTML) {
    const code = fs.readFileSync(modulePath, 'utf8');
    const context = {
        console: console,
        document: {
            dispatchEvent() {},
        },
        window: {
            AIReader: {
                toolTags: {
                    parseToolTags: parseToolTags || (() => ({ intentHint: '', sources: [], conflict: null })),
                    renderMentionHTML: renderMentionHTML || ((value) => value),
                },
            },
        },
        localStorage: {
            getItem() {
                return null;
            },
            setItem() {},
        },
        Event: class Event {
            constructor(type, init) {
                this.type = type;
                this.bubbles = !!(init && init.bubbles);
            }
        },
        CustomEvent: class CustomEvent {
            constructor(type, init) {
                this.type = type;
                this.detail = init && init.detail;
            }
        },
        Utils: utils || {},
        t(key, fallback) {
            return fallback || key;
        },
    };
    context.globalThis = context;

    vm.runInNewContext(code, context, { filename: modulePath });
    return context.window.AIReader.view;
}

function createSubject(parseToolTags) {
    const view = loadView(parseToolTags);
    const subject = Object.create(view);
    subject._state = {
        conversationId: null,
        pinnedPapers: [],
        rewriteLast: false,
    };
    subject._currentContext = () => ({ source: 'ai' });
    return subject;
}

test('sendPayload forwards explicit search_goal_hint into _sendBody body', async () => {
    const subject = createSubject();
    let sentBody = null;
    subject._sendBody = async (body) => {
        sentBody = JSON.parse(JSON.stringify(body));
    };

    await subject.sendPayload({
        content: 'find supporting evidence',
        intent_hint: 'external_search',
        search_goal_hint: 'evidence',
    });

    assert.deepEqual(sentBody, {
        content: 'find supporting evidence',
        context: { source: 'ai' },
        intent_hint: 'external_search',
        search_goal_hint: 'evidence',
    });
});

test('sendPayload does not invent search_goal_hint for ordinary payloads', async () => {
    const subject = createSubject();
    let sentBody = null;
    subject._sendBody = async (body) => {
        sentBody = JSON.parse(JSON.stringify(body));
    };

    await subject.sendPayload({
        content: 'plain question',
    });

    assert.equal(sentBody.search_goal_hint, undefined);
    assert.ok(!Object.prototype.hasOwnProperty.call(sentBody, 'search_goal_hint'));
});

test('loadDraft pins the deep-linked paper when paper metadata is available', () => {
    const view = loadView();
    const subject = Object.create(view);
    subject._state = {};
    subject._renderAll = () => {};

    subject.loadDraft(42, {
        id: 42,
        title: 'ARID1A determines luminal identity',
    });

    assert.deepEqual(JSON.parse(JSON.stringify(subject._state.pinnedPapers)), [{
        paper_id: 42,
        title: 'ARID1A determines luminal identity',
    }]);
    assert.deepEqual(JSON.parse(JSON.stringify(subject._currentContext())), {
        source: 'ai',
        paper_id: 42,
    });
});

test('sendPayload preserves explicit search_goal_hint when parsed @ tags are also present', async () => {
    const subject = createSubject(() => ({
        intentHint: 'external_search',
        sources: ['pubmed'],
        conflict: null,
    }));
    let sentBody = null;
    subject._sendBody = async (body) => {
        sentBody = JSON.parse(JSON.stringify(body));
    };

    await subject.sendPayload({
        content: '@PubMed find evidence for this claim',
        intent_hint: 'external_search',
        search_goal_hint: 'evidence',
    });

    assert.deepEqual(sentBody, {
        content: '@PubMed find evidence for this claim',
        context: { source: 'ai' },
        intent_hint: 'external_search',
        search_goal_hint: 'evidence',
        sources: ['pubmed'],
    });
});

test('_setAssistantText delegates markdown rendering to Utils.renderMarkdown', () => {
    let renderCall = null;
    const view = loadView(null, {
        renderMarkdown(value, options) {
            renderCall = { value, options };
            return '<p class="from-utils">rendered</p>';
        },
    });
    const subject = Object.create(view);
    const parts = {
        text: {
            innerHTML: '',
            textContent: '',
        },
    };

    subject._ensureMessageParts = () => parts;
    subject._clearStreamingStatus = () => {};

    subject._setAssistantText({}, '```js\nconsole.log(1)\n```', true);

    assert.ok(renderCall);
    assert.equal(renderCall.value, '```js\nconsole.log(1)\n```');
    assert.deepEqual(Object.keys(renderCall.options || {}), []);
    assert.equal(parts.text.innerHTML, '<p class="from-utils">rendered</p>');
});

test('_renderMessageContent uses shared mention renderer for user messages', () => {
    let mentionCall = null;
    const view = loadView(null, null, (value) => {
        mentionCall = value;
        return '<span class="ai-token-mention ai-token-tool">@image-gen</span> hi';
    });
    const subject = Object.create(view);
    const parts = {
        text: {
            innerHTML: '',
            textContent: '',
        },
        artifacts: {},
    };

    subject._ensureMessageParts = () => parts;
    subject._clearStreamingStatus = () => {};

    subject._renderMessageContent({}, { role: 'user', content: '@image-gen hi' });

    assert.equal(mentionCall, '@image-gen hi');
    assert.equal(parts.text.innerHTML, '<span class="ai-token-mention ai-token-tool">@image-gen</span> hi');
});
