// content.js — 页面元数据提取（DOI / 标题 / PDF 链接）
// 以 document_idle 注入所有页面，仅响应 popup 的查询消息，不主动发网络请求。
(function () {
    'use strict';

    // DOI 基本形态：10.<4-9 位数字>/<非空白非引号字符>
    const DOI_PATTERN = /10\.\d{4,9}\/[^\s"<>]+/;
    // 页面 URL 中的 doi.org 链接
    const DOI_URL_PATTERN = /doi\.org\/(10\.\d{4,9}\/[^\s?#"'<>]+)/i;
    // arXiv 摘要页 / PDF 页：新版编号（2103.12345）与旧版编号（hep-th/9901001）
    const ARXIV_URL_PATTERN = /arxiv\.org\/(?:abs|pdf)\/(\d{4}\.\d{4,5}|[a-z-]+(?:\.[A-Z]{2})?\/\d{7})/i;

    // 读取第一个命中的 meta 标签内容
    function getMetaContent(selectors) {
        for (const selector of selectors) {
            const el = document.querySelector(selector);
            const content = el && el.getAttribute('content');
            if (content && content.trim()) {
                return content.trim();
            }
        }
        return null;
    }

    // 清理 DOI 字符串：去掉 "doi:" 前缀、doi.org 前缀与结尾标点
    function normalizeDoi(raw) {
        if (!raw) {
            return null;
        }
        let doi = raw.trim();
        doi = doi.replace(/^doi:\s*/i, '');
        doi = doi.replace(/^https?:\/\/(?:dx\.)?doi\.org\//i, '');
        const match = doi.match(DOI_PATTERN);
        if (!match) {
            return null;
        }
        return match[0].replace(/[.,;)]+$/, '');
    }

    // 按优先级提取 DOI
    function extractDoi() {
        // 1. citation_doi（出版商页面标准元数据）
        const metaDoi = normalizeDoi(getMetaContent(['meta[name="citation_doi"]']));
        if (metaDoi) {
            return metaDoi;
        }
        // 2. dc.identifier（仅当内容形如 DOI）
        const dcDoi = normalizeDoi(getMetaContent([
            'meta[name="dc.identifier"]',
            'meta[name="DC.identifier"]'
        ]));
        if (dcDoi) {
            return dcDoi;
        }
        // 3. 页面 URL：doi.org 直链或 arXiv 页面（转为 arXiv DOI）
        const url = location.href;
        const doiUrlMatch = url.match(DOI_URL_PATTERN);
        if (doiUrlMatch) {
            return normalizeDoi(doiUrlMatch[1]);
        }
        const arxivMatch = url.match(ARXIV_URL_PATTERN);
        if (arxivMatch) {
            return '10.48550/arXiv.' + arxivMatch[1];
        }
        // 4. 兜底：在页面可见文本中找第一个 DOI 形态匹配
        const bodyText = document.body ? document.body.innerText : '';
        const textMatch = bodyText.match(DOI_PATTERN);
        if (textMatch) {
            return normalizeDoi(textMatch[0]);
        }
        return null;
    }

    // 按优先级提取标题
    function extractTitle() {
        const title = getMetaContent([
            'meta[name="citation_title"]',
            'meta[property="og:title"]'
        ]);
        if (title) {
            return title;
        }
        return document.title || null;
    }

    // 提取 PDF 直链（相对 URL 解析为绝对 URL）
    function extractPdfUrl() {
        const raw = getMetaContent(['meta[name="citation_pdf_url"]']);
        if (!raw) {
            return null;
        }
        try {
            return new URL(raw, location.href).href;
        } catch (e) {
            return null;
        }
    }

    // 响应 popup 的元数据查询（同步响应，无需 return true）
    chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
        if (!message || message.type !== 'citebox:query') {
            return false;
        }
        sendResponse({
            doi: extractDoi(),
            title: extractTitle(),
            pdfUrl: extractPdfUrl(),
            pageUrl: location.href
        });
        return false;
    });
})();
