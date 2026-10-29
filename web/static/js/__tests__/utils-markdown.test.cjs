'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'utils.js');

function loadUtils() {
    const code = fs.readFileSync(modulePath, 'utf8') + '\nglobalThis.__TEST_UTILS__ = Utils;';
    const context = {
        console: console,
        setTimeout,
        clearTimeout,
        URL,
        URLSearchParams,
        MutationObserver: class MutationObserver {
            observe() {}
            disconnect() {}
        },
        window: {
            location: {
                href: 'http://localhost/ai',
                origin: 'http://localhost',
            },
            history: {
                replaceState() {},
            },
            open() {},
        },
        document: {
            readyState: 'complete',
            body: {},
            addEventListener() {},
            querySelectorAll() {
                return [];
            },
        },
        t(key, fallback) {
            return fallback || key;
        },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: modulePath });
    return context.__TEST_UTILS__;
}

test('renderMarkdown renders pipe tables through the shared markdown entrypoint', () => {
    const Utils = loadUtils();
    const html = Utils.renderMarkdown([
        '| Name | Value |',
        '| --- | --- |',
        '| code | `x` |',
    ].join('\n'));

    assert.match(html, /<table/);
    assert.match(html, /<thead>/);
    assert.match(html, /<tbody>/);
    assert.match(html, /<code class="markdown-inline-code">x<\/code>/);
});

test('renderMarkdown handles nested bold italic text inside table cells', () => {
    const Utils = loadUtils();
    const html = Utils.renderMarkdown([
        '| 文章 | 证据 |',
        '|---|---|',
        '| **Xu et al., 2020, *ARID1A determines luminal identity...*** | ChIP-seq [1] |',
    ].join('\n'));

    assert.match(html, /<strong>Xu et al\., 2020, <em>ARID1A determines luminal identity\.\.\.<\/em><\/strong>/);
    assert.doesNotMatch(html, /\*\*Xu/);
});

const SAMPLE_NOTE = [
    '## Key points',
    '',
    '- Thermal response is explained by **two major loci**.',
    '- Effect sizes are larger at low temperature.',
    '',
    '> Follow up on the replication cohort.',
].join('\n');

test('renderMarkdown emits headings, lists and blockquotes without stray whitespace or breaks', () => {
    const Utils = loadUtils();
    const html = Utils.renderMarkdown(SAMPLE_NOTE);

    assert.equal(html, [
        '<h2 class="markdown-heading markdown-heading-2">Key points</h2>',
        '<ul class="markdown-list">',
        '<li><p class="markdown-paragraph">Thermal response is explained by <strong>two major loci</strong>.</p></li>',
        '<li><p class="markdown-paragraph">Effect sizes are larger at low temperature.</p></li>',
        '</ul>',
        '<blockquote class="markdown-blockquote"><p class="markdown-paragraph">Follow up on the replication cohort.</p></blockquote>',
    ].join(''));
    assert.doesNotMatch(html, /<br>/);
    assert.doesNotMatch(html, />\s+</);
});

test('renderMarkdown keeps code blocks, tables and figures free of template whitespace', () => {
    const Utils = loadUtils();
    const html = Utils.renderMarkdown([
        '```js',
        'const a = 1;',
        '```',
        '',
        '| A | B |',
        '| --- | --- |',
        '| 1 | 2 |',
        '',
        '![Fig 1](/files/figures/1.png)',
    ].join('\n'));

    assert.match(html, /<pre class="markdown-code-block"><code class="language-js">const a = 1;<\/code><\/pre>/);
    assert.match(html, /<table class="markdown-table">/);
    assert.match(html, /<figure class="markdown-figure"><img /);
    assert.doesNotMatch(html, />\s+</);
});

test('markdown preview grid does not stretch rows to fill its minimum height', () => {
    const css = fs.readFileSync(path.resolve(__dirname, '..', '..', 'css', 'features', 'markdown.css'), 'utf8');
    const rule = css.match(/\.markdown-preview\s*\{([^}]*)\}/);
    assert.ok(rule, 'expected a .markdown-preview rule');
    assert.match(rule[1], /align-content:\s*start/);
});

test('markdownToPlainText strips block and inline Markdown syntax for excerpts', () => {
    const Utils = loadUtils();

    assert.equal(
        Utils.markdownToPlainText(SAMPLE_NOTE),
        'Key points Thermal response is explained by two major loci. Effect sizes are larger at low temperature. Follow up on the replication cohort.'
    );
    assert.equal(
        Utils.markdownToPlainText([
            '1. See [docs](https://example.com) and ![Fig 2](/files/figures/2.png)',
            '- [x] done item',
            '---',
            '```python',
            'value = compute()',
            '```',
            '| Gene | Effect |',
            '| --- | --- |',
            '| `abc1` | *strong* |',
            'Keep snake_case_name, 2 * 3 * 4, ~~old~~ and __bold__ text.',
        ].join('\n')),
        'See docs and Fig 2 done item value = compute() Gene Effect abc1 strong Keep snake_case_name, 2 * 3 * 4, old and bold text.'
    );
});

test('markdownToPlainText handles empty input and truncates to maxLength', () => {
    const Utils = loadUtils();

    assert.equal(Utils.markdownToPlainText(''), '');
    assert.equal(Utils.markdownToPlainText(null), '');
    assert.equal(Utils.markdownToPlainText('# Title\n\nSome **long** body text', { maxLength: 10 }), 'Title Some…');
    assert.equal(Utils.markdownToPlainText('short', { maxLength: 10 }), 'short');
});
