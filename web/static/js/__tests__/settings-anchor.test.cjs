'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');

let mod = {};
try {
    mod = require(path.join('..', 'settings-anchor.js'));
} catch (error) {
    mod = {};
}

const { resolveSettingsNavigation } = mod;

test('resolveSettingsNavigation maps legacy #research hash to research settings section', () => {
    assert.equal(typeof resolveSettingsNavigation, 'function');
    const target = resolveSettingsNavigation('#research', {
        defaultCategoryId: 'system',
        categoryIds: ['system', 'ai', 'integrations', 'account'],
        sectionCategoryById: {
            'settings-external-sources': 'integrations',
        },
        legacySectionAliasByHash: {
            research: 'settings-external-sources',
        },
    });
    assert.deepEqual(target, {
        categoryId: 'integrations',
        sectionId: 'settings-external-sources',
    });
});

test('resolveSettingsNavigation preserves direct section anchors', () => {
    assert.equal(typeof resolveSettingsNavigation, 'function');
    const target = resolveSettingsNavigation('#settings-external-sources', {
        defaultCategoryId: 'system',
        categoryIds: ['system', 'ai', 'integrations', 'account'],
        sectionCategoryById: {
            'settings-external-sources': 'integrations',
        },
        legacySectionAliasByHash: {},
    });
    assert.deepEqual(target, {
        categoryId: 'integrations',
        sectionId: 'settings-external-sources',
    });
});

test('resolveSettingsNavigation keeps category links working', () => {
    assert.equal(typeof resolveSettingsNavigation, 'function');
    const target = resolveSettingsNavigation('#category-ai', {
        defaultCategoryId: 'system',
        categoryIds: ['system', 'ai', 'integrations', 'account'],
        sectionCategoryById: {
            'settings-external-sources': 'integrations',
        },
        legacySectionAliasByHash: {},
    });
    assert.deepEqual(target, {
        categoryId: 'ai',
        sectionId: '',
    });
});
