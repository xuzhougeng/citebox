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

    const View = {
        _state: {
            els: null,
            conversationId: null,    // null = draft
            meta: null,              // {title, strict_evidence, ...}
            pinnedPapers: [],
            messages: [],
            turnRuns: [],
            pendingCitations: [],
            rewriteLast: false,
            rewriteOriginalMessageID: null,
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
            if (els.conversation) {
                els.conversation.addEventListener('click', function (e) {
                    const action = e.target.closest('[data-ai-message-action]');
                    if (!action) return;
                    const name = action.dataset.aiMessageAction;
                    if (name === 'edit-resend') {
                        e.preventDefault();
                        self._beginEditLastUserMessage();
                    } else if (name === 'cancel-edit-resend') {
                        e.preventDefault();
                        self._clearRewriteMode();
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
            this._clearRewriteMode({ keepInput: true });
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
            this._clearRewriteMode({ keepInput: true });
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

            // T19: bare DOI in the chat input → import directly, surface a
            // notice in the conversation pane, and skip the LLM round-trip.
            if (looksLikeDOI(content)) {
                await this._handleDOIImport(content);
                return;
            }

            const body = { content: content, context: this._currentContext() };

            // Extract @figure-<id> mentions and pass as context.figure_ids.
            const figureIDs = extractFigureMentions(content);
            if (figureIDs.length > 0) {
                body.context.figure_ids = figureIDs;
            }

            // Parse @ tool tags out of the content. The text is left intact in
            // body.content (so the model sees the user's literal input); the parsed
            // intent + sources ride alongside as routing hints.
            const tt = (window.AIReader && window.AIReader.toolTags && window.AIReader.toolTags.parseToolTags)
                ? window.AIReader.toolTags.parseToolTags(content)
                : { intentHint: '', sources: [], conflict: null };
            if (tt.intentHint) body.intent_hint = tt.intentHint;
            if (Array.isArray(tt.sources) && tt.sources.length > 0) body.sources = tt.sources;
            if (tt.conflict) {
                console.warn('[mention] dropped tool tag family=' + tt.conflict.dropped +
                    ' due to family=' + tt.conflict.kept);
            }

            // Caller-supplied intent_hint (e.g., the shortcut buttons) overrides
            // anything we parsed — keep the existing precedence. When the override
            // doesn't match what we parsed, drop the parsed sources too: the
            // backend only consults sources for external_search, so leaking them
            // alongside a different intent muddles the request body.
            if (payload && payload.intent_hint) {
                if (payload.intent_hint !== body.intent_hint) {
                    delete body.sources;
                }
                body.intent_hint = payload.intent_hint;
            }
            if (payload && payload.search_goal_hint) body.search_goal_hint = payload.search_goal_hint;
            if (this._state.rewriteLast && this._state.conversationId) body.replace_last = true;
            await this._sendBody(body);
        },

        async _sendBody(body) {
            const s = this._state;
            if (s.streaming) return;
            if (!body || !String(body.content || '').trim()) return;
            const content = String(body.content || '').trim();
            const replaceLast = !!body.replace_last && !!s.conversationId;
            if (replaceLast) {
                this._dropLastTurnFromState();
                this._removeRenderedLastTurn();
            }
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
                replaceLast: replaceLast,
            };

            if (!s.conversationId) {
                const ids = (Array.isArray(s.pinnedPapers) ? s.pinnedPapers : [])
                    .map((p) => Number(p && p.paper_id || 0))
                    .filter((id) => Number.isFinite(id) && id > 0);
                if (ids.length > 0) {
                    body.paper_id = ids[0];
                    if (ids.length > 1) body.paper_ids = ids;
                } else if (s._draftPaperId) {
                    body.paper_id = s._draftPaperId;
                }
            }
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
                this._clearRewriteMode({ keepInput: true });
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
                this._decorateLastUserMessage();
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
                parts.text.innerHTML = (typeof Utils !== 'undefined' && typeof Utils.renderMarkdown === 'function')
                    ? Utils.renderMarkdown(message.content || '', {})
                    : '<p class="markdown-paragraph">' + escapeHtml(message.content || '') + '</p>';
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
                parts.text.innerHTML = (typeof Utils !== 'undefined' && typeof Utils.renderMarkdown === 'function')
                    ? Utils.renderMarkdown(content || '', {})
                    : '<p class="markdown-paragraph">' + escapeHtml(content || '') + '</p>';
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
            } else if (evt.type === 'image_prompt_drafting') {
                this._setImageGenStatus('drafting', {});
            } else if (evt.type === 'image_prompt_drafted') {
                this._setImageGenStatus('drafted', { prompt: evt.data && evt.data.prompt });
            } else if (evt.type === 'image_generating') {
                this._setImageGenStatus('generating', evt.data || {});
            } else if (evt.type === 'image_generated') {
                this._setImageGenStatus('generated', { card: evt.data && evt.data.card });
            } else if (evt.type === 'image_failed') {
                this._setImageGenStatus('failed', { reason: evt.data && evt.data.reason, stage: evt.data && evt.data.stage });
            } else if (evt.type === 'final') {
                if (assistantBubble) {
                    assistantBubble.classList.remove('is-streaming');
                    if (s.streaming && s.streaming.userBubbleEl && evt.user_message && evt.user_message.id) {
                        s.streaming.userBubbleEl.dataset.messageId = String(evt.user_message.id);
                    }
                    if (evt.assistant_message && evt.assistant_message.id) {
                        assistantBubble.dataset.messageId = String(evt.assistant_message.id);
                    }
                    if (evt.assistant_message && evt.assistant_message.content) {
                        this._renderMessageContent(assistantBubble, evt.assistant_message);
                    } else if (s.streaming && s.streaming.accText) {
                        this._setAssistantText(assistantBubble, s.streaming.accText, true);
                    } else {
                        this._setAssistantText(assistantBubble, translate('ai.turn_no_text', '模型没有返回文本结果。'), false);
                    }
                    if (s.streaming && s.streaming.process) this._renderProcessInto(assistantBubble, s.streaming.process);
                    if (s.streaming && Array.isArray(s.streaming.cards) && s.streaming.cards.length) {
                        this._renderCardsInto(assistantBubble, s.streaming.cards);
                    }
                    this._hydrateFinalCitations(assistantBubble, evt.assistant_message);
                    this._persistFinalMessages(evt);
                    this._decorateLastUserMessage();
                    document.dispatchEvent(new CustomEvent('ai-reader:conversation-changed'));
                }
            } else if (evt.type === 'error') {
                if (assistantBubble) {
                    assistantBubble.classList.remove('is-streaming');
                    this._setAssistantText(assistantBubble, '⚠ ' + (evt.error || evt.message || '生成失败'), false);
                    assistantBubble.classList.add('ai-message-error');
                }
            }
        },

        _persistFinalMessages(evt) {
            const s = this._state;
            const user = evt && evt.user_message;
            const assistant = evt && evt.assistant_message;
            if (user && user.id && !s.messages.some((m) => String(m.id) === String(user.id))) {
                s.messages.push(user);
            }
            if (assistant && assistant.id && !s.messages.some((m) => String(m.id) === String(assistant.id))) {
                s.messages.push(assistant);
            }
        },

        _beginEditLastUserMessage() {
            const s = this._state;
            if (s.streaming || !s.conversationId || !s.els || !s.els.questionInput) return;
            const lastUser = this._lastUserMessage();
            if (!lastUser) return;
            s.rewriteLast = true;
            s.rewriteOriginalMessageID = lastUser.id || null;
            s.els.questionInput.value = lastUser.content || '';
            s.els.questionInput.dispatchEvent(new Event('input', { bubbles: true }));
            s.els.questionInput.focus();
            if (s.els.questionInput.select) s.els.questionInput.select();
            this._syncRewriteControls();
            this._decorateLastUserMessage();
        },

        _clearRewriteMode(options) {
            const s = this._state;
            const opts = options || {};
            s.rewriteLast = false;
            s.rewriteOriginalMessageID = null;
            if (!opts.keepInput && s.els && s.els.questionInput) {
                s.els.questionInput.value = '';
                s.els.questionInput.dispatchEvent(new Event('input', { bubbles: true }));
            }
            this._syncRewriteControls();
            this._decorateLastUserMessage();
        },

        _syncRewriteControls() {
            const s = this._state;
            if (!s.els || !s.els.runBtn) return;
            s.els.runBtn.textContent = s.rewriteLast
                ? translate('ai.btn_resend', '重新发送')
                : translate('ai.btn_send', '发送问题');
        },

        _lastUserMessage() {
            const s = this._state;
            for (let i = s.messages.length - 1; i >= 0; i -= 1) {
                if (s.messages[i] && s.messages[i].role === 'user') return s.messages[i];
            }
            return null;
        },

        _dropLastTurnFromState() {
            const s = this._state;
            let cut = -1;
            for (let i = s.messages.length - 1; i >= 0; i -= 1) {
                if (s.messages[i] && s.messages[i].role === 'user') {
                    cut = i;
                    break;
                }
            }
            if (cut >= 0) {
                const removedUser = s.messages[cut];
                const removedIDs = new Set(s.messages.slice(cut).map((m) => String(m && m.id || '')).filter(Boolean));
                s.messages = s.messages.slice(0, cut);
                s.turnRuns = (s.turnRuns || []).filter((run) => {
                    const userID = String(run && run.user_message_id || '');
                    const assistantID = String(run && run.assistant_message_id || '');
                    if (removedUser && String(removedUser.id) === userID) return false;
                    return !removedIDs.has(userID) && !removedIDs.has(assistantID);
                });
            }
        },

        _removeRenderedLastTurn() {
            const s = this._state;
            if (!s.els || !s.els.conversation) return;
            const bubbles = Array.from(s.els.conversation.querySelectorAll(':scope > .ai-message'));
            let cut = -1;
            for (let i = bubbles.length - 1; i >= 0; i -= 1) {
                if (bubbles[i].classList.contains('ai-message-user')) {
                    cut = i;
                    break;
                }
            }
            if (cut < 0) return;
            bubbles.slice(cut).forEach((node) => node.remove());
        },

        _decorateLastUserMessage() {
            const s = this._state;
            if (!s.els || !s.els.conversation) return;
            s.els.conversation.querySelectorAll('.ai-message-user-action-slot').forEach((node) => node.remove());
            s.els.conversation.querySelectorAll('.ai-message-user.is-editing-last').forEach((node) => node.classList.remove('is-editing-last'));
            if (s.streaming || !s.conversationId) return;
            const lastUser = this._lastUserMessage();
            if (!lastUser || !lastUser.id) return;
            const bubble = Array.from(s.els.conversation.querySelectorAll('.ai-message-user'))
                .find((node) => String(node.dataset.messageId || '') === String(lastUser.id));
            if (!bubble) return;
            if (s.rewriteLast) bubble.classList.add('is-editing-last');
            const slot = document.createElement('div');
            slot.className = 'ai-message-user-action-slot';
            const editLabel = translate('ai.btn_edit_resend', '编辑重发');
            const cancelLabel = translate('ai.btn_cancel_resend', '取消');
            slot.innerHTML = s.rewriteLast
                ? '<span class="ai-message-user-action-hint">' + escapeHtml(translate('ai.edit_resend_active', '正在编辑最后一问')) + '</span>' +
                    '<button class="ai-message-user-action" type="button" data-ai-message-action="cancel-edit-resend">' + escapeHtml(cancelLabel) + '</button>'
                : '<button class="ai-message-user-action" type="button" data-ai-message-action="edit-resend">' + escapeHtml(editLabel) + '</button>';
            bubble.appendChild(slot);
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

        _setImageGenStatus(status, data) {
            data = data || {};
            const node = this._ensureImageGenStatusNode();
            if (!node) return;
            switch (status) {
                case 'drafting':
                    node.innerHTML = '<p class="ai-status-line">' + escapeHtml(translate('ai.image_gen.status.drafting_prompt', '正在分析文献和图片...')) + '</p>';
                    break;
                case 'drafted':
                    node.innerHTML =
                        '<p class="ai-status-line">' + escapeHtml(translate('ai.image_gen.status.prompt_drafted', '已生成图像 prompt')) + '</p>' +
                        '<details><summary>' + escapeHtml(translate('ai.image_gen.status.prompt_label', '查看 prompt')) + '</summary>' +
                        '<pre>' + escapeHtml(data.prompt || '') + '</pre></details>';
                    break;
                case 'generating': {
                    const cost = (typeof data.cost_estimate_usd === 'number') ? ' ($' + data.cost_estimate_usd.toFixed(2) + ')' : '';
                    node.innerHTML = '<p class="ai-status-line">' + escapeHtml(translate('ai.image_gen.status.generating', '正在生成图片') + cost) + '</p>';
                    break;
                }
                case 'generated':
                    if (window.AIReader && window.AIReader.resultCards && typeof window.AIReader.resultCards.render === 'function') {
                        node.innerHTML = window.AIReader.resultCards.render([{ card_type: 'generated_image', payload: data.card }]);
                    } else {
                        node.innerHTML = '<p class="ai-status-line">' + escapeHtml(translate('ai.image_gen.status.done', '已生成图片')) + '</p>';
                    }
                    break;
                case 'failed':
                    node.innerHTML = '<p class="ai-status-error">' +
                        escapeHtml(translate('ai.image_gen.status.failed', '图像生成失败')) + ': ' + escapeHtml(data.reason || '') +
                    '</p>';
                    break;
            }
            this._scrollConversationToBottom();
        },

        // Find or create a stable status node attached to the current streaming assistant bubble.
        _ensureImageGenStatusNode() {
            const s = this._state;
            const bubble = s.streaming && s.streaming.assistantBubbleEl;
            if (!bubble) return null;
            const artifacts = this._ensureMessageParts(bubble).artifacts;
            let node = artifacts.querySelector(':scope > .ai-image-gen-status');
            if (!node) {
                node = document.createElement('div');
                node.className = 'ai-image-gen-status';
                artifacts.appendChild(node);
            }
            return node;
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
            const pinned = Array.isArray(s.pinnedPapers) ? s.pinnedPapers : [];
            const ids = pinned
                .map((paper) => Number(paper && paper.paper_id || 0))
                .filter((id) => Number.isFinite(id) && id > 0);
            // Fall back to the legacy single-value draft field only if the
            // pinned-papers array hasn't picked it up yet (deep-link prefill).
            if (ids.length === 0 && !s.conversationId) {
                const draftPaperID = Number(s._draftPaperId || 0);
                if (Number.isFinite(draftPaperID) && draftPaperID > 0) {
                    ids.push(draftPaperID);
                }
            }
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

        async _handleDOIImport(text) {
            const s = this._state;
            if (s.els && s.els.questionInput) s.els.questionInput.value = '';
            const tt = (typeof window.t === 'function') ? window.t : function (k, f) { return f || k; };
            this._appendNotice(tt('ai.doi.importing', '正在通过 DOI 导入文献：') + text);
            try {
                const paper = await API.importPaperByDOI({ doi: text });
                this._appendNotice(tt('ai.doi.imported', '已导入：') + '#' + paper.id + ' ' + (paper.title || ''));
            } catch (err) {
                const msg = (err && err.message) || String(err);
                this._appendNotice(tt('ai.doi.failed', 'DOI 导入失败：') + msg);
            }
        },

        _appendNotice(text) {
            const s = this._state;
            if (!s.els || !s.els.conversation) return;
            const div = document.createElement('div');
            div.className = 'ai-message ai-message-system';
            const inner = document.createElement('div');
            inner.className = 'ai-message-content';
            inner.textContent = text;
            div.appendChild(inner);
            s.els.conversation.appendChild(div);
            s.els.conversation.scrollTop = s.els.conversation.scrollHeight;
        },
    };

    function extractFigureMentions(text) {
        const re = /(^|\s)@figure-(\d+)\b/g;
        const ids = [];
        const seen = new Set();
        let m;
        while ((m = re.exec(text)) !== null) {
            const id = parseInt(m[2], 10);
            if (!seen.has(id)) { seen.add(id); ids.push(id); }
        }
        return ids;
    }

    const DOI_RE = /\b10\.\d{4,9}\/[-._;()/:A-Z0-9]+\b/i;
    function looksLikeDOI(text) {
        const trimmed = String(text || '').trim();
        if (!trimmed) return false;
        if (DOI_RE.test(trimmed)) return true;
        const lower = trimmed.toLowerCase();
        if (lower.startsWith('doi:')) return true;
        if (lower.indexOf('doi.org/') !== -1) return true;
        return false;
    }

    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.view = View;
    }
})();
