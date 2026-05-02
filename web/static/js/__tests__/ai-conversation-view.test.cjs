'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'ai-conversation-view.js');

function loadView(parseToolTags) {
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
        Utils: {},
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
