const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const root = path.join(__dirname, '..', '..');

function read(relativePath) {
    return fs.readFileSync(path.join(root, relativePath), 'utf8');
}

const secretNames = [
    'ESIGNER_CREDENTIAL_ID',
    'ESIGNER_USERNAME',
    'ESIGNER_PASSWORD',
    'ESIGNER_TOTP_SECRET',
    'ESIGNER_PIN',
];

test('tag releases require eSigner secrets and sign before packaging', () => {
    const workflow = read('.github/workflows/release-packages.yml');
    const server = read('scripts/package-windows.ps1');
    const desktop = read('scripts/package-desktop-windows.ps1');
    const signer = read('scripts/sign-windows-authenticode.ps1');
    const docs = read('docs/windows-code-signing.md');

    for (const name of secretNames) {
        assert.match(workflow, new RegExp(`secrets\\.${name}\\b`));
        assert.match(signer, new RegExp(`"${name}"`));
        assert.match(docs, new RegExp('`' + name + '`'));
    }

    assert.match(workflow, /CITEBOX_REQUIRE_WINDOWS_SIGNING:/);
    assert.match(workflow, /github\.event_name == 'push'/);
    assert.doesNotMatch(workflow, /continue-on-error/);
    assert.match(workflow, /actions\/setup-java@v4/);
    assert.match(signer, /http:\/\/ts\.ssl\.com/);
    assert.match(signer, /Refusing to Authenticode-sign archive/);
    assert.match(docs, /SmartScreen/);
    assert.match(docs, /Get-AuthenticodeSignature/);
    assert.match(docs, /signtool verify \/pa \/v/);

    function indexes(source, needle) {
        const found = [];
        let from = 0;
        while (from < source.length) {
            const at = source.indexOf(needle, from);
            if (at < 0) {
                break;
            }
            found.push(at);
            from = at + needle.length;
        }
        return found;
    }

    const serverCalls = indexes(server, 'Invoke-CiteboxWindowsSign -Files');
    assert.equal(serverCalls.length, 1);
    assert.ok(serverCalls[0] < server.indexOf('Compress-Archive'));

    const desktopCalls = indexes(desktop, 'Invoke-CiteboxWindowsSign -Files');
    const makensis = desktop.indexOf('& $makensis');
    assert.equal(desktopCalls.length, 2);
    assert.ok(desktopCalls[0] < makensis && makensis < desktopCalls[1]);
    assert.equal(signer.includes('ESIGNER_PASSWORD="'), false);
    assert.equal(signer.includes('ESIGNER_PIN="'), false);
});
