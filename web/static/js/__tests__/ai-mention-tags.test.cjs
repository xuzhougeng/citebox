'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');

const mod = require(path.join('..', 'ai-mention-tags.js'));
const { parseToolTags, commitToolTag, KNOWN_TOOL_TAGS, familyOf } = mod;

test('KNOWN_TOOL_TAGS lists the four MVP tags in canonical case', () => {
    assert.deepEqual(
        KNOWN_TOOL_TAGS.map((t) => t.name),
        ['PubMed', 'SemanticScholar', 'Library', 'Figure']
    );
});

test('familyOf maps each tag to its routing family', () => {
    assert.equal(familyOf('PubMed'), 'external');
    assert.equal(familyOf('SemanticScholar'), 'external');
    assert.equal(familyOf('Library'), 'library');
    assert.equal(familyOf('Figure'), 'figure');
    assert.equal(familyOf('Unknown'), null);
});

test('parseToolTags returns empty result for plain text', () => {
    const r = parseToolTags('帮我查找 ATAC 文章');
    assert.equal(r.intentHint, '');
    assert.deepEqual(r.sources, []);
    assert.equal(r.conflict, null);
});

test('parseToolTags treats a single @PubMed as external_search + pubmed', () => {
    const r = parseToolTags('@PubMed 找综述');
    assert.equal(r.intentHint, 'external_search');
    assert.deepEqual(r.sources, ['pubmed']);
    assert.equal(r.conflict, null);
});

test('parseToolTags is case-insensitive', () => {
    const r = parseToolTags('@pubmed @SEMANTICSCHOLAR foo');
    assert.equal(r.intentHint, 'external_search');
    assert.deepEqual(r.sources, ['pubmed', 'semantic_scholar']);
});

test('parseToolTags stacks same-family tags and dedupes', () => {
    const r = parseToolTags('@PubMed @SemanticScholar @PubMed foo');
    assert.equal(r.intentHint, 'external_search');
    assert.deepEqual(r.sources.sort(), ['pubmed', 'semantic_scholar']);
});

test('parseToolTags applies cross-family last-wins (figure beats earlier external)', () => {
    const r = parseToolTags('@PubMed @Figure 找细胞图');
    assert.equal(r.intentHint, 'figure_lookup');
    assert.deepEqual(r.sources, []);
    assert.deepEqual(r.conflict, { dropped: 'external', kept: 'figure' });
});

test('parseToolTags applies cross-family last-wins (library beats earlier figure)', () => {
    const r = parseToolTags('@Figure @Library foo');
    assert.equal(r.intentHint, 'library_search');
    assert.deepEqual(r.sources, []);
    assert.deepEqual(r.conflict, { dropped: 'figure', kept: 'library' });
});

test('parseToolTags requires a word boundary so @PubMedTutorial is NOT a match', () => {
    const r = parseToolTags('@PubMedTutorial 教程');
    assert.equal(r.intentHint, '');
    assert.deepEqual(r.sources, []);
});

test('parseToolTags requires whitespace or BOL before @ so emails do not trip', () => {
    const r = parseToolTags('contact me at user@PubMed.com');
    assert.equal(r.intentHint, '');
    assert.deepEqual(r.sources, []);
});

test('parseToolTags ignores unknown @Foo tags', () => {
    const r = parseToolTags('@Foo @PubMed bar');
    assert.equal(r.intentHint, 'external_search');
    assert.deepEqual(r.sources, ['pubmed']);
});

test('parseToolTags preserves user-typed text including the tags', () => {
    // Caller does not rely on parseToolTags to strip — it only reports.
    const r = parseToolTags('@Library please summarize');
    assert.equal(r.intentHint, 'library_search');
});

test('commitToolTag with empty input inserts the tag at caret', () => {
    const r = commitToolTag({ value: '', selectionStart: 0, atIndex: 0, query: '' }, 'PubMed');
    assert.equal(r.value, '@PubMed ');
    assert.equal(r.caret, '@PubMed '.length);
});

test('commitToolTag replaces partial query after @ with full tag', () => {
    // input simulates user typing "@pub" with caret right after, atIndex=0, query="pub"
    const r = commitToolTag(
        { value: '@pub', selectionStart: 4, atIndex: 0, query: 'pub' },
        'PubMed'
    );
    assert.equal(r.value, '@PubMed ');
    assert.equal(r.caret, '@PubMed '.length);
});

test('commitToolTag stacking same-family preserves earlier @PubMed when adding @SemanticScholar', () => {
    const r = commitToolTag(
        { value: '@PubMed @sem', selectionStart: '@PubMed @sem'.length, atIndex: 8, query: 'sem' },
        'SemanticScholar'
    );
    assert.equal(r.value, '@PubMed @SemanticScholar ');
});

test('commitToolTag cross-family removes existing tags from a different family', () => {
    // user previously committed @PubMed; now picks @Library — @PubMed must be removed first
    const r = commitToolTag(
        { value: '@PubMed @lib', selectionStart: '@PubMed @lib'.length, atIndex: 8, query: 'lib' },
        'Library'
    );
    assert.equal(r.value, '@Library ');
    assert.equal(r.caret, '@Library '.length);
});

test('commitToolTag cross-family preserves non-tool text and @paper mentions', () => {
    const r = commitToolTag(
        {
            value: '@PubMed @AlphaFold 这篇 @lib',
            selectionStart: '@PubMed @AlphaFold 这篇 @lib'.length,
            atIndex: '@PubMed @AlphaFold 这篇 '.length,
            query: 'lib',
        },
        'Library'
    );
    // Only the @PubMed tool tag is removed; @AlphaFold (a paper mention) is preserved.
    assert.equal(r.value, '@AlphaFold 这篇 @Library ');
});
