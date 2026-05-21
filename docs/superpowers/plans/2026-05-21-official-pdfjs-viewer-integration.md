# Official PDF.js Viewer Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace CiteBox's custom PDF canvas/text selection path with the official PDF.js `PDFViewer` while keeping the CiteBox `/viewer` shell and selection translation menu.

**Architecture:** Keep `web/viewer.html` as the host page and instantiate the official PDF.js viewer inside `viewerStage` for `kind=pdf`. Toolbar actions call official viewer APIs, and copy/translate read `window.getSelection().toString().trim()` from the native browser selection instead of reconstructing text from selection rectangles.

**Tech Stack:** Go asset preparation script, vendored `pdfjs-dist`, plain HTML/CSS/JavaScript, PDF.js `PDFViewer`, Node `node:test`, Playwright manual smoke checks.

---

## File Map

- `scripts/fetch_pdfjs.go`: expand vendored PDF.js assets to include the modern build module and official viewer module/CSS.
- `scripts/fetch_pdfjs_test.go`: cover PDF.js asset path selection and readiness checks.
- `web/static/js/viewer.js`: replace the custom PDF render and selection geometry path with official `PDFViewer` integration plus native selection menu handling.
- `web/static/js/__tests__/viewer-pdf-selection.test.cjs`: replace custom geometry tests with native selection helper tests.
- `web/viewer.html`: remove custom text/selection layer CSS, keep the CiteBox toolbar, and add styles for the embedded official viewer.
- `docs/superpowers/plans/2026-05-21-official-pdfjs-viewer-integration.md`: execution checklist.

No locale files should change in this pass because the existing `viewer.*` keys already cover loading, page controls, zoom controls, copy, translate, and the detail page link.

## Task 1: Cover PDF.js Asset Selection

**Files:**
- Create: `scripts/fetch_pdfjs_test.go`
- Modify: `scripts/fetch_pdfjs.go`

- [ ] **Step 1: Add failing tests for official viewer assets**

Create `scripts/fetch_pdfjs_test.go` with:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectedPathIncludesOfficialViewerAssets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tarPath  string
		wantPath string
		wantOK   bool
	}{
		{name: "license", tarPath: "package/LICENSE", wantPath: "LICENSE", wantOK: true},
		{name: "modern build", tarPath: "package/build/pdf.mjs", wantPath: "build/pdf.mjs", wantOK: true},
		{name: "modern worker", tarPath: "package/build/pdf.worker.mjs", wantPath: "build/pdf.worker.mjs", wantOK: true},
		{name: "legacy build remains available", tarPath: "package/legacy/build/pdf.min.mjs", wantPath: "legacy/build/pdf.min.mjs", wantOK: true},
		{name: "official viewer module", tarPath: "package/web/pdf_viewer.mjs", wantPath: "web/pdf_viewer.mjs", wantOK: true},
		{name: "official viewer css", tarPath: "package/web/pdf_viewer.css", wantPath: "web/pdf_viewer.css", wantOK: true},
		{name: "viewer image asset", tarPath: "package/web/images/loading-icon.gif", wantPath: "web/images/loading-icon.gif", wantOK: true},
		{name: "cmaps", tarPath: "package/cmaps/78-EUC-H.bcmap", wantPath: "cmaps/78-EUC-H.bcmap", wantOK: true},
		{name: "unrelated web app file", tarPath: "package/web/viewer.html", wantPath: "", wantOK: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotPath, gotOK := selectedPath(tt.tarPath)
			if gotOK != tt.wantOK {
				t.Fatalf("selectedPath(%q) ok = %v, want %v", tt.tarPath, gotOK, tt.wantOK)
			}
			if gotPath != tt.wantPath {
				t.Fatalf("selectedPath(%q) path = %q, want %q", tt.tarPath, gotPath, tt.wantPath)
			}
		})
	}
}

