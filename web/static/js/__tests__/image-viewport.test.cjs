'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
function setup() {
    const image = { naturalWidth: 1000, naturalHeight: 800, style: {}, addEventListener() {} };
    const listeners = {};
    const stage = { clientWidth: 500, clientHeight: 400, querySelector: () => image,
        addEventListener: (name, fn) => { listeners[name] = fn; }, setPointerCapture() {},
        getBoundingClientRect: () => ({ left: 0, top: 0, width: 500, height: 400 }) };
    const output = {};
    let disconnected = false;
    const context = { AbortController, ResizeObserver: class { observe() {} disconnect() { disconnected = true; } } };
    vm.runInNewContext(fs.readFileSync(path.join(__dirname, '..', 'image-viewport.js'), 'utf8') + '\nglobalThis.Viewport = ImageViewport;', context);
    const view = new context.Viewport(stage, output);
    return { view, image, stage, output, listeners, disconnected: () => disconnected };
}
test('fit, cursor-centred zoom and reset use native image scale', () => {
    const { view, output } = setup();
    assert.equal(view.state.scale, 0.5);
    view.zoom(2, 100, 50);
    assert.equal(view.state.scale, 1);
    assert.equal(view.state.x, -100);
    assert.equal(view.state.y, -50);
    assert.equal(output.textContent, '100%');
    view.reset();
    assert.equal(view.state.scale, 0.5);
    assert.equal(view.state.x, 0);
});
test('drag clamps image edges and ignores unrelated pointers', () => {
    const { view, listeners } = setup();
    view.zoom(2);
    listeners.pointerdown({ button: 0, pointerId: 1, clientX: 0, clientY: 0, preventDefault() {} });
    listeners.pointermove({ pointerId: 2, clientX: 999, clientY: 999 });
    assert.equal(view.state.x, 0);
    listeners.pointermove({ pointerId: 1, clientX: 999, clientY: 999 });
    assert.equal(view.state.x, 250);
    assert.equal(view.state.y, 200);
});
test('destroy releases event subscriptions and resize observer', () => {
    const { view, disconnected } = setup();
    view.destroy();
    assert.equal(view.events.signal.aborted, true);
    assert.equal(disconnected(), true);
});
