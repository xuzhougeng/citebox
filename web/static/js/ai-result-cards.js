(function () {
    'use strict';

    function escapeHtml(value) {
        return String(value == null ? '' : value)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    function safeUrl(value, opts) {
        opts = opts || {};
        const raw = String(value == null ? '' : value).trim();
        if (!raw) return '';
        if (raw.charAt(0) === '/') {
            if (raw.charAt(1) === '/') return '';
            return raw.replace(/"/g, '%22').replace(/</g, '%3C').replace(/>/g, '%3E');
        }
        try {
            const url = new URL(raw, window.location.origin);
            if ((url.protocol === 'http:' || url.protocol === 'https:') && (opts.allowExternal || url.origin === window.location.origin)) {
                return url.href;
            }
        } catch (e) { /* invalid URL */ }
        return '';
    }

    function translate(key, fallback) {
        if (window.CiteBoxI18n && typeof window.CiteBoxI18n.t === 'function') {
            return window.CiteBoxI18n.t(key, fallback);
        }
        if (typeof window.t === 'function') {
            return window.t(key, fallback);
        }
        return fallback || key;
    }

    function payload(card) {
        if (!card) return {};
        if (card.payload && typeof card.payload === 'object') return card.payload;
        if (typeof card.payload_json === 'string') {
            try { return JSON.parse(card.payload_json); } catch (e) { return {}; }
        }
        return {};
    }

    function cardType(card) {
        return card && (card.type || card.card_type) || '';
    }

    function render(cards) {
        if (!Array.isArray(cards) || cards.length === 0) return '';
        return '<div class="ai-result-cards">' + cards.map(renderCard).join('') + '</div>';
    }

    function metaLine(values) {
        return values.filter((value) => value != null && String(value).trim() !== '').join(' · ');
    }

    function citation(index) {
        return index ? ' [' + escapeHtml(index) + ']' : '';
    }

    function renderSnippets(snippets) {
        if (!Array.isArray(snippets) || snippets.length === 0) return '';
        return '<div class="ai-result-snippets">' + snippets.slice(0, 3).map((snippet) => {
            const location = snippet.location ? '<span>' + escapeHtml(snippet.location) + '</span>' : '';
            return '<blockquote>' +
                '<p>' + escapeHtml(snippet.text || '') + citation(snippet.citation_index) + '</p>' +
                location +
            '</blockquote>';
        }).join('') + '</div>';
    }

    function renderCard(card) {
        const p = payload(card);
        switch (cardType(card)) {
        case 'paper_hit':
            return renderPaperHit(p);
        case 'external_paper':
            return renderExternalPaper(p);
        case 'paper_compare':
        case 'paper_read':
            return renderPaperRead(p, cardType(card));
        case 'figure_result':
            return renderFigureResult(p);
        default:
            return '<article class="ai-result-card ai-result-card-fallback"><pre>' +
                escapeHtml(JSON.stringify(p, null, 2)) +
            '</pre></article>';
        }
    }

    function renderPaperHit(p) {
        const href = p.paper_id ? '/library?paper=' + encodeURIComponent(p.paper_id) : '';
        const meta = metaLine([p.year, p.doi]);
        const snippets = renderSnippets(p.snippets);
        const hasEvidence = snippets !== '';
        const expandLabel = translate('ai.result_expand_evidence', '展开证据');
        const collapseLabel = translate('ai.result_collapse_evidence', '收起证据');
        return '<article class="ai-result-card ai-result-card-paper' + (hasEvidence ? ' ai-result-card-collapsible is-collapsed' : '') + '"' +
            (hasEvidence ? ' data-ai-result-collapsible="paper"' : '') + '>' +
            '<div class="ai-result-card-head">' +
                '<div class="ai-result-card-title-group">' +
                    '<h4>' + escapeHtml(p.title || translate('ai.result_paper_fallback', '文献')) + '</h4>' +
                    (meta ? '<p>' + escapeHtml(meta) + '</p>' : '') +
                '</div>' +
                (hasEvidence
                    ? '<button class="ai-result-card-toggle" type="button" data-ai-result-toggle aria-expanded="false">' +
                        '<span class="ai-result-card-toggle-collapsed">' + escapeHtml(expandLabel) + '</span>' +
                        '<span class="ai-result-card-toggle-expanded">' + escapeHtml(collapseLabel) + '</span>' +
                    '</button>'
                    : '') +
            '</div>' +
            (p.reason ? '<p class="ai-result-reason">' + escapeHtml(p.reason) + '</p>' : '') +
            (hasEvidence ? '<div class="ai-result-card-collapsible-body" hidden>' + snippets + '</div>' : '') +
            (href ? '<div class="ai-result-card-actions"><a class="btn btn-small btn-outline" href="' + escapeHtml(href) + '">' + escapeHtml(translate('ai.result_open_paper', '打开文献')) + '</a></div>' : '') +
        '</article>';
    }

    function renderExternalPaper(p) {
        const meta = metaLine([p.venue, p.year, p.doi]);
        const s2 = safeUrl(p.s2_paper_id ? 'https://www.semanticscholar.org/paper/' + encodeURIComponent(p.s2_paper_id) : '', { allowExternal: true });
        return '<article class="ai-result-card ai-result-card-external">' +
            '<div class="ai-result-card-head">' +
                '<h4>' + escapeHtml(p.title || translate('ai.result_external_paper_fallback', '外部文献')) + '</h4>' +
                (meta ? '<p>' + escapeHtml(meta) + '</p>' : '') +
            '</div>' +
            (p.tldr ? '<p class="ai-result-reason">' + escapeHtml(p.tldr) + citation(p.citation_index) + '</p>' : '') +
            (s2 ? '<a class="btn btn-small btn-outline" href="' + escapeHtml(s2) + '" target="_blank" rel="noopener">' + escapeHtml(translate('ai.result_open_external', '查看来源')) + '</a>' : '') +
        '</article>';
    }

    function renderPaperRead(p, type) {
        const fallback = type === 'paper_read' ? translate('ai.result_paper_read_fallback', '文献阅读') : translate('ai.result_paper_compare_fallback', '文献对比');
        const papers = Array.isArray(p.papers) ? p.papers : [];
        return '<article class="ai-result-card ai-result-card-read">' +
            '<div class="ai-result-card-head">' +
                '<h4>' + escapeHtml(p.query || fallback) + '</h4>' +
                (p.note ? '<p>' + escapeHtml(p.note) + '</p>' : '') +
            '</div>' +
            papers.map((paper) => (
                '<section class="ai-result-paper-section">' +
                    '<strong>' + escapeHtml(paper.title || translate('ai.result_paper_fallback', '文献')) + '</strong>' +
                    renderSnippets(paper.evidence) +
                '</section>'
            )).join('') +
        '</article>';
    }

    function renderFigureResult(p) {
        const src = safeUrl(p.image_url);
        const figureFallback = translate('ai.result_figure_fallback', 'Figure');
        return '<article class="ai-result-card ai-figure-result-card">' +
            (src
                ? '<img src="' + escapeHtml(src) + '" alt="' + escapeHtml(p.display_label || figureFallback) + '" loading="lazy">'
                : '<div class="ai-figure-missing">' + escapeHtml(translate('ai.result_image_unavailable', '图片不可用')) + '</div>') +
            '<div class="ai-result-card-head">' +
                '<h4>' + escapeHtml(p.display_label || figureFallback) + '</h4>' +
                (p.paper_title ? '<p>' + escapeHtml(p.paper_title) + '</p>' : '') +
            '</div>' +
            (p.caption ? '<p class="ai-result-reason">' + escapeHtml(p.caption) + '</p>' : '') +
            (p.notes_text ? '<p class="ai-result-note">' + escapeHtml(p.notes_text) + '</p>' : '') +
        '</article>';
    }

    function bindCollapsibleCards() {
        if (typeof document === 'undefined' || document.__citeboxAIResultCardsBound) return;
        document.__citeboxAIResultCardsBound = true;
        document.addEventListener('click', (event) => {
            const button = event.target.closest('[data-ai-result-toggle]');
            if (!button) return;
            const card = button.closest('[data-ai-result-collapsible="paper"]');
            if (!card) return;
            const body = card.querySelector('.ai-result-card-collapsible-body');
            const isExpanded = button.getAttribute('aria-expanded') === 'true';
            button.setAttribute('aria-expanded', isExpanded ? 'false' : 'true');
            card.classList.toggle('is-collapsed', isExpanded);
            if (body) body.hidden = isExpanded;
        });
    }

    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.resultCards = { render: render };
        bindCollapsibleCards();
    }
})();
