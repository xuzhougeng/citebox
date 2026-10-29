'use strict';

// Guards the design-token contract in docs/ui-style.md: colors, font sizes and
// breakpoints come from web/static/css/variables.css instead of literals, so
// warm, light and dark themes stay consistent.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const CSS_ROOT = path.join(__dirname, '..', '..', 'web', 'static', 'css');
const TOKEN_FILE = path.join(CSS_ROOT, 'variables.css');

function cssFiles(dir = CSS_ROOT) {
    return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) return cssFiles(full);
        return entry.name.endsWith('.css') && full !== TOKEN_FILE ? [full] : [];
    });
}

function stripComments(source) {
    return source.replace(/\/\*[\s\S]*?\*\//g, '');
}

function findAll(pattern, allow = () => false) {
    const hits = [];
    for (const file of cssFiles()) {
        const source = stripComments(fs.readFileSync(file, 'utf8'));
        for (const match of source.matchAll(pattern)) {
            if (allow(match[0])) continue;
            hits.push(`${path.relative(CSS_ROOT, file)}: ${match[0]}`);
        }
    }
    return hits;
}

test('component styles use color tokens instead of hex literals', () => {
    assert.deepEqual(findAll(/#[0-9a-fA-F]{3,8}\b/g), []);
});

test('component styles use RGB tokens instead of raw rgb()/rgba() triples', () => {
    // Pure black and white are allowed for scrims, image backdrops and
    // highlights that intentionally stay the same in every theme.
    const neutral = (value) => /rgba?\(\s*(0\s*,\s*0\s*,\s*0|255\s*,\s*255\s*,\s*255)\b/.test(value);
    assert.deepEqual(findAll(/rgba?\(\s*\d+\s*,\s*\d+\s*,\s*\d+[^)]*\)/g, neutral), []);
});

test('font sizes come from the type scale', () => {
    // Relative em sizes inside inline elements and fluid clamp() display sizes
    // are exempt; everything else must use a --text-* token.
    const allowed = (value) => {
        const size = value.replace(/^font-size:\s*/, '').replace(/\s*!important$/, '').trim();
        return /^var\(--text-[\w-]+\)$/.test(size) || /^[\d.]+em$/.test(size) || size.startsWith('clamp(') || size === 'inherit' || size === '0';
    };
    assert.deepEqual(findAll(/font-size:\s*[^;}]+/g, allowed), []);
});

test('media queries use the shared breakpoints', () => {
    const breakpoints = new Set(['560px', '720px', '960px', '1200px']);
    const hits = findAll(/@media[^{]*\((?:max|min)-width:\s*(\d+px)\)/g, (value) => {
        const width = value.match(/(\d+px)\)/)[1];
        return breakpoints.has(width) || breakpoints.has(`${Number.parseInt(width, 10) - 1}px`) || breakpoints.has(`${Number.parseInt(width, 10) + 1}px`);
    });
    assert.deepEqual(hits, []);
});