func TestAssetsReadyRequiresOfficialViewerAssets(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	required := []string{
		"LICENSE",
		"build/pdf.mjs",
		"build/pdf.worker.mjs",
		"legacy/build/pdf.min.mjs",
		"legacy/build/pdf.worker.min.mjs",
		"web/pdf_viewer.mjs",
		"web/pdf_viewer.css",
		"cmaps/LICENSE",
		"standard_fonts/LiberationSans-Regular.ttf",
		"wasm/qcms_bg.wasm",
	}

	for _, relative := range required {
		fullPath := filepath.Join(targetDir, relative)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("asset"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ready, err := assetsReady(targetDir)
	if err != nil {
		t.Fatalf("assetsReady returned error: %v", err)
	}
	if !ready {
		t.Fatal("assetsReady returned false with every required asset present")
	}

	if err := os.Remove(filepath.Join(targetDir, "web/pdf_viewer.css")); err != nil {
		t.Fatal(err)
	}
	ready, err = assetsReady(targetDir)
	if err != nil {
		t.Fatalf("assetsReady returned error after removing css: %v", err)
	}
	if ready {
		t.Fatal("assetsReady returned true without web/pdf_viewer.css")
	}
}
```

- [ ] **Step 2: Run the asset tests and verify they fail**

Run:

```bash
go test ./scripts
```

Expected: FAIL. The first failure mentions `package/build/pdf.mjs` or `package/web/pdf_viewer.mjs` is not selected.

- [ ] **Step 3: Expand the asset script**

In `scripts/fetch_pdfjs.go`, update the `required` map inside `extractAssets` to:

```go
	required := map[string]bool{
		"LICENSE":                         false,
		"build/pdf.mjs":                   false,
		"build/pdf.worker.mjs":            false,
		"legacy/build/pdf.min.mjs":        false,
		"legacy/build/pdf.worker.min.mjs": false,
		"web/pdf_viewer.mjs":              false,
		"web/pdf_viewer.css":              false,
	}
```

Update `selectedPath` to include the official viewer assets while leaving existing cMap/font/wasm behavior intact:

```go
func selectedPath(tarPath string) (string, bool) {
	switch {
	case tarPath == "package/LICENSE":
		return "LICENSE", true
	case tarPath == "package/build/pdf.mjs":
		return "build/pdf.mjs", true
	case tarPath == "package/build/pdf.worker.mjs":
		return "build/pdf.worker.mjs", true
	case tarPath == "package/legacy/build/pdf.min.mjs":
		return "legacy/build/pdf.min.mjs", true
	case tarPath == "package/legacy/build/pdf.worker.min.mjs":
		return "legacy/build/pdf.worker.min.mjs", true
	case tarPath == "package/web/pdf_viewer.mjs":
		return "web/pdf_viewer.mjs", true
	case tarPath == "package/web/pdf_viewer.css":
		return "web/pdf_viewer.css", true
	case strings.HasPrefix(tarPath, "package/web/images/"):
		return strings.TrimPrefix(tarPath, "package/"), true
	case strings.HasPrefix(tarPath, "package/cmaps/"):
		return strings.TrimPrefix(tarPath, "package/"), true
	case strings.HasPrefix(tarPath, "package/standard_fonts/"):
		return strings.TrimPrefix(tarPath, "package/"), true
	case strings.HasPrefix(tarPath, "package/wasm/"):
		return strings.TrimPrefix(tarPath, "package/"), true
	default:
		return "", false
	}
}
```

Update the `required` list in `assetsReady` to:

```go
	required := []string{
		filepath.Join(targetDir, "LICENSE"),
		filepath.Join(targetDir, "build/pdf.mjs"),
		filepath.Join(targetDir, "build/pdf.worker.mjs"),
		filepath.Join(targetDir, "legacy/build/pdf.min.mjs"),
		filepath.Join(targetDir, "legacy/build/pdf.worker.min.mjs"),
		filepath.Join(targetDir, "web/pdf_viewer.mjs"),
		filepath.Join(targetDir, "web/pdf_viewer.css"),
		filepath.Join(targetDir, "cmaps/LICENSE"),
		filepath.Join(targetDir, "standard_fonts/LiberationSans-Regular.ttf"),
		filepath.Join(targetDir, "wasm/qcms_bg.wasm"),
	}
```

- [ ] **Step 4: Verify asset tests pass**

Run:

```bash
go test ./scripts
```

Expected: PASS.

- [ ] **Step 5: Prepare local vendor assets**

Run:

```bash
make prepare-web-assets
```

Expected: exits 0 and `web/static/vendor/pdfjs/web/pdf_viewer.mjs`, `web/static/vendor/pdfjs/web/pdf_viewer.css`, `web/static/vendor/pdfjs/build/pdf.mjs`, and `web/static/vendor/pdfjs/build/pdf.worker.mjs` exist.

- [ ] **Step 6: Commit asset preparation changes**

Run:

```bash
git add scripts/fetch_pdfjs.go scripts/fetch_pdfjs_test.go web/static/vendor/pdfjs
git commit -m "Add official PDF.js viewer assets"
```

Expected: commit succeeds. If `web/static/vendor/pdfjs` is ignored, confirm the assets are generated by `make prepare-web-assets` and commit only the script/test files.

## Task 2: Cover Native PDF Selection Helpers

**Files:**
- Modify: `web/static/js/__tests__/viewer-pdf-selection.test.cjs`
- Modify: `web/static/js/viewer.js`

- [ ] **Step 1: Replace custom geometry tests with native selection tests**

Replace `web/static/js/__tests__/viewer-pdf-selection.test.cjs` with:

```js
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'viewer.js');

function createElement(name) {
    return {
        name,
        children: new Set(),
        classList: {
            classes: new Set(),
            add(value) { this.classes.add(value); },
            remove(value) { this.classes.delete(value); },
            contains(value) { return this.classes.has(value); },
        },
        style: {},
        contains(node) {
            if (node === this) return true;
            return this.children.has(node) || this.children.has(node?.parentElement);
        },
        appendChild(child) {
            child.parentElement = this;
            this.children.add(child);
        },
        querySelector(selector) {
            if (selector === '[data-pdf-scroll]') return this.pdfScroll || null;
            return null;
        },
        getBoundingClientRect() {
            return { left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 };
        },
    };
}

function loadViewerPage(selection) {
    const code = `${fs.readFileSync(modulePath, 'utf8')}\nglobalThis.__RESOURCE_VIEWER_PAGE__ = ResourceViewerPage;`;
    const context = {
        console,
        URL,
        URLSearchParams,
        Node: { TEXT_NODE: 3 },
        navigator: {},
        window: {
            location: {
                href: 'http://localhost/viewer?kind=pdf&src=%2Fpaper.pdf',
                origin: 'http://localhost',
                search: '?kind=pdf&src=%2Fpaper.pdf',
            },
            history: { length: 1 },
            innerWidth: 1280,
            innerHeight: 800,
            setTimeout,
            clearTimeout,
            getSelection() {
                return selection;
            },
        },
        document: {
            referrer: '',
            addEventListener() {},
            createElement(tagName) {
                return createElement(tagName);
            },
            head: createElement('head'),
        },
        t(key, fallback) {
            return fallback || key;
        },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: modulePath });
    return context.__RESOURCE_VIEWER_PAGE__;
}

