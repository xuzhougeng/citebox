'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

test('figure notes export preserves filters and large identifiers in the download request', async () => {
    let requested;
    const context = { URLSearchParams, window: {}, t: (key, fallback) => fallback,
        fetch: async url => {
            requested = url;
            return { ok: true, status: 200, blob: async () => 'zip', headers: { get: () => 'attachment; filename="notes.zip"' } };
        }
    };
    vm.runInNewContext(fs.readFileSync(path.join(__dirname, '..', 'api.js'), 'utf8') + '\nglobalThis.api=API;', context);
    const result = await context.api.exportFigureNotes({ paper_id: '1777519479295165603', keyword: 'sample & control', tag_id: '', has_notes: '1', language: 'en' });
    const query = new URL(requested, 'http://localhost').searchParams;
    assert.equal(query.get('paper_id'), '1777519479295165603');
    assert.equal(query.get('keyword'), 'sample & control');
    assert.equal(query.get('has_notes'), '1');
    assert.equal(query.has('tag_id'), false);
    assert.equal(result.filename, 'notes.zip');
    assert.equal(result.blob, 'zip');
});
