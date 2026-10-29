'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
function setup() {
    const replacements = [];
    const fresh = {};
    const template = { childNodes: [], querySelector: selector => fresh[selector] ||= { selector } };
    const existing = { querySelector: selector => ({ replaceWith: node => replacements.push({ selector, node }) }) };
    const context = { URL, window: { location: { origin: 'http://localhost', href: 'http://localhost/library' } },
        document: { createElement: () => template }, t: (key, fallback) => fallback,
        Utils: { escapeHTML: value => String(value || ''), renderMarkdown: value => value, formatDate: () => '', resourceViewerURL: () => '/viewer?kind=pdf&src=%2Ffiles%2Fpapers%2Fp.pdf' } };
    vm.runInNewContext(fs.readFileSync(path.join(__dirname, '..', 'paper-viewer.js'), 'utf8') + '\nglobalThis.viewer=PaperNoteViewer; PaperViewer.renderTagChips=()=>"";', context);
    const viewer = context.viewer;
    viewer.paper = { id: 1, title: 'Paper', pdf_url: '/files/papers/p.pdf' };
    viewer.pdfPaperID = 1;
    viewer.showPDF = true;
    viewer.noteDraft = 'Unsaved observations';
    viewer.body = { querySelector: () => existing, replaceChildren: () => assert.fail('must retain the existing iframe browsing context') };
    return { viewer, replacements, template };
}
test('switching note modes updates only note sections and preserves the live PDF', () => {
    const { viewer, replacements, template } = setup();
    viewer.noteMode = 'preview';
    viewer.render();
    assert.deepEqual(replacements.map(item => item.selector), ['.note-lightbox-main', '.note-lightbox-side']);
    assert.match(template.innerHTML, /Unsaved observations/);
    assert.match(template.innerHTML, /embed=1/);
});
test('embedded reader ignores close navigation', () => {
    const context = { window: {}, document: { addEventListener() {} } };
    vm.runInNewContext(fs.readFileSync(path.join(__dirname, '..', 'viewer.js'), 'utf8') + '\nglobalThis.viewer=ResourceViewerPage;', context);
    const view = context.viewer;
    view.embedded = true;
    view.navigateBack = () => assert.fail('embedded PDF must not navigate away from notes');
    view.destroyPDFState = () => assert.fail('Escape must not unload an embedded PDF');
    view.close();
});
function renderPaperNote(paper, noteMode = 'write') {
    const { viewer, template } = setup();
    viewer.showPDF = false;
    viewer.pdfPaperID = null;
    viewer.paper = { id: 1, title: 'Paper', pdf_url: '/files/papers/p.pdf', original_filename: 'scan_0042.pdf', ...paper };
    viewer.noteMode = noteMode;
    viewer.body = { querySelector: () => null, replaceChildren() {} };
    viewer.bindPDFDivider = () => {};
    viewer.render();
    return template.innerHTML;
}
test('paper note header shows citation byline and a segmented mode switch', () => {
    const html = renderPaperNote({ authors_text: 'A. Author, B. Author', journal: 'Journal X', published_at: '2023-05-01' }, 'preview');
    assert.match(html, /A\. Author, B\. Author · Journal X · 2023/);
    assert.doesNotMatch(html, /scan_0042\.pdf/);
    assert.doesNotMatch(html, /Paper Notes/);
    assert.match(html, /class="segmented-control note-mode-toggle"/);
    assert.match(html, /data-note-mode="preview" aria-pressed="true"/);
    assert.match(html, /data-note-mode="write" aria-pressed="false"/);
    assert.equal((html.match(/btn-primary/g) || []).length, 1, 'only the save action is primary');
    assert.match(html, /btn-primary" type="button" data-paper-note-action="save-notes"/);
});
test('paper note header falls back to the file name without citation metadata', () => {
    const html = renderPaperNote({});
    assert.match(html, /note-lightbox-byline[^>]*>scan_0042\.pdf</);
});
function loadResourceViewer(search) {
    const context = { URL, URLSearchParams, t: (key, fallback) => fallback,
        window: { location: { search, origin: 'http://localhost', href: `http://localhost/viewer${search}` } },
        document: { addEventListener() {} } };
    vm.runInNewContext(fs.readFileSync(path.join(__dirname, '..', 'viewer.js'), 'utf8') + '\nglobalThis.viewer=ResourceViewerPage;', context);
    const view = context.viewer;
    const retry = { addEventListener(type, handler) { this.handler = handler; } };
    view.stage = { innerHTML: '', querySelector: selector => (selector === '[data-viewer-retry]' ? retry : null) };
    return { view, retry };
}
test('resource load failures show an actionable message and keep technical detail collapsed', () => {
    const { view, retry } = loadResourceViewer('?kind=pdf&src=%2Ffiles%2Fpapers%2Fp.pdf');
    let rendered = 0;
    view.render = () => { rendered += 1; };
    view.renderResourceError(new Error('Importing a module script failed.'));
    assert.match(view.stage.innerHTML, /请重试，或直接打开原文件/);
    assert.match(view.stage.innerHTML, /<details class="viewer-empty-detail">[\s\S]*Importing a module script failed\.[\s\S]*<\/details>/);
    assert.doesNotMatch(view.stage.innerHTML, /<p>Importing a module/);
    assert.match(view.stage.innerHTML, /href="http:\/\/localhost\/files\/papers\/p\.pdf" target="_blank"/);
    retry.handler();
    assert.equal(rendered, 1);
});
test('invalid resource addresses show the localized validation message without actions', () => {
    const { view } = loadResourceViewer('?kind=pdf');
    view.renderResourceError(new Error('缺少资源地址。'));
    assert.match(view.stage.innerHTML, /<p>缺少资源地址。<\/p>/);
    assert.doesNotMatch(view.stage.innerHTML, /data-viewer-retry|<details/);
});