function rect(left, top, right, bottom) {
    return {
        left,
        top,
        right,
        bottom,
        width: right - left,
        height: bottom - top,
    };
}

test('currentPDFSelectionText reads the browser native selection text', () => {
    const selection = {
        toString() {
            return '  Among the genomic alterations observed in ER+ breast cancer,\nARID1A being  ';
        },
    };
    const viewer = loadViewerPage(selection);

    assert.equal(
        viewer.currentPDFSelectionText(),
        'Among the genomic alterations observed in ER+ breast cancer,\nARID1A being'
    );
});

test('selectionBelongsToPDFViewer accepts selections inside the PDF scroll area', () => {
    const scroll = createElement('scroll');
    const textLayer = createElement('textLayer');
    const textNode = { nodeType: 3, parentElement: textLayer };
    scroll.appendChild(textLayer);
    const selection = {
        anchorNode: textNode,
        focusNode: textNode,
        toString() { return 'ER+ breast cancer'; },
    };
    const viewer = loadViewerPage(selection);
    viewer.stage = createElement('stage');
    viewer.stage.pdfScroll = scroll;

    assert.equal(viewer.selectionBelongsToPDFViewer(selection), true);
});

test('selectionBelongsToPDFViewer rejects selections outside the PDF scroll area', () => {
    const scroll = createElement('scroll');
    const outside = createElement('outside');
    const selection = {
        anchorNode: outside,
        focusNode: outside,
        toString() { return 'outside text'; },
    };
    const viewer = loadViewerPage(selection);
    viewer.stage = createElement('stage');
    viewer.stage.pdfScroll = scroll;

    assert.equal(viewer.selectionBelongsToPDFViewer(selection), false);
});

test('pdfSelectionClientRect unions visible native selection rects', () => {
    const selection = {
        rangeCount: 1,
        getRangeAt() {
            return {
                getClientRects() {
                    return [
                        rect(40, 80, 140, 104),
                        rect(38, 112, 220, 136),
                    ];
                },
                getBoundingClientRect() {
                    return rect(40, 80, 220, 136);
                },
            };
        },
        toString() { return 'two selected lines'; },
    };
    const viewer = loadViewerPage(selection);

    assert.deepEqual(viewer.pdfSelectionClientRect(selection), rect(38, 80, 220, 136));
});
```

- [ ] **Step 2: Run the selection tests and verify they fail**

Run:

```bash
node --test web/static/js/__tests__/viewer-pdf-selection.test.cjs
```

Expected: FAIL. The failure mentions `selectionBelongsToPDFViewer` or `pdfSelectionClientRect` is not a function.

- [ ] **Step 3: Add native selection helper methods**

In `web/static/js/viewer.js`, replace `currentPDFSelectionText()` with this implementation and add the new helpers directly after it:

```js
    currentPDFSelectionText(selection = window.getSelection?.()) {
        return String(selection?.toString?.() || this.pdfState.selectionText || '').trim();
    },

    selectionBelongsToPDFViewer(selection = window.getSelection?.()) {
        const scroll = this.stage?.querySelector?.('[data-pdf-scroll]');
        if (!scroll || !selection) return false;
        return this.nodeBelongsToElement(selection.anchorNode, scroll)
            && this.nodeBelongsToElement(selection.focusNode, scroll);
    },

    nodeBelongsToElement(node, element) {
        if (!node || !element) return false;
        const textNodeType = typeof Node !== 'undefined' ? Node.TEXT_NODE : 3;
        const candidate = node.nodeType === textNodeType ? node.parentElement : node;
        return !!candidate && typeof element.contains === 'function' && element.contains(candidate);
    },

    pdfSelectionClientRect(selection = window.getSelection?.()) {
        if (!selection?.rangeCount) return null;
        const rects = [];
        for (let index = 0; index < selection.rangeCount; index += 1) {
            const range = selection.getRangeAt(index);
            const clientRects = Array.from(range.getClientRects?.() || []);
            rects.push(...clientRects.filter((rect) => rect.width > 0 && rect.height > 0));
            if (!clientRects.length) {
                const bounds = range.getBoundingClientRect?.();
                if (bounds?.width > 0 && bounds?.height > 0) {
                    rects.push(bounds);
                }
            }
        }
        if (!rects.length) return null;
        const left = Math.min(...rects.map((rect) => rect.left));
        const top = Math.min(...rects.map((rect) => rect.top));
        const right = Math.max(...rects.map((rect) => rect.right));
        const bottom = Math.max(...rects.map((rect) => rect.bottom));
        return {
            left,
            top,
            right,
            bottom,
            width: right - left,
            height: bottom - top
        };
    },
