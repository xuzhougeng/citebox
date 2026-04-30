// ai-mention.js — @ palette logic.
//
// Owns the mention popover UI, keyboard nav, and the callback hooks that the
// question input fires when the user types `@`. The module is stateless from
// the outside — call attach() once with the input/popover elements and a
// providers object, then dismiss() to force-close.
//
// Public API:
//   window.AIReader.mention.attach(input, popover, providers)
//   window.AIReader.mention.dismiss()
//
// providers contract (object with these keys, each a function):
//   - getPapers()          => Paper[]        (current selectable papers; e.g. pinned + recent)
//   - getRolePrompts()     => RolePrompt[]   (each has .name and optionally .description/.prompt)
//   - onPickPaper(paper)   => void           (called when user selects a paper from the popover)
//   - onPickRole(role)     => void           (called when user selects a role prompt)
//   - shortPaperLabel(title) => string       (optional; truncates a paper title for inline mention)

(function () {
    'use strict';

    // ---------------------------------------------------------------------------
    // Internal helpers
    // ---------------------------------------------------------------------------

    function escapeHTML(value) {
        // Delegate to global Utils if available; otherwise inline a safe fallback.
        if (typeof Utils !== 'undefined' && typeof Utils.escapeHTML === 'function') {
            return Utils.escapeHTML(value);
        }
        return String(value || '')
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    function tr(key, fallback) {
        if (typeof t === 'function') return t(key, fallback);
        return fallback || key;
    }

    function defaultShortLabel(title) {
        const text = String(title || '').trim();
        if (!text) return 'paper';
        const head = Array.from(text).slice(0, 5).join('');
        return text.length > head.length ? head + '…' : head;
    }

    // ---------------------------------------------------------------------------
    // Module
    // ---------------------------------------------------------------------------

    const Mention = {
        _state: {
            active: false,
            query: '',
            atIndex: -1,        // character position of the @ in the textarea value
            anchor: null,       // textarea element
            popover: null,
            providers: null,
            items: [],          // computed list (mixed papers + roles)
            activeIndex: 0,
            // triggerRange kept for compat with public API sketch; derived from atIndex
        },

        // Bound event handler references (so we can removeEventListener later).
        _handlers: null,

        // -----------------------------------------------------------------------
        // Public API
        // -----------------------------------------------------------------------

        attach(input, popover, providers) {
            if (!input || !popover || !providers) {
                console.warn('AIReader.mention.attach: missing required arguments');
                return;
            }

            // If already attached to different elements, detach first.
            this._detach();

            const s = this._state;
            s.anchor = input;
            s.popover = popover;
            s.providers = providers;

            // Build bound handler refs so we can cleanly remove them later.
            const onInput = () => this._onInput(input);
            const onKeyDown = (e) => this._onKeyDown(e, input);
            const onBlur = () => {
                // Small delay so click on a popover item fires before dismiss.
                s._blurTimer = window.setTimeout(() => this.dismiss(), 150);
            };
            const onFocus = () => {
                if (s._blurTimer) {
                    window.clearTimeout(s._blurTimer);
                    s._blurTimer = 0;
                }
            };
            const onPopoverMouseDown = (e) => {
                // Prevent textarea blur when clicking inside the popover.
                e.preventDefault();
                const li = e.target.closest('[data-mention-index]');
                if (li) {
                    const idx = Number(li.dataset.mentionIndex);
                    if (Number.isFinite(idx) && idx >= 0 && idx < s.items.length) {
                        this._commitChoice(input, s.items[idx]);
                    }
                }
            };

            input.addEventListener('input', onInput);
            input.addEventListener('keydown', onKeyDown);
            input.addEventListener('blur', onBlur);
            input.addEventListener('focus', onFocus);
            popover.addEventListener('mousedown', onPopoverMouseDown);

            this._handlers = { input, popover, onInput, onKeyDown, onBlur, onFocus, onPopoverMouseDown };
        },

        dismiss() {
            const s = this._state;
            s.active = false;
            if (s.popover) {
                s.popover.hidden = true;
                s.popover.innerHTML = '';
            }
            s.query = '';
            s.atIndex = -1;
            s.items = [];
            s.activeIndex = 0;
        },

        // -----------------------------------------------------------------------
        // Private — lifecycle
        // -----------------------------------------------------------------------

        _detach() {
            const h = this._handlers;
            if (!h) return;
            h.input.removeEventListener('input', h.onInput);
            h.input.removeEventListener('keydown', h.onKeyDown);
            h.input.removeEventListener('blur', h.onBlur);
            h.input.removeEventListener('focus', h.onFocus);
            h.popover.removeEventListener('mousedown', h.onPopoverMouseDown);
            this._handlers = null;
            this.dismiss();
        },

        // -----------------------------------------------------------------------
        // Private — trigger detection
        // -----------------------------------------------------------------------

        // Returns { atIndex, query } when the caret is positioned right after an
        // @-word (possibly empty) that was preceded by start-of-string or whitespace.
        // Returns null when the popover should not be open.
        _detectTrigger(input) {
            const value = input.value;
            const caret = input.selectionStart;
            if (!Number.isFinite(caret)) return null;
            const before = value.slice(0, caret);
            const match = before.match(/(^|[\s\n])@([^\s@\n]*)$/);
            if (!match) return null;
            const queryLen = match[2].length;
            const atIndex = before.length - queryLen - 1; // position of '@' character
            return { atIndex, query: match[2] };
        },

        // -----------------------------------------------------------------------
        // Private — input handler
        // -----------------------------------------------------------------------

        _onInput(input) {
            const trigger = this._detectTrigger(input);
            if (!trigger) {
                this.dismiss();
                return;
            }
            this._openWith(trigger.atIndex, trigger.query, input);
        },

        _openWith(atIndex, query, input) {
            const s = this._state;
            s.active = true;
            s.atIndex = atIndex;
            s.query = query;
            s.items = this._buildItems(query);
            s.activeIndex = 0;
            this._renderItems();
        },

        // -----------------------------------------------------------------------
        // Private — keyboard handler
        // -----------------------------------------------------------------------

        _onKeyDown(event, input) {
            const s = this._state;
            if (!s.active) return;

            const items = s.items;

            if (event.key === 'ArrowDown') {
                event.preventDefault();
                s.activeIndex = (s.activeIndex + 1) % Math.max(items.length, 1);
                this._refreshActive();
                return;
            }

            if (event.key === 'ArrowUp') {
                event.preventDefault();
                s.activeIndex = (s.activeIndex - 1 + items.length) % Math.max(items.length, 1);
                this._refreshActive();
                return;
            }

            if (event.key === 'Enter' || event.key === 'Tab') {
                const item = items[s.activeIndex];
                if (item) {
                    event.preventDefault();
                    this._commitChoice(input, item);
                }
                return;
            }

            if (event.key === 'Escape') {
                event.preventDefault();
                this.dismiss();
            }
        },

        // -----------------------------------------------------------------------
        // Private — build item list
        // -----------------------------------------------------------------------

        _buildItems(query) {
            const s = this._state;
            if (!s.providers) return [];
            const q = String(query || '').trim().toLowerCase();
            const items = [];

            // Role prompts first.
            const roles = typeof s.providers.getRolePrompts === 'function'
                ? s.providers.getRolePrompts()
                : [];
            roles
                .filter((role) => {
                    if (!q) return true;
                    return String(role.name || '').toLowerCase().includes(q);
                })
                .slice(0, 6)
                .forEach((role) => {
                    items.push({
                        type: 'role',
                        name: role.name,
                        title: '@' + role.name,
                        meta: String(role.description || role.prompt || '').slice(0, 80),
                        _raw: role,
                    });
                });

            // Papers second.
            const papers = typeof s.providers.getPapers === 'function'
                ? s.providers.getPapers()
                : [];
            papers
                .filter((paper) => {
                    if (!q) return true;
                    const haystack = [
                        paper.title,
                        paper.original_filename,
                        paper.group_name,
                        paper.doi,
                    ].filter(Boolean).join(' ').toLowerCase();
                    return haystack.includes(q);
                })
                .slice(0, 8)
                .forEach((paper) => {
                    const metaParts = [
                        paper.original_filename,
                        paper.group_name || tr('ai.paper_no_group', '未分组'),
                        tr('ai.paper_figures', '图片 ${count}').replace('${count}', paper.figure_count || 0),
                    ].filter(Boolean);
                    items.push({
                        type: 'paper',
                        id: paper.id,
                        title: paper.title,
                        meta: metaParts.join(' · '),
                        isCurrent: typeof s.providers.isCurrentPaper === 'function'
                            ? s.providers.isCurrentPaper(paper)
                            : false,
                        _raw: paper,
                    });
                });

            return items;
        },

        // -----------------------------------------------------------------------
        // Private — render popover
        // -----------------------------------------------------------------------

        _renderItems() {
            const s = this._state;
            if (!s.popover) return;

            const items = s.items;
            if (!items.length) {
                // No matches AND user hasn't typed anything → just stay closed
                // (avoids the popover popping up on stray "@" alone, including
                // when the browser autofills a leftover textarea value on load).
                if (!s.query) {
                    this.dismiss();
                    return;
                }
                s.popover.hidden = false;
                s.popover.innerHTML =
                    '<div class="ai-mention-empty">' +
                    escapeHTML(tr('ai.mention_empty', '没有匹配的文献或角色。')) +
                    '</div>';
                return;
            }

            const renderItem = (item, globalIndex) => {
                const active = globalIndex === s.activeIndex ? ' active' : '';
                const current = item.isCurrent ? ' is-current' : '';
                const badge = item.isCurrent
                    ? '<span class="ai-mention-item-badge">' +
                      escapeHTML(tr('ai.paper_active_badge', '当前文献')) +
                      '</span>'
                    : '';
                const icon = item.type === 'role' ? '@' : '📄';
                const dataset = item.type === 'role'
                    ? 'data-mention-type="role" data-role-name="' + escapeHTML(item.name) + '"'
                    : 'data-mention-type="paper" data-paper-id="' + item.id + '"';
                return '<li class="ai-mention-item' + active + current +
                    '" role="option" data-mention-index="' + globalIndex + '" ' + dataset + '>' +
                    '<span class="ai-mention-item-icon" aria-hidden="true">' + icon + '</span>' +
                    '<div class="ai-mention-item-body">' +
                    '<span class="ai-mention-item-title">' + escapeHTML(item.title) + '</span>' +
                    (item.meta ? '<span class="ai-mention-item-meta">' + escapeHTML(item.meta) + '</span>' : '') +
                    '</div>' +
                    badge +
                    '</li>';
            };

            const sections = [];
            const roleItems = items.filter((it) => it.type === 'role');
            const paperItems = items.filter((it) => it.type === 'paper');
            let cursor = 0;

            if (roleItems.length) {
                const roleHTML = roleItems.map((item) => renderItem(item, cursor++)).join('');
                sections.push(
                    '<div class="ai-mention-section" data-section="roles">' +
                    '<div class="ai-mention-section-title">' +
                    escapeHTML(tr('ai.mention_section_roles', '角色 Prompt')) +
                    '</div>' +
                    '<ul class="ai-mention-list" role="presentation">' + roleHTML + '</ul>' +
                    '</div>'
                );
            }
            if (paperItems.length) {
                const paperHTML = paperItems.map((item) => renderItem(item, cursor++)).join('');
                sections.push(
                    '<div class="ai-mention-section" data-section="papers">' +
                    '<div class="ai-mention-section-title">' +
                    escapeHTML(tr('ai.mention_section_papers', '文献')) +
                    '</div>' +
                    '<ul class="ai-mention-list" role="presentation">' + paperHTML + '</ul>' +
                    '</div>'
                );
            }

            s.popover.hidden = false;
            s.popover.innerHTML = sections.join('');
            this._scrollActiveIntoView();
        },

        // -----------------------------------------------------------------------
        // Private — active-item refresh (after keyboard nav)
        // -----------------------------------------------------------------------

        _refreshActive() {
            const s = this._state;
            if (!s.popover) return;
            s.popover.querySelectorAll('.ai-mention-item').forEach((el) => {
                el.classList.remove('active');
            });
            const target = s.popover.querySelector('[data-mention-index="' + s.activeIndex + '"]');
            if (target) target.classList.add('active');
            this._scrollActiveIntoView();
        },

        _scrollActiveIntoView() {
            const s = this._state;
            if (!s.popover) return;
            const target = s.popover.querySelector('.ai-mention-item.active');
            if (target) target.scrollIntoView({ block: 'nearest' });
        },

        // -----------------------------------------------------------------------
        // Private — commit a choice
        // -----------------------------------------------------------------------

        _commitChoice(input, item) {
            const s = this._state;
            if (!item || !input) return;

            const caret = input.selectionStart;
            const beforeAt = Number.isFinite(s.atIndex) && s.atIndex >= 0
                ? input.value.slice(0, s.atIndex)
                : input.value.slice(0, caret);
            const after = input.value.slice(caret);

            if (item.type === 'role') {
                const mention = '@' + item.name + ' ';
                input.value = beforeAt + mention + after;
                const newCaret = beforeAt.length + mention.length;
                input.setSelectionRange(newCaret, newCaret);
                if (s.providers && typeof s.providers.onPickRole === 'function') {
                    s.providers.onPickRole(item._raw || { name: item.name });
                }
            } else if (item.type === 'paper') {
                const shortFn = s.providers && typeof s.providers.shortPaperLabel === 'function'
                    ? s.providers.shortPaperLabel
                    : defaultShortLabel;
                const shortLabel = shortFn(item.title || '');
                const mention = '@' + shortLabel + ' ';
                input.value = beforeAt + mention + after;
                const newCaret = beforeAt.length + mention.length;
                input.setSelectionRange(newCaret, newCaret);
                if (s.providers && typeof s.providers.onPickPaper === 'function') {
                    s.providers.onPickPaper(item._raw || { id: item.id, title: item.title });
                }
            }

            this.dismiss();
            input.focus();
            // Dispatch an 'input' event so any mirror / highlight logic updates.
            input.dispatchEvent(new Event('input', { bubbles: true }));
        },
    };

    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.mention = Mention;
    }
})();
