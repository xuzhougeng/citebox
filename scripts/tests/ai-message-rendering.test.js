const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

function loadAssistantMarkdownRenderer() {
    let source = fs.readFileSync(path.join(__dirname, '../../web/static/js/ai-conversation-view.js'), 'utf8');
    source = source.replace(
        '    const View = {',
        '    window.__renderAssistantMarkdown = renderAssistantMarkdown;\n\n    const View = {'
    );
    const context = {
        console,
        localStorage: {
            getItem() { return null; },
            setItem() {},
        },
        window: {
            AIReader: {},
            location: { href: 'http://localhost/ai' },
        },
    };
    vm.createContext(context);
    vm.runInContext(source, context, { filename: 'ai-conversation-view.js' });
    return context.window.__renderAssistantMarkdown;
}

function loadResultCardsRenderer() {
    const source = fs.readFileSync(path.join(__dirname, '../../web/static/js/ai-result-cards.js'), 'utf8');
    const context = {
        console,
        window: {
            AIReader: {},
            location: { origin: 'http://localhost' },
        },
        URL,
    };
    vm.createContext(context);
    vm.runInContext(source, context, { filename: 'ai-result-cards.js' });
    return context.window.AIReader.resultCards.render;
}

test('assistant markdown renders pipe tables as table markup', () => {
    const render = loadAssistantMarkdownRenderer();
    const html = render([
        '根据结果：',
        '',
        '| 文章 | 证据 |',
        '|---|---|',
        '| **Xu et al.** | 出现 **ChIP-seq** [1] |',
        '| s41586 | Windows for ChIP-seq [2] |',
    ].join('\n'));

    assert.match(html, /<table class="ai-message-table">/);
    assert.match(html, /<th>文章<\/th>/);
    assert.match(html, /<td><strong>Xu et al\.<\/strong><\/td>/);
    assert.doesNotMatch(html, /<p>\| 文章 \| 证据 \|<\/p>/);
});

test('assistant markdown keeps truncated pipe rows inside the preceding table', () => {
    const render = loadAssistantMarkdownRenderer();
    const html = render([
        '| 文章 | 证据 |',
        '|---|---|',
        '| **Xu et al.** | 出现 **ChIP-seq** [1] |',
        '| **s41586** | 结果卡片中提到：“',
    ].join('\n'));

    assert.match(html, /<table class="ai-message-table">/);
    assert.match(html, /<td><strong>s41586<\/strong><\/td><td>结果卡片中提到：“<\/td>/);
    assert.doesNotMatch(html, /<p>\| <strong>s41586<\/strong>/);
});

test('assistant markdown handles nested bold italic table cells without raw markers', () => {
    const render = loadAssistantMarkdownRenderer();
    const html = render([
        '| 文章 | 证据 |',
        '|---|---|',
        '| **Xu et al., 2020, *ARID1A determines luminal identity...*** | ChIP-seq [1] |',
    ].join('\n'));

    assert.match(html, /<td><strong>Xu et al\., 2020, <em>ARID1A determines luminal identity\.\.\.<\/em><\/strong><\/td>/);
    assert.doesNotMatch(html, /\*\*Xu/);
});

test('result cards leave citation tokens unwrapped for single hydration', () => {
    const render = loadResultCardsRenderer();
    const html = render([{
        type: 'paper_hit',
        payload: {
            paper_id: 1,
            title: 'ChIP-seq paper',
            snippets: [{ text: 'Uses ChIP-seq data.', citation_index: 1 }],
        },
    }]);

    assert.match(html, /Uses ChIP-seq data\. \[1\]/);
    assert.doesNotMatch(html, /<sup>\[1\]<\/sup>/);
});

test('paper hit snippets highlight backend search terms safely', () => {
    const render = loadResultCardsRenderer();
    const html = render([{
        type: 'paper_hit',
        payload: {
            paper_id: 1,
            title: 'ChIP-seq paper',
            highlight_terms: ['ChIP-seq', '<unsafe>'],
            snippets: [{ text: 'Uses ChIP-seq and <unsafe> marker.', citation_index: 1 }],
        },
    }]);

    assert.match(html, /Uses <mark class="ai-result-highlight">ChIP-seq<\/mark> and <mark class="ai-result-highlight">&lt;unsafe&gt;<\/mark> marker\. \[1\]/);
    assert.doesNotMatch(html, /<unsafe>/);
    assert.doesNotMatch(html, /<sup>\[1\]<\/sup>/);
});

test('paper hit result cards render collapsed evidence by default', () => {
    const render = loadResultCardsRenderer();
    const html = render([{
        type: 'paper_hit',
        payload: {
            paper_id: 1,
            title: 'ChIP-seq paper',
            reason: 'Sub-Agent accepted this paper.',
            snippets: [{ text: 'Uses ChIP-seq data.', citation_index: 1 }],
        },
    }]);

    assert.match(html, /ai-result-card-paper/);
    assert.match(html, /data-ai-result-collapsible="paper"/);
    assert.match(html, /aria-expanded="false"/);
    assert.match(html, /class="ai-result-card-collapsible-body" hidden/);
    assert.match(html, /Sub-Agent accepted this paper\./);
    assert.match(html, /打开文献/);
});