```

- [ ] **Step 4: Verify native selection helper tests pass**

Run:

```bash
node --test web/static/js/__tests__/viewer-pdf-selection.test.cjs
```

Expected: PASS.

- [ ] **Step 5: Commit native helper test coverage**

Run:

```bash
git add web/static/js/__tests__/viewer-pdf-selection.test.cjs web/static/js/viewer.js
git commit -m "Cover native PDF selection helpers"
```

Expected: commit succeeds.

## Task 3: Replace Custom PDF Rendering with Official PDFViewer

**Files:**
- Modify: `web/static/js/viewer.js`

- [ ] **Step 1: Update PDF state shape**

In `defaultPDFState()`, replace the custom render fields with official viewer fields:

```js
    defaultPDFState() {
        return {
            pdfjsLib: null,
            pdfjsViewerLib: null,
            loadingTask: null,
            pdfDocument: null,
            resource: null,
            eventBus: null,
            linkService: null,
            pdfViewer: null,
            pageNumber: 1,
            pageCount: 0,
            scale: 1,
            fitMode: true,
            selectionText: '',
            selectionClientRect: null
        };
    },
```

- [ ] **Step 2: Import the compatible PDF.js modules**

Update `ensurePDFJSReady()` to use the modern build module that `web/pdf_viewer.mjs` imports internally:

```js
    async ensurePDFJSReady() {
        if (this.pdfState.pdfjsLib) {
            return this.pdfState.pdfjsLib;
        }
        const pdfjsLib = await import('/static/vendor/pdfjs/build/pdf.mjs');
        pdfjsLib.GlobalWorkerOptions.workerSrc = '/static/vendor/pdfjs/build/pdf.worker.mjs';
        this.pdfState.pdfjsLib = pdfjsLib;
        return pdfjsLib;
    },
```

Add these methods after `ensurePDFJSReady()`:

```js
    async ensurePDFViewerReady() {
        const pdfjsLib = await this.ensurePDFJSReady();
        if (!this.pdfState.pdfjsViewerLib) {
            this.ensurePDFViewerStyles();
            this.pdfState.pdfjsViewerLib = await import('/static/vendor/pdfjs/web/pdf_viewer.mjs');
        }
        return {
            pdfjsLib,
            pdfjsViewerLib: this.pdfState.pdfjsViewerLib
        };
    },

    ensurePDFViewerStyles() {
        if (document.getElementById?.('pdfjsViewerStylesheet')) return;
        const link = document.createElement('link');
        link.id = 'pdfjsViewerStylesheet';
        link.rel = 'stylesheet';
        link.href = '/static/vendor/pdfjs/web/pdf_viewer.css';
        document.head.appendChild(link);
    },
```

- [ ] **Step 3: Replace the PDF host markup**

In `renderPDFResource(resource)`, replace the current `this.pdfState = ...` object and `this.stage.innerHTML = ...` with:

```js
        const previousPDFState = this.pdfState || this.defaultPDFState();
        this.pdfState = {
            ...this.defaultPDFState(),
            pdfjsLib: previousPDFState.pdfjsLib,
            pdfjsViewerLib: previousPDFState.pdfjsViewerLib,
            resource,
            pageNumber: this.initialPDFPageNumber(),
            scale: this.isNarrowPDFViewport() ? 0.9 : 1,
            fitMode: !this.isNarrowPDFViewport()
        };
        this.stage.className = 'viewer-stage pdf-mode';
        this.stage.innerHTML = `
            <div class="viewer-pdf-scroll" data-pdf-scroll tabindex="0">
                <div class="pdfViewer viewer-pdf-official" data-pdf-viewer></div>
                <div class="viewer-pdf-loading" data-pdf-loading>${t('viewer.pdf_loading', '正在加载 PDF...')}</div>
            </div>
        `;
```

Keep the existing detail-page link block immediately after this markup.

- [ ] **Step 4: Replace manual document loading with official viewer setup**

Replace `loadPDFDocument(href)` with:

```js
    async loadPDFDocument(href) {
        const { pdfjsLib, pdfjsViewerLib } = await this.ensurePDFViewerReady();
        const scrollElement = this.stage?.querySelector('[data-pdf-scroll]');
        const viewerElement = this.stage?.querySelector('[data-pdf-viewer]');
        const loadingElement = this.stage?.querySelector('[data-pdf-loading]');
        if (!scrollElement || !viewerElement) {
            throw new Error(t('viewer.err_invalid_resource', '资源地址无效或不受支持。'));
        }

        const eventBus = new pdfjsViewerLib.EventBus();
        const linkService = new pdfjsViewerLib.PDFLinkService({ eventBus });
        const textLayerMode = pdfjsViewerLib.TextLayerMode?.ENABLE ?? 1;
        const pdfViewer = new pdfjsViewerLib.PDFViewer({
            container: scrollElement,
            viewer: viewerElement,
            eventBus,
            linkService,
            textLayerMode,
            annotationMode: pdfjsLib.AnnotationMode?.ENABLE,
        });
        linkService.setViewer(pdfViewer);

        this.pdfState.eventBus = eventBus;
        this.pdfState.linkService = linkService;
        this.pdfState.pdfViewer = pdfViewer;

        eventBus.on('pagesinit', () => {
            this.applyPDFViewerScale();
            this.syncPDFToolbar();
            if (loadingElement) {
                loadingElement.hidden = true;
            }
        });
        eventBus.on('pagesloaded', () => {
            this.pdfState.pageCount = pdfViewer.pagesCount || this.pdfState.pageCount;
            this.syncPDFToolbar();
        });
        eventBus.on('pagechanging', (event) => {
            this.pdfState.pageNumber = event.pageNumber || pdfViewer.currentPageNumber || 1;
            this.clearPDFSelection();
            this.syncPDFToolbar();
        });
        eventBus.on('scalechanging', () => {
            this.pdfState.scale = pdfViewer.currentScale || this.pdfState.scale || 1;
            this.clearPDFSelection();
            this.syncPDFToolbar();
        });

        const loadingTask = pdfjsLib.getDocument({
            url: href,
            cMapUrl: '/static/vendor/pdfjs/cmaps/',
            cMapPacked: true,
            standardFontDataUrl: '/static/vendor/pdfjs/standard_fonts/',
            wasmUrl: '/static/vendor/pdfjs/wasm/'
        });
        this.pdfState.loadingTask = loadingTask;
        const pdfDocument = await loadingTask.promise;
        this.pdfState.pdfDocument = pdfDocument;
        this.pdfState.pageCount = pdfDocument.numPages || 0;
        pdfViewer.setDocument(pdfDocument);
        linkService.setDocument(pdfDocument, null);
        this.syncPDFToolbar();
    },
