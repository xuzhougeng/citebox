(function (root, factory) {
    const api = factory();
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    }
    root.CiteBoxResearchErrors = api;
}(typeof globalThis !== 'undefined' ? globalThis : this, function () {
    'use strict';

    function fallbackTranslate(_key, fallback) {
        return fallback || '';
    }

    function parseJSON(text) {
        if (!text) return null;
        try {
            return JSON.parse(text);
        } catch (error) {
            return null;
        }
    }

    function parseResearchErrorResponse(status, bodyText, translate) {
        const t = typeof translate === 'function' ? translate : fallbackTranslate;
        const rawText = typeof bodyText === 'string' ? bodyText.trim() : '';
        const payload = parseJSON(rawText);
        const payloadMessage = payload && typeof payload.error === 'string' ? payload.error : '';
        const usedAPIKey = payload && typeof payload.used_api_key === 'boolean'
            ? payload.used_api_key
            : null;
        const rateLimited = status === 503 && (usedAPIKey !== null || /rate limited|限流/i.test(payloadMessage || rawText));

        let message = payloadMessage || rawText || String(status);
        if (rateLimited) {
            if (usedAPIKey === true) {
                message = t(
                    'research.error.rateLimitedWithKey',
                    'Semantic Scholar 限流，请稍后重试。本次请求已携带 API Key。'
                );
            } else if (usedAPIKey === false) {
                message = t(
                    'research.error.rateLimitedWithoutKey',
                    'Semantic Scholar 限流，请稍后重试。本次请求未携带 API Key，请检查设置。'
                );
            } else {
                message = t('research.error.rateLimited', 'Semantic Scholar 限流，请稍后重试。');
            }
        }

        return {
            status,
            payload,
            rateLimited,
            usedAPIKey,
            message,
        };
    }

    return {
        parseResearchErrorResponse,
    };
}));
