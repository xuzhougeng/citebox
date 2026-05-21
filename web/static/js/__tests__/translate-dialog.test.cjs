'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'translate.js');

function createElement(tagName) {
    const listeners = new Map();
    return {
        tagName,
        className: '',
        innerHTML: '',
        removed: false,
        focused: false,
        attributes: new Map(),
        addEventListener(type, callback) {
            listeners.set(type, callback);
        },
        dispatch(type, event) {
            const callback = listeners.get(type);
            if (callback) {
                return callback(event);
            }
            return undefined;
        },
        focus() {
            this.focused = true;
        },
        remove() {
            this.removed = true;
        },
        setAttribute(name, value) {
            this.attributes.set(name, String(value));
        },
    };
}

function loadTranslate() {
    const code = `${fs.readFileSync(modulePath, 'utf8')}\nglobalThis.__DESKTOP_TRANSLATE__ = DesktopTranslate;`;
    let appended = null;
    const context = {
        console,
        window: {
            t(key, fallback) { return fallback || key; },
        },
        document: {
            body: {
                appendChild(element) {
                    appended = element;
                },
            },
            createElement,
            getElementById() {
                return null;
            },
            addEventListener() {},
        },
        navigator: {
            clipboard: {
                writeText() {
                    return Promise.resolve();
                },
            },
        },
        Utils: {
            escapeHTML(value) {
                return String(value ?? '');
            },
            isDesktopApp() {
                return true;
            },
            showToast() {},
        },
        t(key, fallback) {
            return fallback || key;
        },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: modulePath });
    return {
        DesktopTranslate: context.__DESKTOP_TRANSLATE__,
        getAppended: () => appended,
    };
}

test('translation result dialog closes itself on Escape without letting the key event continue', () => {
    const { DesktopTranslate, getAppended } = loadTranslate();
    DesktopTranslate.renderResultDialog({
        title: 'Translate',
        loading: false,
        sourceLanguage: '其他语言',
        targetLanguage: '中文',
        translation: '译文',
    });
    const overlay = getAppended();
    let prevented = false;
    let stopped = false;
    let immediateStopped = false;

    overlay.dispatch('keydown', {
        key: 'Escape',
        preventDefault() { prevented = true; },
        stopPropagation() { stopped = true; },
        stopImmediatePropagation() { immediateStopped = true; },
    });

    assert.equal(prevented, true);
    assert.equal(stopped, true);
    assert.equal(immediateStopped, true);
    assert.equal(overlay.removed, true);
    assert.equal(DesktopTranslate.resultDialog, null);
});
