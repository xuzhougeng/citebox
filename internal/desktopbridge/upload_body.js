// Desktop webviews hand request bodies to the in-process Go handler
// (WebView2's WebResourceRequested on Windows, a custom URL scheme on macOS
// and Linux). Those paths drop the file-backed parts of a FormData body: the
// multipart envelope arrives intact but every file part is 0 bytes (seen with
// WebView2 on Windows; wailsapp/wails#6068 reports the same on macOS), so a PDF
// upload fails with "PDF 内容为空" (issue #73). In-memory bytes do arrive, so
// fetch calls whose FormData holds a File or Blob are re-encoded into one
// multipart byte array before they are sent.
(function(root) {
    'use strict';

    function hasBlobEntry(formData) {
        for (const [, value] of formData.entries()) {
            if (value instanceof Blob) {
                return true;
            }
        }
        return false;
    }

    // Same escaping the HTML spec uses for multipart/form-data names.
    function escapeName(value) {
        return String(value)
            .replace(/\r/g, '%0D')
            .replace(/\n/g, '%0A')
            .replace(/"/g, '%22');
    }

    function normalizeNewlines(value) {
        return String(value).replace(/\r\n|\r|\n/g, '\r\n');
    }

    function randomBoundary() {
        const bytes = new Uint8Array(16);
        if (root.crypto && typeof root.crypto.getRandomValues === 'function') {
            root.crypto.getRandomValues(bytes);
        } else {
            for (let i = 0; i < bytes.length; i += 1) {
                bytes[i] = Math.floor(Math.random() * 256);
            }
        }
        return '----CiteBoxFormBoundary' + Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
    }

    async function encodeMultipart(formData, boundary) {
        const encoder = new TextEncoder();
        const marker = boundary || randomBoundary();
        const chunks = [];

        for (const [name, value] of formData.entries()) {
            let head = '--' + marker + '\r\nContent-Disposition: form-data; name="' + escapeName(normalizeNewlines(name)) + '"';
            if (value instanceof Blob) {
                const filename = typeof value.name === 'string' ? value.name : 'blob';
                head += '; filename="' + escapeName(filename) + '"\r\n'
                    + 'Content-Type: ' + (value.type || 'application/octet-stream') + '\r\n\r\n';
                chunks.push(encoder.encode(head));
                chunks.push(new Uint8Array(await value.arrayBuffer()));
                chunks.push(encoder.encode('\r\n'));
            } else {
                chunks.push(encoder.encode(head + '\r\n\r\n' + normalizeNewlines(value) + '\r\n'));
            }
        }
        chunks.push(encoder.encode('--' + marker + '--\r\n'));

        const total = chunks.reduce((sum, chunk) => sum + chunk.byteLength, 0);
        const body = new Uint8Array(total);
        let offset = 0;
        for (const chunk of chunks) {
            body.set(chunk, offset);
            offset += chunk.byteLength;
        }
        return { body, contentType: 'multipart/form-data; boundary=' + marker };
    }

    async function materializeInit(init) {
        if (!init || typeof FormData === 'undefined' || !(init.body instanceof FormData) || !hasBlobEntry(init.body)) {
            return init;
        }
        const encoded = await encodeMultipart(init.body);
        const headers = new Headers(init.headers || undefined);
        headers.set('Content-Type', encoded.contentType);
        return Object.assign({}, init, { body: encoded.body, headers });
    }

    function wrapFetch(originalFetch) {
        return async function citeboxDesktopFetch(input, init) {
            return originalFetch(input, await materializeInit(init));
        };
    }

    const api = { encodeMultipart, materializeInit, wrapFetch };

    if (typeof module === 'object' && module.exports) {
        module.exports = api;
        return;
    }
    if (typeof root.fetch === 'function') {
        root.fetch = wrapFetch(root.fetch.bind(root));
    }
})(typeof window !== 'undefined' ? window : globalThis);
