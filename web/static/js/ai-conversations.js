// ai-conversations.js — sidebar conversation list / search / CRUD.
// Pure data-driven module; reads from /api/ai/conversations*.
//
// Public API: window.AIReader.conversations.{init, refresh, setActive, setSearchQuery, openInlineRename, openManager}

(function () {
    'use strict';

    // ---------------------------------------------------------------------------
    // HTML escape helper (no XSS through user-controlled text)
    // ---------------------------------------------------------------------------
    function escapeHtml(s) {
        if (typeof Utils !== 'undefined' && typeof Utils.escapeHTML === 'function') {
            return Utils.escapeHTML(s);
        }
        return String(s == null ? '' : s)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    function tr(key, fallback) {
        if (window.CiteBoxI18n && typeof window.CiteBoxI18n.t === 'function') {
            return window.CiteBoxI18n.t(key, fallback);
        }
        if (typeof window.t === 'function') {
            return window.t(key, fallback);
        }
        return fallback || key;
    }

    const SIDEBAR_LIMIT = 10;
    const MANAGER_LIMIT = 200;
    const SEARCH_DEBOUNCE_MS = 250;

    // ---------------------------------------------------------------------------
    // Module
    // ---------------------------------------------------------------------------
    const Conversations = {
        _state: {
            items: [],
            activeID: null,
            query: '',
            searchTimer: null,
            sidebarLimit: SIDEBAR_LIMIT,
            managerLimit: MANAGER_LIMIT,
            container: null,
            emptyEl: null,
            searchEl: null,
            newBtn: null,
            moreBtn: null,
            managerOverlay: null,
            managerList: null,
            managerSearchEl: null,
            managerEmptyEl: null,
            managerItems: [],
            managerQuery: '',
            managerSearchTimer: null,
            managerKeyHandler: null,
            onSelect: null,
            getActiveID: null,
        },

        // -----------------------------------------------------------------------
        // Public: init
        // -----------------------------------------------------------------------
        init(opts) {
            const s = this._state;
            s.container   = opts.container   || null;
            s.emptyEl     = opts.emptyEl     || null;
            s.searchEl    = opts.searchEl    || null;
            s.newBtn      = opts.newBtn      || null;
            s.moreBtn     = opts.moreBtn     || null;
            s.onSelect    = typeof opts.onSelect    === 'function' ? opts.onSelect    : function () {};
            s.getActiveID = typeof opts.getActiveID === 'function' ? opts.getActiveID : function () { return s.activeID; };

            const self = this;

            if (s.container) {
                s.container.addEventListener('click', function (e) {
                    self._onContainerClick(e);
                });
                s.container.addEventListener('dblclick', function (e) {
                    self._onContainerDoubleClick(e);
                });
            }

            if (s.searchEl) {
                s.searchEl.addEventListener('input', function () {
                    self.setSearchQuery(s.searchEl.value);
                });
            }

            if (s.moreBtn) {
                s.moreBtn.addEventListener('click', function () {
                    self.openManager();
                });
            }

            if (s.newBtn) {
                s.newBtn.addEventListener('click', function () {
                    self.setActive(null);
                    s.onSelect(null);
                });
            }
        },

        // -----------------------------------------------------------------------
        // Public: refresh — fetch list then re-render
        // -----------------------------------------------------------------------
        async refresh() {
            const s = this._state;
            s.items = await this._fetchConversations(s.query, s.sidebarLimit);
            this._render();
        },

        // -----------------------------------------------------------------------
        // Public: openManager — show searchable conversation management modal
        // -----------------------------------------------------------------------
        async openManager() {
            const s = this._state;
            this._ensureManager();
            if (!s.managerOverlay) return;

            s.managerQuery = '';
            if (s.managerSearchEl) {
                s.managerSearchEl.value = '';
            }

            s.managerOverlay.classList.remove('hidden');
            document.body.classList.add('modal-open');
            this._renderManager({ loading: true });
            await this._refreshManager();
            if (s.managerSearchEl) {
                setTimeout(function () { s.managerSearchEl.focus(); }, 0);
            }
        },

        // -----------------------------------------------------------------------
        // Public: closeManager — hide conversation management modal
        // -----------------------------------------------------------------------
        closeManager() {
            const s = this._state;
            if (!s.managerOverlay) return;
            s.managerOverlay.classList.add('hidden');
            if (!document.querySelector('.modal-shell:not(.hidden)')) {
                document.body.classList.remove('modal-open');
            }
        },

        // -----------------------------------------------------------------------
        // Private: _fetchConversations — shared list loader
        // -----------------------------------------------------------------------
        async _fetchConversations(query, limit) {
            const params = new URLSearchParams();
            const q = (query || '').trim();
            if (q) params.set('q', q);
            params.set('limit', String(limit || SIDEBAR_LIMIT));
            const url = '/api/ai/conversations?' + params.toString();
            try {
                const res = await fetch(url, { headers: { 'Content-Type': 'application/json' } });
                if (!res.ok) {
                    return [];
                } else {
                    const body = await res.json();
                    return Array.isArray(body.items) ? body.items : [];
                }
            } catch (_e) {
                return [];
            }
        },

        // -----------------------------------------------------------------------
        // Public: setActive — update the active row highlight without a fetch
        // -----------------------------------------------------------------------
        setActive(id) {
            this._state.activeID = id == null ? null : String(id);
            this._render();
            this._renderManager();
        },

        // -----------------------------------------------------------------------
        // Public: setSearchQuery — debounced 250ms then refresh
        // -----------------------------------------------------------------------
        setSearchQuery(q) {
            const s = this._state;
            clearTimeout(s.searchTimer);
            const self = this;
            s.searchTimer = setTimeout(function () {
                s.query = (q || '').trim();
                self.refresh();
            }, SEARCH_DEBOUNCE_MS);
        },

        // -----------------------------------------------------------------------
        // Public: openInlineRename — replace the title cell with an <input>
        // -----------------------------------------------------------------------
        openInlineRename(rowEl) {
            if (!rowEl) return;
            const id = rowEl.dataset.id;
            if (!id) return;

            const titleEl = rowEl.querySelector('.ai-conversation-row-title');
            if (!titleEl) return;

            // Prevent double-opening
            if (rowEl.querySelector('input.ai-conv-rename-input')) return;

            const originalText = titleEl.textContent;
            rowEl.classList.add('is-renaming');
            const input = document.createElement('input');
            input.type = 'text';
            input.className = 'ai-conv-rename-input';
            input.value = originalText === tr('ai.active_title_default', 'New Conversation') ? '' : originalText;
            input.placeholder = tr('ai.active_title_default', 'New Conversation');
            input.setAttribute('aria-label', tr('ai.rename_conversation', 'Rename Conversation'));

            // Replace the text node with the input
            titleEl.textContent = '';
            titleEl.appendChild(input);

            input.focus();
            input.select();
            input.addEventListener('click', function (e) { e.stopPropagation(); });
            input.addEventListener('dblclick', function (e) { e.stopPropagation(); });

            let committed = false;
            const self = this;

            function commit() {
                if (committed) return;
                committed = true;
                const newTitle = input.value.trim();
                // Restore display immediately (optimistic)
                titleEl.textContent = newTitle || tr('ai.active_title_default', 'New Conversation');
                rowEl.classList.remove('is-renaming');
                if (newTitle && newTitle !== originalText) {
                    self._commitRename(id, newTitle);
                }
            }

            function cancel() {
                if (committed) return;
                committed = true;
                titleEl.textContent = originalText;
                rowEl.classList.remove('is-renaming');
            }

            input.addEventListener('blur', commit);

            input.addEventListener('keydown', function (e) {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    input.removeEventListener('blur', commit);
                    commit();
                } else if (e.key === 'Escape') {
                    e.preventDefault();
                    input.removeEventListener('blur', commit);
                    cancel();
                }
            });
        },

        // -----------------------------------------------------------------------
        // Private: _render — rebuild the list DOM
        // -----------------------------------------------------------------------
        _render() {
            const s = this._state;
            const container = s.container;
            const emptyEl   = s.emptyEl;

            if (!container) return;

            const items = s.items || [];

            if (items.length === 0) {
                container.innerHTML = '';
                container.hidden = true;
                if (emptyEl) emptyEl.hidden = false;
                return;
            }

            if (emptyEl) emptyEl.hidden = true;
            container.hidden = false;

            // Build a fragment to minimise reflows
            const fragment = document.createDocumentFragment();
            items.forEach(function (item) {
                const li = document.createElement('li');
                li.className = 'ai-conversation-row';
                li.dataset.id = String(item.id);
                if (String(item.id) === s.activeID) {
                    li.classList.add('is-active');
                }
                li.innerHTML = this._renderRow(item);
                fragment.appendChild(li);
            }, this);

            container.innerHTML = '';
            container.appendChild(fragment);
        },

        // -----------------------------------------------------------------------
        // Private: _ensureManager — create modal once and bind its events
        // -----------------------------------------------------------------------
        _ensureManager() {
            const s = this._state;
            if (s.managerOverlay) return;

            const closeLabel = escapeHtml(tr('ai.btn_close_modal', 'Close'));
            const title = escapeHtml(tr('ai.conversation_manager_title', 'Conversation Manager'));
            const desc = escapeHtml(tr('ai.conversation_manager_desc', 'View, search, and manage older conversations.'));
            const placeholder = escapeHtml(tr('ai.conversation_manager_search_placeholder', 'Search all conversations...'));

            const overlay = document.createElement('div');
            overlay.className = 'modal-shell ai-conversation-manager hidden';
            overlay.setAttribute('role', 'dialog');
            overlay.setAttribute('aria-modal', 'true');
            overlay.setAttribute('aria-labelledby', 'aiConversationManagerTitle');
            overlay.innerHTML = (
                '<div class="modal-dialog ai-conversation-manager-dialog">' +
                    '<button class="modal-close" type="button" aria-label="' + closeLabel + '">×</button>' +
                    '<header class="ai-conversation-manager-head">' +
                        '<div>' +
                            '<h3 id="aiConversationManagerTitle">' + title + '</h3>' +
                            '<p>' + desc + '</p>' +
                        '</div>' +
                    '</header>' +
                    '<div class="ai-conversation-manager-body">' +
                        '<input class="form-input ai-conversation-manager-search" type="text" placeholder="' + placeholder + '" autocomplete="off">' +
                        '<ul class="ai-conversation-list ai-conversation-manager-list" role="list"></ul>' +
                        '<p class="ai-conversation-manager-empty" hidden>' +
                            escapeHtml(tr('ai.conversation_manager_empty', 'No matching conversations.')) +
                        '</p>' +
                    '</div>' +
                '</div>'
            );

            document.body.appendChild(overlay);
            s.managerOverlay = overlay;
            s.managerList = overlay.querySelector('.ai-conversation-manager-list');
            s.managerSearchEl = overlay.querySelector('.ai-conversation-manager-search');
            s.managerEmptyEl = overlay.querySelector('.ai-conversation-manager-empty');

            const self = this;
            overlay.querySelector('.modal-close').addEventListener('click', function () {
                self.closeManager();
            });
            overlay.addEventListener('click', function (e) {
                if (e.target === overlay) self.closeManager();
            });
            s.managerList.addEventListener('click', function (e) {
                self._onContainerClick(e);
            });
            s.managerList.addEventListener('dblclick', function (e) {
                self._onContainerDoubleClick(e);
            });
            s.managerSearchEl.addEventListener('input', function () {
                clearTimeout(s.managerSearchTimer);
                s.managerSearchTimer = setTimeout(function () {
                    s.managerQuery = s.managerSearchEl.value.trim();
                    self._refreshManager();
                }, SEARCH_DEBOUNCE_MS);
            });
            s.managerKeyHandler = function (e) {
                if (e.key !== 'Escape') return;
                if (!s.managerOverlay || s.managerOverlay.classList.contains('hidden')) return;
                if (typeof Utils !== 'undefined' && typeof Utils.isTopVisibleModal === 'function') {
                    if (!Utils.isTopVisibleModal(s.managerOverlay)) return;
                }
                self.closeManager();
            };
            document.addEventListener('keydown', s.managerKeyHandler);
        },

        // -----------------------------------------------------------------------
        // Private: _refreshManager — fetch modal list then render
        // -----------------------------------------------------------------------
        async _refreshManager() {
            const s = this._state;
            if (!s.managerOverlay || s.managerOverlay.classList.contains('hidden')) return;
            s.managerItems = await this._fetchConversations(s.managerQuery, s.managerLimit);
            this._renderManager();
        },

        // -----------------------------------------------------------------------
        // Private: _renderManager — rebuild the modal list DOM
        // -----------------------------------------------------------------------
        _renderManager(options) {
            const s = this._state;
            const list = s.managerList;
            if (!list) return;

            const opts = options || {};
            if (opts.loading) {
                list.hidden = false;
                if (s.managerEmptyEl) s.managerEmptyEl.hidden = true;
                list.innerHTML = (
                    '<li class="ai-conversation-manager-loading">' +
                        escapeHtml(tr('ai.conversation_manager_loading', 'Loading conversations...')) +
                    '</li>'
                );
                return;
            }

            const items = s.managerItems || [];
            if (items.length === 0) {
                list.innerHTML = '';
                list.hidden = true;
                if (s.managerEmptyEl) s.managerEmptyEl.hidden = false;
                return;
            }

            list.hidden = false;
            if (s.managerEmptyEl) s.managerEmptyEl.hidden = true;

            const fragment = document.createDocumentFragment();
            items.forEach(function (item) {
                const li = document.createElement('li');
                li.className = 'ai-conversation-row ai-conversation-manager-row';
                li.dataset.id = String(item.id);
                if (String(item.id) === s.activeID) {
                    li.classList.add('is-active');
                }
                li.innerHTML = this._renderRow(item);
                fragment.appendChild(li);
            }, this);

            list.innerHTML = '';
            list.appendChild(fragment);
        },

        // -----------------------------------------------------------------------
        // Private: _renderRow — return HTML string for one list item's inner HTML
        // -----------------------------------------------------------------------
        _renderRow(item) {
            const title   = escapeHtml(item.title || '');
            const display = title || escapeHtml(tr('ai.active_title_default', 'New Conversation'));
            const meta    = escapeHtml(this._formatPinSummary(item.pinned_papers));
            const renameLabel = escapeHtml(tr('ai.rename_conversation', 'Rename Conversation'));
            const deleteLabel = escapeHtml(tr('ai.delete_conversation_aria', 'Delete Conversation'));

            return (
                '<div class="ai-conversation-row-title">' + display + '</div>' +
                '<div class="ai-conversation-row-meta">' + meta + '</div>' +
                '<div class="ai-conversation-row-actions">' +
                    '<button class="ai-conversation-row-action" data-action="rename" aria-label="' + renameLabel + '" type="button">✎</button>' +
                    '<button class="ai-conversation-row-action" data-action="delete" aria-label="' + deleteLabel + '" type="button">×</button>' +
                '</div>'
            );
        },

        // -----------------------------------------------------------------------
        // Private: _formatPinSummary — "Smith 2020 · Lee 2021" or "暂无 pin 文献"
        // -----------------------------------------------------------------------
        _formatPinSummary(pinned) {
            if (!Array.isArray(pinned) || pinned.length === 0) {
                return tr('ai.no_pinned_papers', 'No pinned papers');
            }
            return pinned
                .slice(0, 3)
                .map(function (p) {
                    // Support both {title} and {paper: {title}} shapes
                    const raw = (p && (p.title || (p.paper && p.paper.title))) || '';
                    return raw.trim() || '?';
                })
                .join(' · ');
        },

        // -----------------------------------------------------------------------
        // Private: _onContainerClick — delegated event dispatch
        // -----------------------------------------------------------------------
        async _onContainerClick(e) {
            const s = this._state;
            const btn = e.target.closest('[data-action]');
            if (btn) {
                const action = btn.dataset.action;
                const rowEl  = btn.closest('.ai-conversation-row');
                if (!rowEl) return;
                const id = rowEl.dataset.id;

                if (action === 'rename') {
                    this.openInlineRename(rowEl);
                } else if (action === 'delete') {
                    if (typeof Utils !== 'undefined' && typeof Utils.confirm === 'function') {
                        const confirmed = await Utils.confirm(
                            tr('ai.confirm_delete_conversation', 'This will remove the conversation and its message history.'),
                            tr('ai.confirm_delete_conversation_title', 'Delete Conversation')
                        );
                        if (!confirmed) return;
                        await this._deleteConversation(id);
                    }
                }
                return;
            }

            // Row click (anywhere except action buttons)
            const rowEl = e.target.closest('.ai-conversation-row');
            if (rowEl) {
                if (e.detail >= 2) {
                    this.openInlineRename(rowEl);
                    return;
                }
                const id = rowEl.dataset.id;
                const fromManager = Boolean(s.managerOverlay && s.managerOverlay.contains(rowEl));
                this.setActive(id);
                s.onSelect(id);
                if (fromManager) {
                    this.closeManager();
                }
            }
        },

        // -----------------------------------------------------------------------
        // Private: _onContainerDoubleClick — enter inline rename from the row
        // -----------------------------------------------------------------------
        _onContainerDoubleClick(e) {
            if (e.target.closest('[data-action], input.ai-conv-rename-input')) return;
            const rowEl = e.target.closest('.ai-conversation-row');
            if (!rowEl) return;
            e.preventDefault();
            this.openInlineRename(rowEl);
        },

        // -----------------------------------------------------------------------
        // Private: _deleteConversation — DELETE call then refresh
        // -----------------------------------------------------------------------
        async _deleteConversation(id) {
            const s = this._state;
            try {
                await fetch('/api/ai/conversations/' + encodeURIComponent(id), {
                    method: 'DELETE',
                    headers: { 'Content-Type': 'application/json' },
                });
            } catch (_e) {
                // swallow — refresh will reflect true state
            }
            // If the deleted conversation was active, notify caller to reset pane
            if (String(id) === s.activeID) {
                s.activeID = null;
                s.onSelect(null);
            }
            await this.refresh();
            await this._refreshManager();
        },

        // -----------------------------------------------------------------------
        // Private: _commitRename — PATCH title
        // -----------------------------------------------------------------------
        async _commitRename(id, newTitle) {
            try {
                await fetch('/api/ai/conversations/' + encodeURIComponent(id), {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ title: newTitle }),
                });
            } catch (_e) {
                // Silently ignore network errors; the optimistic update already showed the new title
            }
            // Sync items cache so a subsequent render shows the new title without a full fetch
            const s = this._state;
            [s.items, s.managerItems].forEach(function (list) {
                const item = list.find(function (x) { return String(x.id) === String(id); });
                if (item) item.title = newTitle;
            });
            if (String(id) === String(s.activeID)) {
                const view = window.AIReader && window.AIReader.view;
                if (view && view._state) {
                    if (view._state.meta) view._state.meta.title = newTitle;
                    if (view._state.els && view._state.els.title) {
                        view._state.els.title.textContent = newTitle;
                    }
                }
            }
            this._render();
            this._renderManager();
        },
    };

    // ---------------------------------------------------------------------------
    // Register on window.AIReader namespace
    // ---------------------------------------------------------------------------
    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.conversations = Conversations;
    }
})();
