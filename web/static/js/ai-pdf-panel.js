// ai-pdf-panel.js — right-side literature panel for the AI assistant page.
//
// Features:
//   - Embedded PDF preview of a pinned paper (pdf.js viewer + text layer).
//   - Text selection → floating "引用到提问" button → excerpt chips.
//   - Figure grid with checkboxes → checked figures ride as vision context.
//   - Context chips tray above the composer; payload merges into
//     body.context ({ figure_ids, excerpts }) on send.
//
// Public API: window.AIReader.pdfPanel
//   init(opts) / open(paperId?) / close() / toggle()
//   refreshPinned(papers) / getContextPayload() / consumeExcerpts()
//   hasContext()
// Pure helper mergePanelContext(body, panelCtx) is exported for Node tests.
(function (browserRoot, factory) {
    'use strict';

    const api = factory(browserRoot);
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    }
    if (browserRoot) {
        browserRoot.AIReader = browserRoot.AIReader || {};
        browserRoot.AIReader.pdfPanel = api.Panel;
        browserRoot.AIReader.pdfPanelMergeContext = api.mergePanelContext;
    }
})(typeof window !== 'undefined' ? window : null, function (root) {
    'use strict';

    const MAX_EXCERPTS = 8;
    const MAX_EXCERPT_CHARS = 2000;
    const STORAGE_KEY = 'citebox_ai_pdf_panel';

    function t(key, fallback) {
        if (root && typeof root.t === 'function') return root.t(key, fallback);
        return fallback || key;
    }

    function escapeHtml(s) {
        return String(s == null ? '' : s)
            .replace(/&/g, '&amp;').replace(/</g, '&lt;')
            .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    // api.js / utils.js declare `const API` / `const Utils` at classic-script
    // top level: reachable as bare globals but NOT as window properties.
    // Resolve through both paths so the panel works regardless.
    function apiClient() {
        if (root && root.API) return root.API;
        if (typeof API !== 'undefined') return API;
        return null;
    }

    function utilsRef() {
        if (root && root.Utils) return root.Utils;
        if (typeof Utils !== 'undefined') return Utils;
        return null;
    }

    function toast(message, kind) {
        const u = utilsRef();
        if (u && typeof u.showToast === 'function') u.showToast(message, kind);
    }

    function truncateText(s, n) {
        const text = String(s || '');
        return text.length <= n ? text : text.slice(0, n) + '…';
    }

    // mergePanelContext folds the panel's checked figures + text excerpts into
    // an outgoing message body (mutates and returns body). Figure IDs merge as
    // a de-duplicated union with any @figure-N mentions already extracted;
    // excerpts are capped defensively to mirror the backend limits.
    function mergePanelContext(body, panelCtx) {
        if (!body || !panelCtx) return body;
        const existing = Array.isArray(body.context && body.context.figure_ids)
            ? body.context.figure_ids : [];
        const extra = Array.isArray(panelCtx.figure_ids) ? panelCtx.figure_ids : [];
        const seen = new Set();
        const merged = [];
        existing.concat(extra).forEach((id) => {
            const n = Number(id);
            if (!Number.isFinite(n) || n <= 0 || seen.has(n)) return;
            seen.add(n);
            merged.push(n);
        });
        if (merged.length > 0) {
            body.context = body.context || {};
            body.context.figure_ids = merged;
        }

        const excerpts = Array.isArray(panelCtx.excerpts) ? panelCtx.excerpts : [];
        const cleaned = [];
        excerpts.forEach((entry) => {
            if (cleaned.length >= MAX_EXCERPTS) return;
            const text = String(entry && entry.text || '').trim().slice(0, MAX_EXCERPT_CHARS);
            if (!text) return;
            const out = { text: text };
            const paperID = Number(entry && entry.paper_id || 0);
            const page = Number(entry && entry.page || 0);
            if (Number.isFinite(paperID) && paperID > 0) out.paper_id = paperID;
            if (Number.isFinite(page) && page > 0) out.page = page;
            cleaned.push(out);
        });
        if (cleaned.length > 0) {
            body.context = body.context || {};
            body.context.excerpts = cleaned;
        }
        return body;
    }

    const Panel = {
        _state: {
            shell: null,
            panel: null,
            toggleBtn: null,
            tray: null,
            pinChips: null,
            paperSelect: null,
            figCountEl: null,
            pdfSection: null,
            figuresSection: null,
            emptySection: null,
            figureGrid: null,
            loadingEl: null,
            quoteFab: null,
            open: false,
            activeTab: 'pdf',
            pinned: [],
            currentPaperId: 0,
            paperDetail: null,
            selectedFigures: new Map(), // figure_id -> {id,label,image_url,caption}
            excerpts: [],               // [{paper_id,page,text}]
            pdf: null,                  // pdf.js wiring state
            pdfLoadToken: 0,
            selectionTimer: 0,
            selectionText: '',
            selectionPage: 0,
            resizeObserver: null,
            listenersBound: false,
        },

        init(opts) {
            const s = this._state;
            opts = opts || {};
            s.shell = opts.shell || (root.document && root.document.querySelector('.ai-page-shell'));
            s.panel = opts.panel || (root.document && root.document.getElementById('aiPdfPanel'));
            s.toggleBtn = opts.toggleBtn || (root.document && root.document.getElementById('aiPdfPanelToggle'));
            s.tray = opts.tray || (root.document && root.document.getElementById('aiContextTray'));
            s.pinChips = opts.pinChips || (root.document && root.document.getElementById('aiPinChips'));
            if (!s.panel) return;

            this._buildSkeleton();
            this._bindListeners();

            const self = this;
            if (s.toggleBtn) {
                s.toggleBtn.addEventListener('click', function () { self.toggle(); });
            }
            this.bindTrayEvents();
            if (s.pinChips) {
                // Clicking a pin chip (outside its remove button) previews the paper.
                s.pinChips.addEventListener('click', function (event) {
                    const chip = event.target && event.target.closest
                        ? event.target.closest('.ai-pin-chip[data-paper-id]') : null;
                    if (!chip || (event.target.closest && event.target.closest('.ai-pin-chip-remove'))) return;
                    const pid = parseInt(chip.dataset.paperId, 10);
                    if (pid > 0) self.open(pid);
                });
            }
            this._syncToggleAvailability();
        },

        // ---- skeleton -------------------------------------------------------

        _buildSkeleton() {
            const s = this._state;
            s.panel.className = 'ai-pdf-panel';
            s.panel.innerHTML = `
                <header class="ai-pdf-panel-head">
                    <select class="ai-pdf-panel-paper" data-role="paper-select"
                        aria-label="${escapeHtml(t('ai.pdf_panel_switch_paper', '切换预览文献'))}"></select>
                    <div class="ai-pdf-panel-tabs" role="tablist">
                        <button type="button" class="ai-pdf-panel-tab is-active" data-role="tab" data-tab="pdf"
                            data-i18n="ai.pdf_panel_tab_pdf">PDF</button>
                        <button type="button" class="ai-pdf-panel-tab" data-role="tab" data-tab="figures">
                            ${escapeHtml(t('ai.pdf_panel_tab_figures', '图片'))}
                            <span class="ai-pdf-panel-figcount" data-role="figcount" hidden></span>
                        </button>
                    </div>
                    <button type="button" class="ai-pdf-panel-close" data-role="close"
                        aria-label="${escapeHtml(t('ai.pdf_panel_close', '关闭面板'))}">×</button>
                </header>
                <div class="ai-pdf-panel-body">
                    <div class="ai-pdf-panel-pdf" data-role="pdf-section">
                        <div class="ai-pdf-scroll" data-role="pdf-scroll" tabindex="0">
                            <div class="pdfViewer ai-pdf-viewer" data-role="pdf-viewer"></div>
                            <div class="ai-pdf-loading" data-role="pdf-loading" hidden>
                                ${escapeHtml(t('ai.pdf_panel_loading', '正在加载 PDF…'))}
                            </div>
                        </div>
                    </div>
                    <div class="ai-pdf-panel-figures" data-role="figures-section" hidden>
                        <div class="ai-pdf-figure-grid" data-role="figure-grid"></div>
                    </div>
                    <div class="ai-pdf-panel-empty" data-role="empty-section" hidden>
                        <p>${escapeHtml(t('ai.pdf_panel_empty', '先 pin 一篇文献，即可在此预览 PDF、划选引用、勾选图片作为提问上下文。'))}</p>
                    </div>
                </div>`;
            s.paperSelect = s.panel.querySelector('[data-role="paper-select"]');
            s.figCountEl = s.panel.querySelector('[data-role="figcount"]');
            s.pdfSection = s.panel.querySelector('[data-role="pdf-section"]');
            s.figuresSection = s.panel.querySelector('[data-role="figures-section"]');
            s.emptySection = s.panel.querySelector('[data-role="empty-section"]');
            s.figureGrid = s.panel.querySelector('[data-role="figure-grid"]');
            s.loadingEl = s.panel.querySelector('[data-role="pdf-loading"]');

            // Floating quote button (fixed-position, single instance).
            const fab = root.document.createElement('button');
            fab.type = 'button';
            fab.className = 'ai-pdf-quote-fab hidden';
            fab.textContent = t('ai.pdf_panel_quote_action', '引用到提问');
            s.quoteFab = fab;
            root.document.body.appendChild(fab);
        },

        _bindListeners() {
            const s = this._state;
            if (s.listenersBound) return;
            s.listenersBound = true;
            const self = this;

            s.panel.querySelector('[data-role="close"]').addEventListener('click', function () { self.close(); });
            s.paperSelect.addEventListener('change', function () {
                const pid = parseInt(s.paperSelect.value, 10);
                if (pid > 0) self._activatePaper(pid);
            });
            s.panel.querySelectorAll('[data-role="tab"]').forEach(function (btn) {
                btn.addEventListener('click', function () { self._setTab(btn.dataset.tab); });
            });

            // Figure grid: checkbox toggles selection; clicking the thumbnail previews.
            s.figureGrid.addEventListener('click', function (event) {
                const card = event.target && event.target.closest
                    ? event.target.closest('[data-figure-id]') : null;
                if (!card) return;
                const fid = parseInt(card.dataset.figureId, 10);
                if (!(fid > 0)) return;
                if (event.target.closest && event.target.closest('input[type="checkbox"]')) {
                    // Checkbox change handler below owns the state flip.
                    return;
                }
                if (event.target.closest && event.target.closest('.ai-pdf-figure-check')) {
                    return;
                }
                self._previewFigure(card.dataset.imageUrl || '');
            });
            s.figureGrid.addEventListener('change', function (event) {
                const input = event.target;
                if (!input || input.type !== 'checkbox') return;
                const card = input.closest('[data-figure-id]');
                if (!card) return;
                const fid = parseInt(card.dataset.figureId, 10);
                self._setFigureSelected(fid, input.checked, {
                    id: fid,
                    label: card.dataset.label || ('figure-' + fid),
                    image_url: card.dataset.imageUrl || '',
                    caption: card.dataset.caption || '',
                });
            });

            // Quote fab: keep the text selection alive while clicking.
            s.quoteFab.addEventListener('mousedown', function (event) { event.preventDefault(); });
            s.quoteFab.addEventListener('click', function () { self._commitSelection(); });

            // Selection tracking, scoped to the panel's PDF scroll container.
            root.document.addEventListener('selectionchange', function () { self._scheduleSelectionRefresh(); });
            const scroll = s.panel.querySelector('[data-role="pdf-scroll"]');
            scroll.addEventListener('pointerup', function () { self._scheduleSelectionRefresh(); });

            // Keep the PDF fitted while the panel resizes.
            if (typeof root.ResizeObserver === 'function') {
                let resizeTimer = 0;
                s.resizeObserver = new root.ResizeObserver(function () {
                    root.clearTimeout(resizeTimer);
                    resizeTimer = root.setTimeout(function () { self._refitPDF(); }, 150);
                });
                s.resizeObserver.observe(scroll);
            }
        },

        // ---- open / close ----------------------------------------------------

        isOpen() {
            return this._state.open;
        },

        open(paperId) {
            const s = this._state;
            if (s.open) {
                if (paperId && paperId !== s.currentPaperId) this._activatePaper(paperId);
                return;
            }
            s.open = true;
            s.panel.hidden = false;
            if (s.shell) s.shell.classList.add('ai-pdf-open');
            this._persistOpen(true);
            const target = paperId || s.currentPaperId
                || (s.pinned.length ? Number(s.pinned[0].paper_id) : 0);
            this._syncBody();
            if (target > 0) this._activatePaper(target);
        },

        close() {
            const s = this._state;
            s.open = false;
            s.panel.hidden = true;
            if (s.shell) s.shell.classList.remove('ai-pdf-open');
            this._persistOpen(false);
            this._hideQuoteFab();
        },

        toggle() {
            if (this._state.open) {
                this.close();
            } else {
                this.open();
            }
        },

        _persistOpen(open) {
            try {
                root.localStorage.setItem(STORAGE_KEY, open ? '1' : '0');
            } catch (e) { /* ignore */ }
        },

        _restoreOpen() {
            try {
                return root.localStorage.getItem(STORAGE_KEY) === '1';
            } catch (e) {
                return false;
            }
        },

        // ---- pinned papers ----------------------------------------------------

        refreshPinned(papers) {
            const s = this._state;
            s.pinned = Array.isArray(papers) ? papers : [];
            const ids = new Set(s.pinned.map((p) => Number(p && p.paper_id || 0)));
            if (s.currentPaperId && !ids.has(s.currentPaperId)) {
                s.currentPaperId = s.pinned.length ? Number(s.pinned[0].paper_id) : 0;
                s.paperDetail = null;
            }
            this._renderPaperSelect();
            this._syncToggleAvailability();
            this._syncBody();

            if (s.open && s.currentPaperId > 0) {
                this._activatePaper(s.currentPaperId);
            } else if (!s.open && this._restoreOpen() && s.pinned.length > 0) {
                this.open();
            }
        },

        _renderPaperSelect() {
            const s = this._state;
            if (!s.paperSelect) return;
            s.paperSelect.innerHTML = s.pinned.map((p) => {
                const id = Number(p && p.paper_id || 0);
                const title = (p && p.title) || ('#' + id);
                return `<option value="${escapeHtml(id)}"${id === s.currentPaperId ? ' selected' : ''}>${escapeHtml(truncateText(title, 42))}</option>`;
            }).join('');
            s.paperSelect.hidden = s.pinned.length <= 1;
        },

        _syncToggleAvailability() {
            const s = this._state;
            if (!s.toggleBtn) return;
            const hasPapers = s.pinned.length > 0;
            s.toggleBtn.disabled = !hasPapers;
            s.toggleBtn.title = hasPapers
                ? t('ai.pdf_panel_toggle_hint', '预览 pin 文献的 PDF，划选引用、勾选图片作为上下文')
                : t('ai.pdf_panel_no_pin_hint', '先 pin 一篇文献');
        },

        _syncBody() {
            const s = this._state;
            const hasPaper = s.currentPaperId > 0;
            s.emptySection.hidden = hasPaper;
            s.pdfSection.hidden = !hasPaper || s.activeTab !== 'pdf';
            s.figuresSection.hidden = !hasPaper || s.activeTab !== 'figures';
        },

        _setTab(tab) {
            const s = this._state;
            s.activeTab = tab === 'figures' ? 'figures' : 'pdf';
            s.panel.querySelectorAll('[data-role="tab"]').forEach(function (btn) {
                btn.classList.toggle('is-active', btn.dataset.tab === s.activeTab);
            });
            this._syncBody();
            if (s.activeTab !== 'pdf') this._hideQuoteFab();
        },

        // ---- paper activation -------------------------------------------------

        async _activatePaper(paperId) {
            const s = this._state;
            paperId = Number(paperId || 0);
            if (!(paperId > 0)) return;
            s.currentPaperId = paperId;
            this._renderPaperSelect();
            this._syncBody();
            this._hideQuoteFab();

            let detail = null;
            const api = apiClient();
            if (api && typeof api.getPaper === 'function') {
                try {
                    detail = await api.getPaper(paperId);
                } catch (e) {
                    console.error('[ai-pdf-panel] getPaper failed', e);
                }
            }
            if (s.currentPaperId !== paperId) return; // switched meanwhile
            s.paperDetail = detail;
            this._renderFigures(detail);
            this._loadPDF(detail);
        },

        // ---- PDF preview ------------------------------------------------------

        async _ensurePDFViewerReady() {
            const s = this._state;
            if (!s.pdf) {
                s.pdf = {
                    pdfjsLib: null, pdfjsViewerLib: null,
                    pdfDocument: null, pdfViewer: null,
                    eventBus: null, linkService: null,
                    loadingTask: null, pageNumber: 1, pagesReady: false,
                };
            }
            if (!s.pdf.pdfjsLib) {
                const pdfjsLib = await import('/static/vendor/pdfjs/build/pdf.mjs');
                pdfjsLib.GlobalWorkerOptions.workerSrc = '/static/vendor/pdfjs/build/pdf.worker.mjs';
                s.pdf.pdfjsLib = pdfjsLib;
            }
            if (!s.pdf.pdfjsViewerLib) {
                this._ensurePDFViewerCompatibility();
                this._ensurePDFViewerStyles();
                s.pdf.pdfjsViewerLib = await import('/static/vendor/pdfjs/web/pdf_viewer.mjs');
            }
            return s.pdf;
        },

        _ensurePDFViewerCompatibility() {
            if (typeof Map === 'undefined' || typeof Map.prototype.getOrInsertComputed === 'function') return;
            Object.defineProperty(Map.prototype, 'getOrInsertComputed', {
                configurable: true,
                writable: true,
                value(key, callback) {
                    if (this.has(key)) {
                        return this.get(key);
                    }
                    const value = callback(key);
                    this.set(key, value);
                    return value;
                }
            });
        },

        _ensurePDFViewerStyles() {
            if (root.document.getElementById('pdfjsViewerStylesheet')) return;
            const link = root.document.createElement('link');
            link.id = 'pdfjsViewerStylesheet';
            link.rel = 'stylesheet';
            link.href = '/static/vendor/pdfjs/web/pdf_viewer.css';
            root.document.head.appendChild(link);
        },

        async _loadPDF(detail) {
            const s = this._state;
            const paperId = s.currentPaperId;
            const token = ++s.pdfLoadToken;
            const scroll = s.panel.querySelector('[data-role="pdf-scroll"]');
            const viewerEl = s.panel.querySelector('[data-role="pdf-viewer"]');

            this._teardownPDFDocument();
            // Clear stale page divs before constructing a fresh PDFViewer.
            viewerEl.innerHTML = '';
            const href = detail && String(detail.pdf_url || '').trim();
            if (!href || href.indexOf('/files/papers/') !== 0) {
                if (s.loadingEl) {
                    s.loadingEl.hidden = false;
                    s.loadingEl.textContent = t('ai.pdf_panel_no_pdf', '该文献没有可预览的 PDF。');
                }
                return;
            }
            if (s.loadingEl) {
                s.loadingEl.hidden = false;
                s.loadingEl.textContent = t('ai.pdf_panel_loading', '正在加载 PDF…');
            }

            const pdf = await this._ensurePDFViewerReady();
            if (token !== s.pdfLoadToken) return;

            const eventBus = new pdf.pdfjsViewerLib.EventBus();
            const linkService = new pdf.pdfjsViewerLib.PDFLinkService({ eventBus: eventBus });
            const textLayerMode = pdf.pdfjsViewerLib.TextLayerMode && pdf.pdfjsViewerLib.TextLayerMode.ENABLE;
            const pdfViewer = new pdf.pdfjsViewerLib.PDFViewer({
                container: scroll,
                viewer: viewerEl,
                eventBus: eventBus,
                linkService: linkService,
                textLayerMode: textLayerMode == null ? 1 : textLayerMode,
                annotationMode: pdf.pdfjsLib.AnnotationMode && pdf.pdfjsLib.AnnotationMode.DISABLE != null
                    ? pdf.pdfjsLib.AnnotationMode.DISABLE : 0,
            });
            linkService.setViewer(pdfViewer);
            pdf.eventBus = eventBus;
            pdf.linkService = linkService;
            pdf.pdfViewer = pdfViewer;
            pdf.pagesReady = false;

            const self = this;
            eventBus.on('pagesinit', function () {
                if (token !== s.pdfLoadToken) return;
                pdf.pagesReady = true;
                pdfViewer.currentScaleValue = 'page-width';
                if (s.loadingEl) s.loadingEl.hidden = true;
            });
            eventBus.on('pagechanging', function () {
                if (token !== s.pdfLoadToken) return;
                pdf.pageNumber = pdfViewer.currentPageNumber || 1;
                self._hideQuoteFab();
            });

            const loadingTask = pdf.pdfjsLib.getDocument({
                url: href,
                cMapUrl: '/static/vendor/pdfjs/cmaps/',
                cMapPacked: true,
                standardFontDataUrl: '/static/vendor/pdfjs/standard_fonts/',
                wasmUrl: '/static/vendor/pdfjs/wasm/',
            });
            pdf.loadingTask = loadingTask;
            try {
                pdf.pdfDocument = await loadingTask.promise;
            } catch (err) {
                if (token !== s.pdfLoadToken) return;
                console.error('[ai-pdf-panel] pdf load failed', err);
                if (s.loadingEl) {
                    s.loadingEl.hidden = false;
                    s.loadingEl.textContent = t('ai.pdf_panel_load_error', 'PDF 加载失败。');
                }
                return;
            }
            if (token !== s.pdfLoadToken) return;
            if (paperId !== s.currentPaperId) return;
            pdfViewer.setDocument(pdf.pdfDocument);
            linkService.setDocument(pdf.pdfDocument, null);
        },

        _teardownPDFDocument() {
            const s = this._state;
            const pdf = s.pdf;
            if (!pdf) return;
            if (pdf.pdfViewer && typeof pdf.pdfViewer.setDocument === 'function') {
                try { pdf.pdfViewer.setDocument(null); } catch (e) { /* teardown race */ }
            }
            if (pdf.pdfDocument && typeof pdf.pdfDocument.destroy === 'function') {
                pdf.pdfDocument.destroy().catch(function () {});
            }
            if (pdf.loadingTask && typeof pdf.loadingTask.destroy === 'function') {
                pdf.loadingTask.destroy().catch(function () {});
            }
            pdf.pdfDocument = null;
            pdf.pdfViewer = null;
            pdf.eventBus = null;
            pdf.linkService = null;
            pdf.loadingTask = null;
            pdf.pagesReady = false;
        },

        _refitPDF() {
            const s = this._state;
            const pdf = s.pdf;
            if (!s.open || s.activeTab !== 'pdf') return;
            if (!pdf || !pdf.pdfViewer || !pdf.pagesReady) return;
            pdf.pdfViewer.currentScaleValue = 'page-width';
        },

        // ---- text selection → excerpts -----------------------------------------

        _scheduleSelectionRefresh() {
            const s = this._state;
            if (!s.open || s.activeTab !== 'pdf' || !s.pdf || !s.pdf.pdfDocument) return;
            root.clearTimeout(s.selectionTimer);
            const self = this;
            s.selectionTimer = root.setTimeout(function () { self._refreshSelection(); }, 0);
        },

        _selectionInsidePanel(selection) {
            const s = this._state;
            const scroll = s.panel.querySelector('[data-role="pdf-scroll"]');
            if (!scroll || !selection) return false;
            return this._nodeInside(selection.anchorNode, scroll) && this._nodeInside(selection.focusNode, scroll);
        },

        _nodeInside(node, element) {
            if (!node || !element) return false;
            const textNodeType = typeof Node !== 'undefined' ? Node.TEXT_NODE : 3;
            const candidate = node.nodeType === textNodeType ? node.parentElement : node;
            return !!candidate && typeof element.contains === 'function' && element.contains(candidate);
        },

        _selectionPageNumber(selection) {
            const node = selection && selection.anchorNode;
            const textNodeType = typeof Node !== 'undefined' ? Node.TEXT_NODE : 3;
            const candidate = node && (node.nodeType === textNodeType ? node.parentElement : node);
            const pageEl = candidate && candidate.closest ? candidate.closest('.page[data-page-number]') : null;
            const page = pageEl ? parseInt(pageEl.dataset.pageNumber, 10) : 0;
            return Number.isFinite(page) ? page : 0;
        },

        _refreshSelection() {
            const s = this._state;
            if (!s.open || s.activeTab !== 'pdf' || !s.pdf || !s.pdf.pdfDocument) return;
            const selection = root.getSelection && root.getSelection();
            const text = String(selection && selection.toString ? selection.toString() : '').trim();
            if (!text || !this._selectionInsidePanel(selection)) {
                this._hideQuoteFab();
                return;
            }
            if (!selection.rangeCount) {
                this._hideQuoteFab();
                return;
            }
            const rect = selection.getRangeAt(0).getBoundingClientRect();
            if (!rect || (!rect.width && !rect.height)) {
                this._hideQuoteFab();
                return;
            }
            s.selectionText = text;
            s.selectionPage = this._selectionPageNumber(selection);
            this._showQuoteFab(rect);
        },

        _showQuoteFab(rect) {
            const s = this._state;
            const fab = s.quoteFab;
            if (!fab) return;
            fab.classList.remove('hidden');
            fab.style.left = '0px';
            fab.style.top = '0px';
            const fabRect = fab.getBoundingClientRect();
            const left = Math.max(12, Math.min(rect.left + rect.width / 2 - fabRect.width / 2, root.innerWidth - fabRect.width - 12));
            const topCandidate = rect.top - fabRect.height - 8;
            const top = topCandidate > 12
                ? topCandidate
                : Math.min(root.innerHeight - fabRect.height - 12, rect.bottom + 8);
            fab.style.left = left + 'px';
            fab.style.top = Math.max(12, top) + 'px';
        },

        _hideQuoteFab() {
            const s = this._state;
            if (s.quoteFab) s.quoteFab.classList.add('hidden');
            s.selectionText = '';
            s.selectionPage = 0;
        },

        _commitSelection() {
            const s = this._state;
            const text = String(s.selectionText || '').trim();
            if (!text) {
                this._hideQuoteFab();
                return;
            }
            if (s.excerpts.length >= MAX_EXCERPTS) {
                toast(t('ai.pdf_panel_excerpt_full', '引用片段已达上限'), 'error');
            } else {
                s.excerpts.push({
                    paper_id: s.currentPaperId,
                    page: s.selectionPage || 0,
                    text: text.slice(0, MAX_EXCERPT_CHARS),
                });
                this._renderTray();
                toast(t('ai.pdf_panel_excerpt_added', '已加入引用'), 'success');
            }
            if (root.getSelection) {
                const selection = root.getSelection();
                if (selection && selection.removeAllRanges) selection.removeAllRanges();
            }
            this._hideQuoteFab();
        },

        // ---- figures tab --------------------------------------------------------

        _flattenFigures(detail) {
            const out = [];
            const figures = detail && Array.isArray(detail.figures) ? detail.figures : [];
            figures.forEach(function (fig) {
                out.push(fig);
                (Array.isArray(fig.subfigures) ? fig.subfigures : []).forEach(function (sub) {
                    out.push(sub);
                });
            });
            return out;
        },

        _figureLabel(fig) {
            const label = String(fig.display_label || '').trim();
            if (label) return label;
            let base = t('ai.pdf_panel_figure_label', '第 {page} 页图 {index}')
                .replace('{page}', fig.page_number || '?')
                .replace('{index}', fig.figure_index || '?');
            if (fig.subfigure_label) base += ' ' + fig.subfigure_label;
            return base;
        },

        _renderFigures(detail) {
            const s = this._state;
            const figures = this._flattenFigures(detail);

            if (!figures.length) {
                s.figureGrid.innerHTML = '<p class="ai-pdf-figures-empty">' +
                    escapeHtml(t('ai.pdf_panel_figures_empty', '本篇文献还没有提取图片。')) + '</p>';
            } else {
                const self = this;
                s.figureGrid.innerHTML = figures.map(function (fig) {
                    const fid = Number(fig.id);
                    const label = self._figureLabel(fig);
                    const checked = s.selectedFigures.has(fid) ? ' checked' : '';
                    return `
                        <figure class="ai-pdf-figure-card${checked ? ' is-selected' : ''}"
                            data-figure-id="${escapeHtml(fid)}"
                            data-label="${escapeHtml(label)}"
                            data-image-url="${escapeHtml(fig.image_url || '')}"
                            data-caption="${escapeHtml(fig.caption || '')}">
                            <div class="ai-pdf-figure-thumb">
                                <img src="${escapeHtml(fig.image_url || '')}" alt="${escapeHtml(label)}" loading="lazy">
                                <label class="ai-pdf-figure-check">
                                    <input type="checkbox"${checked}>
                                </label>
                            </div>
                            <figcaption title="${escapeHtml(fig.caption || '')}">${escapeHtml(label)}</figcaption>
                        </figure>`;
                }).join('');
            }
            this._syncFigureCount();
        },

        _setFigureSelected(fid, selected, info) {
            const s = this._state;
            if (selected) {
                s.selectedFigures.set(fid, info);
            } else {
                s.selectedFigures.delete(fid);
            }
            const card = s.figureGrid.querySelector('[data-figure-id="' + fid + '"]');
            if (card) card.classList.toggle('is-selected', !!selected);
            this._syncFigureCount();
            this._renderTray();
        },

        _syncFigureCount() {
            const s = this._state;
            const count = s.selectedFigures.size;
            if (s.figCountEl) {
                s.figCountEl.hidden = count === 0;
                s.figCountEl.textContent = count > 0 ? String(count) : '';
            }
        },

        _previewFigure(imageUrl) {
            if (!imageUrl) return;
            const modal = root.document.getElementById('aiGeneratedImageModal');
            const img = root.document.getElementById('aiGeneratedImagePreview');
            if (modal && img) {
                img.src = imageUrl;
                modal.classList.remove('hidden');
            }
        },

        // ---- context tray & payload ----------------------------------------------

        _renderTray() {
            const s = this._state;
            if (!s.tray) return;
            const parts = [];
            s.excerpts.forEach(function (excerpt, index) {
                const pageLabel = excerpt.page > 0
                    ? t('ai.context_tray_page', '第 {page} 页').replace('{page}', excerpt.page) + ' · '
                    : '';
                parts.push(
                    '<span class="ai-context-chip ai-context-chip-excerpt" data-excerpt-index="' + index + '">' +
                    '<span class="ai-context-chip-text" title="' + escapeHtml(excerpt.text) + '">' +
                    escapeHtml(pageLabel) + '“' + escapeHtml(truncateText(excerpt.text, 60)) + '”</span>' +
                    '<button type="button" class="ai-context-chip-remove" data-remove-excerpt="' + index + '"' +
                    ' aria-label="' + escapeHtml(t('ai.context_tray_remove_excerpt', '移除引用')) + '">×</button>' +
                    '</span>'
                );
            });
            s.selectedFigures.forEach(function (fig, fid) {
                parts.push(
                    '<span class="ai-context-chip ai-context-chip-figure" data-figure-id="' + escapeHtml(fid) + '">' +
                    (fig.image_url ? '<img src="' + escapeHtml(fig.image_url) + '" alt="">' : '') +
                    '<span class="ai-context-chip-text">' + escapeHtml(fig.label || ('figure-' + fid)) + '</span>' +
                    '<button type="button" class="ai-context-chip-remove" data-remove-figure="' + escapeHtml(fid) + '"' +
                    ' aria-label="' + escapeHtml(t('ai.context_tray_remove_figure', '移除图片')) + '">×</button>' +
                    '</span>'
                );
            });
            s.tray.innerHTML = parts.join('');
            s.tray.hidden = parts.length === 0;
        },

        // Called once from view: tray click delegation (remove chips).
        bindTrayEvents() {
            const s = this._state;
            if (!s.tray || s.tray.dataset.panelBound === '1') return;
            s.tray.dataset.panelBound = '1';
            const self = this;
            s.tray.addEventListener('click', function (event) {
                const removeExcerpt = event.target && event.target.dataset
                    ? event.target.dataset.removeExcerpt : undefined;
                const removeFigure = event.target && event.target.dataset
                    ? event.target.dataset.removeFigure : undefined;
                if (removeExcerpt !== undefined) {
                    const index = parseInt(removeExcerpt, 10);
                    if (Number.isFinite(index)) {
                        s.excerpts.splice(index, 1);
                        self._renderTray();
                    }
                } else if (removeFigure !== undefined) {
                    const fid = parseInt(removeFigure, 10);
                    self._setFigureSelected(fid, false, null);
                    const card = s.figureGrid
                        ? s.figureGrid.querySelector('[data-figure-id="' + fid + '"] input[type="checkbox"]')
                        : null;
                    if (card) card.checked = false;
                }
            });
        },

        hasContext() {
            const s = this._state;
            return s.excerpts.length > 0 || s.selectedFigures.size > 0;
        },

        getContextPayload() {
            const s = this._state;
            return {
                figure_ids: Array.from(s.selectedFigures.keys()),
                excerpts: s.excerpts.map(function (excerpt) {
                    return { paper_id: excerpt.paper_id, page: excerpt.page, text: excerpt.text };
                }),
            };
        },

        // Snapshot of what will be sent, for decorating the live user bubble.
        describeContext() {
            const s = this._state;
            return {
                figures: Array.from(s.selectedFigures.values()),
                excerpts: s.excerpts.slice(),
            };
        },

        // Excerpts are one-shot: clear them once a turn completes. Checked
        // figures stay checked until the user unticks them.
        consumeExcerpts() {
            const s = this._state;
            if (!s.excerpts.length) return;
            s.excerpts = [];
            this._renderTray();
        },
    };

    return {
        Panel: Panel,
        mergePanelContext: mergePanelContext,
        MAX_EXCERPTS: MAX_EXCERPTS,
        MAX_EXCERPT_CHARS: MAX_EXCERPT_CHARS,
    };
});
