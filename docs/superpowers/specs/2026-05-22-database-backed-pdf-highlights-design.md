# Database-Backed PDF Highlights Design

## Goal

Replace the temporary browser-local PDF highlight prototype with a durable CiteBox annotation model stored in SQLite. Users should be able to select text in the PDF reader, create a highlight, leave and reopen the paper, and see the same highlight restored from the database.

This design intentionally keeps the first release narrow: durable yellow highlights only. It does not add a Zotero-style annotations sidebar, highlight colors, comments, search, export, or cloud sync.

## Current Context

The PDF reader already uses the official pdf.js viewer on `/viewer?kind=pdf&src=/files/papers/example.pdf&paper_id=42`. It has a real text layer and a selection menu with copy, translate, and AI ask actions.

The current local prototype stores highlights in `localStorage` with normalized page fragments. That is not a sufficient product model because highlights are reading data tied to a paper, not browser UI preferences. The database-backed implementation must remove that storage path and replace it with API-backed create/list/delete behavior.

Existing backend layering:

- `internal/model` defines JSON response types.
- `internal/repository` owns SQLite queries and migrations.
- `internal/service` owns validation and workflow logic.
- `internal/handler` owns HTTP request/response handling.
- `internal/app/server.go` wires routes.

## Recommended Approach

Use a new normalized table: `pdf_annotations`.

Alternative considered:

- Store all highlights as JSON on `papers`. This is simpler initially but makes create/delete operations coarse, complicates future comments/colors, and makes page-level queries awkward.
- Keep highlights in `localStorage`. This avoids backend work but loses data across browsers, devices, cache clears, database exports, and paper deletion semantics.

The new table is the right boundary because annotations have their own identity, timestamps, type, quote text, geometry, and future note/color fields.

## Data Model

Create `pdf_annotations`:

```sql
CREATE TABLE IF NOT EXISTS pdf_annotations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    paper_id INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    type TEXT NOT NULL DEFAULT 'highlight' CHECK (type IN ('highlight')),
    page_start INTEGER NOT NULL DEFAULT 1,
    page_end INTEGER NOT NULL DEFAULT 1,
    quote_text TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT 'yellow',
    fragments_json TEXT NOT NULL DEFAULT '[]',
    note_text TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pdf_annotations_paper_id
    ON pdf_annotations(paper_id, id);

CREATE INDEX IF NOT EXISTS idx_pdf_annotations_paper_page
    ON pdf_annotations(paper_id, page_start, page_end);
```

`fragments_json` stores normalized rectangles:

```json
[
  { "page": 3, "left": 0.12, "top": 0.34, "width": 0.28, "height": 0.018 }
]
```

Fragment values are relative to the PDF page box in the current pdf.js rendered page. They are floats between `0` and `1`. The frontend renders them back as percentages so highlights survive zooming and rerendering.

`page_start` and `page_end` are derived from fragments and used for ordering and future page filtering. `quote_text` stores the selected text for context and later export/search. The first release does not add FTS for annotation text.

## API

Add paper-scoped routes:

- `GET /api/papers/{paper_id}/pdf-annotations`
- `POST /api/papers/{paper_id}/pdf-annotations`
- `DELETE /api/papers/{paper_id}/pdf-annotations/{annotation_id}`

`GET` response:

```json
{
  "success": true,
  "annotations": [
    {
      "id": 10,
      "paper_id": 42,
      "type": "highlight",
      "page_start": 3,
      "page_end": 3,
      "quote_text": "selected text",
      "color": "yellow",
      "fragments": [
        { "page": 3, "left": 0.12, "top": 0.34, "width": 0.28, "height": 0.018 }
      ],
      "note_text": "",
      "created_at": "2026-05-22T10:00:00Z",
      "updated_at": "2026-05-22T10:00:00Z"
    }
  ]
}
```

`POST` request:

```json
{
  "type": "highlight",
  "quote_text": "selected text",
  "color": "yellow",
  "fragments": [
    { "page": 3, "left": 0.12, "top": 0.34, "width": 0.28, "height": 0.018 }
  ]
}
```

`POST` response:

```json
{
  "success": true,
  "annotation": {
    "id": 11,
    "paper_id": 42,
    "type": "highlight",
    "page_start": 3,
    "page_end": 3,
    "quote_text": "selected text",
    "color": "yellow",
    "fragments": [
      { "page": 3, "left": 0.12, "top": 0.34, "width": 0.28, "height": 0.018 }
    ],
    "note_text": "",
    "created_at": "2026-05-22T10:00:00Z",
    "updated_at": "2026-05-22T10:00:00Z"
  }
}
```

`DELETE` response:

```json
{ "success": true }
```

Validation rules:

- `paper_id` must refer to an existing paper.
- `type` must be `highlight`; empty type defaults to `highlight`.
- `quote_text` must be non-empty after trimming and must be capped at 10,000 characters.
- `color` defaults to `yellow`; first release only accepts `yellow`.
- `fragments` must contain 1 to 200 rectangles.
- Every fragment requires `page >= 1`, `left/top >= 0`, `width/height > 0`, and `left + width <= 1.001`, `top + height <= 1.001` to tolerate floating-point rounding.
- `page_start` and `page_end` are computed server-side from fragments, not trusted from the client.
- Deleting an annotation must verify it belongs to the requested `paper_id`.

## Frontend Behavior

The PDF reader requires `paper_id` for durable highlights.

On PDF load:

1. If `paper_id` is present, call `API.listPDFAnnotations(paperId)`.
2. Store returned annotations in `pdfState.highlights`.
3. Render highlight layers after `pagesinit`, `pagesloaded`, `pagerendered`, `textlayerrendered`, and scale changes.

When selecting text:

1. Build highlight fragments from each `window.getSelection().getRangeAt(index).getClientRects()` result.
2. Match each client rect to the pdf.js page with the largest intersection.
3. Normalize fragment coordinates against the page bounding box.
4. On `高亮` / `Highlight`, call `API.createPDFAnnotation(paperId, payload)`.
5. Render the annotation returned by the backend.
6. Clear the native browser selection.

When `paper_id` is missing:

- Hide the `高亮` action or show an error toast if invoked.
- Do not use `localStorage` as a fallback for this first database-backed release.

Clicking an existing highlight:

- First release may expose delete through a small inline menu or by direct click plus confirmation.
- If the menu is implemented, it should support only `删除高亮` / `Delete highlight`.
- Deleting calls `API.deletePDFAnnotation(paperId, annotationId)` and removes the annotation from `pdfState.highlights` after success.

## Files

Backend:

- `internal/model/paper.go`: add `PDFAnnotation` and `PDFAnnotationFragment`.
- `internal/repository/schema/schema.go`: create table and indexes during initialization.
- `internal/repository/pdf_annotation_repo.go`: list/create/delete annotation queries.
- `internal/repository/pdf_annotation_repo_test.go`: schema, validation-adjacent persistence, cascade delete, and ownership delete tests.
- `internal/service/library_service.go` or focused companion file: add service methods for list/create/delete with validation.
- `internal/service/library_service_pdf_annotations_test.go`: service validation tests.
- `internal/handler/paper.go`: add handlers for annotation routes.
- `internal/app/server.go`: route annotation endpoints before generic paper detail routing.
- `docs/database.md`: document `pdf_annotations`.
- `docs/api.md`: document annotation endpoints.

Frontend:

- `web/static/js/api.js`: add list/create/delete helpers.
- `web/static/js/viewer.js`: replace localStorage highlight persistence with API calls.
- `web/viewer.html`: keep highlight markup and add delete menu styles if needed.
- `web/static/locales/zh-CN/viewer.json` and `web/static/locales/en/viewer.json`: add any missing delete/error strings.
- `web/static/js/__tests__/viewer-pdf-selection.test.cjs`: update tests so create/list/delete use mocked API helpers instead of localStorage.

## TDD Strategy

Implementation must start by removing the localStorage prototype from production code and writing failing tests for the database-backed behavior.

Recommended red-green order:

1. Repository migration and CRUD tests for `pdf_annotations`.
2. Service validation tests for fragments, quote text, color, and ownership.
3. Handler/API tests for list, create, and delete route behavior if an existing handler test harness is available; otherwise use service/repository tests plus route wiring checks.
4. Frontend API helper tests.
5. Frontend viewer tests proving `highlightPDFSelection` calls `API.createPDFAnnotation`, renders the returned annotation, and does not use `localStorage`.

No production implementation for a layer should be written before its failing test is observed.

## Error Handling

- Repository errors should use existing `wrapDBError` / `ensureRowsAffected` patterns.
- Missing paper or annotation ownership mismatch should return the same not-found semantics used elsewhere in the paper layer.
- Frontend API failures should keep the selection state cleared but show an error toast; failed creates must not render a durable highlight.
- If annotation listing fails on PDF load, the reader should remain usable and show a toast instead of blocking PDF viewing.

## Out of Scope

- Multiple colors.
- Free-text comments on highlights.
- Annotation sidebar.
- Searching annotations.
- Exporting annotations.
- Syncing annotations outside the CiteBox SQLite database.
- Migrating localStorage prototype data into the database.

## Acceptance Criteria

- A highlight created in the PDF reader persists after leaving and reopening the same paper.
- Highlights are stored in SQLite and included in normal database backup/export flows.
- Deleting a paper cascades and deletes its annotations.
- Deleting a highlight removes it from both the database and the PDF reader.
- No PDF highlight persistence path uses `localStorage`.
- Existing copy, translate, and AI ask selection actions still work.
- Existing image viewer behavior is unchanged.
- `go test ./...`, focused JS tests, JS syntax checks, and `git diff --check` pass.