```

- [ ] **Step 5: Remove manual page rendering methods from the production path**

Delete these methods from `web/static/js/viewer.js`:

```text
renderPDFPage
calculatePDFPageFitScale
cancelPDFRenderTask
isPDFRenderCancelledError
beginPDFSelectionDrag
updatePDFSelectionDrag
endPDFSelectionDrag
cancelPDFSelectionDrag
updatePDFSelectionFromDrag
normalizedClientRect
collectPDFSelectionFromClientRect
collectPDFSelectionLinesFromDrag
collectPDFSelectionLinesFromArea
buildPDFTextLineSegments
createPDFTextLineSegment
findPDFLineAtPoint
pdfLineXDistance
filterPDFLinesToAnchorColumn
comparePDFSelectionPositions
formatPDFSelectionText
shouldJoinPDFTextWithoutLeadingSpace
buildPDFSelectionRects
pdfSelectionClientBounds
medianNumber
rectsIntersect
renderPDFSelectionRects
```

This deletion is intentional: after the official viewer is active, CiteBox must not compute PDF text selection or draw yellow selection rectangles.

- [ ] **Step 6: Add official viewer scale helper**

Add this method near `setPDFScale()`:

```js
    applyPDFViewerScale() {
        const pdfViewer = this.pdfState.pdfViewer;
        if (!pdfViewer) return;
        if (this.pdfState.fitMode) {
            pdfViewer.currentScaleValue = 'page-width';
            this.pdfState.scale = pdfViewer.currentScale || this.pdfState.scale || 1;
            return;
        }
        pdfViewer.currentScale = this.clampPDFScale(this.pdfState.scale);
        this.pdfState.scale = pdfViewer.currentScale || this.pdfState.scale || 1;
    },
```

- [ ] **Step 7: Update page, zoom, resize, and destroy behavior**

Replace `goToPDFPage`, `setPDFScale`, `syncPDFToolbar`, `schedulePDFRerender`, and `destroyPDFState` with:

```js
    async goToPDFPage(pageNumber) {
        const pdfViewer = this.pdfState.pdfViewer;
        if (!pdfViewer) return;
        const pageCount = this.pdfState.pageCount || pdfViewer.pagesCount || 1;
        const nextPage = Math.min(Math.max(Math.floor(Number(pageNumber) || 1), 1), Math.max(pageCount, 1));
        pdfViewer.currentPageNumber = nextPage;
        this.pdfState.pageNumber = pdfViewer.currentPageNumber || nextPage;
        this.syncPDFToolbar();
    },

    async setPDFScale(scale) {
        const pdfViewer = this.pdfState.pdfViewer;
        if (!pdfViewer) return;
        this.pdfState.fitMode = false;
        this.pdfState.scale = this.clampPDFScale(scale);
        this.applyPDFViewerScale();
        this.syncPDFToolbar();
    },

    syncPDFToolbar() {
        if (!this.pdfControls || this.pdfControls.hidden) return;
        const pdfViewer = this.pdfState.pdfViewer;
        const pageCount = this.pdfState.pageCount || pdfViewer?.pagesCount || 0;
        const pageNumber = Math.min(
            Math.max(pdfViewer?.currentPageNumber || this.pdfState.pageNumber || 1, 1),
            Math.max(pageCount, 1)
        );
        this.pdfState.pageNumber = pageNumber;
        if (pdfViewer?.currentScale) {
            this.pdfState.scale = pdfViewer.currentScale;
        }
        if (this.pdfPageInput) {
            this.pdfPageInput.max = String(Math.max(pageCount, 1));
            this.pdfPageInput.value = String(pageNumber);
        }
        if (this.pdfPageTotal) {
            this.pdfPageTotal.textContent = `/ ${pageCount || 1}`;
        }
        if (this.pdfPrevButton) {
            this.pdfPrevButton.disabled = pageNumber <= 1;
        }
        if (this.pdfNextButton) {
            this.pdfNextButton.disabled = !pageCount || pageNumber >= pageCount;
        }
        if (this.pdfZoomLabel) {
            this.pdfZoomLabel.textContent = `${Math.round((this.pdfState.scale || 1) * 100)}%`;
        }
    },

    schedulePDFRerender() {
        if (!this.pdfState?.pdfViewer || !this.pdfState.fitMode) return;
        window.clearTimeout(this.resizeTimer);
        this.resizeTimer = window.setTimeout(() => {
            this.applyPDFViewerScale();
            this.syncPDFToolbar();
        }, 120);
    },

    destroyPDFState() {
        this.clearPDFSelection();
        if (this.pdfState.pdfViewer && typeof this.pdfState.pdfViewer.setDocument === 'function') {
            this.pdfState.pdfViewer.setDocument(null);
        }
        if (this.pdfState.loadingTask && typeof this.pdfState.loadingTask.destroy === 'function') {
            this.pdfState.loadingTask.destroy().catch(() => {});
        }
        if (this.pdfState.pdfDocument && typeof this.pdfState.pdfDocument.destroy === 'function') {
            this.pdfState.pdfDocument.destroy().catch(() => {});
        }
        const pdfjsLib = this.pdfState.pdfjsLib;
        const pdfjsViewerLib = this.pdfState.pdfjsViewerLib;
        this.pdfState = {
            ...this.defaultPDFState(),
            pdfjsLib,
            pdfjsViewerLib
        };
    },
