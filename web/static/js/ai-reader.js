// ai-reader.js — bootstrap for the /ai page.
//
// Reads URL params, initialises the conversation sidebar / view / pin /
// mention modules, loads AI settings once, and brokers callbacks between
// modules. Persistence and streaming are owned by the dedicated modules in
// ai-conversations.js / ai-conversation-view.js / ai-pin.js / ai-mention.js.

(function () {
    'use strict';

    function $(id) { return document.getElementById(id); }

    const Reader = {
        settings: null,
        _allPapers: [],

        async init() {
            // Cache settings (best-effort)
            try {
                const res = await fetch('/api/ai/settings');
                if (res.ok) {
                    const body = await res.json();
                    this.settings = body || null;
                }
            } catch (e) { /* offline / fresh install — leave settings null */ }
            window.AIReader = window.AIReader || {};
            window.AIReader.settings = this.settings;

            // Fire-and-forget: cache the library so the @ palette can offer
            // every paper, not just the ones already pinned.
            this._loadAllPapers();

            this._initModules();
            this._bindTitleRename();
            this._bindQuestionMirror();
            await this._dispatchEntry();
        },

        async _loadAllPapers() {
            try {
                const res = await fetch('/api/papers?page_size=200');
                if (!res.ok) return;
                const body = await res.json();
                Reader._allPapers = Array.isArray(body && body.papers) ? body.papers : [];
            } catch (e) { /* leave cache empty — picker still works */ }
        },

        _bindQuestionMirror() {
            // The textarea uses color: transparent so a sibling .ai-question-mirror
            // can render the syntax-highlighted version. Without an active sync,
            // the textarea is invisible. Plain textContent is enough for v1; the
            // colored backgrounds for @ tokens are a polish task.
            const input = $('aiQuestionInput');
            const mirror = $('aiQuestionMirror');
            if (!input || !mirror) return;
            const resizeToContent = () => {
                input.style.height = 'auto';
                const maxHeight = parseFloat(window.getComputedStyle(input).maxHeight);
                const contentHeight = input.scrollHeight;
                const hasMaxHeight = Number.isFinite(maxHeight) && maxHeight > 0;
                const nextHeight = hasMaxHeight ? Math.min(contentHeight, maxHeight) : contentHeight;
                input.style.height = nextHeight + 'px';
                input.style.overflowY = hasMaxHeight && contentHeight > maxHeight ? 'auto' : 'hidden';
                mirror.style.overflowY = input.style.overflowY;
            };
            const sync = () => {
                // Trailing space avoids the last visible line collapsing when the
                // textarea ends with a newline.
                mirror.textContent = input.value + ' ';
                resizeToContent();
                mirror.scrollTop = input.scrollTop;
            };
            input.addEventListener('input', sync);
            input.addEventListener('scroll', () => { mirror.scrollTop = input.scrollTop; });
            sync();
        },

        _initModules() {
            const conversations = window.AIReader && window.AIReader.conversations;
            const view = window.AIReader && window.AIReader.view;
            const pin = window.AIReader && window.AIReader.pin;
            const mention = window.AIReader && window.AIReader.mention;

            if (!conversations || !view || !pin) {
                console.error('[ai-reader] required modules not loaded');
                return;
            }

            // sidebar
            conversations.init({
                container: $('aiConversationList'),
                emptyEl: $('aiSidebarEmpty'),
                searchEl: $('aiConversationSearch'),
                newBtn: $('aiNewConversation'),
                onSelect: (id) => {
                    if (id == null) {
                        view.loadDraft(0);
                    } else {
                        view.load(id);
                    }
                    conversations.setActive(id);
                },
            });

            // main pane
            view.init({
                conversation: $('aiConversation'),
                title: $('aiActiveTitle'),
                pinChips: $('aiPinChips'),
                strictEvidence: $('aiStrictEvidenceToggle'),
                externalEvidence: $('aiExternalEvidenceToggle'),
                runBtn: $('runAIReaderButton'),
                stopBtn: $('stopAIReaderButton'),
                exportBtn: $('aiExportConversation'),
                deleteBtn: $('aiDeleteConversation'),
                questionInput: $('aiQuestionInput'),
            });

            // pin chips — read state from the view module
            pin.init({
                container: $('aiPinChips'),
                getConversationId: () => view._state && view._state.conversationId,
                getPinned: () => (view._state && view._state.pinnedPapers) || [],
                setPinnedCallback: (papers) => {
                    if (view._state) view._state.pinnedPapers = papers;
                },
            });

            // mention palette
            if (mention && typeof mention.attach === 'function') {
                mention.attach(
                    $('aiQuestionInput'),
                    $('aiMentionPopover'),
                    {
                        // Show every paper in the library, with the currently-pinned
                        // ones lifted to the top of the list.
                        getPapers: () => {
                            const all = Reader._allPapers || [];
                            const pinned = (view._state && view._state.pinnedPapers) || [];
                            const pinnedIDs = new Set(pinned.map((p) => p.paper_id));
                            const head = [];
                            const tail = [];
                            all.forEach((p) => {
                                if (pinnedIDs.has(p.id)) {
                                    head.push(p);
                                } else {
                                    tail.push(p);
                                }
                            });
                            return head.concat(tail);
                        },
                        isCurrentPaper: (paper) => {
                            const pinned = (view._state && view._state.pinnedPapers) || [];
                            return pinned.some((p) => p.paper_id === paper.id);
                        },
                        getRolePrompts: () => (Reader.settings && Reader.settings.role_prompts) || [],
                        // Auto-pin on first @-mention (β + γ flow per spec § 3).
                        // Active conversation: server-side pin via /papers POST.
                        // Draft: stash on view._state so the first send carries paper_id.
                        onPickPaper: (paper) => {
                            const id = paper && paper.id;
                            if (!id) return;
                            const pinned = (view._state && view._state.pinnedPapers) || [];
                            if (pinned.some((p) => p.paper_id === id)) return;
                            if (view._state && view._state.conversationId) {
                                window.AIReader.pin.pin(id);
                            } else if (view._state) {
                                view._state._draftPaperId = id;
                            }
                        },
                        onPickRole: () => { /* role label inserted by mention module */ },
                    }
                );
            }

            // when a conversation changes (created via meta event, deleted, renamed) refresh sidebar
            document.addEventListener('ai-reader:conversation-changed', () => {
                conversations.refresh().then(() => {
                    if (view._state && view._state.conversationId) {
                        conversations.setActive(view._state.conversationId);
                    } else {
                        conversations.setActive(null);
                    }
                });
            });
        },

        _bindTitleRename() {
            const el = $('aiActiveTitle');
            if (!el) return;
            const view = window.AIReader.view;
            el.addEventListener('click', () => {
                if (!view._state || !view._state.conversationId) return; // can't rename a draft
                if (el.classList.contains('is-editing')) return;
                this._enterRename(el, view);
            });
        },

        _enterRename(el, view) {
            const original = el.textContent;
            el.classList.add('is-editing');
            el.contentEditable = 'true';
            el.focus();
            // place caret at end
            try {
                const r = document.createRange();
                r.selectNodeContents(el);
                r.collapse(false);
                const sel = window.getSelection();
                sel.removeAllRanges();
                sel.addRange(r);
            } catch (e) { /* ignore */ }
            const commit = async (save) => {
                el.contentEditable = 'false';
                el.classList.remove('is-editing');
                el.removeEventListener('keydown', onKey);
                el.removeEventListener('blur', onBlur);
                if (save) {
                    const v = (el.textContent || '').trim();
                    if (v && v !== original) {
                        await view.rename(v);
                    } else {
                        el.textContent = original;
                    }
                } else {
                    el.textContent = original;
                }
            };
            const onKey = (e) => {
                if (e.key === 'Enter') { e.preventDefault(); commit(true); }
                else if (e.key === 'Escape') { e.preventDefault(); commit(false); }
            };
            const onBlur = () => commit(true);
            el.addEventListener('keydown', onKey);
            el.addEventListener('blur', onBlur);
        },

        async _dispatchEntry() {
            const view = window.AIReader.view;
            const conversations = window.AIReader.conversations;
            const params = new URLSearchParams(window.location.search);
            const convParam = parseInt(params.get('conversation') || '', 10);
            const paperParam = parseInt(params.get('paper_id') || '', 10);

            await conversations.refresh();

            if (Number.isFinite(convParam) && convParam > 0) {
                await view.load(convParam);
                conversations.setActive(convParam);
                return;
            }
            if (Number.isFinite(paperParam) && paperParam > 0) {
                view.loadDraft(paperParam);
                conversations.setActive(null);
                return;
            }
            // default: load the most recent conversation if any
            const list = conversations._state.items || [];
            if (list.length > 0) {
                await view.load(list[0].id);
                conversations.setActive(list[0].id);
            } else {
                view.loadDraft(0);
                conversations.setActive(null);
            }
        },
    };

    document.addEventListener('DOMContentLoaded', () => Reader.init());

    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.boot = Reader;
        // Back-compat shim: main.js still calls AIReaderPage.init() on /ai. The
        // bootstrap already runs from DOMContentLoaded above, so this is a no-op.
        window.AIReaderPage = { init: function () {} };
    }
})();
