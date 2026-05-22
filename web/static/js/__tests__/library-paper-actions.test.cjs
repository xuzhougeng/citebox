const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

function loadLibraryPage() {
    const source = `${fs.readFileSync(path.join(__dirname, '../library.js'), 'utf8')}\nwindow.__LibraryPage = LibraryPage;`;
    const context = {
        console,
        window: {
            location: { href: 'http://localhost/library' },
            localStorage: {
                getItem() { return null; },
                setItem() {},
            },
        },
        Utils: {
            escapeHTML(value) {
                return String(value ?? '')
                    .replace(/&/g, '&amp;')
                    .replace(/</g, '&lt;')
                    .replace(/>/g, '&gt;')
                    .replace(/"/g, '&quot;')
                    .replace(/'/g, '&#039;');
            },
            statusLabel(value) {
                return value;
            },
            statusTone() {
                return 'success';
            },
            formatDate() {
                return '2026/05/02 19:58';
            },
        },
        t(key, fallback) {
            if (key === 'library.btn_read_pdf') return '阅读 PDF';
            if (key === 'library.meta_click_detail') return '点击查看详情';
            return fallback || key;
        },
    };
    context.window.t = context.t;
    vm.createContext(context);
    vm.runInContext(source, context, { filename: 'library.js' });
    return context.window.__LibraryPage;
}

test('library paper row opens details from file metadata and reads PDF from action button', () => {
    const page = loadLibraryPage();
    const paperList = { innerHTML: '' };

    page.paperList = paperList;
    page.state.papers = [{
        id: 7,
        title: 'ARID1A determines luminal identity',
        original_filename: 'Xu et al. - 2020.pdf',
        extraction_status: 'completed',
        group_name: '',
        figure_count: 12,
        updated_at: '2026-05-02T19:58:00Z',
        pdf_url: '/files/papers/xu.pdf',
        tags: [],
    }];

    page.renderPaperList();

    assert.match(paperList.innerHTML, /paper-list-meta-file" data-action="open"/);
    assert.doesNotMatch(paperList.innerHTML, /paper-list-meta-file" data-action="open-pdf"/);
    assert.match(paperList.innerHTML, /data-action="open-pdf">阅读 PDF<\/button>/);
    assert.match(paperList.innerHTML, /title="点击查看详情"/);
});
