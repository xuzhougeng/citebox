'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');

let mod = {};
try {
    mod = require(path.join('..', 'research-errors.js'));
} catch (error) {
    mod = {};
}

const { parseResearchErrorResponse } = mod;

function t(_key, fallback) {
    return fallback;
}

test('parseResearchErrorResponse explains that the rate-limited request used an API key', () => {
    assert.equal(typeof parseResearchErrorResponse, 'function');
    const parsed = parseResearchErrorResponse(503, JSON.stringify({
        code: 'UNAVAILABLE',
        error: 'Semantic Scholar 限流，请稍后再试',
        used_api_key: true,
    }), t);
    assert.equal(parsed.rateLimited, true);
    assert.equal(parsed.usedAPIKey, true);
    assert.match(parsed.message, /已携带 API Key/);
});

test('parseResearchErrorResponse explains that the rate-limited request did not use an API key', () => {
    assert.equal(typeof parseResearchErrorResponse, 'function');
    const parsed = parseResearchErrorResponse(503, JSON.stringify({
        code: 'UNAVAILABLE',
        error: 'Semantic Scholar 限流，请稍后再试',
        used_api_key: false,
    }), t);
    assert.equal(parsed.rateLimited, true);
    assert.equal(parsed.usedAPIKey, false);
    assert.match(parsed.message, /未携带 API Key/);
});
