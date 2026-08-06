// background.js — MV3 service worker：统一代发所有网络请求
// 消息约定：
//   citebox:import          popup 请求入库，payload: { doi, title, pdfUrl, pageUrl }
//   citebox:testConnection  options 请求测试连接，payload: { server, token }
'use strict';

const DEFAULT_SERVER = 'http://localhost:8080';

// 规范化服务器地址：去空白、去末尾斜杠
function normalizeServer(url) {
    const trimmed = (url || '').trim();
    return (trimmed || DEFAULT_SERVER).replace(/\/+$/, '');
}

// 读取扩展配置
async function getConfig() {
    const stored = await chrome.storage.sync.get({ serverUrl: DEFAULT_SERVER, token: '' });
    return { server: normalizeServer(stored.serverUrl), token: (stored.token || '').trim() };
}

// URL 是否看起来指向 PDF
function looksLikePdfUrl(url) {
    return /\.pdf(\?|#|$)/i.test(url || '');
}

// 由标题或页面 URL 推导上传文件名，保证以 .pdf 结尾
function buildFileName(title, pageUrl) {
    let base = (title || '').trim().replace(/[\\/:*?"<>|]/g, ' ').replace(/\s+/g, ' ').trim();
    if (!base) {
        try {
            const path = new URL(pageUrl).pathname;
            base = decodeURIComponent(path.split('/').filter(Boolean).pop() || '');
        } catch (e) {
            base = '';
        }
        base = base.replace(/\.pdf$/i, '');
    }
    if (!base) {
        base = 'document';
    }
    if (base.length > 120) {
        base = base.slice(0, 120);
    }
    return base + '.pdf';
}

// 解析入库接口（import-by-doi 与 /api/papers 语义相同）的响应
async function parseImportResponse(resp) {
    let body = null;
    try {
        body = await resp.json();
    } catch (e) {
        // 非 JSON 响应，按 HTTP 状态码处理
    }
    if (resp.status === 202 && body && body.success) {
        return { status: 'imported', paper: body.paper || null };
    }
    if (resp.status === 409) {
        return { status: 'duplicate', paper: (body && body.paper) || null };
    }
    if (resp.status === 401) {
        return { status: 'unauthorized', error: (body && body.error) || '' };
    }
    const errText = (body && body.error) || ('HTTP ' + resp.status);
    return { status: 'error', error: errText };
}

// 策略 1：按 DOI 入库
async function importByDoi(server, token, doi, title) {
    let resp;
    try {
        resp = await fetch(server + '/api/papers/import-by-doi', {
            method: 'POST',
            headers: {
                'Authorization': 'Bearer ' + token,
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(title ? { doi: doi, title: title } : { doi: doi })
        });
    } catch (e) {
        return { status: 'error', error: chrome.i18n.getMessage('networkError', String(e)), networkFailed: true };
    }
    return parseImportResponse(resp);
}

// 策略 2：抓取页面 PDF 后以 multipart 上传
async function importByPdfUpload(server, token, pdfSourceUrl, title, pageUrl) {
    let pdfResp;
    try {
        // credentials: 'include' 带上浏览器 cookie，可获取机构订阅的付费墙 PDF
        pdfResp = await fetch(pdfSourceUrl, { credentials: 'include' });
    } catch (e) {
        return { status: 'error', error: chrome.i18n.getMessage('networkError', String(e)) };
    }
    const contentType = (pdfResp.headers.get('content-type') || '').toLowerCase();
    if (!pdfResp.ok || (!contentType.includes('pdf') && !looksLikePdfUrl(pdfSourceUrl))) {
        return { status: 'error', error: chrome.i18n.getMessage('noPdfFound') };
    }
    const blob = await pdfResp.blob();
    const form = new FormData();
    form.append('pdf', blob, buildFileName(title, pageUrl));
    if (title) {
        form.append('title', title);
    }
    let resp;
    try {
        resp = await fetch(server + '/api/papers', {
            method: 'POST',
            headers: { 'Authorization': 'Bearer ' + token },
            body: form
        });
    } catch (e) {
        return { status: 'error', error: chrome.i18n.getMessage('networkError', String(e)) };
    }
    return parseImportResponse(resp);
}

// 入库主流程：先 DOI，失败则降级为 PDF 抓取上传
async function handleImport(payload) {
    const { doi, title, pdfUrl, pageUrl } = payload || {};
    const { server, token } = await getConfig();
    if (!token) {
        return { status: 'unauthorized', error: chrome.i18n.getMessage('missingToken'), server: server };
    }

    if (doi) {
        const result = await importByDoi(server, token, doi, title);
        // 成功 / 重复 / 未认证：直接返回，不降级
        if (result.status !== 'error') {
            result.server = server;
            return result;
        }
        // 其他失败：若有可用 PDF 来源则自动降级到策略 2
        const candidate = pdfUrl || (looksLikePdfUrl(pageUrl) ? pageUrl : null);
        if (!candidate) {
            result.server = server;
            return result;
        }
        const fallback = await importByPdfUpload(server, token, candidate, title, pageUrl);
        fallback.server = server;
        return fallback;
    }

    // 无 DOI：直接用 citation_pdf_url 或页面 URL 抓取 PDF
    const candidate = pdfUrl || pageUrl;
    if (!candidate) {
        return { status: 'error', error: chrome.i18n.getMessage('noPdfFound'), server: server };
    }
    const result = await importByPdfUpload(server, token, candidate, title, pageUrl);
    result.server = server;
    return result;
}

// 测试连接：GET /api/papers?limit=1，按状态码区分结果
async function handleTestConnection(payload) {
    const server = normalizeServer(payload && payload.server);
    const token = ((payload && payload.token) || '').trim();
    if (!token) {
        return { ok: false, reason: 'missingToken' };
    }
    let resp;
    try {
        resp = await fetch(server + '/api/papers?limit=1', {
            headers: { 'Authorization': 'Bearer ' + token }
        });
    } catch (e) {
        return { ok: false, reason: 'network' };
    }
    if (resp.status === 200) {
        return { ok: true };
    }
    if (resp.status === 401) {
        return { ok: false, reason: 'unauthorized' };
    }
    return { ok: false, reason: 'http', httpStatus: resp.status };
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (!message || typeof message.type !== 'string') {
        return false;
    }
    if (message.type === 'citebox:import') {
        // 异步响应：return true 保持消息通道
        handleImport(message.payload).then(sendResponse);
        return true;
    }
    if (message.type === 'citebox:testConnection') {
        handleTestConnection(message.payload).then(sendResponse);
        return true;
    }
    return false;
});
