'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function loadFiguresPage() {
    const context = {
        window: { location: { search: '' } },
        document: { documentElement: { lang: 'en' }, getElementById: () => null, addEventListener() {} },
        URLSearchParams,
        console,
        t: (key, fallback) => fallback || key,
        Utils: { escapeHTML: (value) => String(value), showToast() {} },
        API: {}
    };
    vm.runInNewContext(
        fs.readFileSync(path.join(__dirname, '..', 'browser-pages.js'), 'utf8')
            + '\nglobalThis.page = FiguresPage; globalThis.normalizeTags = normalizeFigureTagIDs;',
        context
    );
    const page = context.page;
    page.state.filters.tag_ids = [];
    page.state.filters.tag_mode = 'all';
    page.tags = [{ id: 1, name: 'Heatmap' }, { id: 2, name: 'UMAP' }, { id: 3, name: 'Western blot' }];
    return { page, normalizeTags: context.normalizeTags };
}

test('normalizeFigureTagIDs accepts arrays, single ids, and comma lists', () => {
    const { normalizeTags } = loadFiguresPage();
    assert.deepEqual([...normalizeTags('3')], ['3']);
    assert.deepEqual([...normalizeTags('1, 2,,2')], ['1', '2']);
    assert.deepEqual([...normalizeTags([4, '5', ''])], ['4', '5']);
    assert.deepEqual([...normalizeTags('')], []);
    assert.deepEqual([...normalizeTags(undefined)], []);
});

test('buildFigureParams joins multiple tag ids and omits tag_mode for match all', () => {
    const { page } = loadFiguresPage();
    page.state.filters.tag_ids = ['1', '2'];
    const params = page.buildFigureParams(2);
    assert.equal(params.page, 2);
    assert.equal(params.tag_id, '1,2');
    assert.equal('tag_mode' in params, false);
});

test('buildFigureParams sends tag_mode=any only with two or more tags', () => {
    const { page } = loadFiguresPage();
    page.state.filters.tag_ids = ['1', '3'];
    page.state.filters.tag_mode = 'any';
    assert.equal(page.buildFigureParams(1).tag_mode, 'any');

    page.state.filters.tag_ids = ['1'];
    const single = page.buildFigureParams(1);
    assert.equal(single.tag_id, '1');
    assert.equal('tag_mode' in single, false);

    page.state.filters.tag_ids = [];
    const none = page.buildFigureParams(1);
    assert.equal(none.tag_id, '');
    assert.equal('tag_mode' in none, false);
});

test('notes export filter params carry the same tag scope without paging', () => {
    const { page } = loadFiguresPage();
    page.state.filters.tag_ids = ['2', '3'];
    page.state.filters.tag_mode = 'any';
    const params = page.buildFilterParams();
    assert.equal(params.tag_id, '2,3');
    assert.equal(params.tag_mode, 'any');
    assert.equal('tag_ids' in params, false);
    assert.equal('page' in params, false);
});

test('setTagSelection maps an incoming single tag or comma list into tag_ids', () => {
    const { page } = loadFiguresPage();
    page.setTagSelection('7');
    assert.deepEqual([...page.state.filters.tag_ids], ['7']);
    assert.equal(page.state.filters.tag_mode, 'all');

    page.setTagSelection('1,2', 'any');
    assert.deepEqual([...page.state.filters.tag_ids], ['1', '2']);
    assert.equal(page.state.filters.tag_mode, 'any');

    page.setTagSelection('1', 'any');
    assert.equal(page.state.filters.tag_mode, 'all');
});

test('renderSummary lists selected tag names and the match-any mode', () => {
    const { page } = loadFiguresPage();
    page.state.filters.keyword = '';
    page.state.filters.group_id = '';
    page.state.filters.figure_type = '';
    page.state.filters.tag_ids = ['1', '2'];
    assert.match(page.renderSummary(5), /图片标签：Heatmap、UMAP</);

    page.state.filters.tag_mode = 'any';
    assert.match(page.renderSummary(5), /图片标签：Heatmap、UMAP（任一满足）</);
});