```

- [ ] **Step 8: Update the fit-width button handler**

In `bindPDFControls()`, replace the fit-width handler body with:

```js
        this.pdfFitButton?.addEventListener('click', async () => {
            this.pdfState.fitMode = true;
            this.applyPDFViewerScale();
            this.syncPDFToolbar();
        });
```

- [ ] **Step 9: Verify JavaScript syntax**

Run:

```bash
node --check web/static/js/viewer.js
```

Expected: exits 0.

- [ ] **Step 10: Run focused JavaScript tests**

Run:

```bash
node --test web/static/js/__tests__/viewer-pdf-selection.test.cjs web/static/js/__tests__/utils-resource-viewer.test.cjs
```

Expected: PASS.

- [ ] **Step 11: Commit official viewer rendering**

Run:

```bash
git add web/static/js/viewer.js web/static/js/__tests__/viewer-pdf-selection.test.cjs
git commit -m "Use official PDF.js viewer for PDFs"
```

Expected: commit succeeds.

## Task 4: Wire Native Selection Menu Behavior

**Files:**
- Modify: `web/static/js/viewer.js`
- Modify: `web/static/js/__tests__/viewer-pdf-selection.test.cjs`

- [ ] **Step 1: Remove custom drag-selection event wiring**

In `init()`, remove the PDF-specific pointer handlers:

```js
        this.stage?.addEventListener('pointerdown', (event) => {
            this.beginPDFSelectionDrag(event);
        });
```

Remove these calls from the document pointer handlers:

```js
            this.updatePDFSelectionDrag(event);
```

```js
            this.endPDFSelectionDrag(event);
```

```js
            this.cancelPDFSelectionDrag(event);
```

- [ ] **Step 2: Add native selection event wiring**

In `init()`, after the document pointer handlers, add:

```js
        document.addEventListener('selectionchange', () => {
            this.schedulePDFSelectionMenuRefresh();
        });
        document.addEventListener('pointerup', () => {
            this.schedulePDFSelectionMenuRefresh();
        });
```

- [ ] **Step 3: Add selection menu refresh methods**

Add these methods near `showPDFSelectionMenu()`:

```js
    schedulePDFSelectionMenuRefresh() {
        if (!this.pdfState?.pdfDocument) return;
        window.clearTimeout(this.pdfSelectionTimer);
        this.pdfSelectionTimer = window.setTimeout(() => {
            this.refreshPDFSelectionMenu();
        }, 0);
    },

    refreshPDFSelectionMenu() {
        if (!this.pdfState?.pdfDocument) return;
        const selection = window.getSelection?.();
        const text = this.currentPDFSelectionText(selection);
        if (!text || !this.selectionBelongsToPDFViewer(selection)) {
            this.hidePDFSelectionMenu();
            this.pdfState.selectionText = '';
            this.pdfState.selectionClientRect = null;
            return;
        }
        const rect = this.pdfSelectionClientRect(selection);
        if (!rect) {
            this.hidePDFSelectionMenu();
            return;
        }
        this.pdfState.selectionText = text;
        this.pdfState.selectionClientRect = rect;
        this.showPDFSelectionMenu(rect);
    },
```

- [ ] **Step 4: Simplify clearing selection**

Replace `clearPDFSelection(options = {})` with:

```js
    clearPDFSelection() {
        this.hidePDFSelectionMenu();
        this.pdfState.selectionText = '';
        this.pdfState.selectionClientRect = null;
        window.getSelection?.().removeAllRanges?.();
    },
