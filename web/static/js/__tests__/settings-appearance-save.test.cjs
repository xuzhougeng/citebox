'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

function setup(api) {
    const events = [];
    const context = {
        window: { location: { reload: () => events.push('reload') } },
        document: { getElementById: () => ({}) },
        API: api, t: (key, fallback) => fallback,
        CiteBoxTheme: { get: () => 'warm', apply: theme => events.push(theme) },
        CiteBoxI18n: { get: () => 'zh-CN', set: async () => { throw new Error('offline'); } }
    };
    vm.runInNewContext(fs.readFileSync(path.join(__dirname, '..', 'settings.js'), 'utf8') + '\nglobalThis.settings = SettingsPage;', context);
    context.settings.setInlineStatus = (element, text, tone) => events.push(tone);
    return { settings: context.settings, events };
}
test('appearance save does not apply an unpersisted theme and restores the control on failure', async () => {
    const { settings, events } = setup({ updateAppearanceSettings: async () => { throw new Error('offline'); } });
    const select = { value: 'dark', disabled: false };
    await settings.saveAppearancePreference('theme', select);
    assert.equal(select.value, 'warm');
    assert.equal(select.disabled, false);
    assert.deepEqual(events, ['saving', 'error']);
});
test('theme is applied only after the server accepts it', async () => {
    let complete;
    const { settings, events } = setup({ updateAppearanceSettings: () => new Promise(resolve => { complete = resolve; }) });
    const select = { value: 'dark', disabled: false };
    const pending = settings.saveAppearancePreference('theme', select);
    assert.deepEqual(events, ['saving']);
    assert.equal(select.disabled, true);
    complete({ theme: 'dark' });
    await pending;
    assert.deepEqual(events, ['saving', 'dark', 'success']);
    assert.equal(select.disabled, false);
});
test('language save failure does not reload or discard the old language', async () => {
    const { settings, events } = setup({});
    const select = { value: 'en', disabled: false };
    await settings.saveAppearancePreference('language', select);
    assert.equal(select.value, 'zh-CN');
    assert.deepEqual(events, ['saving', 'error']);
});
