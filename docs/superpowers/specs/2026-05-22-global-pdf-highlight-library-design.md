# Global PDF Highlight Library Design

## Goal

Add a global highlight library to CiteBox. Users should be able to open one navigation entry, see PDF highlights from every paper, search them, click any highlight, and land back in the PDF reader at the original highlighted position.

This builds on the existing database-backed PDF highlight work. It does not replace the current PDF reader selection flow or the `pdf_annotations` table.

## Current Context

CiteBox already stores durable PDF highlights in SQLite through `pdf_annotations`. Each annotation has a paper ID, quote text, page range, color, note text, timestamps, and normalized PDF-page fragments.

The PDF reader already accepts:

```text
/viewer?kind=pdf&src=/files/papers/example.pdf&paper_id=42&page=3
```

It loads annotations for `paper_id`, renders highlight fragments, and supports the `page` query parameter. It does not yet accept an annotation ID, scroll to a specific rendered fragment, or visually emphasize the target annotation after navigation.

The top navigation already has a "More" dropdown managed by `web/static/js/main.js`. Secondary navigation entries are controlled by `AppNav.SECONDARY_HREFS`.

## Recommended Approach

Create a dedicated `/highlights` page and place it in the "More" dropdown.

Alternatives considered:

- Add a highlights mode inside `/library`. This avoids a new page, but mixes two primary object types: papers and annotations. It would make filtering, empty states, and future annotation management harder to understand.
- Show highlights only inside paper details. This is useful later, but it does not solve the global "find what I highlighted across papers" workflow.
- Add an annotation sidebar inside the PDF reader first. This is a stronger reading workflow, but it is larger than the requested global entry and can come later.

The dedicated page is the right first step because it gives highlights their own searchable surface while preserving the existing paper library and PDF reader boundaries.

## User Experience

Navigation:

- Add a new top-level route: `/highlights`.
- Add the navigation link to all shared page nav markup with `data-i18n="nav.highlights"`.
- Add `/highlights` to `AppNav.SECONDARY_HREFS` so it appears under "More".

Page layout:

- Compact page header: "Highlight Library" / "高亮库".
- Search input for quote text and paper title.
- Sort selector with a narrow first release: newest first and oldest first.
- Results list with one row per annotation.

Each row shows:

- The highlighted quote text, clamped for scanning.
- Paper title.
- Page label, using `page_start` or `page_start-page_end`.
- Creation or update date.
- Actions: open in PDF, delete.

Click behavior:

- Clicking a row or the open action navigates to:

```text
/viewer?kind=pdf&src=<paper.pdf_url>&paper_id=<paper.id>&page=<annotation.page_start>&annotation_id=<annotation.id>&back=<current page>
```

- The PDF reader opens the requested page, waits until highlights are rendered, scrolls the matching highlight fragment into view, and applies a temporary target style.

Empty states:

- No annotations: explain that PDF text can be selected in the reader and highlighted.
- No search results: tell the user no highlights match the current query.
- Paper missing PDF URL: disable open action and show an error toast if invoked.

## Backend API

Add a global annotation listing endpoint:

```text
GET /api/pdf-annotations?query=&sort=updated_desc&page=1&page_size=50
```

Response:

```json
{
  "success": true,
  "annotations": [
    {
      "id": 10,
      "paper_id": 42,
      "paper_title": "Example paper",
      "paper_original_filename": "example.pdf",
      "paper_pdf_url": "/files/papers/example.pdf",
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
  ],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total": 1,
    "total_pages": 1
  }
}
```

Validation:

- `page` defaults to `1`.
- `page_size` defaults to `50` and is capped at `100`.
- `sort` accepts `updated_desc`, `updated_asc`, `created_desc`, and `created_asc`.
- `query` searches annotation quote text, paper title, original filename, and DOI with SQLite `LIKE` matching for the first release.

Deletion can reuse the existing paper-scoped delete endpoint from the global page:

```text
DELETE /api/papers/{paper_id}/pdf-annotations/{annotation_id}
```

No schema change is required for the first release.

## Backend Components

Model:

- Add a list item response type that embeds PDF annotation fields plus paper display fields and PDF URL.

Repository:

- Add a method on `PDFAnnotationRepository` for global listing.
- Join `pdf_annotations` to `papers`.
- Apply query filter, sort, limit, and offset.
- Return total count with the same filter.

