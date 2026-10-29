'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const root = path.resolve(__dirname, '../../..');
const settingsModulePath = path.join(root, 'static/js/settings.js');

function loadSettingsPage(api) {
    const code = fs.readFileSync(settingsModulePath, 'utf8') + '\nmodule.exports = SettingsPage;';
    const context = {
        console,
        module: { exports: {} },
        exports: {},
        t(_key, fallback) {
            return fallback || _key;
        },
        window: {},
        document: {},
        API: api,
        Utils: {
            escapeHTML(value) {
                return String(value).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
            },
        },
        setTimeout() {
            return 1;
        },
        clearTimeout() {},
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: settingsModulePath });
    return context.module.exports;
}

function makeSubject(api = {}) {
    const subject = Object.create(loadSettingsPage(api));
    const hidden = new Set(['hidden']);
    subject.magpiePanel = { classList: { toggle(name, on) { if (on) hidden.add(name); else hidden.delete(name); } }, hidden };
    subject.magpieSummary = { innerHTML: '' };
    subject.magpieStatus = { textContent: '', classList: { add() {}, remove() {} } };
    // Mirrors a <select>: only a value present among the options can be selected.
    subject.magpieReasoningEffortSelect = {
        innerHTML: '',
        disabled: false,
        _value: '',
        get options() {
            return [...this.innerHTML.matchAll(/<option value="([^"]*)">([^<]*)<\/option>/g)].map(([, value, label]) => ({ value, label }));
        },
        get value() { return this._value; },
        set value(next) { this._value = this.options.some((option) => option.value === next) ? next : ''; },
    };
    return subject;
}

const connected = {
    connected: true,
    model: 'codex/gpt-6.1-sol',
    context_tokens: 500000,
    max_output_tokens: 128000,
    reasoning_efforts: ['low', 'medium', 'high', 'xhigh'],
    default_reasoning_effort: 'medium',
    reasoning_effort: 'high',
};

test('Magpie panel lists only the model levels and selects the effective one', () => {
    const subject = makeSubject();
    subject.renderMagpieSettings(connected);
    const select = subject.magpieReasoningEffortSelect;
    assert.equal(subject.magpiePanel.hidden.has('hidden'), false);
    assert.deepEqual(select.options.map((option) => option.value), ['low', 'medium', 'high', 'xhigh']);
    assert.equal(select.options[1].label, 'medium（默认）');
    assert.equal(select.value, 'high');
    assert.equal(select.disabled, false);
    assert.match(subject.magpieSummary.innerHTML, /codex\/gpt-6\.1-sol/);
    assert.match(subject.magpieSummary.innerHTML, /128,000/);
});

test('Magpie panel offers no levels when the model reports none', () => {
    const subject = makeSubject();
    subject.renderMagpieSettings({ connected: true, model: 'p/opaque', max_output_tokens: 8192, reasoning_efforts: [] });
    const select = subject.magpieReasoningEffortSelect;
    assert.deepEqual(select.options.map((option) => option.value), ['']);
    assert.equal(select.disabled, true);
    assert.match(subject.magpieSummary.innerHTML, /未知/);

    subject.renderMagpieSettings({ connected: false });
    assert.equal(subject.magpiePanel.hidden.has('hidden'), true);
});

test('choosing a Magpie level saves only the Magpie setting', async () => {
    const calls = [];
    const subject = makeSubject({
        async updateMagpieSettings(data) {
            calls.push(['magpie', data]);
            return { ...connected, reasoning_effort: data.reasoning_effort };
        },
        async updateAIModelSettings(data) {
            calls.push(['models', data]);
        },
    });
    subject.renderMagpieSettings(connected);
    subject.magpieReasoningEffortSelect.value = 'low';
    await subject.saveMagpieReasoningEffort();
    assert.deepEqual(JSON.parse(JSON.stringify(calls)), [['magpie', { reasoning_effort: 'low' }]]);
    assert.equal(subject.magpieReasoningEffortSelect.value, 'low');
});

test('a rejected Magpie level is re-read from the server', async () => {
    const subject = makeSubject({
        async updateMagpieSettings() {
            throw new Error('当前 Magpie 模型不支持该思考强度');
        },
        async getMagpieSettings() {
            return { ...connected, reasoning_efforts: ['low', 'high'], default_reasoning_effort: 'high', reasoning_effort: 'high' };
        },
    });
    subject.renderMagpieSettings(connected);
    subject.magpieReasoningEffortSelect.value = 'xhigh';
    await subject.saveMagpieReasoningEffort();
    assert.deepEqual(subject.magpieReasoningEffortSelect.options.map((option) => option.value), ['low', 'high']);
    assert.equal(subject.magpieReasoningEffortSelect.value, 'high');
    assert.match(subject.magpieStatus.textContent, /不支持/);
});

test('settings page, API client and copy are wired for the Magpie panel', () => {
    const html = fs.readFileSync(path.join(root, 'settings.html'), 'utf8');
    assert.match(html, /id="aiMagpieReasoningEffortSelect"/);
    const api = fs.readFileSync(path.join(root, 'static/js/api.js'), 'utf8');
    assert.match(api, /\/ai\/settings\/magpie/);
    const zh = JSON.parse(fs.readFileSync(path.join(root, 'static/locales/zh-CN/settings.json'), 'utf8'));
    const en = JSON.parse(fs.readFileSync(path.join(root, 'static/locales/en/settings.json'), 'utf8'));
    const keys = (locale) => Object.keys(locale).filter((key) => key.startsWith('settings.ai.magpie_')).sort();
    assert.ok(keys(zh).length > 0);
    assert.deepEqual(keys(zh), keys(en));
});
