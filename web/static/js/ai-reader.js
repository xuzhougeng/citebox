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
        externalSourcesEnabled: null,
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
            // Also cache the enabled external-search sources so the @ palette can
            // gray out disabled rows. Best-effort: failure leaves the set null and
            // the palette will treat all known sources as enabled.
            try {
                const res = await fetch('/api/settings/ai-external-search');
                if (res.ok) {
                    const body = await res.json();
                    this.externalSourcesEnabled = Array.isArray(body && body.sources)
                        ? body.sources.map((s) => String(s || '').toLowerCase())
                        : null;
                }
            } catch (e) { /* offline — leave null */ }
            window.AIReader = window.AIReader || {};
            window.AIReader.settings = this.settings;
            window.AIReader.externalSourcesEnabled = this.externalSourcesEnabled;

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
                Reader._allPapers = Array.isArray(body && body.papers)
                    ? body.papers
                    : (Array.isArray(body && body.items) ? body.items : []);
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
                moreBtn: $('aiMoreConversations'),
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

            if (window.AIReader.composer) {
                window.AIReader.composer.init({
                    input: $('aiQuestionInput'),
                    sendBtn: $('runAIReaderButton'),
                    stopBtn: $('stopAIReaderButton'),
                    shortcutRoot: $('aiIntentShortcuts'),
                    onSend: (payload) => view.sendPayload(payload),
                });
            }

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
                        getToolTags: () => {
                            const enabled = (window.AIReader && Array.isArray(window.AIReader.externalSourcesEnabled))
                                ? new Set(window.AIReader.externalSourcesEnabled)
                                : null; // null = "settings unknown, treat all as enabled"
                            const known = (window.AIReader && window.AIReader.toolTags && window.AIReader.toolTags.KNOWN_TOOL_TAGS) || [];
                            const t = (typeof window !== 'undefined' && typeof window.t === 'function') ? window.t : (key, fallback) => (fallback || key);
                            const descKeyMap = {
                                PubMed: ['ai.tool_pubmed_desc', '外部源 · PubMed'],
                                SemanticScholar: ['ai.tool_semantic_scholar_desc', '外部源 · Semantic Scholar'],
                                Library: ['ai.tool_library_desc', '本地文本检索（不含图）'],
                                Figure: ['ai.tool_figure_desc', '本地图片检索'],
                            };
                            return known.map((tag) => {
                                const isExternal = tag.family === 'external';
                                const isDisabled = isExternal && enabled !== null && !enabled.has(tag.source);
                                const desc = descKeyMap[tag.name];
                                return {
                                    name: tag.name,
                                    family: tag.family,
                                    source: tag.source,
                                    description: desc ? t(desc[0], desc[1]) : '',
                                    disabled: isDisabled,
                                    disabledReason: isDisabled ? t('ai.mention_tool_disabled', '未启用，前往设置 →') : '',
                                };
                            });
                        },
                        onPickToolTag: () => { /* nothing — value is read on submit */ },
                        onPickDisabledTag: () => {
                            window.location.href = '/settings#settings-external-sources';
                        },
                        // Auto-pin on first @-mention (β + γ flow per spec § 3).
                        // Active conversation: server-side pin via /papers POST.
                        // Draft: append to pinnedPapers and re-render chips so the
                        // user sees the pin immediately. The first /messages call
                        // forwards paper_id + paper_ids to the server, which auto-
                        // pins each entry on the freshly-created conversation.
                        onPickPaper: (paper) => {
                            const id = paper && paper.id;
                            if (!id) return;
                            if (!view._state) return;
                            const pinned = view._state.pinnedPapers || [];
                            if (pinned.some((p) => p.paper_id === id)) return;
                            if (view._state.conversationId) {
                                window.AIReader.pin.pin(id);
                                return;
                            }
                            const next = pinned.concat([{
                                paper_id: id,
                                title: (paper && paper.title) || '',
                            }]);
                            view._state.pinnedPapers = next;
                            // Keep the legacy single-value field in sync with the
                            // first pin so that prefilled deep-links keep working.
                            view._state._draftPaperId = next[0].paper_id;
                            if (window.AIReader.pin && typeof window.AIReader.pin.setPinned === 'function') {
                                window.AIReader.pin.setPinned(next);
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
