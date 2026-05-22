'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const apiModulePath = path.resolve(__dirname, '..', 'api.js');
const libraryModulePath = path.resolve(__dirname, '..', 'library.js');
const paperViewerModulePath = path.resolve(__dirname, '..', 'paper-viewer.js');

function loadAPI() {
    const requests = [];
    const requestOptions = [];
    const code = `${fs.readFileSync(apiModulePath, 'utf8')}\nglobalThis.__TEST_API__ = API;`;
    const context = {
        console,
        URL,
        URLSearchParams,
        fetch(path, options = {}) {
            requests.push(path);
            requestOptions.push(options);
            return Promise.resolve({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ ok: true }),
            });
        },
        window: {
            location: {
                href: 'http://localhost/library',
                pathname: '/library',
            },
        },
        localStorage: {
            removeItem() {},
        },
        sessionStorage: {
            removeItem() {},
        },
        t(key, fallback) {
            return fallback || key;
        },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: apiModulePath });
    return { API: context.__TEST_API__, requests, requestOptions };
}

function loadLibraryContext(search) {
    const code = `${fs.readFileSync(libraryModulePath, 'utf8')}\nglobalThis.__TEST_LIBRARY__ = LibraryPage;`;
    const viewerURLs = [];
    const openedViewerURLs = [];
    const context = {
        console,
        URL,
        URLSearchParams,
        window: {
            location: {
                href: `http://localhost/library${search}`,
                search,
            },
            localStorage: {
                getItem() { return null; },
                setItem() {},
            },
        },
        document: {
            addEventListener() {},
        },
        Utils: {
            resourceViewerURL(kind, src, back, options = {}) {
                viewerURLs.push({ kind, src, back, options });
                const params = new URLSearchParams({ kind, src });
                if (back) params.set('back', back);
                if (options.paperId) params.set('paper_id', options.paperId);
                return `/viewer?${params.toString()}`;
            },
            openResourceViewer(kind, src, back, options = {}) {
                openedViewerURLs.push({ kind, src, back, options });
                const href = this.resourceViewerURL(kind, src, back, options);
                context.window.location.href = href;
                return href;
            },
            showToast() {},
        },
        t(key, fallback) {
            return fallback || key;
        },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: libraryModulePath });
    return { LibraryPage: context.__TEST_LIBRARY__, context, viewerURLs, openedViewerURLs };
}

function loadLibraryWithSearch(search) {
    return loadLibraryContext(search).LibraryPage;
}

function loadPaperViewerContext(api) {
    const code = `${fs.readFileSync(paperViewerModulePath, 'utf8')}\nglobalThis.__TEST_PAPER_VIEWER__ = PaperViewer;\nglobalThis.__TEST_PAPER_NOTE_VIEWER__ = PaperNoteViewer;`;
    const createTestElement = () => ({
        className: '',
        id: '',
        innerHTML: '',
        classList: {
            add() {},
            contains() { return false; },
            remove() {},
        },
        addEventListener() {},
        querySelector() { return createTestElement(); },
    });
    const modal = createTestElement();
    const body = createTestElement();
    const context = {
        console,
        requestAnimationFrame(callback) {
            if (typeof callback === 'function') callback();
        },
        window: {
            location: {
                href: 'http://localhost/library',
            },
            scrollX: 0,
            scrollY: 0,
            scrollTo() {},
            t(key, fallback) { return fallback || key; },
            open() {},
        },
        document: {
            body: {
                appendChild() {},
                classList: {
                    add() {},
                    remove() {},
                },
            },
            addEventListener() {},
            createElement() {
                return createTestElement();
            },
            getElementById(id) {
                if (id === 'paperModal') return modal;
                if (id === 'paperModalBody') return body;
                if (id === 'closePaperModal') return { addEventListener() {} };
                return null;
            },
            querySelector() { return null; },
        },
        API: api,
        Utils: {
            bindCommaSeparatedTagInputAutocomplete() { return null; },
            escapeHTML(value) { return String(value ?? ''); },
            formatDate(value) { return String(value || ''); },
            formatFileSize(value) { return `${value}`; },
            formatPartialDate(value) { return String(value || ''); },
            isProcessingStatus() { return false; },
            joinTags() { return ''; },
            mergeScopedTagCatalog() {},
            renderMarkdown(value) { return String(value || ''); },
            resourceViewerURL(kind, src, back, options = {}) {
                const params = new URLSearchParams({ kind, src: src || '' });
                if (back) params.set('back', back);
                if (options.paperId) params.set('paper_id', options.paperId);
                return `/viewer?${params.toString()}`;
            },
            showToast() {},
            splitTags() { return []; },
            statusLabel(value) { return value || ''; },
            statusTone() { return 'default'; },
        },
        t(key, fallback) {
            return fallback || key;
        },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: paperViewerModulePath });
    return {
        PaperViewer: context.__TEST_PAPER_VIEWER__,
        PaperNoteViewer: context.__TEST_PAPER_NOTE_VIEWER__,
        context,
    };
}

function loadPaperViewerWithAPI(api) {
    return loadPaperViewerContext(api).PaperViewer;
}

