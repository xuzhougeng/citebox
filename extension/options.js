// options.js — 设置页：服务器地址与集成令牌的保存、连接测试
'use strict';

const DEFAULT_SERVER = 'http://localhost:8080';

// 用 data-i18n 属性统一替换页面文案
function applyI18n() {
    document.querySelectorAll('[data-i18n]').forEach((el) => {
        const text = chrome.i18n.getMessage(el.getAttribute('data-i18n'));
        if (text) {
            el.textContent = text;
        }
    });
    document.querySelectorAll('[data-i18n-placeholder]').forEach((el) => {
        const text = chrome.i18n.getMessage(el.getAttribute('data-i18n-placeholder'));
        if (text) {
            el.placeholder = text;
        }
    });
}

function setStatus(text, kind) {
    const el = document.getElementById('status');
    el.textContent = text || '';
    el.className = kind || '';
}

async function init() {
    applyI18n();

    const serverInput = document.getElementById('server-url');
    const tokenInput = document.getElementById('token');
    const saveBtn = document.getElementById('btn-save');
    const testBtn = document.getElementById('btn-test');

    // 回填已保存的配置
    const stored = await chrome.storage.sync.get({ serverUrl: DEFAULT_SERVER, token: '' });
    serverInput.value = stored.serverUrl;
    tokenInput.value = stored.token;

    saveBtn.addEventListener('click', async () => {
        const serverUrl = serverInput.value.trim() || DEFAULT_SERVER;
        const token = tokenInput.value.trim();
        if (!token) {
            setStatus(chrome.i18n.getMessage('fillRequired'), 'err');
            return;
        }
        await chrome.storage.sync.set({ serverUrl: serverUrl, token: token });
        setStatus(chrome.i18n.getMessage('saved'), 'ok');
    });

    testBtn.addEventListener('click', async () => {
        const serverUrl = serverInput.value.trim() || DEFAULT_SERVER;
        const token = tokenInput.value.trim();
        if (!token) {
            setStatus(chrome.i18n.getMessage('fillRequired'), 'err');
            return;
        }
        testBtn.disabled = true;
        setStatus(chrome.i18n.getMessage('testing'), '');
        let result = null;
        try {
            // 网络请求统一由 background 代发
            result = await chrome.runtime.sendMessage({
                type: 'citebox:testConnection',
                payload: { server: serverUrl, token: token }
            });
        } catch (e) {
            result = { ok: false, reason: 'network' };
        }
        testBtn.disabled = false;
        if (result && result.ok) {
            setStatus(chrome.i18n.getMessage('testSuccess'), 'ok');
            return;
        }
        switch (result && result.reason) {
            case 'unauthorized':
            case 'missingToken':
                setStatus(chrome.i18n.getMessage('testInvalidToken'), 'err');
                break;
            case 'http':
                setStatus(chrome.i18n.getMessage('testHttpError', String(result.httpStatus)), 'err');
                break;
            default:
                setStatus(chrome.i18n.getMessage('testUnreachable'), 'err');
                break;
        }
    });
}

document.addEventListener('DOMContentLoaded', init);
