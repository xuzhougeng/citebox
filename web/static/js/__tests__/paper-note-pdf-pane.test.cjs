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
