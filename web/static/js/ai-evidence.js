// ai-evidence.js — citation footnotes for strict-evidence assistant replies.
//
// Listens for `ai-reader:message-rendered` (dispatched by ai-conversation-view
// after a final event) and replaces "[n]" tokens inside the message element
// with <sup class="ai-citation"> elements. On hover/focus, a shared tooltip
// renders the matching citation's snippet text + section + a "view in paper"
// link.

(function () {
    'use strict';

    function escapeHtml(s) {
        return String(s == null ? '' : s)
            .replace(/&/g, '&amp;').replace(/</g, '&lt;')
            .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    let tooltipEl = null;
    let hideTimer = null;

    function t(key, fallback) {
        return window.CiteBoxI18n ? window.CiteBoxI18n.t(key, fallback) : fallback;
    }

    function citationURL(citation) {
        const raw = citation.source_url || (citation.s2_paper_id ? 'https://www.semanticscholar.org/paper/' + encodeURIComponent(citation.s2_paper_id) : '');
        if (!raw) return '';
        try {
            const url = new URL(raw, window.location.origin);
            if (!['https:', 'http:'].includes(url.protocol)) return '';
            return url.href;
        } catch (_) { return ''; }
    }


    function ensureTooltip() {
        if (tooltipEl) return tooltipEl;
        tooltipEl = document.createElement('div');
        tooltipEl.className = 'ai-citation-tooltip';
        tooltipEl.hidden = true;
        tooltipEl.addEventListener('mouseenter', () => clearTimeout(hideTimer));
        tooltipEl.addEventListener('mouseleave', hideTooltip);
        tooltipEl.addEventListener('focusin', () => clearTimeout(hideTimer));
        tooltipEl.addEventListener('focusout', scheduleHideTooltip);
        document.body.appendChild(tooltipEl);
        return tooltipEl;
    }

    function showTooltip(anchor, citation) {
        clearTimeout(hideTimer);
        const tooltip = ensureTooltip();
        const paperTitle = citation.title || citation._title || '';
        const section = citation.snippet && (citation.snippet.section || citation.snippet.snippetKind) || '';
        const text = citation.snippet && citation.snippet.text || '';
        const truncated = text.length > 400 ? text.slice(0, 400) + '…' : text;
        const href = citationURL(citation);
        const srcLink = href ? `<a class="src" href="${escapeHtml(href)}">${escapeHtml(t('ai.citation_open_source', 'Open source'))}</a>` : '';
        const page = citation.page ? t('ai.citation_page', 'Page {page}').replace('{page}', citation.page) : '';
        tooltip.innerHTML = '' +
            (paperTitle ? `<h4>${escapeHtml(paperTitle)}</h4>` : '') +
            ([section, page].filter(Boolean).length ? `<div class="meta">${escapeHtml([section, page].filter(Boolean).join(' · '))}</div>` : '') +
            `<div class="snippet">${escapeHtml(truncated)}</div>` + srcLink;
        const r = anchor.getBoundingClientRect();
        tooltip.style.left = (window.scrollX + r.left) + 'px';
        tooltip.style.top = (window.scrollY + r.bottom + 4) + 'px';
        tooltip.hidden = false;
    }

    function scheduleHideTooltip() {
        clearTimeout(hideTimer);
        hideTimer = setTimeout(hideTooltip, 150);
    }

    function hideTooltip() {
        if (tooltipEl) tooltipEl.hidden = true;
    }

    const Evidence = {
        init() {
            document.addEventListener('ai-reader:message-rendered', (e) => {
                if (!e.detail || !e.detail.element) return;
                let citations = e.detail.citations;
                if (typeof citations === 'string') {
                    try { citations = JSON.parse(citations); } catch (err) { return; }
                }
                if (!Array.isArray(citations) || !citations.length) return;
                this.hydrate(e.detail.element, citations);
            });
        },

        hydrate(messageEl, citations) {
            // Build an index 1..N → citation
            const byIndex = {};
            citations.forEach((c) => { byIndex[c.i] = c; });

            // Resolve the paper title for each citation by reading from the active
            // conversation's pinned papers list (best-effort; falls back to s2_paper_id).
            const pinned = (window.AIReader && window.AIReader.view && window.AIReader.view._state && window.AIReader.view._state.pinnedPapers) || [];
            const pinnedByID = {};
            pinned.forEach((p) => { pinnedByID[p.paper_id] = p; });
            citations.forEach((c) => {
                const p = pinnedByID[c.paper_id];
                if (p && p.title) c._title = p.title;
            });

            // Walk text nodes and replace [n] tokens.
            const walker = document.createTreeWalker(messageEl, NodeFilter.SHOW_TEXT, null);
            const replacements = [];
            let node;
            while ((node = walker.nextNode())) {
                const text = node.nodeValue;
                if (!/\[\d+\]/.test(text)) continue;
                const parent = textNodeParentElement(node);
                if (parent && parent.closest('.ai-citation, a, code, pre')) continue;
                replacements.push(node);
            }
            replacements.forEach((textNode) => {
                const text = textNode.nodeValue;
                const frag = document.createDocumentFragment();
                let last = 0;
                const re = /\[(\d+)\]/g;
                let m;
                while ((m = re.exec(text)) !== null) {
                    if (m.index > last) {
                        frag.appendChild(document.createTextNode(text.slice(last, m.index)));
                    }
                    const idx = parseInt(m[1], 10);
                    const c = byIndex[idx];
                    if (c) {
                        const href = citationURL(c);
                        const sup = document.createElement(href ? 'a' : 'sup');
                        if (href) sup.href = href;
                        sup.className = 'ai-citation';
                        sup.dataset.cite = String(idx);
                        sup.textContent = '[' + idx + ']';
                        sup.tabIndex = 0;
                        sup.addEventListener('mouseenter', () => showTooltip(sup, c));
                        sup.addEventListener('focus', () => showTooltip(sup, c));
                        sup.addEventListener('mouseleave', scheduleHideTooltip);
                        sup.addEventListener('blur', scheduleHideTooltip);
                        frag.appendChild(sup);
                    } else {
                        frag.appendChild(document.createTextNode(m[0]));
                    }
                    last = m.index + m[0].length;
                }
                if (last < text.length) {
                    frag.appendChild(document.createTextNode(text.slice(last)));
                }
                textNode.parentNode.replaceChild(frag, textNode);
            });
        },
    };

    function textNodeParentElement(node) {
        return node && node.parentElement || null;
    }

    document.addEventListener('DOMContentLoaded', () => Evidence.init());
    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.evidence = Evidence;
    }
})();
