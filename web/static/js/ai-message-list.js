(function () {
    'use strict';

    const MessageList = {
        init(opts) {
            opts = opts || {};
            this.container = opts.container || null;
        },

        appendMessage(message) {
            if (!this.container) return null;
            const role = (message && message.role) || 'assistant';
            const bubble = document.createElement('div');
            bubble.className = 'ai-message ai-message-' + role;
            if (message && message.id) bubble.dataset.messageId = String(message.id);

            const content = document.createElement('div');
            content.className = 'ai-message-content';
            const text = (message && message.content) || '';
            content.textContent = text;
            bubble.appendChild(content);

            if (role === 'assistant' && text.trim()) {
                bubble.appendChild(this._buildTTSButton(text));
            }

            this.container.appendChild(bubble);
            this.scrollToBottom();
            return bubble;
        },

        _buildTTSButton(text) {
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'ai-message-tts-btn';
            btn.title = '朗读';
            btn.textContent = '🔊';
            btn.addEventListener('click', async () => {
                if (btn.dataset.busy === '1') return;
                btn.dataset.busy = '1';
                const original = btn.textContent;
                btn.textContent = '⏳';
                try {
                    const r = await fetch('/api/ai/tts', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ text }),
                    });
                    if (!r.ok) {
                        let msg = '朗读失败';
                        try { const j = await r.json(); if (j && j.message) msg = j.message; } catch (_) {}
                        btn.title = msg;
                        return;
                    }
                    const blob = await r.blob();
                    const url = URL.createObjectURL(blob);
                    const audio = new Audio(url);
                    audio.addEventListener('ended', () => URL.revokeObjectURL(url));
                    await audio.play();
                } finally {
                    btn.textContent = original;
                    btn.dataset.busy = '0';
                }
            });
            return btn;
        },

        scrollToBottom() {
            if (!this.container) return;
            this.container.scrollTop = this.container.scrollHeight;
        },
    };

    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.messageList = MessageList;
    }
})();
