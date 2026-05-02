'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');

const modulePath = path.resolve(__dirname, '..', 'ai-composer.js');

function loadComposerModule() {
    delete require.cache[modulePath];
    return require(modulePath);
}

class FakeClassList {
    constructor() {
        this.names = new Set();
    }

    add(name) {
        this.names.add(name);
    }

    remove(name) {
        this.names.delete(name);
    }

    contains(name) {
        return this.names.has(name);
    }
}

class FakeButton {
    constructor(shortcutId) {
        this.dataset = { shortcutId: shortcutId };
        this.classList = new FakeClassList();
        this.listeners = {};
    }

    addEventListener(eventName, handler) {
        this.listeners[eventName] = handler;
    }

    click() {
        this.listeners.click();
    }
}

class FakeShortcutRoot {
    constructor(shortcutIds) {
        this.buttons = shortcutIds.map((shortcutId) => new FakeButton(shortcutId));
        this.html = '';
    }

    set innerHTML(value) {
        this.html = value;
    }

    get innerHTML() {
        return this.html;
    }

    querySelectorAll(selector) {
        if (selector === '[data-shortcut-id]') {
            return this.buttons;
        }
        if (selector === '.is-active') {
            return this.buttons.filter((button) => button.classList.contains('is-active'));
        }
        return [];
    }
}

test('Node require exports the module without mutating globalThis.AIReader', () => {
    const priorValue = globalThis.AIReader;
    delete globalThis.AIReader;

    try {
        const mod = loadComposerModule();
        assert.ok(mod);
        assert.equal(globalThis.AIReader, undefined);
    } finally {
        if (priorValue === undefined) {
            delete globalThis.AIReader;
        } else {
            globalThis.AIReader = priorValue;
        }
    }
});

test('SHORTCUT_SPECS includes the expected shortcut ids', () => {
    const { SHORTCUT_SPECS } = loadComposerModule();
    assert.deepEqual(
        SHORTCUT_SPECS.map((spec) => spec.id),
        [
            'library_search',
            'external_search_discovery',
            'external_search_evidence',
            'paper_read',
            'figure_lookup',
        ]
    );
});

test('payloadForShortcut external_search_discovery emits external_search + discovery', () => {
    const { payloadForShortcut } = loadComposerModule();
    assert.deepEqual(payloadForShortcut('external_search_discovery', 'find recent reviews'), {
        content: 'find recent reviews',
        intent_hint: 'external_search',
        search_goal_hint: 'discovery',
    });
});

test('payloadForShortcut external_search_evidence emits external_search + evidence', () => {
    const { payloadForShortcut } = loadComposerModule();
    assert.deepEqual(payloadForShortcut('external_search_evidence', 'find the source'), {
        content: 'find the source',
        intent_hint: 'external_search',
        search_goal_hint: 'evidence',
    });
});

test('payloadForShortcut library_search omits search_goal_hint', () => {
    const { payloadForShortcut } = loadComposerModule();
    assert.deepEqual(payloadForShortcut('library_search', 'search my papers'), {
        content: 'search my papers',
        intent_hint: 'library_search',
    });
});

test('Composer submit resets a one-turn shortcut after sending', () => {
    const { Composer, SHORTCUT_SPECS } = loadComposerModule();
    const sentPayloads = [];
    const input = {
        value: 'first question',
        focus() {},
    };
    const shortcutRoot = new FakeShortcutRoot(SHORTCUT_SPECS.map((spec) => spec.id));
    const composer = Object.create(Composer);

    composer.init({
        input: input,
        shortcutRoot: shortcutRoot,
        onSend(payload) {
            sentPayloads.push(payload);
        },
    });

    const discoveryButton = shortcutRoot.buttons.find((button) => button.dataset.shortcutId === 'external_search_discovery');
    discoveryButton.click();
    composer.submit();

    assert.deepEqual(sentPayloads[0], {
        content: 'first question',
        intent_hint: 'external_search',
        search_goal_hint: 'discovery',
    });
    assert.equal(composer.activeShortcutId, '');
    assert.equal(shortcutRoot.querySelectorAll('.is-active').length, 0);

    input.value = 'second question';
    composer.submit();

    assert.deepEqual(sentPayloads[1], {
        content: 'second question',
    });
});
