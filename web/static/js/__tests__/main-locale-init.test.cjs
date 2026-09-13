const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

test('page rendering waits for translations while navigation is enhanced immediately', async () => {
    let finishLocale;
    let start;
    const calls = [];
    const ready = new Promise((resolve) => { finishLocale = resolve; });
    const context = {
        window: {
            location: { pathname: '/library' },
            addEventListener() {},
            CiteBoxI18n: { init: () => ready }
        },
        document: {
            addEventListener(event, callback) {
                if (event === 'DOMContentLoaded') start = callback;
            }
        },
        LibraryPage: { init: () => calls.push('library') },
        record: (value) => calls.push(value)
    };
    vm.createContext(context);
    vm.runInContext(fs.readFileSync(path.join(__dirname, '..', 'main.js'), 'utf8'), context);
    vm.runInContext(`
        AppNav.init = () => record('navigation');
        AppNavigationHotkeys.init = () => record('hotkeys');
        AppUpdateNotice.init = () => record('updates');
    `, context);

    const pending = start();
    assert.deepEqual(calls, ['navigation']);
    finishLocale();
    await pending;
    assert.deepEqual(calls, ['navigation', 'hotkeys', 'library', 'updates']);
});
