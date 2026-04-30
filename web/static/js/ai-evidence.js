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

    function ensureTooltip() {
        if (tooltipEl) return tooltipEl;
        tooltipEl = document.createElement('div');
        tooltipEl.className = 'ai-citation-tooltip';
        tooltipEl.hidden = true;
        document.body.appendChild(tooltipEl);
        return tooltipEl;
    }

    function showTooltip(anchor, citation) {
        const t = ensureTooltip();
        const paperTitle = citation._title || '';
        const section = citation.snippet && (citation.snippet.section || citation.snippet.snippetKind) || '';
        const text = citation.snippet && citation.snippet.text || '';
        const truncated = text.length > 400 ? text.slice(0, 400) + '…' : text;
        let srcLink = '';
        if (citation.s2_paper_id) {
            srcLink = `<a class="src" href="https://www.semanticscholar.org/paper/${escapeHtml(citation.s2_paper_id)}" target="_blank" rel="noopener">在 Semantic Scholar 中查看 →</a>`;
        }
        t.innerHTML = '' +
            (paperTitle ? `<h4>${escapeHtml(paperTitle)}</h4>` : '') +
            (section ? `<div class="meta">${escapeHtml(section)}</div>` : '') +
            `<div class="snippet">${escapeHtml(truncated)}</div>` +
            srcLink;
        const r = anchor.getBoundingClientRect();
        t.style.left = (window.scrollX + r.left) + 'px';
        t.style.top = (window.scrollY + r.bottom + 4) + 'px';
        t.hidden = false;
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
                        const sup = document.createElement('sup');
                        sup.className = 'ai-citation';
                        sup.dataset.cite = String(idx);
                        sup.textContent = '[' + idx + ']';
                        sup.tabIndex = 0;
                        sup.addEventListener('mouseenter', () => showTooltip(sup, c));
                        sup.addEventListener('focus', () => showTooltip(sup, c));
                        sup.addEventListener('mouseleave', hideTooltip);
                        sup.addEventListener('blur', hideTooltip);
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

    document.addEventListener('DOMContentLoaded', () => Evidence.init());
    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.evidence = Evidence;
    }
})();