Service:

- Validate query parameters.
- Cap page size.
- Preserve paper ownership checks by continuing to use the existing delete service for deletes.

Handler:

- Add a new handler for `GET /api/pdf-annotations`.
- Keep existing `/api/papers/{paper_id}/pdf-annotations` behavior unchanged.

Server wiring:

- Register `/highlights` as a static page route.
- Register `/api/pdf-annotations` before broader paper route matching if route order matters.

## Frontend Components

Files:

- `web/highlights.html`
- `web/static/js/highlights.js`
- Locale files in both `web/static/locales/zh-CN/` and `web/static/locales/en/`
- Existing shared nav locale files for `nav.highlights`
- Existing CSS files or a focused feature stylesheet, following the current page patterns

Frontend state:

- `query`
- `sort`
- `page`
- `pageSize`
- `annotations`
- `pagination`
- `loading`

API helper:

- Add `API.listPDFAnnotationsGlobal(params)`.

Navigation helper:

- Extend `Utils.resourceViewerURL` and `Utils.parseResourceViewerNavigationURL` to pass through `annotation_id`.
- Existing callers continue working because the new option is optional.

## PDF Reader Targeting

Viewer URL:

```text
/viewer?...&page=3&annotation_id=10
```

Reader behavior:

1. Parse `annotation_id` during PDF resource initialization.
2. Store it in `pdfState.targetAnnotationId`.
3. After annotations load and after highlight layers render, look for `[data-highlight-id="<id>"]`.
4. Scroll the first matching marker into view with centered block alignment.
5. Add a temporary CSS class such as `is-target-highlight`.
6. Remove the emphasis after a short timeout.

Fallbacks:

- If the annotation is not in the loaded list, navigate to `page` only and show no blocking error.
- If the page has not rendered yet, retry targeting after `pagerendered` and scheduled highlight renders.
- If `annotation_id` is present without `paper_id`, ignore it because annotations are paper scoped.

## Error Handling

- API list failure shows a localized error toast and keeps the previous list if available.
- Delete failure shows a localized error toast.
- Delete success removes the row locally and updates pagination metadata on the next refresh.
- Open action for an annotation without `paper_pdf_url` shows the existing "no PDF URL" message.
- Viewer targeting failure should not block normal PDF reading.

## I18n

All new visible strings must be added in both Chinese and English locale files. New hardcoded UI text in HTML or JavaScript is not allowed except as fallback values passed to existing `t(...)` calls.

Suggested keys:

- `nav.highlights`
- `highlights.hero_eyebrow`
- `highlights.hero_title`
- `highlights.hero_text`
- `highlights.search_label`
- `highlights.search_placeholder`
- `highlights.sort_label`
- `highlights.sort_updated_desc`
- `highlights.sort_updated_asc`
- `highlights.sort_created_desc`
- `highlights.sort_created_asc`
- `highlights.open_pdf`
- `highlights.delete`
- `highlights.empty_title`
- `highlights.empty_text`
- `highlights.no_results_title`
- `highlights.no_results_text`
- `highlights.load_failed`
- `highlights.delete_confirm`
- `highlights.delete_failed`

## Testing

Backend:

- Repository test for global listing with two papers and multiple annotations.
- Repository test for query matching quote text and paper title.
- Service test for pagination and sort validation.
- Handler test for `GET /api/pdf-annotations`.

Frontend:

- JS test for `Utils.resourceViewerURL` including `annotation_id`.
- JS test for `parseResourceViewerNavigationURL` preserving `annotation_id`.
- JS test for highlights page rendering rows and creating the correct viewer URL.
- JS test for viewer target highlighting when `annotation_id` matches a rendered marker.

Manual verification:

- Create highlights in two different PDFs.
- Open `/highlights`.
- Search for quote text and paper title.
- Click a result and verify the PDF opens on the correct page and scrolls to the highlight.
- Delete a highlight from the global page and verify it disappears from the PDF reader after reload.

## Out of Scope

- Multiple highlight colors.
- Highlight comments or editable annotation notes.
- Exporting highlights.
- Full-text search indexing for annotations.
- A PDF reader sidebar.
- Cross-device sync beyond the existing local SQLite database.
