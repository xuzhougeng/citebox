'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function setup(fetch, pathname = '/ai') {
    const storage = new Map([['citebox_lang', 'en']]);
    const errors = [];
    let reloads = 0;
    const target = { insertBefore(node) { this.switcher = node; } };
    const context = {
        fetch, AbortController, setTimeout, clearTimeout,
        window: { location: { pathname, reload() { reloads++; } } },
        localStorage: { getItem: key => storage.get(key), setItem: (key, value) => storage.set(key, value) },
        Utils: { showToast: message => errors.push(message) },
        document: {
            addEventListener() {},
            querySelector: selector => selector === '.login-footer' ? target : null,
            createElement() {
                return { children: [], listeners: {}, setAttribute() {},
                    appendChild(node) { this.children.push(node); },
                    querySelectorAll() { return this.children; },
                    addEventListener(type, handler) { this.listeners[type] = handler; }
                };
            }
        }
    };
    vm.runInNewContext(fs.readFileSync(path.join(__dirname, '..', 'i18n.js'), 'utf8'), context);
    const i18n = context.CiteBoxI18n;
    i18n._injectStyles = () => {};
    i18n.injectSwitcher();
    return { i18n, storage, errors, button: target.switcher.children[0], reloads: () => reloads };
}

test('language switch waits for successful persistence before changing cache and reloading', async () => {
    let finish;
    const { button, storage, reloads } = setup(() => new Promise(resolve => { finish = resolve; }));
    const switching = button.listeners.click();
    assert.equal(reloads(), 0);
    assert.equal(storage.get('citebox_lang'), 'en');
    assert.equal(button.disabled, true);
    finish({ ok: true });
    await switching;
    assert.equal(storage.get('citebox_lang'), 'zh-CN');
    assert.equal(reloads(), 1);
});

for (const failure of ['network', 'server']) {
    test(`language ${failure} failure preserves the current language and allows retry`, async () => {
        const { button, storage, errors, reloads } = setup(async () => {
            if (failure === 'network') throw new Error('Offline');
            return { ok: false, status: 500 };
        });
        await button.listeners.click();
        assert.equal(reloads(), 0);
        assert.equal(storage.get('citebox_lang'), 'en');
        assert.equal(errors.length, 1);
        assert.equal(button.disabled, false);
    });
}

test('login language still works before authentication', async () => {
    const { button, storage, reloads } = setup(async () => ({ ok: false, status: 401 }), '/login');
    await button.listeners.click();
    assert.equal(storage.get('citebox_lang'), 'zh-CN');
    assert.equal(reloads(), 1);
});
