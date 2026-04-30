// ai-conversation-view.js — main pane: messages, streaming, pin/strict-evidence/export/delete actions.
// Public API: window.AIReader.view.{init, load, loadDraft, sendCurrentInput, sendPayload, stop, setStrictEvidence, rename}

(function () {
    'use strict';

    const EXTERNAL_EVIDENCE_KEY = 'citebox_ai_external_evidence';

    function escapeHtml(value) {
        return String(value == null ? '' : value)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    function translate(key, fallback) {
        if (window.CiteBoxI18n && typeof window.CiteBoxI18n.t === 'function') {
            return window.CiteBoxI18n.t(key, fallback);
        }
        if (typeof t === 'function') {
            return t(key, fallback);
        }
        return fallback || key;
    }

    function inlineMarkdown(text) {
        return escapeHtml(text)
            .replace(/\*\*([^*\n]*?)\*([^*\n]+?)\*\*\*/g, '<strong>$1<em>$2</em></strong>')
            .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
            .replace(/(^|[^*])\*([^*]+)\*(?!\*)/g, '$1<em>$2</em>')
            .replace(/`([^`]+)`/g, '<code>$1</code>');
    }

    function isTableRow(line) {
        const trimmed = String(line || '').trim();
        return trimmed.startsWith('|') && trimmed.slice(1).includes('|');
    }

    function splitTableRow(line) {
        return String(line || '')
            .trim()
            .replace(/^\|/, '')
            .replace(/\|$/, '')
            .split('|')
            .map((cell) => cell.trim());
    }

    function isTableSeparator(line) {
        if (!isTableRow(line)) return false;
        const cells = splitTableRow(line);
        return cells.length > 1 && cells.every((cell) => /^:?-{3,}:?$/.test(cell));
    }

    function renderTable(lines, startIndex) {
        const headers = splitTableRow(lines[startIndex]);
        let index = startIndex + 2;
        const rows = [];
        while (index < lines.length && isTableRow(lines[index]) && !isTableSeparator(lines[index])) {
            rows.push(splitTableRow(lines[index]));
            index += 1;
        }
        const cellCount = headers.length;
        const renderCells = (cells, tag) => cells.slice(0, cellCount).map((cell) => (
            '<' + tag + '>' + inlineMarkdown(cell) + '</' + tag + '>'
        )).join('');
        const body = rows.map((cells) => (
            '<tr>' + renderCells(cells.concat(Array(cellCount).fill('')), 'td') + '</tr>'
        )).join('');
        return {
            html: '<div class="ai-message-table-wrap"><table class="ai-message-table"><thead><tr>' +
                renderCells(headers, 'th') +
                '</tr></thead><tbody>' +
                body +
                '</tbody></table></div>',
            nextIndex: index,
        };
    }

    function renderAssistantMarkdown(text) {
        const lines = String(text || '').split(/\r?\n/);
        const html = [];
        let listOpen = false;
        const closeList = () => {
            if (listOpen) {
                html.push('</ul>');
                listOpen = false;
            }
        };
        let index = 0;
        while (index < lines.length) {
            const line = lines[index];
            const trimmed = line.trim();
            if (!trimmed) {
                closeList();
                index += 1;
                continue;
            }
            if (isTableRow(trimmed) && index + 1 < lines.length && isTableSeparator(lines[index + 1])) {
                closeList();
                const table = renderTable(lines, index);
                html.push(table.html);
                index = table.nextIndex;
                continue;
            }
            const heading = trimmed.match(/^(#{1,4})\s+(.+)$/);
            if (heading) {
                closeList();
                html.push('<h3>' + inlineMarkdown(heading[2]) + '</h3>');
                index += 1;
                continue;
            }
            const item = trimmed.match(/^[-*]\s+(.+)$/);
            if (item) {
                if (!listOpen) {
                    html.push('<ul>');
                    listOpen = true;
                }
                html.push('<li>' + inlineMarkdown(item[1]) + '</li>');
                index += 1;
                continue;
            }
            closeList();
            html.push('<p>' + inlineMarkdown(trimmed) + '</p>');
            index += 1;
        }
        closeList();
        return html.join('');
    }

    const View = {
        _state: {
            els: null,
            conversationId: null,    // null = draft
            meta: null,              // {title, strict_evidence, ...}
            pinnedPapers: [],
            messages: [],
            turnRuns: [],
            pendingCitations: [],
            streaming: null,         // { abortController, assistantBubbleEl, accText: '', userBubbleEl }
        },

        init(els) {
            this._state.els = els;
            const self = this;
            if (els.stopBtn) els.stopBtn.addEventListener('click', function () { self.stop(); });
            if (els.questionInput) {
                els.questionInput.addEventListener('keydown', function (e) {
                    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
                        e.preventDefault();
                        const composer = window.AIReader && window.AIReader.composer;
                        if (composer && typeof composer.submit === 'function') {
                            composer.submit();
                        } else {
                            self.sendCurrentInput();
                        }
                    }
                });
            }
            if (els.strictEvidence) {
                els.strictEvidence.addEventListener('change', function () { self.setStrictEvidence(els.strictEvidence.checked); });
            }
            if (els.externalEvidence) {
                els.externalEvidence.checked = this._loadExternalEvidencePreference();
                els.externalEvidence.addEventListener('change', function () {
                    self._saveExternalEvidencePreference(els.externalEvidence.checked);
                    self._syncEvidenceControls();
                });
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
            s.turnRuns = conv.turn_runs || [];
            s.pendingCitations = [];
            s._draftPaperId = 0;
            this._renderAll();
        },

        loadDraft(prefilledPaperId) {
            const s = this._state;
            s.conversationId = null;
            s.meta = { title: '', strict_evidence: false, title_locked: false };
            s.pinnedPapers = [];
            s.messages = [];
            s.turnRuns = [];
            s.pendingCitations = [];
            s._draftPaperId = prefilledPaperId || 0;
            this._renderAll();
        },

        async sendCurrentInput() {
            const s = this._state;
            if (s.streaming) return;
            if (!s.els || !s.els.questionInput) return;
            const content = s.els.questionInput.value.trim();
            if (!content) return;
            await this.sendPayload({ content: content });
        },

        async sendPayload(payload) {
            const content = (payload && payload.content || '').trim();
            if (!content) return;
            const body = { content: content, context: this._currentContext() };
            if (payload && payload.intent_hint) body.intent_hint = payload.intent_hint;
            await this._sendBody(body);
        },

        async _sendBody(body) {
            const s = this._state;
            if (s.streaming) return;
            if (!body || !String(body.content || '').trim()) return;
            const content = String(body.content || '').trim();
            // optimistic user bubble
            const userBubble = this._appendMessageBubble({ role: 'user', content: content });
            const assistantBubble = this._appendMessageBubble({ role: 'assistant', content: '', streaming: true });

            const path = s.conversationId
                ? ('/api/ai/conversations/' + s.conversationId + '/messages')
                : '/api/ai/conversations/new/messages';
            const ctrl = new AbortController();
            s.pendingCitations = [];
            s.streaming = {
                abortController: ctrl,
                userBubbleEl: userBubble,
                assistantBubbleEl: assistantBubble,
                accText: '',
                process: null,
                cards: [],
                pendingCitations: [],
            };

            if (!s.conversationId && s._draftPaperId) body.paper_id = s._draftPaperId;
            if (s.els.strictEvidence) {
                body.strict_evidence = !!s.els.strictEvidence.checked;
            }
            if (s.els.externalEvidence) {
                body.include_external_evidence = !!s.els.externalEvidence.checked;
            }

            s.els.questionInput.value = '';
            s.els.questionInput.dispatchEvent(new Event('input', { bubbles: true }));
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
            this._syncEvidenceControls();
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
            if (typeof Utils === 'undefined' || typeof Utils.confirm !== 'function') return;
            const confirmed = await Utils.confirm(
                t('ai.confirm_delete_conversation', '删除后会移除当前会话及其消息记录。'),
                t('ai.confirm_delete_conversation_title', '删除对话')
            );
            if (!confirmed) return;
            await fetch('/api/ai/conversations/' + s.conversationId, { method: 'DELETE' });
            s.conversationId = null;
            s.meta = null;
            s.pinnedPapers = [];
            s.messages = [];
            s.turnRuns = [];
            s.pendingCitations = [];
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
            this._syncEvidenceControls();
            if (window.AIReader && window.AIReader.pin && typeof window.AIReader.pin.setPinned === 'function') {
                window.AIReader.pin.setPinned(s.pinnedPapers);
            }
            if (s.els.conversation) {
                s.els.conversation.innerHTML = '';
                const assistantByID = {};
                const citationsByID = {};
                for (const m of s.messages) {
                    const bubble = this._appendMessageBubble(m);
                    if (bubble && m.role === 'assistant' && m.id) {
                        assistantByID[String(m.id)] = bubble;
                        if (m.citations_json) citationsByID[String(m.id)] = m.citations_json;
                    }
                }
                this._attachTurnRunArtifacts(assistantByID);
                Object.keys(citationsByID).forEach((id) => {
                    this._dispatchCitationHydration(assistantByID[id], citationsByID[id]);
                });
            }
        },

        _appendMessageBubble(message) {
            // build a div, append to els.conversation, return ref
            const s = this._state;
            if (!s.els || !s.els.conversation) return null;
            const div = document.createElement('div');
            div.className = 'ai-message ai-message-' + (message.role === 'assistant' ? 'assistant' : 'user');
            if (message.id) div.dataset.messageId = String(message.id);
            this._renderMessageContent(div, message);
            if (message.streaming) div.classList.add('is-streaming');
            s.els.conversation.appendChild(div);
            s.els.conversation.scrollTop = s.els.conversation.scrollHeight;
            return div;
        },

        _renderMessageContent(el, message) {
            if (!el) return;
            const parts = this._ensureMessageParts(el);
            if (message && message.role === 'assistant' && !message.streaming) {
                this._clearStreamingStatus(el);
                parts.text.innerHTML = renderAssistantMarkdown(message.content || '');
            } else if (message && message.role === 'assistant' && message.streaming && !String(message.content || '').trim()) {
                this._setStreamingStatus(el, translate('ai.streaming_thinking', '思考中…'));
            } else {
                this._clearStreamingStatus(el);
                parts.text.textContent = (message && message.content) || '';
            }
        },

        _ensureMessageParts(el) {
            let text = el.querySelector(':scope > .ai-message-text');
            let artifacts = el.querySelector(':scope > .ai-message-artifacts');
            if (!text) {
                text = document.createElement('div');
                text.className = 'ai-message-text';
                while (el.firstChild) text.appendChild(el.firstChild);
                el.appendChild(text);
            }
            if (!artifacts) {
                artifacts = document.createElement('div');
                artifacts.className = 'ai-message-artifacts';
                el.appendChild(artifacts);
            }
            return { text: text, artifacts: artifacts };
        },

        _setAssistantText(el, content, markdown) {
            const parts = this._ensureMessageParts(el);
            this._clearStreamingStatus(el);
            if (markdown) {
                parts.text.innerHTML = renderAssistantMarkdown(content || '');
            } else {
                parts.text.textContent = content || '';
            }
        },

        _setStreamingStatus(el, label) {
            const parts = this._ensureMessageParts(el);
            el.classList.add('has-streaming-status');
            parts.text.textContent = label || translate('ai.streaming_thinking', '思考中…');
        },

        _clearStreamingStatus(el) {
            if (!el || !el.classList) return;
            el.classList.remove('has-streaming-status');
        },

        _attachTurnRunArtifacts(assistantByID) {
            const s = this._state;
            if (!assistantByID || !Array.isArray(s.turnRuns)) return;
            s.turnRuns.forEach((run) => {
                const id = run && run.assistant_message_id;
                const bubble = id ? assistantByID[String(id)] : null;
                if (!bubble) return;
                const process = this._parseJSON(run.process_summary_json);
                if (process) this._renderProcessInto(bubble, process);
                if (Array.isArray(run.cards) && run.cards.length) this._renderCardsInto(bubble, run.cards);
            });
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
                // Re-render pin chips now that the draft has a real conversation ID
                // so the "+ pin" affordance appears without requiring a page reload.
                if (window.AIReader && window.AIReader.pin && typeof window.AIReader.pin.setPinned === 'function') {
                    window.AIReader.pin.setPinned(s.pinnedPapers || []);
                }
            } else if (evt.type === 'delta' && evt.delta) {
                if (s.streaming) {
                    s.streaming.accText += evt.delta;
                    if (assistantBubble) this._setAssistantText(assistantBubble, s.streaming.accText, false);
                    if (s.els && s.els.conversation) s.els.conversation.scrollTop = s.els.conversation.scrollHeight;
                }
            } else if (evt.type === 'process') {
                const summary = evt.process || evt.data;
                if (s.streaming) s.streaming.process = summary;
                this._appendProcess(summary);
            } else if (evt.type === 'cards') {
                const cards = evt.cards || evt.data || [];
                if (s.streaming) s.streaming.cards = cards;
                this._appendCards(cards);
            } else if (evt.type === 'citations') {
                const citations = evt.citations || evt.data || [];
                s.pendingCitations = citations;
                if (s.streaming) s.streaming.pendingCitations = citations;
            } else if (evt.type === 'final') {
                if (assistantBubble) {
                    assistantBubble.classList.remove('is-streaming');
                    if (evt.assistant_message && evt.assistant_message.content) {
                        this._renderMessageContent(assistantBubble, evt.assistant_message);
                    } else if (s.streaming && s.streaming.accText) {
                        this._setAssistantText(assistantBubble, s.streaming.accText, true);
                    }
                    if (s.streaming && s.streaming.process) this._renderProcessInto(assistantBubble, s.streaming.process);
                    if (s.streaming && Array.isArray(s.streaming.cards) && s.streaming.cards.length) {
                        this._renderCardsInto(assistantBubble, s.streaming.cards);
                    }
                    this._hydrateFinalCitations(assistantBubble, evt.assistant_message);
                }
            } else if (evt.type === 'error') {
                if (assistantBubble) {
                    assistantBubble.classList.remove('is-streaming');
                    this._setAssistantText(assistantBubble, '⚠ ' + (evt.error || evt.message || '生成失败'), false);
                    assistantBubble.classList.add('ai-message-error');
                }
            }
        },

        _appendProcess(summary) {
            const s = this._state;
            const bubble = s.streaming && s.streaming.assistantBubbleEl;
            if (bubble) this._renderProcessInto(bubble, summary);
        },

        _appendCards(cards) {
            const s = this._state;
            const bubble = s.streaming && s.streaming.assistantBubbleEl;
            if (bubble) this._renderCardsInto(bubble, cards);
        },

        _renderProcessInto(bubble, summary) {
            if (!bubble || !window.AIReader || !window.AIReader.processStrip) return;
            const html = window.AIReader.processStrip.render(summary);
            const artifacts = this._ensureMessageParts(bubble).artifacts;
            let slot = artifacts.querySelector(':scope > .ai-message-process-slot');
            if (!slot) {
                slot = document.createElement('div');
                slot.className = 'ai-message-process-slot';
                artifacts.prepend(slot);
            }
            slot.innerHTML = html || '';
            this._scrollConversationToBottom();
        },

        _renderCardsInto(bubble, cards) {
            if (!bubble || !window.AIReader || !window.AIReader.resultCards) return;
            const html = window.AIReader.resultCards.render(cards);
            const artifacts = this._ensureMessageParts(bubble).artifacts;
            let slot = artifacts.querySelector(':scope > .ai-message-cards-slot');
            if (!slot) {
                slot = document.createElement('div');
                slot.className = 'ai-message-cards-slot';
                artifacts.appendChild(slot);
            }
            slot.innerHTML = html || '';
            this._scrollConversationToBottom();
        },

        _hydrateFinalCitations(bubble, assistantMessage) {
            const s = this._state;
            const citations = assistantMessage && assistantMessage.citations_json ||
                s.pendingCitations ||
                (s.streaming && s.streaming.pendingCitations) ||
                [];
            this._dispatchCitationHydration(bubble, citations);
        },

        _dispatchCitationHydration(element, citations) {
            if (!element || !citations) return;
            if (typeof citations === 'string' && !citations.trim()) return;
            if (Array.isArray(citations) && !citations.length) return;
            document.dispatchEvent(new CustomEvent('ai-reader:message-rendered', {
                detail: { element: element, citations: citations },
            }));
        },

        _toggleSendingState(on) {
            const s = this._state;
            if (!s.els) return;
            if (s.els.runBtn) s.els.runBtn.disabled = !!on;
            if (s.els.stopBtn) s.els.stopBtn.hidden = !on;
        },

        _scrollConversationToBottom() {
            const s = this._state;
            if (s.els && s.els.conversation) {
                s.els.conversation.scrollTop = s.els.conversation.scrollHeight;
            }
        },

        _parseJSON(value) {
            if (!value) return null;
            if (typeof value === 'object') return value;
            if (typeof value !== 'string') return null;
            try { return JSON.parse(value); } catch (e) { return null; }
        },

        _currentContext() {
            const s = this._state;
            const context = { source: 'ai' };
            const draftPaperID = Number(s._draftPaperId || 0);
            if (!s.conversationId && Number.isFinite(draftPaperID) && draftPaperID > 0) {
                context.paper_id = draftPaperID;
                return context;
            }
            const pinned = Array.isArray(s.pinnedPapers) ? s.pinnedPapers : [];
            const ids = pinned.map((paper) => Number(paper && paper.paper_id || 0)).filter((id) => Number.isFinite(id) && id > 0);
            if (ids.length === 1) {
                context.paper_id = ids[0];
            } else if (ids.length > 1) {
                context.paper_id = ids[0];
                context.paper_ids = ids;
            }
            return context;
        },

        _handleStreamError(err) {
            const s = this._state;
            if (s.streaming && s.streaming.assistantBubbleEl) {
                s.streaming.assistantBubbleEl.classList.remove('is-streaming');
                if (err && err.name === 'AbortError') {
                    this._setAssistantText(s.streaming.assistantBubbleEl, (s.streaming.accText || '') + '\n[已停止]', false);
                    s.streaming.assistantBubbleEl.classList.add('ai-message-stopped');
                } else {
                    this._setAssistantText(s.streaming.assistantBubbleEl, '⚠ ' + (err && err.message ? err.message : '生成失败'), false);
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

        _syncEvidenceControls() {
            const s = this._state;
            if (!s.els || !s.els.externalEvidence) return;
            s.els.externalEvidence.disabled = false;
            s.els.externalEvidence.closest('label')?.classList.remove('is-disabled');
        },

        _loadExternalEvidencePreference() {
            try {
                return localStorage.getItem(EXTERNAL_EVIDENCE_KEY) === '1';
            } catch (e) {
                return false;
            }
        },

        _saveExternalEvidencePreference(on) {
            try {
                localStorage.setItem(EXTERNAL_EVIDENCE_KEY, on ? '1' : '0');
            } catch (e) { /* ignore */ }
        },
    };

    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.view = View;
    }
})();
