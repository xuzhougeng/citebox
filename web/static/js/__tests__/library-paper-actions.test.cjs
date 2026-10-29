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

function samplePaper(overrides = {}) {
    return {
        id: 7,
        title: 'ARID1A determines luminal identity',
        original_filename: 'Xu et al. - 2020.pdf',
        extraction_status: 'completed',
        extractor_message: 'auto extraction is not configured',
        authors_text: 'Ada Lovelace, Alan Turing, Grace Hopper, Barbara Liskov',
        journal: 'Nature Communications',
        published_at: '2020-03-04',
        group_name: '',
        figure_count: 12,
        created_at: '2026-05-01T08:00:00Z',
        updated_at: '2026-05-02T19:58:00Z',
        pdf_url: '/files/papers/xu.pdf',
        tags: [],
        ...overrides,
    };
}

test('library defaults to all statuses', () => {
    const page = loadLibraryPage();
    assert.equal(page.state.filters.status, '');
    assert.equal(page.DEFAULT_FILTERS.status, '');
});

test('library paper row opens details from the title button and reads PDF from action button', () => {
    const page = loadLibraryPage();
    const paperList = { innerHTML: '' };

    page.paperList = paperList;
    page.state.papers = [samplePaper()];

    page.renderPaperList();

    const html = paperList.innerHTML;
    assert.match(html, /<button type="button" class="paper-title-link" data-action="open" title="点击查看详情">ARID1A determines luminal identity<\/button>/);
    assert.match(html, /data-action="open-pdf">阅读 PDF<\/button>/);
    assert.doesNotMatch(html, /Xu et al\. - 2020\.pdf/, 'original filename belongs in the detail modal');
    assert.match(html, /Ada Lovelace, Alan Turing, Grace Hopper 等/);
    assert.match(html, /Nature Communications/);
    assert.match(html, /2020/);
});

test('library paper row hides completed status and its extractor notice', () => {
    const page = loadLibraryPage();
    const paperList = { innerHTML: '' };
    page.paperList = paperList;
    page.state.papers = [samplePaper()];

    page.renderPaperList();

    assert.doesNotMatch(paperList.innerHTML, /status-pill/);
    assert.doesNotMatch(paperList.innerHTML, /auto extraction is not configured/);
});

test('library paper row shows status and message for failed papers with re-extract in the menu', () => {
    const page = loadLibraryPage();
    const paperList = { innerHTML: '' };
    page.paperList = paperList;
    page.state.papers = [samplePaper({ extraction_status: 'failed', extractor_message: 'extractor timed out' })];

    page.renderPaperList();

    assert.match(paperList.innerHTML, /status-pill[^>]*>failed</);
    assert.match(paperList.innerHTML, /extractor timed out/);
    assert.match(paperList.innerHTML, /class="action-menu-item" data-action="reextract"/);
});

test('library delete lives in the overflow menu', () => {
    const page = loadLibraryPage();
    const paperList = { innerHTML: '' };
    page.paperList = paperList;
    page.state.papers = [samplePaper()];

    page.renderPaperList();

    assert.match(paperList.innerHTML, /<details class="action-menu no-caret">[\s\S]*class="action-menu-item danger" data-action="delete"[\s\S]*<\/details>/);
    assert.doesNotMatch(paperList.innerHTML, /class="btn[^"]*" type="button" data-action="delete"/);
});

test('library compact view renders a table with clickable rows', () => {
    const page = loadLibraryPage();
    const paperList = { innerHTML: '' };
    page.paperList = paperList;
    page.state.viewMode = 'compact';
    page.state.papers = [samplePaper()];

    page.renderPaperList();

    assert.match(paperList.innerHTML, /<table class="paper-table">/);
    assert.match(paperList.innerHTML, /<tr class="paper-table-row" data-paper-id="7" data-row-open>/);
    assert.match(paperList.innerHTML, /aria-sort="descending"/);
    assert.match(paperList.innerHTML, /data-action="open"/);
});

test('library formats authors and years with graceful fallbacks', () => {
    const page = loadLibraryPage();
    assert.equal(page.formatAuthors(''), '');
    assert.equal(page.formatAuthors('Lovelace, Ada; Turing, Alan'), 'Lovelace, Ada; Turing, Alan');
    assert.equal(page.formatAuthors('A, B, C, D'), 'A, B, C 等');
    assert.equal(page.publishedYear('2023-01-18'), '2023');
    assert.equal(page.publishedYear(''), '');
    assert.equal(page.paperMetaLine({ title: 'x' }), '');
});

test('library lists only non-default filters as removable chips', () => {
    const page = loadLibraryPage();
    page.state.groups = [{ id: 3, name: 'Reading' }];
    page.state.filters.keyword = 'luminal';
    page.state.filters.group_id = '3';

    const chips = page.activeFilterChips();

    assert.deepEqual(Array.from(chips, (chip) => chip.key), ['keyword', 'group_id']);
    assert.match(chips[1].label, /Reading/);
});
