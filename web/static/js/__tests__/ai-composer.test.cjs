'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');

const mod = require(path.join('..', 'ai-composer.js'));
const { SHORTCUT_SPECS, payloadForShortcut } = mod;

test('SHORTCUT_SPECS includes the expected shortcut ids', () => {
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
    assert.deepEqual(payloadForShortcut('external_search_discovery', 'find recent reviews'), {
        content: 'find recent reviews',
        intent_hint: 'external_search',
        search_goal_hint: 'discovery',
    });
});

test('payloadForShortcut external_search_evidence emits external_search + evidence', () => {
    assert.deepEqual(payloadForShortcut('external_search_evidence', 'find the source'), {
        content: 'find the source',
        intent_hint: 'external_search',
        search_goal_hint: 'evidence',
    });
});

test('payloadForShortcut library_search omits search_goal_hint', () => {
    assert.deepEqual(payloadForShortcut('library_search', 'search my papers'), {
        content: 'search my papers',
        intent_hint: 'library_search',
    });
});
