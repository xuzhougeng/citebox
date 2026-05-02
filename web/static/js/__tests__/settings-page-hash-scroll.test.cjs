'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'settings.js');

function makeElement(tagName = 'div') {
    return {
        tagName: String(tagName).toUpperCase(),
        id: '',
        className: '',
        dataset: {},
        children: [],
        hidden: false,
        innerHTML: '',
        textContent: '',
        style: {},
        value: '',
        checked: false,
        disabled: false,
        open: false,
        appendChild(child) {
            this.children.push(child);
            return child;
        },
        addEventListener() {},
        setAttribute() {},
        querySelector() {
            return null;
        },
        querySelectorAll() {
            return [];
        },
        classList: {
            add() {},
            remove() {},
            toggle() {},
            contains() {
                return false;
            },
        },
    };
}

function loadSettingsPage(contextOverrides = {}) {
    const code = fs.readFileSync(modulePath, 'utf8') + '\nmodule.exports = SettingsPage;';
    const sidebar = makeElement('aside');
    const stack = makeElement('section');
    const sectionTitle = { dataset: { i18n: 'settings.research.title' }, textContent: '文献搜索服务 API' };
    const section = makeElement('details');
    section.id = 'settings-external-sources';
    section.querySelector = (selector) => selector === 'summary h2[data-i18n]' ? sectionTitle : null;
    stack.querySelectorAll = () => [section];

    const document = {
        getElementById(id) {
            if (id === 'settingsAnchorSidebar') return sidebar;
            return makeElement();
        },
        querySelector(selector) {
            if (selector === '.settings-content') return stack;
            return null;
        },
        createElement(tagName) {
            return makeElement(tagName);
        },
        addEventListener() {},
    };

    const context = {
        console,
        module: { exports: {} },
        exports: {},
        document,
        window: {
            location: { hash: '#settings-external-sources' },
            t(_key, fallback) {
                return fallback || _key;
            },
            setTimeout(fn) {
                if (typeof fn === 'function') fn();
                return 1;
            },
            addEventListener() {},
            requestAnimationFrame(fn) {
                if (typeof fn === 'function') fn();
            },
            scrollTo() {},
        },
        history: {
            replaceState() {},
        },
        CiteBoxI18n: {
            applyDOM() {},
        },
        Utils: {
            showToast() {},
        },
        API: {},
        setTimeout(fn) {
            if (typeof fn === 'function') fn();
            return 1;
        },
        clearTimeout() {},
        ...contextOverrides,
    };
    context.globalThis = context;
    if (!context.window.document) {
        context.window.document = document;
    }
    vm.runInNewContext(code, context, { filename: modulePath });
    return context.module.exports;
}

function createSubject(SettingsPage) {
    const subject = Object.create(SettingsPage);
    subject.settingsCategoryList = null;
    subject.settingsAnchorList = null;
    subject.applyCalls = [];
    subject.applySettingsHash = function (hash, options) {
        this.applyCalls.push({ hash, options: { ...options } });
    };
    subject.loadAISettings = async () => {};
    subject.loadExtractorSettings = async () => {};
    subject.loadWolaiSettings = async () => {};
    subject.loadDesktopCloseSettings = async () => {};
    subject.loadVersionStatus = async () => {};
    subject.loadAuthSettings = async () => {};
    subject.loadTTSSettings = async () => {};
    subject.loadResearchSettings = async () => {};
    return subject;
}

test('initial hash application defers scrolling until bootstrap settles', async () => {
    const SettingsPage = loadSettingsPage();
    const subject = createSubject(SettingsPage);

    subject.buildAnchorSidebar();
    assert.deepEqual(subject.applyCalls, [
        {
            hash: '#settings-external-sources',
            options: { scroll: false },
        },
    ]);

    await subject.bootstrap();
    assert.deepEqual(subject.applyCalls, [
        {
            hash: '#settings-external-sources',
            options: { scroll: false },
        },
        {
            hash: '#settings-external-sources',
            options: { scroll: true },
        },
    ]);
});
