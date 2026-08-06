// popup.js — 弹窗逻辑：读取配置 → 查询页面元数据 → 触发入库 → 展示结果
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
}

// 规范化服务器地址（与 background 保持一致）
function normalizeServer(url) {
    const trimmed = (url || '').trim();
    return (trimmed || DEFAULT_SERVER).replace(/\/+$/, '');
}

function showView(id) {
    ['view-config-missing', 'view-unreadable', 'view-main'].forEach((viewId) => {
        document.getElementById(viewId).hidden = viewId !== id;
    });
}

function setStatus(text, kind) {
    const el = document.getElementById('status');
    el.textContent = text || '';
    el.className = kind || '';
}

// 生成 CiteBox Web 端查看链接，不确定 paper id 时退化为文库页
function buildPaperLink(server, paper) {
    if (paper && paper.id) {
        return server + '/viewer?paper_id=' + encodeURIComponent(paper.id);
    }
    return server + '/library';
}

async function init() {
    applyI18n();

    document.getElementById('btn-open-options').addEventListener('click', () => {
        chrome.runtime.openOptionsPage();
    });

    const stored = await chrome.storage.sync.get({ serverUrl: DEFAULT_SERVER, token: '' });
    const server = normalizeServer(stored.serverUrl);
    if (!stored.token) {
        showView('view-config-missing');
        return;
    }

    // 向当前标签页的 content script 查询元数据
    let info = null;
    try {
        const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
        if (!tab || tab.id === undefined) {
            throw new Error('no active tab');
        }
        info = await chrome.tabs.sendMessage(tab.id, { type: 'citebox:query' });
    } catch (e) {
        // chrome:// 等受限页面或 content script 未注入时进入这里
        showView('view-unreadable');
        return;
    }
    if (!info) {
        showView('view-unreadable');
        return;
    }

    document.getElementById('field-title').textContent =
        info.title || chrome.i18n.getMessage('notDetected');
    document.getElementById('field-doi').textContent =
        info.doi || chrome.i18n.getMessage('notDetected');
    showView('view-main');

    const saveBtn = document.getElementById('btn-save');
    const openLink = document.getElementById('open-link');

    saveBtn.addEventListener('click', async () => {
        saveBtn.disabled = true;
        saveBtn.textContent = chrome.i18n.getMessage('saving');
        setStatus('', '');
        openLink.hidden = true;

        let result = null;
        try {
            result = await chrome.runtime.sendMessage({
                type: 'citebox:import',
                payload: {
                    doi: info.doi,
                    title: info.title,
                    pdfUrl: info.pdfUrl,
                    pageUrl: info.pageUrl
                }
            });
        } catch (e) {
            result = { status: 'error', error: String(e) };
        }
        result = result || { status: 'error', error: '' };
        const resultServer = normalizeServer(result.server || server);

        switch (result.status) {
            case 'imported':
                setStatus(chrome.i18n.getMessage('imported'), 'ok');
                openLink.href = buildPaperLink(resultServer, result.paper);
                openLink.hidden = false;
                break;
            case 'duplicate':
                setStatus(chrome.i18n.getMessage('alreadyExists'), 'warn');
                openLink.href = buildPaperLink(resultServer, result.paper);
                openLink.hidden = false;
                break;
            case 'unauthorized':
                setStatus(chrome.i18n.getMessage('checkToken'), 'err');
                saveBtn.disabled = false;
                break;
            default:
                setStatus(result.error || chrome.i18n.getMessage('unknownError'), 'err');
                saveBtn.disabled = false;
                break;
        }
        saveBtn.textContent = chrome.i18n.getMessage('saveToCitebox');
    });
}

document.addEventListener('DOMContentLoaded', init);
