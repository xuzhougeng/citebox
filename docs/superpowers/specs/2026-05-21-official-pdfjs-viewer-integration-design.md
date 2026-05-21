# Official PDF.js Viewer Integration Design

## Context

CiteBox currently renders PDFs in `web/viewer.html` with a hand-built canvas, PDF.js `TextLayer`, and custom drag-selection/highlight logic in `web/static/js/viewer.js`. This has proven fragile: real papers split text into many positioned spans for superscripts, italics, justification gaps, hyphenation, and two-column layouts. The custom algorithm can make selected text and visual highlight diverge, which is unacceptable for selection translation.

The product direction is to stop emulating PDF text selection ourselves and use the official PDF.js viewer components for PDF layout, scrolling, text layer behavior, and native browser selection.

## Goals

- Keep the CiteBox file viewer entry point: library file clicks still open `/viewer`.
- Preserve the CiteBox outer shell: back button, filename, PDF detail-page link, and existing visual context stay in the host page.
- Use official PDF.js viewer components for PDF rendering and text selection, so highlight behavior comes from the browser/PDF.js text layer rather than custom geometry.
- Add selection translation on top of native selection: after the user selects text, show a floating `复制 / 翻译` menu and pass `window.getSelection().toString()` to the existing translation flow.
- Remove the custom PDF selection rectangle/highlight algorithm from the production path.

## Non-Goals

- Do not add search, outline, annotations, or double-page mode in this pass.
- Do not ship another release/tag until the PDF reading and selection behavior has been manually accepted.
- Do not replace image viewing behavior in `/viewer`.
- Do not change manual PDF extraction or upload extraction flows, except for shared PDF.js asset preparation if needed.

## Architecture

`web/viewer.html` remains the host page. For `kind=pdf`, `viewer.js` creates an embedded official PDF.js viewer area inside the existing stage:

- `viewerStage`
  - PDF scroll container
  - official PDF.js `PDFViewer` container
  - native PDF.js text layers
  - CiteBox floating selection menu

The implementation should import PDF.js core plus official viewer component modules from vendored assets. The required set is:

- `legacy/build/pdf.min.mjs`
- `legacy/build/pdf.worker.min.mjs`
- `web/pdf_viewer.mjs`
- `web/pdf_viewer.css`
- any viewer assets required by that CSS/module

`scripts/fetch_pdfjs.go` and packaging scripts already call the same asset-preparation path, so expanding the selected files there keeps server and desktop packages aligned.

## UI Design

The top toolbar stays CiteBox-owned:

- `返回`
- document kind and filename
- page navigation
- zoom controls
- fit-width control
- `详情页`

The PDF body becomes official PDF.js viewer content. It should look quieter than the first custom reader:

- neutral page background
- white paper pages with moderate shadow
- native selection color allowed or lightly themed with CSS only if it does not break selection correctness
- no custom yellow text-fragment highlight layer

The selection menu appears near the browser selection bounds after `selectionchange` or `pointerup`:

- `复制`: writes selected text to clipboard
- `翻译`: calls `DesktopTranslate.translateText(text, { title: t('viewer.pdf_translate_title', ...) })`
- menu hides on scroll, page change, zoom change, Escape, or empty selection

## Data Flow

1. `/viewer?kind=pdf&src=...&back=...&paper_id=...` loads.
2. `viewer.js` initializes PDF.js worker and official viewer component.
3. PDF.js loads the document with existing cMap/font/wasm paths.
4. CiteBox toolbar commands call official viewer APIs for page and zoom.
5. User selects text in the official text layer.
6. CiteBox reads `window.getSelection().toString().trim()`.
7. Copy/translate actions use that exact selected text.

The important invariant is: the text passed to translation is the browser’s actual selected text, not reconstructed from intersecting rectangles.

## Error Handling

- If official viewer assets fail to load, show the existing viewer error state with a localized message.
- If the PDF fails to load or render, show the existing PDF render failure copy.
- If selection text is empty, do not show the selection menu.
- If clipboard write fails, keep the current copy-failure toast behavior.
- If translation is unavailable, keep the existing no-content/unavailable toast behavior.

## I18n

No new hardcoded user-facing text should be introduced. If new messages are needed, add matching keys in:

- `web/static/locales/zh-CN/viewer.json`
- `web/static/locales/en/viewer.json`

Existing strings for copy, translate, loading, page, zoom, fit-width, and detail page should be reused where possible.

## Testing

Automated checks:

- JS syntax: `node --check web/static/js/viewer.js`
- focused viewer tests for resource URL behavior and selection menu behavior
- `go test ./internal/app`
- `git diff --check`

Manual browser checks with Playwright:

- open a real two-column PDF from the library
- select the right-column paragraph that previously failed
- verify native highlight follows the visible selected text
- verify copied/translated text contains `ER+ breast cancer` and `ARID1A being`
- verify page navigation, zoom, fit-width, and detail-page link still work
- verify image viewer behavior is unchanged
- check desktop/mobile viewport layout
- check browser console has no errors or warnings

## Rollout

Implementation should land on `main` without creating a tag or release. A release can be published only after the user manually accepts the reader behavior from screenshots or local testing.
