'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'ai-reader.js');

function makeElement() {
    return {
        style: {},
        value: '',
        textContent: '',
        innerHTML: '',
        scrollHeight: 48,
        scrollTop: 0,
        addEventListener(type, handler) {
            this._listeners = this._listeners || {};
            this._listeners[type] = handler;
        },
        dispatch(type) {
            if (this._listeners && this._listeners[type]) {
                this._listeners[type]();
            }
        },
    };
}

function loadBoot(renderMentionHTML) {
    const input = makeElement();
    const mirror = makeElement();
    const byId = {
        aiQuestionInput: input,
        aiQuestionMirror: mirror,
    };
    const code = fs.readFileSync(modulePath, 'utf8');
    const document = {
        getElementById(id) {
            return byId[id] || null;
        },
        addEventListener() {},
    };
    const context = {
        console,
        document,
        window: {
            AIReader: {
                toolTags: {
                    renderMentionHTML,
                },
            },
            getComputedStyle() {
                return { maxHeight: '240' };
            },
        },
        localStorage: {
            getItem() { return null; },
            setItem() {},
        },
        history: {
            replaceState() {},
        },
        URLSearchParams,
        t(_key, fallback) {
            return fallback || _key;
        },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: modulePath });
    return { boot: context.window.AIReader.boot, input, mirror };
}

function loadBootForDispatch(options = {}) {
    const code = fs.readFileSync(modulePath, 'utf8');
    const loadDraftCalls = [];
    const activeCalls = [];
    const context = {
        console,
        document: {
            getElementById() { return null; },
            addEventListener() {},
        },
        window: {
            location: {
                search: options.search || '',
            },
            AIReader: {
                view: {
                    loadDraft(paperId, paper) {
                        loadDraftCalls.push({ paperId, paper });
                    },
                    load() {},
                },
                conversations: {
                    _state: { items: [] },
                    async refresh() {},
                    setActive(value) {
                        activeCalls.push(value);
                    },
                },
            },
        },
        API: {
            async getPaper(id) {
                return options.paper || { id, title: `Paper ${id}` };
            },
        },
        URLSearchParams,
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: modulePath });
    return {
        boot: context.window.AIReader.boot,
        loadDraftCalls,
        activeCalls,
    };
}

test('_bindQuestionMirror uses shared mention HTML renderer', () => {
    let seen = null;
    const { boot, input, mirror } = loadBoot((value) => {
        seen = value;
        return '<span class="tokenized">@image-gen</span>';
    });
    input.value = '@image-gen draw it';

    boot._bindQuestionMirror();
    input.dispatch('input');

    assert.equal(seen, '@image-gen draw it ');
    assert.equal(mirror.innerHTML, '<span class="tokenized">@image-gen</span>');
});

test('paper_id entry loads a draft with paper metadata for pinning', async () => {
    const paper = { id: 42, title: 'ARID1A determines luminal identity' };
    const { boot, loadDraftCalls, activeCalls } = loadBootForDispatch({
        search: '?paper_id=42',
        paper,
    });

    await boot._dispatchEntry();

    assert.deepEqual(loadDraftCalls, [{ paperId: 42, paper }]);
    assert.deepEqual(activeCalls, [null]);
});
