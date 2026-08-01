'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const noteViewer = fs.readFileSync(path.join(__dirname, '..', 'note-viewer.js'), 'utf8');
const api = fs.readFileSync(path.join(__dirname, '..', 'api.js'), 'utf8');

test('figure note viewer exposes Save to Notion action', () => {
    assert.match(noteViewer, /data-note-action="save-notion"/);
    assert.match(noteViewer, /API\.saveFigureNoteToNotion/);
    assert.doesNotMatch(noteViewer, /openExternalURL\(result\.target_page_url\)/);
});

test('Notion figure-note API posts the current draft', () => {
    assert.match(api, /saveFigureNoteToNotion\(id, data\)/);
    assert.match(api, /\/notion\/figures\/\$\{id\}\/notes/);
});
