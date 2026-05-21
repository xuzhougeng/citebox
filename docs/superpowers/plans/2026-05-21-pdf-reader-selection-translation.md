# PDF Reader Selection Translation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace paper PDF iframe viewing with a CiteBox pdf.js reader that supports page navigation, zoom, fit width, explicit detail links, and selected-text translation.

**Architecture:** Extend the existing `/viewer` resource controller so image viewing stays unchanged and `kind=pdf` uses a new PDF-reader branch. Keep URL construction in `Utils`, keep paper-library actions in `LibraryPage`, and reuse the existing translation dialog from `DesktopTranslate`.

**Tech Stack:** Plain JavaScript, pdf.js 5.5.207 `TextLayer`, existing CiteBox i18n JSON, CSS in `viewer.html`, Node built-in test runner.

---

## File Map

- `web/static/js/utils.js`: accept optional resource viewer query parameters such as `paper_id`.
- `web/static/js/__tests__/utils-resource-viewer.test.cjs`: regression tests for resource viewer URL building and parsing.
- `web/static/js/library.js`: open PDF reader from file metadata and add explicit detail action.
- `web/manual.html`: add `详情页` link next to PDF action.
- `web/static/js/manual.js`: set manual detail link and pass `paper_id` to reader URL.
- `web/viewer.html`: add PDF toolbar controls, translation menu markup support, load shared API/Utils/translate scripts.
- `web/static/js/viewer.js`: implement PDF reader state, pdf.js loading/rendering, text layer, selection menu, copy, translate, and detail link.
- `web/static/locales/zh-CN/*.json` and `web/static/locales/en/*.json`: add aligned viewer, library, and manual strings.

## Task 1: Resource Viewer URL Metadata

**Files:**
- Modify: `web/static/js/utils.js`
- Create: `web/static/js/__tests__/utils-resource-viewer.test.cjs`

- [x] **Step 1: Write the failing test**

Create `web/static/js/__tests__/utils-resource-viewer.test.cjs` with tests that load `Utils` in a VM, call `resourceViewerURL('pdf', '/files/papers/a.pdf', '/library', { paperId: 42 })`, and assert the URL contains `paper_id=42`. Also parse the URL and assert `parseResourceViewerNavigationURL` returns `{ kind: 'pdf', src: '/files/papers/a.pdf', back: '/library', paperId: 42 }`.

- [x] **Step 2: Run the failing test**

Run: `node --test web/static/js/__tests__/utils-resource-viewer.test.cjs`

Expected: FAIL because `paper_id` is not present and the parsed result has no `paperId`.

- [x] **Step 3: Implement URL option support**

Update `Utils.resourceViewerURL(kind, src, back, options)` to append `paper_id` when `options.paperId` is a positive number. Update `Utils.buildResourceViewerNavigationURL`, `Utils.openResourceViewer`, and `Utils.parseResourceViewerNavigationURL` to preserve and return the `paperId` field.

- [x] **Step 4: Verify the test passes**

Run: `node --test web/static/js/__tests__/utils-resource-viewer.test.cjs`

Expected: PASS.

## Task 2: Library and Manual Entrypoints

**Files:**
- Modify: `web/static/js/library.js`
- Modify: `web/manual.html`
- Modify: `web/static/js/manual.js`
- Modify: `web/static/locales/zh-CN/library.json`
- Modify: `web/static/locales/en/library.json`
- Modify: `web/static/locales/zh-CN/manual.json`
- Modify: `web/static/locales/en/manual.json`

- [x] **Step 1: Write the failing check**

Run: `rg -n "data-action=\"open-pdf\"|manualDetailLink|library.btn_detail" web/static/js/library.js web/manual.html web/static/js/manual.js web/static/locales/zh-CN/library.json web/static/locales/en/library.json`

Expected: no matches before implementation.

- [x] **Step 2: Update library actions**

In `web/static/js/library.js`, change the file metadata action from `data-action="open"` to `data-action="open-pdf"` and set the title to a new i18n key for opening the PDF reader. In the click handler, handle `open-pdf` by finding the paper in `this.state.papers`, validating `paper.pdf_url`, and navigating to `Utils.resourceViewerURL('pdf', paper.pdf_url, window.location.href, { paperId })`. Add an explicit footer button with `data-action="open"` and key `library.btn_detail`.

- [x] **Step 3: Update manual actions**

In `web/manual.html`, add `<a id="manualDetailLink" class="btn btn-outline" href="/library" data-i18n="manual.detail_page">详情页</a>` next to `manualOpenPDFLink`. In `web/static/js/manual.js`, cache `manualDetailLink`, set `this.detailLink.href = /library?paper_id=<id>`, and set `manualOpenPDFLink` with `Utils.resourceViewerURL('pdf', paper.pdf_url, window.location.href, { paperId: paper.id })`.

- [x] **Step 4: Add i18n keys**

Add Chinese and English keys for `library.meta_click_pdf`, `library.btn_detail`, and `manual.detail_page`.

- [x] **Step 5: Run syntax checks**

Run:

```bash
node --check web/static/js/library.js
node --check web/static/js/manual.js
```

Expected: both commands exit 0.

## Task 3: PDF Reader UI and Rendering

