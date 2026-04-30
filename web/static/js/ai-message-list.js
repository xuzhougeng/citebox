(function () {
    'use strict';

    const MessageList = {
        init(opts) {
            opts = opts || {};
            this.container = opts.container || null;
        },

        appendMessage(message) {
            if (!this.container) return null;
            const bubble = document.createElement('div');
            bubble.className = 'ai-message ai-message-' + ((message && message.role) || 'assistant');
            if (message && message.id) bubble.dataset.messageId = String(message.id);

            const content = document.createElement('div');
            content.className = 'ai-message-content';
            content.textContent = message && message.content || '';
            bubble.appendChild(content);

            this.container.appendChild(bubble);
            this.scrollToBottom();
            return bubble;
        },

        appendHTML(target, html) {
            if (!target || !html) return;
            target.insertAdjacentHTML('beforeend', html);
            this.scrollToBottom();
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
