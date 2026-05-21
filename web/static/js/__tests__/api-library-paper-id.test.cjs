'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const apiModulePath = path.resolve(__dirname, '..', 'api.js');
const libraryModulePath = path.resolve(__dirname, '..', 'library.js');

function loadAPI() {
    const requests = [];
    const code = `${fs.readFileSync(apiModulePath, 'utf8')}\nglobalThis.__TEST_API__ = API;`;
    const context = {
        console,
        URL,
        URLSearchParams,
        fetch(path) {
            requests.push(path);
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
    return { API: context.__TEST_API__, requests };
}

function loadLibraryWithSearch(search) {
    const code = `${fs.readFileSync(libraryModulePath, 'utf8')}\nglobalThis.__TEST_LIBRARY__ = LibraryPage;`;
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
        t(key, fallback) {
            return fallback || key;
        },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: libraryModulePath });
    return context.__TEST_LIBRARY__;
}

test('API paper paths preserve large string identifiers', async () => {
    const { API, requests } = loadAPI();
    const paperId = '1777519479295165603';

    await API.getPaper(paperId);
    await API.reextractPaper(paperId);

    assert.equal(requests[0], `/api/papers/${paperId}`);
    assert.equal(requests[1], `/api/papers/${paperId}/reextract`);
});

test('library launch state preserves large paper identifiers as strings', () => {
    const paperId = '1777519479295165603';
    const LibraryPage = loadLibraryWithSearch(`?paper_id=${paperId}&from=duplicate`);

    LibraryPage.readLaunchState();

    assert.equal(LibraryPage.launchState.paperId, paperId);
    assert.equal(LibraryPage.launchState.fromDuplicate, true);
});
