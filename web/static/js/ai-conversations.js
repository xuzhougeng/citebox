// ai-conversations.js — sidebar conversation list / search / CRUD.
// Pure data-driven module; reads from /api/ai/conversations*.
//
// Public API: window.AIReader.conversations.{init, refresh, setActive, setSearchQuery, openInlineRename}

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

    // ---------------------------------------------------------------------------
    // Module
    // ---------------------------------------------------------------------------
    const Conversations = {
        _state: {
            items: [],
            activeID: null,
            query: '',
            searchTimer: null,
            container: null,
            emptyEl: null,
            searchEl: null,
            newBtn: null,
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
            const qs = s.query
                ? '?q=' + encodeURIComponent(s.query) + '&limit=50'
                : '?limit=50';
            const url = '/api/ai/conversations' + qs;

            try {
                const res = await fetch(url, { headers: { 'Content-Type': 'application/json' } });
                if (!res.ok) {
                    s.items = [];
                } else {
                    const body = await res.json();
                    s.items = Array.isArray(body.items) ? body.items : [];
                }
            } catch (_e) {
                s.items = [];
            }

            this._render();
        },

        // -----------------------------------------------------------------------
        // Public: setActive — update the active row highlight without a fetch
        // -----------------------------------------------------------------------
        setActive(id) {
            this._state.activeID = id == null ? null : String(id);
            this._render();
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
            }, 250);
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
                            t('ai.confirm_delete_conversation', '删除后会移除当前会话及其消息记录。'),
                            t('ai.confirm_delete_conversation_title', '删除对话')
                        );
                        if (!confirmed) return;
                        this._deleteConversation(id);
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
                this.setActive(id);
                s.onSelect(id);
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
            const item = s.items.find(function (x) { return String(x.id) === String(id); });
            if (item) item.title = newTitle;
            if (String(id) === String(s.activeID)) {
                const view = window.AIReader && window.AIReader.view;
                if (view && view._state) {
                    if (view._state.meta) view._state.meta.title = newTitle;
                    if (view._state.els && view._state.els.title) {
                        view._state.els.title.textContent = newTitle;
                    }
                }
            }
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