test('API paper paths preserve large string identifiers', async () => {
    const { API, requests } = loadAPI();
    const paperId = '1777519479295165603';

    await API.getPaper(paperId);
    await API.reextractPaper(paperId);

    assert.equal(requests[0], `/api/papers/${paperId}`);
    assert.equal(requests[1], `/api/papers/${paperId}/reextract`);
});

test('API PDF annotation helpers use paper-scoped JSON routes', async () => {
    const { API, requests, requestOptions } = loadAPI();
    const payload = {
        type: 'highlight',
        quote_text: 'selected text',
        color: 'yellow',
        fragments: [{ page: 3, left: 0.12, top: 0.34, width: 0.28, height: 0.018 }],
    };

    await API.listPDFAnnotations('42');
    await API.createPDFAnnotation('42', payload);
    await API.deletePDFAnnotation('42', '11');

    assert.equal(requests[0], '/api/papers/42/pdf-annotations');
    assert.equal(requests[1], '/api/papers/42/pdf-annotations');
    assert.equal(requestOptions[1].method, 'POST');
    assert.equal(requestOptions[1].headers['Content-Type'], 'application/json');
    assert.deepEqual(JSON.parse(requestOptions[1].body), payload);
    assert.equal(requests[2], '/api/papers/42/pdf-annotations/11');
    assert.equal(requestOptions[2].method, 'DELETE');
});

test('global PDF annotation list API builds query string', async () => {
    const { API, requests } = loadAPI();

    await API.listPDFAnnotationsGlobal({
        query: 'immune',
        sort: 'created_desc',
        page: 2,
        page_size: 25,
    });

    assert.equal(requests[0], '/api/pdf-annotations?query=immune&sort=created_desc&page=2&page_size=25');
});

test('library launch state preserves large paper identifiers as strings', () => {
    const paperId = '1777519479295165603';
    const LibraryPage = loadLibraryWithSearch(`?paper_id=${paperId}&from=duplicate`);

    LibraryPage.readLaunchState();

    assert.equal(LibraryPage.launchState.paperId, paperId);
    assert.equal(LibraryPage.launchState.fromDuplicate, true);
});

test('library PDF opener preserves large paper identifiers as strings', () => {
    const paperId = '1777519479295165603';
    const { LibraryPage, context, openedViewerURLs } = loadLibraryContext('');

    LibraryPage.state = {
        papers: [
            {
                id: paperId,
                pdf_url: '/files/papers/paper.pdf',
            },
        ],
    };
    LibraryPage.openPaperPDF(paperId);

    assert.equal(openedViewerURLs[0].options.paperId, paperId);
    assert.equal(typeof openedViewerURLs[0].options.paperId, 'string');
    assert.match(context.window.location.href, new RegExp(`paper_id=${paperId}`));
});

test('paper detail viewer keeps requested large paper id after loading rounded JSON id', async () => {
    const paperId = '1777519479295165603';
    const requests = [];
    const PaperViewer = loadPaperViewerWithAPI({
        getPaper(id) {
            requests.push(id);
            return Promise.resolve({
                id: 1777519479295165700,
                title: 'Rounded response',
                original_filename: 'paper.pdf',
                extraction_status: 'completed',
                pdf_url: '/files/papers/paper.pdf',
                figures: [],
                tags: [],
            });
        },
        listGroups() {
            return Promise.resolve({ groups: [] });
        },
    });

    await PaperViewer.open(paperId);

    assert.deepEqual(requests, [paperId]);
    assert.equal(PaperViewer.paper.id, paperId);
    assert.equal(typeof PaperViewer.paper.id, 'string');
});

test('paper detail viewer keeps current large paper id when payloads contain rounded ids', () => {
    const paperId = '1777519479295165603';
    const PaperViewer = loadPaperViewerWithAPI({});
    const paper = PaperViewer.normalizePaperIdentity({ id: 1777519479295165700, title: 'Updated' }, paperId);

    assert.equal(paper.id, paperId);
    assert.equal(paper.title, 'Updated');
});

test('paper note PDF opener includes large paper identifier for reader detail link', () => {
    const paperId = '1777519479295165603';
    const { PaperNoteViewer } = loadPaperViewerContext({});

    PaperNoteViewer.open({
        paper: {
            id: paperId,
            title: 'Paper note',
            original_filename: 'paper.pdf',
            extraction_status: 'completed',
            pdf_url: '/files/papers/paper.pdf',
            tags: [],
        },
    });

    assert.match(PaperNoteViewer.body.innerHTML, new RegExp(`paper_id=${paperId}`));
});

test('paper detail viewer restores the library scroll position after closing', async () => {
    const paperId = '1777519479295165603';
    let restoredPosition = null;
    const { PaperViewer, context } = loadPaperViewerContext({
        getPaper() {
            return Promise.resolve({
                id: paperId,
                title: 'Scroll restoration',
                original_filename: 'paper.pdf',
                extraction_status: 'completed',
                pdf_url: '/files/papers/paper.pdf',
                figures: [],
                tags: [],
            });
        },
        listGroups() {
            return Promise.resolve({ groups: [] });
        },
    });
    context.window.scrollX = 12;
    context.window.scrollY = 2400;
    context.window.scrollTo = (x, y) => {
        restoredPosition = { x, y };
    };

    await PaperViewer.open(paperId);
    context.window.scrollY = 0;
    PaperViewer.close();

    assert.deepEqual(restoredPosition, { x: 12, y: 2400 });
});
