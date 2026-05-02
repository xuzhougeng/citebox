// ai-pin.js — pin chip area + picker.
// Public API: window.AIReader.pin.{init, setPinned, refresh, pin, unpin, openPicker}

(function () {
    'use strict';

    function escapeHtml(s) {
        return String(s == null ? '' : s)
            .replace(/&/g, '&amp;').replace(/</g, '&lt;')
            .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    const Pin = {
        _state: {
            container: null,
            getConversationId: null,
            getPinned: null,
            setPinnedCallback: null,
            pickerOverlay: null,
            pickerSearchTimer: null,
        },

        init(opts) {
            const s = this._state;
            s.container = opts.container;
            s.getConversationId = opts.getConversationId || (() => null);
            s.getPinned = opts.getPinned || (() => []);
            s.setPinnedCallback = opts.setPinnedCallback || (() => {});
            const self = this;
            if (s.container) {
                s.container.addEventListener('click', function (e) {
                    const action = e.target && e.target.dataset && e.target.dataset.action;
                    if (action === 'unpin') {
                        const pid = parseInt(e.target.dataset.paperId, 10);
                        if (pid) self.unpin(pid);
                    } else if (action === 'open-picker' || (e.target.closest && e.target.closest('[data-action="open-picker"]'))) {
                        self.openPicker();
                    }
                });
            }
        },

        setPinned(papers) {
            const list = Array.isArray(papers) ? papers : [];
            const s = this._state;
            if (!s.container) return;
            const chips = list.map((p) =>
                `<span class="ai-pin-chip" data-paper-id="${escapeHtml(p.paper_id)}">` +
                  `<span title="${escapeHtml(p.title || '未命名')}">📌 ${escapeHtml(p.title || '未命名')}</span>` +
                  `<button class="ai-pin-chip-remove" type="button" data-action="unpin" data-paper-id="${escapeHtml(p.paper_id)}" aria-label="移除">×</button>` +
                `</span>`
            ).join('');
            const draft = !s.getConversationId();
            const addChip = draft ? '' :
                `<span class="ai-pin-chip add" data-action="open-picker" role="button" tabindex="0">+ pin</span>`;
            s.container.innerHTML = chips + addChip;
        },

        async refresh() {
            const s = this._state;
            const id = s.getConversationId();
            if (!id) return;
            try {
                const res = await fetch('/api/ai/conversations/' + id);
                if (!res.ok) return;
                const conv = await res.json();
                this.setPinned(conv.pinned_papers || []);
                s.setPinnedCallback(conv.pinned_papers || []);
            } catch (e) {
                console.error('[ai-pin] refresh', e);
            }
        },

        async pin(paperID) {
            const s = this._state;
            const id = s.getConversationId();
            if (!id) {
                console.warn('[ai-pin] pin requires an existing conversation');
                return;
            }
            try {
                const res = await fetch('/api/ai/conversations/' + id + '/papers', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ paper_id: paperID }),
                });
                if (!res.ok) {
                    const body = await res.text();
                    if (typeof Utils !== 'undefined' && Utils.showToast) {
                        Utils.showToast('pin 失败: ' + body, 'error');
                    } else {
                        console.error('[ai-pin] pin failed', body);
                    }
                    return;
                }
                await this.refresh();
            } catch (e) {
                console.error('[ai-pin] pin error', e);
            }
        },

        async unpin(paperID) {
            const s = this._state;
            const id = s.getConversationId();
            if (!id) return;
            try {
                await fetch('/api/ai/conversations/' + id + '/papers/' + paperID, { method: 'DELETE' });
                const next = (s.getPinned() || []).filter((p) => p.paper_id !== paperID);
                this.setPinned(next);
                s.setPinnedCallback(next);
            } catch (e) {
                console.error('[ai-pin] unpin error', e);
            }
        },

        openPicker() {
            const self = this;
            this._closePicker();
            const overlay = document.createElement('div');
            overlay.className = 'modal-shell ai-pin-picker';
            overlay.innerHTML = `
                <div class="modal-dialog ai-pin-picker-dialog">
                    <button class="modal-close" type="button" aria-label="关闭">×</button>
                    <h3>Pin 文献</h3>
                    <input class="form-input ai-pin-picker-search" type="text" placeholder="搜索文献…" autocomplete="off">
                    <ul class="ai-pin-picker-results" role="list"></ul>
                </div>
            `;
            document.body.appendChild(overlay);
            this._state.pickerOverlay = overlay;
            const search = overlay.querySelector('.ai-pin-picker-search');
            const results = overlay.querySelector('.ai-pin-picker-results');
            search.focus();

            search.addEventListener('input', function () {
                clearTimeout(self._state.pickerSearchTimer);
                self._state.pickerSearchTimer = setTimeout(() => self._runPickerSearch(search.value, results), 200);
            });
            results.addEventListener('click', function (e) {
                const li = e.target.closest('li[data-paper-id]');
                if (!li) return;
                const pid = parseInt(li.dataset.paperId, 10);
                if (pid) self.pin(pid).then(() => self._closePicker());
            });
            overlay.querySelector('.modal-close').addEventListener('click', () => self._closePicker());
            overlay.addEventListener('click', function (e) {
                if (e.target === overlay) self._closePicker();
            });
            document.addEventListener('keydown', self._onPickerKey = function (e) {
                if (e.key === 'Escape') self._closePicker();
            });

            self._runPickerSearch('', results);
        },

        _closePicker() {
            const s = this._state;
            if (s.pickerOverlay && s.pickerOverlay.parentNode) {
                s.pickerOverlay.parentNode.removeChild(s.pickerOverlay);
            }
            s.pickerOverlay = null;
            if (this._onPickerKey) document.removeEventListener('keydown', this._onPickerKey);
        },

        async _runPickerSearch(q, resultsEl) {
            try {
                const url = '/api/papers?limit=20' + (q ? '&q=' + encodeURIComponent(q) : '');
                const res = await fetch(url);
                if (!res.ok) {
                    resultsEl.innerHTML = '<li>加载失败</li>';
                    return;
                }
                const body = await res.json();
                const list = Array.isArray(body.items) ? body.items : (Array.isArray(body) ? body : (body.papers || []));
                if (!list.length) {
                    resultsEl.innerHTML = '<li class="empty">没有找到文献</li>';
                    return;
                }
                resultsEl.innerHTML = list.map((p) =>
                    `<li data-paper-id="${escapeHtml(p.id)}" class="ai-pin-picker-result">` +
                      `<div class="title">${escapeHtml(p.title || '未命名')}</div>` +
                      (p.doi ? `<div class="meta">DOI: ${escapeHtml(p.doi)}</div>` : '') +
                    `</li>`
                ).join('');
            } catch (e) {
                resultsEl.innerHTML = '<li>加载失败</li>';
            }
        },
    };

    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.pin = Pin;
    }
})();