```

- [ ] **Step 5: Add tests for refresh and clear behavior**

Append these tests to `web/static/js/__tests__/viewer-pdf-selection.test.cjs`:

```js
test('refreshPDFSelectionMenu stores native selection text and bounds', () => {
    const scroll = createElement('scroll');
    const textLayer = createElement('textLayer');
    const textNode = { nodeType: 3, parentElement: textLayer };
    scroll.appendChild(textLayer);
    const selection = {
        anchorNode: textNode,
        focusNode: textNode,
        rangeCount: 1,
        getRangeAt() {
            return {
                getClientRects() {
                    return [rect(100, 160, 260, 184)];
                },
                getBoundingClientRect() {
                    return rect(100, 160, 260, 184);
                },
            };
        },
        toString() { return 'ARID1A being'; },
    };
    const viewer = loadViewerPage(selection);
    viewer.stage = createElement('stage');
    viewer.stage.pdfScroll = scroll;
    viewer.pdfState = {
        ...viewer.defaultPDFState(),
        pdfDocument: {},
    };
    let shownRect = null;
    viewer.showPDFSelectionMenu = (value) => {
        shownRect = value;
    };

    viewer.refreshPDFSelectionMenu();

    assert.equal(viewer.pdfState.selectionText, 'ARID1A being');
    assert.deepEqual(viewer.pdfState.selectionClientRect, rect(100, 160, 260, 184));
    assert.deepEqual(shownRect, rect(100, 160, 260, 184));
});

test('clearPDFSelection removes native browser ranges', () => {
    let cleared = false;
    const selection = {
        toString() { return 'selected'; },
        removeAllRanges() {
            cleared = true;
        },
    };
    const viewer = loadViewerPage(selection);
    viewer.pdfState = {
        ...viewer.defaultPDFState(),
        selectionText: 'selected',
        selectionClientRect: rect(1, 2, 3, 4),
    };
    viewer.hidePDFSelectionMenu = () => {};

    viewer.clearPDFSelection();

    assert.equal(cleared, true);
    assert.equal(viewer.pdfState.selectionText, '');
    assert.equal(viewer.pdfState.selectionClientRect, null);
});
```

- [ ] **Step 6: Verify selection tests pass**

Run:

```bash
node --test web/static/js/__tests__/viewer-pdf-selection.test.cjs
```

Expected: PASS.

- [ ] **Step 7: Commit native selection menu behavior**

Run:

```bash
git add web/static/js/viewer.js web/static/js/__tests__/viewer-pdf-selection.test.cjs
git commit -m "Use native PDF selection for translation"
```

Expected: commit succeeds.

## Task 5: Clean Up Viewer CSS

**Files:**
- Modify: `web/viewer.html`

- [ ] **Step 1: Remove custom PDF canvas/text selection CSS**

In `web/viewer.html`, delete the CSS blocks for:

```text
.viewer-pdf-page
.viewer-pdf-canvas
.viewer-pdf-selection-layer
.viewer-pdf-selection-rect
.viewer-pdf-text-layer
.viewer-pdf-text-layer :is(span, br)
.viewer-pdf-text-layer span
.viewer-pdf-text-layer br
.viewer-pdf-text-layer ::selection
.viewer-pdf-text-layer *::selection
```

- [ ] **Step 2: Add embedded official viewer CSS**

Keep `.viewer-pdf-scroll` and replace it with:

```css
        .viewer-pdf-scroll {
            position: relative;
            width: 100%;
            height: 100%;
            min-height: 0;
            overflow: auto;
            padding: 1.25rem 0;
            background: #d6d0c7;
            scrollbar-gutter: stable both-edges;
        }

        .viewer-pdf-official {
            --scale-factor: 1;
            position: relative;
            min-height: 100%;
        }

        .viewer-pdf-scroll .page {
            margin: 0 auto 1rem;
            border: none;
            background: #fff;
            box-shadow: 0 12px 34px rgba(35, 26, 20, 0.2);
        }

        .viewer-pdf-scroll .textLayer {
            opacity: 1;
        }

        .viewer-pdf-scroll .textLayer ::selection {
            background: rgba(255, 212, 0, 0.32);
        }
```

- [ ] **Step 3: Keep loading overlay scoped to the PDF scroll area**

Replace `.viewer-pdf-loading` with:

```css
        .viewer-pdf-loading {
            position: absolute;
            inset: 0;
            z-index: 4;
            display: flex;
            align-items: center;
            justify-content: center;
            background: rgba(214, 208, 199, 0.88);
            color: #3d332d;
            font-size: 0.95rem;
            line-height: 1.5;
        }
```

- [ ] **Step 4: Adjust mobile PDF body spacing**

In the `@media (max-width: 720px)` block, replace the PDF-specific body rules with:

```css
            .viewer-pdf-scroll {
                padding: 0.75rem 0;
            }

            .viewer-pdf-scroll .page {
                margin-bottom: 0.75rem;
                box-shadow: 0 8px 24px rgba(35, 26, 20, 0.18);
            }
