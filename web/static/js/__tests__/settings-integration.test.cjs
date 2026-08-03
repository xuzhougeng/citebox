'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('path');
const vm = require('node:vm');

const settingsModulePath = path.resolve(__dirname, '..', 'settings.js');
const settingsHTMLPath = path.resolve(__dirname, '..', '..', '..', 'settings.html');
const apiModulePath = path.resolve(__dirname, '..', 'api.js');
const zhLocalePath = path.resolve(__dirname, '..', '..', 'locales', 'zh-CN', 'settings.json');
const enLocalePath = path.resolve(__dirname, '..', '..', 'locales', 'en', 'settings.json');

function loadSettingsPage() {
    const code = fs.readFileSync(settingsModulePath, 'utf8') + '\nmodule.exports = SettingsPage;';
    const context = {
        console,
        module: { exports: {} },
        exports: {},
        t(_key, fallback) {
            return fallback || _key;
        },
        window: {
            t(_key, fallback) {
                return fallback || _key;
            },
        },
        document: {},
        API: {},
        Utils: {
            escapeHTML(value) {
                return String(value);
            },
            formatDate(value) {
                return `formatted:${value}`;
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

function makeIntegrationSubject() {
    const SettingsPage = loadSettingsPage();
    const subject = Object.create(SettingsPage);
    subject.integrationEnabledInput = { checked: false };
    subject.integrationPortInput = { value: '' };
    subject.integrationSummary = { innerHTML: '' };
    subject.integrationTokenSummary = { innerHTML: '' };
    subject.integrationFreshToken = '';
    subject.integrationSettings = null;
    return subject;
}

test('settings page exposes the research-context integration section and controls', () => {
    const html = fs.readFileSync(settingsHTMLPath, 'utf8');

    assert.match(html, /<details id="settings-integration"/);
    assert.match(html, /data-i18n="settings\.integration\.title"/);
    for (const id of [
        'integrationSettingsForm',
        'integrationEnabledInput',
        'integrationPortInput',
        'integrationSummary',
        'integrationStatus',
        'integrationTokenSummary',
        'integrationTokenReveal',
        'integrationTokenValueInput',
        'integrationTokenStatus',
        'rotateIntegrationTokenButton',
        'revokeIntegrationTokenButton',
        'copyIntegrationTokenButton',
        'copyIntegrationConfigButton',
        'integrationCopyConfigHint',
    ]) {
        assert.match(html, new RegExp(`id="${id}"`), `expected ${id} control in settings.html`);
    }
    assert.match(html, /data-i18n="settings\.integration\.token_once_warning"/);
    assert.match(html, /data-i18n="settings\.integration\.copy_config"/);
});

test('settings sidebar maps the integration section to the integrations category', () => {
    const source = fs.readFileSync(settingsModulePath, 'utf8');
    assert.match(source, /'settings\.integration\.title':\s*'integrations'/);
});

test('api client wires the integration settings endpoints', () => {
    const source = fs.readFileSync(apiModulePath, 'utf8');
    assert.match(source, /getIntegrationSettings\(\)/);
    assert.match(source, /settings\/integration`/);
    assert.match(source, /updateIntegrationSettings\(data\)/);
    assert.match(source, /rotateIntegrationToken\(\)/);
    assert.match(source, /settings\/integration\/token\/rotate/);
    assert.match(source, /deleteIntegrationToken\(\)/);
    assert.match(source, /settings\/integration\/token`/);
});

test('integrationSettingsPayload defaults the port and reads the enable checkbox', () => {
    const subject = makeIntegrationSubject();
    const payload = () => JSON.parse(JSON.stringify(subject.integrationSettingsPayload()));

    assert.deepEqual(payload(), { enabled: false, port: 19831 });

    subject.integrationEnabledInput.checked = true;
    subject.integrationPortInput.value = '20001';
    assert.deepEqual(payload(), { enabled: true, port: 20001 });

    subject.integrationPortInput.value = 'not-a-port';
    assert.deepEqual(payload(), { enabled: true, port: 19831 });
});

test('renderIntegrationSummary shows running state and URL only when enabled', () => {
    const subject = makeIntegrationSubject();

    subject.renderIntegrationSummary({ enabled: true, port: 19831, url: 'http://127.0.0.1:19831/mcp' });
    assert.match(subject.integrationSummary.innerHTML, /http:\/\/127\.0\.0\.1:19831\/mcp/);
    assert.match(subject.integrationSummary.innerHTML, /运行中/);

    subject.renderIntegrationSummary({ enabled: false, port: 19831, url: 'http://127.0.0.1:19831/mcp' });
    assert.doesNotMatch(subject.integrationSummary.innerHTML, /http:\/\/127\.0\.0\.1:19831\/mcp/);
    assert.match(subject.integrationSummary.innerHTML, /已停止/);
});

test('renderIntegrationTokenSummary handles missing and active tokens', () => {
    const subject = makeIntegrationSubject();

    subject.renderIntegrationTokenSummary({ token: null });
    assert.match(subject.integrationTokenSummary.innerHTML, /尚未生成 Token/);

    subject.renderIntegrationTokenSummary({
        token: {
            active: true,
            created_at: '2026-08-01T02:03:04Z',
            last_used_at: null,
            scopes: ['library:read'],
        },
    });
    assert.match(subject.integrationTokenSummary.innerHTML, /已激活/);
    assert.match(subject.integrationTokenSummary.innerHTML, /formatted:2026-08-01T02:03:04Z/);
    assert.match(subject.integrationTokenSummary.innerHTML, /从未使用/);

    subject.renderIntegrationTokenSummary({
        token: {
            active: true,
            created_at: '2026-08-01T02:03:04Z',
            last_used_at: '2026-08-02T10:00:00Z',
            scopes: ['library:read'],
        },
    });
    assert.match(subject.integrationTokenSummary.innerHTML, /formatted:2026-08-02T10:00:00Z/);
});

test('integrationWispConfigPayload requires the freshly rotated plaintext token', () => {
    const subject = makeIntegrationSubject();
    subject.integrationSettings = { enabled: true, port: 19831, url: 'http://127.0.0.1:19831/mcp', token: { active: true } };

    assert.equal(subject.integrationWispConfigPayload(), null);

    subject.integrationFreshToken = 'cbx_test_token';
    assert.deepEqual(JSON.parse(JSON.stringify(subject.integrationWispConfigPayload())), {
        url: 'http://127.0.0.1:19831/mcp',
        headers: {
            Authorization: 'Bearer cbx_test_token',
        },
    });
});

test('integration locale keys stay aligned between zh-CN and en', () => {
    const zh = JSON.parse(fs.readFileSync(zhLocalePath, 'utf8'));
    const en = JSON.parse(fs.readFileSync(enLocalePath, 'utf8'));
    const zhKeys = Object.keys(zh).filter((key) => key.startsWith('settings.integration.'));
    const enKeys = Object.keys(en).filter((key) => key.startsWith('settings.integration.'));

    assert.ok(zhKeys.length > 0, 'expected zh-CN integration locale keys');
    assert.deepEqual(zhKeys.sort(), enKeys.sort());
});
