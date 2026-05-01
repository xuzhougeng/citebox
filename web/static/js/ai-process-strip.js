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

    function normalizeSummary(summary) {
        if (!summary) return null;
        if (typeof summary === 'string') {
            try { return JSON.parse(summary); } catch (e) { return null; }
        }
        return summary;
    }

    function render(summary) {
        const data = normalizeSummary(summary);
        if (!data || !Array.isArray(data.stages) || data.stages.length === 0) return '';
        const stages = data.stages.map((stage) => {
            const label = stage && stage.label || '';
            const hasCount = stage && stage.count != null && stage.count !== '';
            const count = hasCount ? ' ' + stage.count + (stage.unit || '') : '';
            const status = stage && stage.status ? ' data-status="' + escapeHtml(stage.status) + '"' : '';
            const detail = stage && stage.detail ? String(stage.detail) : '';
            const detailAttrs = detail
                ? ' title="' + escapeHtml(detail) + '" aria-label="' + escapeHtml(label + count + '：' + detail) + '"'
                : '';
            return '<span class="ai-process-stage"' + status + detailAttrs + '>' +
                escapeHtml(label) + escapeHtml(count) +
            '</span>';
        }).join('<span class="ai-process-sep">·</span>');
        const note = data.note ? '<span class="ai-process-note">' + escapeHtml(data.note) + '</span>' : '';
        return '<div class="ai-process-strip">' + stages + note + '</div>';
    }

    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.processStrip = { render: render };
    }
})();