**Files:**
- Modify: `web/viewer.html`
- Modify: `web/static/js/viewer.js`
- Modify: `web/static/locales/zh-CN/viewer.json`
- Modify: `web/static/locales/en/viewer.json`

- [x] **Step 1: Write the failing check**

Run: `rg -n "viewerPdfControls|renderPDFResource|viewer.pdf_prev|pdf-selection-menu" web/viewer.html web/static/js/viewer.js web/static/locales/zh-CN/viewer.json web/static/locales/en/viewer.json`

Expected: no matches before implementation.

- [x] **Step 2: Add reader controls and scripts**

In `web/viewer.html`, add a hidden PDF toolbar area with previous/next, page input, total page label, zoom out, zoom label, zoom in, fit-width, and detail link controls. Load `i18n.js`, `utils.js`, `api.js`, `translate.js`, then `viewer.js` so the viewer can reuse existing helpers.

- [x] **Step 3: Implement PDF state and loading**

In `web/static/js/viewer.js`, add a `pdfState` object with `pdfjsLib`, `loadingTask`, `pdfDocument`, `pageNumber`, `pageCount`, `scale`, `fitMode`, `renderToken`, `renderTask`, and `textLayer`. Add `ensurePDFJSReady`, `loadPDFDocument`, `renderPDFResource`, and `renderPDFPage` methods. Use `new pdfjsLib.TextLayer({ textContentSource, container, viewport }).render()` for selectable text.

- [x] **Step 4: Implement page and zoom controls**

Bind PDF toolbar controls in `init()`. Implement `goToPDFPage`, `setPDFScale`, `fitPDFWidth`, `syncPDFToolbar`, and cancellation of stale render tasks. Keep image viewer handlers guarded so they only act on image viewports.

- [x] **Step 5: Add CSS**

In the existing `web/viewer.html` style block, add PDF reader classes: `.viewer-pdf-controls`, `.viewer-pdf-page-input`, `.viewer-pdf-scroll`, `.viewer-pdf-page`, `.viewer-pdf-canvas`, `.viewer-pdf-text-layer`, and `.viewer-selection-menu`. Ensure text layer spans are transparent but selectable.

- [x] **Step 6: Add i18n keys**

Add Chinese and English keys for PDF labels, page controls, zoom controls, fit width, detail link, copy, translate, loading, and render failure messages.

- [x] **Step 7: Run syntax check**

Run: `node --check web/static/js/viewer.js`

Expected: exit 0.

## Task 4: Selection Copy and Translate

**Files:**
- Modify: `web/static/js/viewer.js`
- Modify: `web/viewer.html`
- Modify: `web/static/locales/zh-CN/viewer.json`
- Modify: `web/static/locales/en/viewer.json`

- [x] **Step 1: Write the failing check**

Run: `rg -n "showPDFSelectionMenu|translatePDFSelection|copyPDFSelection|viewer.pdf_translate_selection" web/static/js/viewer.js web/static/locales/zh-CN/viewer.json web/static/locales/en/viewer.json`

Expected: no matches before implementation.

- [x] **Step 2: Implement selection menu**

Add a hidden menu element in the PDF page markup with `data-pdf-selection-action="copy"` and `data-pdf-selection-action="translate"`. In `viewer.js`, listen for `selectionchange`, pointerup, and keyup to show the menu near the selection range when selected text belongs to the PDF page.

- [x] **Step 3: Implement copy and translate actions**

Implement `currentPDFSelectionText`, `copyPDFSelection`, and `translatePDFSelection`. Use `navigator.clipboard.writeText` when available with a hidden textarea fallback. Call `DesktopTranslate.translateText(text, { title: t('viewer.pdf_translate_title', 'PDF 划选翻译') })` for translation.

- [x] **Step 4: Verify syntax**

Run: `node --check web/static/js/viewer.js`

Expected: exit 0.

## Task 5: Final Verification

**Files:**
- All touched files.

- [x] **Step 1: Run focused JS tests**

Run:

```bash
node --test web/static/js/__tests__/utils-resource-viewer.test.cjs
node --check web/static/js/viewer.js
node --check web/static/js/library.js
node --check web/static/js/manual.js
```

Expected: all exit 0.

- [x] **Step 2: Run frontend smoke with local server**

Run `make run` or an available local server, open `/library`, `/manual?paper_id=<existing-id>`, and `/viewer?kind=pdf&src=<paper-pdf-url>&paper_id=<id>` in a browser. Confirm the reader renders a PDF page, controls work, file-name click opens reader, detail link opens paper details, and image viewer still opens images.

- [x] **Step 3: Decide whether backend tests are needed**

If no Go files changed, record that `go test ./...` was not required for this frontend-only change. If any Go routing changes were added, run `go test ./...`.

## Self-Review

- Spec coverage: tasks cover pdf.js reader, text layer, navigation, zoom, selected-text copy/translate, library entry change, explicit detail action, manual detail entry, i18n, and verification.
- Placeholder scan: no `TBD`, `TODO`, or deferred implementation instructions remain.
- Type consistency: `paperId`, `paper_id`, `resourceViewerURL`, `parseResourceViewerNavigationURL`, and `DesktopTranslate.translateText` are named consistently with existing code.
