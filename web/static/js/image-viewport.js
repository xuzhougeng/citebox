// Image navigation shared by embedded reading surfaces. Scale uses native pixels.
const ImageViewport = class {
    constructor(stage, output, state = null) {
        this.stage = stage;
        this.image = stage.querySelector('img');
        this.output = output;
        this.state = state || { scale: 1, x: 0, y: 0, fit: true };
        this.events = new AbortController();
        const on = (name, handler, options = {}) => stage.addEventListener(name, handler, { ...options, signal: this.events.signal });
        on('wheel', event => {
            event.preventDefault();
            const rect = stage.getBoundingClientRect();
            this.zoom(event.deltaY < 0 ? 1.2 : 1 / 1.2, event.clientX - rect.left - rect.width / 2, event.clientY - rect.top - rect.height / 2);
        }, { passive: false });
        on('dblclick', () => this.state.fit ? this.zoom(2) : this.reset());
        on('pointerdown', event => {
            if (event.button !== 0 && event.button !== 1) return;
            event.preventDefault();
            this.drag = { id: event.pointerId, x: event.clientX, y: event.clientY, originX: this.state.x, originY: this.state.y };
            stage.setPointerCapture(event.pointerId);
        });
        on('pointermove', event => {
            if (this.drag?.id !== event.pointerId) return;
            this.state.x = this.drag.originX + event.clientX - this.drag.x;
            this.state.y = this.drag.originY + event.clientY - this.drag.y;
            this.apply();
        });
        const stop = () => { this.drag = null; };
        on('pointerup', stop);
        on('pointercancel', stop);
        on('lostpointercapture', stop);
        this.image.addEventListener('load', () => this.apply(), { signal: this.events.signal });
        this.resize = new ResizeObserver(() => this.apply());
        this.resize.observe(stage);
        this.apply();
    }
    zoom(factor, x = 0, y = 0) {
        const previous = this.state.scale;
        const scale = Math.max(0.05, Math.min(8, previous * factor));
        this.state = { scale, x: x - (x - this.state.x) * scale / previous, y: y - (y - this.state.y) * scale / previous, fit: false };
        this.apply();
    }
    reset() {
        this.state = { scale: 1, x: 0, y: 0, fit: true };
        this.apply();
    }
    apply() {
        const { stage, image, state } = this;
        if (!image.naturalWidth || !stage.clientWidth || !stage.clientHeight) return;
        if (state.fit) state.scale = Math.min(1, stage.clientWidth / image.naturalWidth, stage.clientHeight / image.naturalHeight);
        const maxX = Math.max(0, (image.naturalWidth * state.scale - stage.clientWidth) / 2);
        const maxY = Math.max(0, (image.naturalHeight * state.scale - stage.clientHeight) / 2);
        state.x = Math.max(-maxX, Math.min(maxX, state.x));
        state.y = Math.max(-maxY, Math.min(maxY, state.y));
        image.style.transform = `translate(-50%, -50%) translate(${state.x}px, ${state.y}px) scale(${state.scale})`;
        if (this.output) this.output.textContent = `${Math.round(state.scale * 100)}%`;
    }
    destroy() {
        this.events.abort();
        this.resize.disconnect();
        this.drag = null;
    }
};
