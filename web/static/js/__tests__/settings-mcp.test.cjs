'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const html = fs.readFileSync(path.join(__dirname, '..', '..', '..', 'settings.html'), 'utf8');

test('settings page exposes Notion MCP OAuth controls inside the Notion section', () => {
    assert.match(html, /id="mcpSettingsForm"/);
    assert.match(html, /id="mcpURLInput"/);
    assert.match(html, /id="testMCPButton"/);
    assert.match(html, /<option value="oauth">OAuth<\/option>/);
    assert.match(html, />Notion<\/h2>/);
    assert.match(html, />Notion MCP<\/h3>/);
    assert.match(html, /@Notion/);
});

test('settings page exposes a separate Notion API token workflow for native exports', () => {
    assert.match(html, /id="notionAPISettingsForm"/);
    assert.match(html, /id="notionAPITokenInput"[^>]*type="password"/);
    assert.match(html, /id="testNotionAPITokenButton"/);
    assert.match(html, /id="saveNotionAPITokenButton"/);
    assert.match(html, /id="removeNotionAPITokenButton"/);
    assert.match(html, /Notion File Upload API/);
});

test('settings page does not expose a dedicated Notion connection type', () => {
    assert.doesNotMatch(html, /<option[^>]*value="notion"/i);
    assert.doesNotMatch(html, />\s*连接 Notion\s*</i);
});