```

- [ ] **Step 5: Verify HTML does not add hardcoded copy**

Run:

```bash
git diff -- web/viewer.html
```

Expected: the diff only changes CSS and does not add new user-visible strings.

- [ ] **Step 6: Commit viewer CSS cleanup**

Run:

```bash
git add web/viewer.html
git commit -m "Style embedded PDF.js viewer"
```

Expected: commit succeeds.

## Task 6: Run Automated Verification

**Files:**
- Verify: `scripts/fetch_pdfjs.go`
- Verify: `web/static/js/viewer.js`
- Verify: `web/static/js/__tests__/viewer-pdf-selection.test.cjs`
- Verify: `web/static/js/__tests__/utils-resource-viewer.test.cjs`
- Verify: `web/viewer.html`

- [ ] **Step 1: Check Go tests for asset preparation**

Run:

```bash
go test ./scripts
```

Expected: PASS.

- [ ] **Step 2: Check JavaScript syntax**

Run:

```bash
node --check web/static/js/viewer.js
```

Expected: exits 0.

- [ ] **Step 3: Run focused JavaScript tests**

Run:

```bash
node --test web/static/js/__tests__/viewer-pdf-selection.test.cjs web/static/js/__tests__/utils-resource-viewer.test.cjs
```

Expected: PASS.

- [ ] **Step 4: Run application package tests**

Run:

```bash
go test ./internal/app
```

Expected: PASS.

- [ ] **Step 5: Check whitespace and patch safety**

Run:

```bash
git diff --check
```

Expected: exits 0.

- [ ] **Step 6: Commit verification plan progress if the checklist was updated**

Run:

```bash
git add docs/superpowers/plans/2026-05-21-official-pdfjs-viewer-integration.md
git commit -m "Track official PDF.js viewer plan"
```

Expected: commit succeeds if this plan file changed during execution. If the plan file was already committed before implementation, skip this commit.

## Task 7: Manual Browser Smoke Check

**Files:**
- Verify in browser: `/viewer?kind=pdf&src=...`
- Verify in browser: image resource viewer route

- [ ] **Step 1: Start the local app**

Run:

```bash
make run
```

Expected: server starts at `http://localhost:8080` or prints the configured local URL. Keep the process running for browser checks.

- [ ] **Step 2: Open a real two-column paper PDF**

Open a URL matching:

```text
http://localhost:8080/viewer?kind=pdf&src=/files/papers/paper_1777519479295165603.pdf&back=/library&paper_id=1777519479295165603
```

Expected: CiteBox toolbar is visible, the PDF body renders with the official viewer pages, and there are no oversized controls or clipped title text.

- [ ] **Step 3: Verify toolbar actions**

Perform these checks:

```text
Click 下一页 -> page input becomes 2.
Click 上一页 -> page input becomes 1.
Click 放大 -> zoom label increases.
Click 缩小 -> zoom label decreases.
Click 适合宽度 -> page width fits the scroll container.
Click 详情页 -> navigation opens the original library detail entry.
```

Expected: every action works without a console error.

- [ ] **Step 4: Verify native selection on the failing right-column paragraph**

Select the paragraph that begins:

```text
Among the genomic alterations observed in ER+ breast cancer
```

Expected:

```text
The browser's native highlight follows only the selected visible text.
The floating menu appears near the selected text.
Clicking 复制 copies text containing "ER+ breast cancer" and "ARID1A being".
Clicking 翻译 sends the same native selection text to DesktopTranslate.translateText.
No custom yellow rectangle fragments remain after clearing selection.
```

- [ ] **Step 5: Verify image viewer regression**

Open an image through the existing resource viewer URL.

Expected:

```text
Image zoom works with the wheel.
Image drag works when zoomed.
复原视图 resets the transform.
The PDF toolbar is hidden.
```

- [ ] **Step 6: Check desktop and mobile layout**

Use Playwright or browser responsive mode with:

```text
Desktop: 1440 x 1000
Mobile: 390 x 844
```

Expected: the toolbar wraps cleanly, PDF pages are centered, text is not clipped by the toolbar, and the selection menu does not cover the selected text when there is room above or below it.

- [ ] **Step 7: Check console warnings and errors**

Use Playwright console collection or browser devtools.

Expected:

```text
0 JavaScript errors.
0 PDF.js asset 404s.
0 unhandled promise rejections.
```

- [ ] **Step 8: Stop the local app**

Stop the `make run` process with Ctrl-C.

Expected: no long-running local server process remains from this check.

## Task 8: Push Without Release

**Files:**
- Verify: git history and remote state

- [ ] **Step 1: Confirm the worktree is clean**

Run:

```bash
git status --short
```

Expected: no output.

- [ ] **Step 2: Push the branch**

Run:

```bash
git push
```

Expected: push succeeds.

- [ ] **Step 3: Confirm no release or tag was created**

Run:

```bash
git tag --points-at HEAD
```

Expected: no output.

Do not run `git tag`, `gh release create`, or any release workflow trigger for this feature until the user manually accepts the actual PDF reading and selection behavior.

## Self-Review

- Spec coverage: the plan keeps `/viewer`, preserves the CiteBox toolbar and detail-page entry, imports official PDF.js viewer assets, uses official `PDFViewer` for rendering/text layers, reads native selection text for copy/translation, removes the custom rectangle reconstruction path, preserves image viewing, avoids search/outline/annotation/double-page work, and explicitly blocks release creation.
- Placeholder scan: the plan contains concrete file paths, code snippets, commands, and expected outputs for every implementation step.
- Type consistency: the plan consistently uses `pdfjsLib`, `pdfjsViewerLib`, `eventBus`, `linkService`, `pdfViewer`, `selectionText`, and `selectionClientRect` as the PDF state fields, and uses `selectionBelongsToPDFViewer`, `pdfSelectionClientRect`, `refreshPDFSelectionMenu`, and `schedulePDFSelectionMenuRefresh` as the native selection helper names.
