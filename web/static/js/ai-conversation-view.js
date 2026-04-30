// ai-conversation-view.js — main pane: messages, streaming, pin/strict-evidence/export/delete actions.
// Public API: window.AIReader.view.{init, load, loadDraft, sendCurrentInput, stop, setStrictEvidence, rename}

(function () {
    'use strict';

    const View = {
        _state: {
            els: null,
            conversationId: null,    // null = draft
            meta: null,              // {title, strict_evidence, ...}
            pinnedPapers: [],
            messages: [],
            streaming: null,         // { abortController, assistantBubbleEl, accText: '', userBubbleEl }
        },

        init(els) {
            this._state.els = els;
            const self = this;
            if (els.runBtn) els.runBtn.addEventListener('click', function () { self.sendCurrentInput(); });
            if (els.stopBtn) els.stopBtn.addEventListener('click', function () { self.stop(); });
            if (els.questionInput) {
                els.questionInput.addEventListener('keydown', function (e) {
                    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
                        e.preventDefault();
                        self.sendCurrentInput();
                    }
                });
            }
            if (els.strictEvidence) {
                els.strictEvidence.addEventListener('change', function () { self.setStrictEvidence(els.strictEvidence.checked); });
            }
            if (els.exportBtn) {
                els.exportBtn.addEventListener('click', function () {
                    if (self._state.conversationId) {
                        window.location.href = '/api/ai/conversations/' + self._state.conversationId + '/export';
                    }
                });
            }
            if (els.deleteBtn) {
                els.deleteBtn.addEventListener('click', function () { self._handleDelete(); });
            }
        },

        async load(conversationId) {
            const s = this._state;
            s.conversationId = conversationId;
            const res = await fetch('/api/ai/conversations/' + conversationId);
            if (!res.ok) {
                this._showError('加载会话失败');
                return;
            }
            const conv = await res.json();
            s.meta = conv;
            s.pinnedPapers = conv.pinned_papers || [];
            s.messages = conv.recent_messages || [];
            this._renderAll();
        },

        loadDraft(prefilledPaperId) {
            const s = this._state;
            s.conversationId = null;
            s.meta = { title: '', strict_evidence: false, title_locked: false };
            s.pinnedPapers = [];
            s.messages = [];
            s._draftPaperId = prefilledPaperId || 0;
            this._renderAll();
        },

        async sendCurrentInput() {
            const s = this._state;
            if (s.streaming) return;
            if (!s.els || !s.els.questionInput) return;
            const content = s.els.questionInput.value.trim();
            if (!content) return;

            // optimistic user bubble
            const userBubble = this._appendMessageBubble({ role: 'user', content: content });
            const assistantBubble = this._appendMessageBubble({ role: 'assistant', content: '', streaming: true });

            const path = s.conversationId
                ? ('/api/ai/conversations/' + s.conversationId + '/messages')
                : '/api/ai/conversations/new/messages';
            const ctrl = new AbortController();
            s.streaming = { abortController: ctrl, userBubbleEl: userBubble, assistantBubbleEl: assistantBubble, accText: '' };

            const body = { content: content };
            if (!s.conversationId && s._draftPaperId) body.paper_id = s._draftPaperId;

            s.els.questionInput.value = '';
            this._toggleSendingState(true);

            try {
                const res = await fetch(path, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                    signal: ctrl.signal,
                });
                if (!res.ok) throw new Error('HTTP ' + res.status);
                await this._consumeNdjson(res.body, assistantBubble);
            } catch (e) {
                this._handleStreamError(e);
            } finally {
                this._toggleSendingState(false);
                s.streaming = null;
            }
        },

        stop() {
            const s = this._state;
            if (s.streaming && s.streaming.abortController) {
                s.streaming.abortController.abort();
            }
        },

        async setStrictEvidence(on) {
            const s = this._state;
            if (!s.conversationId) return;
            await fetch('/api/ai/conversations/' + s.conversationId, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ strict_evidence: !!on }),
            });
            if (s.meta) s.meta.strict_evidence = !!on;
        },

        async rename(newTitle) {
            const s = this._state;
            if (!s.conversationId) return;
            const t = (newTitle || '').trim();
            if (!t) return;
            await fetch('/api/ai/conversations/' + s.conversationId, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ title: t }),
            });
            if (s.meta) s.meta.title = t;
            if (s.els && s.els.title) s.els.title.textContent = t;
            document.dispatchEvent(new CustomEvent('ai-reader:conversation-changed'));
        },

        // private
        async _handleDelete() {
            const s = this._state;
            if (!s.conversationId) return;
            if (!window.confirm('确认删除当前会话？')) return;
            await fetch('/api/ai/conversations/' + s.conversationId, { method: 'DELETE' });
            s.conversationId = null;
            s.meta = null;
            s.pinnedPapers = [];
            s.messages = [];
            this._renderAll();
            document.dispatchEvent(new CustomEvent('ai-reader:conversation-changed'));
        },

        _renderAll() {
            // render title, strict-evidence toggle, pin chips (delegate to ai-pin.setPinned in 2.6),
            // message list (clear container, append each message bubble)
            const s = this._state;
            if (!s.els) return;
            const meta = s.meta || {};
            if (s.els.title) s.els.title.textContent = meta.title || '新对话';
            if (s.els.strictEvidence) s.els.strictEvidence.checked = !!meta.strict_evidence;
            if (window.AIReader && window.AIReader.pin && typeof window.AIReader.pin.setPinned === 'function') {
                window.AIReader.pin.setPinned(s.pinnedPapers);
            }
            if (s.els.conversation) {
                s.els.conversation.innerHTML = '';
                for (const m of s.messages) {
                    this._appendMessageBubble(m);
                }
            }
        },

        _appendMessageBubble(message) {
            // build a div, append to els.conversation, return ref
            const s = this._state;
            if (!s.els || !s.els.conversation) return null;
            const div = document.createElement('div');
            div.className = 'ai-message ai-message-' + (message.role === 'assistant' ? 'assistant' : 'user');
            div.textContent = message.content || '';
            if (message.streaming) div.classList.add('is-streaming');
            s.els.conversation.appendChild(div);
            s.els.conversation.scrollTop = s.els.conversation.scrollHeight;
            return div;
        },

        async _consumeNdjson(body, assistantBubble) {
            const reader = body.getReader();
            const decoder = new TextDecoder();
            let buf = '';
            while (true) {
                const { value, done } = await reader.read();
                if (done) break;
                buf += decoder.decode(value, { stream: true });
                let nl;
                while ((nl = buf.indexOf('\n')) >= 0) {
                    const line = buf.slice(0, nl).trim();
                    buf = buf.slice(nl + 1);
                    if (!line) continue;
                    let evt;
                    try { evt = JSON.parse(line); } catch (e) { continue; }
                    this._handleEvent(evt, assistantBubble);
                }
            }
        },

        _handleEvent(evt, assistantBubble) {
            const s = this._state;
            if (evt.type === 'meta' && !s.conversationId && evt.conversation_id) {
                s.conversationId = evt.conversation_id;
                try {
                    const url = new URL(window.location.href);
                    url.searchParams.set('conversation', String(evt.conversation_id));
                    url.searchParams.delete('paper_id');
                    history.replaceState({}, '', url.toString());
                } catch (e) { /* ignore */ }
                document.dispatchEvent(new CustomEvent('ai-reader:conversation-changed'));
            } else if (evt.type === 'delta' && evt.delta) {
                if (s.streaming) {
                    s.streaming.accText += evt.delta;
                    if (assistantBubble) assistantBubble.textContent = s.streaming.accText;
                    if (s.els && s.els.conversation) s.els.conversation.scrollTop = s.els.conversation.scrollHeight;
                }
            } else if (evt.type === 'final') {
                if (assistantBubble) {
                    assistantBubble.classList.remove('is-streaming');
                    if (evt.assistant_message && evt.assistant_message.content) {
                        assistantBubble.textContent = evt.assistant_message.content;
                    }
                    if (evt.assistant_message && evt.assistant_message.citations_json) {
                        document.dispatchEvent(new CustomEvent('ai-reader:message-rendered', {
                            detail: { element: assistantBubble, citations: evt.assistant_message.citations_json },
                        }));
                    }
                }
            } else if (evt.type === 'error') {
                if (assistantBubble) {
                    assistantBubble.classList.remove('is-streaming');
                    assistantBubble.textContent = '⚠ ' + (evt.error || evt.message || '生成失败');
                    assistantBubble.classList.add('ai-message-error');
                }
            }
        },

        _toggleSendingState(on) {
            const s = this._state;
            if (!s.els) return;
            if (s.els.runBtn) s.els.runBtn.disabled = !!on;
            if (s.els.stopBtn) s.els.stopBtn.hidden = !on;
        },

        _handleStreamError(err) {
            const s = this._state;
            if (s.streaming && s.streaming.assistantBubbleEl) {
                s.streaming.assistantBubbleEl.classList.remove('is-streaming');
                if (err && err.name === 'AbortError') {
                    s.streaming.assistantBubbleEl.textContent = (s.streaming.accText || '') + '\n[已停止]';
                    s.streaming.assistantBubbleEl.classList.add('ai-message-stopped');
                } else {
                    s.streaming.assistantBubbleEl.textContent = '⚠ ' + (err && err.message ? err.message : '生成失败');
                    s.streaming.assistantBubbleEl.classList.add('ai-message-error');
                }
            }
        },

        _showError(msg) {
            // simple console + try Utils.showToast if present
            console.error('[ai-view]', msg);
            if (typeof Utils !== 'undefined' && typeof Utils.showToast === 'function') {
                Utils.showToast(msg, 'error');
            }
        },
    };

    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.view = View;
    }
})();
