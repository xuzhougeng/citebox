'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const scriptPath = path.resolve(__dirname, '..', '..', '..', '..', 'internal', 'desktopbridge', 'upload_body.js');

// Runs the script as the desktop webview does: a classic script with a window.
function installInFakeWindow() {
    const calls = [];
    const window = {
        crypto: globalThis.crypto,
        fetch: async (input, init) => {
            calls.push({ input, init });
            return { ok: true };
        }
    };
    vm.runInNewContext(fs.readFileSync(scriptPath, 'utf8'), {
        window,
        Blob,
        File,
        FormData,
        Headers,
        TextEncoder,
        Uint8Array,
        Array,
        Math,
        Object,
        String
    }, { filename: scriptPath });
    return { window, calls };
}

async function decode(init) {
    const response = new Response(init.body, {
        headers: { 'Content-Type': init.headers.get('Content-Type') }
    });
    return response.formData();
}

test('desktop fetch re-encodes FormData files into an in-memory multipart body', async () => {
    const { window, calls } = installInFakeWindow();
    const pdfBytes = new Uint8Array([0x25, 0x50, 0x44, 0x46, 0x2d, 0x31, 0x2e, 0x37, 0x0a, 0x00, 0xff]);
    const form = new FormData();
    form.append('pdf', new File([pdfBytes], '论文 "draft".pdf', { type: 'application/pdf' }));
    form.append('title', 'Line one\nLine two');
    form.append('extraction_mode', 'manual');

    await window.fetch('/api/papers', { method: 'POST', body: form, headers: { 'X-Test': '1' } });

    assert.equal(calls.length, 1);
    const { init } = calls[0];
    assert.ok(init.body instanceof Uint8Array, 'body is a byte array, not FormData');
    assert.match(init.headers.get('Content-Type'), /^multipart\/form-data; boundary=----CiteBoxFormBoundary[0-9a-f]{32}$/);
    assert.equal(init.headers.get('X-Test'), '1');
    assert.equal(init.method, 'POST');

    const decoded = await decode(init);
    const file = decoded.get('pdf');
    assert.equal(file.type, 'application/pdf');
    assert.equal(file.name, '论文 "draft".pdf');
    assert.ok(Buffer.from(init.body).toString('utf8').includes('filename="论文 %22draft%22.pdf"'), 'filename is escaped like a browser does');
    assert.deepEqual(new Uint8Array(await file.arrayBuffer()), pdfBytes);
    assert.equal(decoded.get('title'), 'Line one\r\nLine two');
    assert.equal(decoded.get('extraction_mode'), 'manual');
});

test('desktop fetch leaves requests without file parts unchanged', async () => {
    const { window, calls } = installInFakeWindow();
    const jsonInit = { method: 'POST', body: '{"a":1}' };
    const textForm = new FormData();
    textForm.append('q', 'value');
    const formInit = { method: 'POST', body: textForm };

    await window.fetch('/api/a', jsonInit);
    await window.fetch('/api/b', formInit);
    await window.fetch('/api/c');

    assert.equal(calls[0].init, jsonInit);
    assert.equal(calls[1].init, formInit);
    assert.equal(calls[2].init, undefined);
});
