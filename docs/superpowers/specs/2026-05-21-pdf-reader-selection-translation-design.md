# PDF Reader and Selection Translation Design

## Goal

Add a first-party PDF reading experience to CiteBox by extending the existing `/viewer` resource page. The reader replaces the current browser PDF iframe for paper PDFs, while keeping the existing image viewer behavior unchanged.

The first release focuses on reading and selection translation:

- Open paper PDFs from the library in the CiteBox PDF reader.
- Keep the paper detail modal available through an explicit detail entry.
- Add a detail-page entry on the manual extraction page.
- Let users select PDF text and translate it through the existing AI translation flow.

## Scope

Included:

- Use `pdf.js` on `/viewer?kind=pdf&src=...` to render PDF pages.
- Render a selectable PDF text layer on top of each page.
- Provide single-page navigation with previous page, next page, current page input, total pages, zoom controls, and fit-width.
- Show copy and translate actions when text is selected in the PDF reader.
- Reuse the existing `API.translateWithAI` endpoint and `DesktopTranslate.translateText` result dialog for translation.
- Change the library file-name click target from paper detail modal to PDF reader.
- Add a dedicated paper detail action in the library row.
- Add a `详情页` / `Details` link on the manual extraction page, near the existing PDF action.

Excluded from this release:

- PDF search.
- PDF outline/table of contents.
- PDF annotation storage.
- Double-page reading mode.
- Backend schema changes.
- New AI translation backend endpoints.

## UX

### Library

The file metadata row remains the PDF-oriented click target. Clicking the file name opens `/viewer?kind=pdf&src=<paper.pdf_url>&back=<library-url>`.

Paper details remain available as an explicit action button in the row footer. This avoids losing access to the existing detail modal after file-name clicks become reader clicks.

### Manual Extraction

The hero actions include two destinations:

- `打开原 PDF` opens the PDF reader.
- `详情页` opens the library paper detail modal through `/library?paper_id=<id>`.

This preserves the existing reader-style PDF entry and adds the old detail destination requested by the user.

### PDF Reader

The existing viewer shell remains the resource viewer. For PDFs, the stage becomes a pdf.js reader instead of an iframe:

- Toolbar: back, document title, page controls, zoom controls, fit-width, detail link when a paper id is available.
- Stage: one rendered page canvas with text layer.
- Loading and error states match current viewer behavior.

Text selection behavior:

- When selected text is non-empty, show a compact floating menu near the selection.
- `复制` copies selected text.
- `翻译` calls the existing AI translation dialog.
- If the AI translation model is not configured, the existing backend error is shown in the translation dialog.

## Data Flow

The PDF reader receives the file URL through the existing `src` query parameter. The library and manual extraction links may also pass `paper_id` so the reader can build a detail-page link.

For PDF rendering:

1. Load `pdf.js` from `/static/vendor/pdfjs/legacy/build/pdf.min.mjs`.
2. Configure the worker and asset URLs in the same way as `manual.js` and `paper-viewer.js`.
3. Load the document from `src`.
4. Render the current page to a canvas.
5. Render the page text content into a selectable text layer.

For translation:

1. Read `window.getSelection().toString()`.
2. Show copy/translate menu inside the viewer.
3. On translate, call `DesktopTranslate.translateText(selectedText, { title })`.
4. `DesktopTranslate` continues to call `API.translateWithAI`, so model configuration and language direction stay centralized.

## Implementation Notes

- Keep image viewer state and gesture code intact.
- Add PDF-specific state to `ResourceViewerPage`, rather than introducing a second page controller.
- Guard render tasks so rapid page changes or zoom changes do not leave stale canvases.
- Use i18n keys in `viewer.json`, `library.json`, and `manual.json`; do not add new hardcoded user-visible copy.
- Keep the reader usable in browser and desktop modes.

## Testing

Automated checks:

- Add or update focused JavaScript tests if the existing test harness can cover resource URL behavior or PDF-reader state helpers.
- Run `node --check` on touched frontend JavaScript files.
- Run `go test ./...` if backend routing or API behavior changes; otherwise backend tests are not required for this frontend-only change.

Manual checks:

- From the library, click a PDF file name and confirm the PDF reader opens.
- In the library, click the paper detail action and confirm the existing modal opens.
- In the manual extraction page, click `详情页` and confirm it opens the paper detail entry.
- In the PDF reader, navigate pages, zoom, fit width, select text, copy selected text, and translate selected text.
- Confirm image resources still open with the existing image viewer behavior.
