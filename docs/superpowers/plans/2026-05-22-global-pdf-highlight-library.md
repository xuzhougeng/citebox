# Global PDF Highlight Library Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a global PDF highlight library page where users can search all saved PDF highlights and click any result to reopen the PDF at the original highlighted position.

**Architecture:** Reuse the existing `pdf_annotations` SQLite table and paper-scoped create/delete APIs. Add a global read-only annotation listing API that joins annotations to papers, then add a `/highlights` page and extend the PDF viewer URL/state model with optional `annotation_id` targeting. Keep first release scoped to yellow highlights, search, pagination, open, and delete.

**Tech Stack:** Go, SQLite, standard `net/http`, native HTML/CSS/JavaScript, Node `node:test` frontend tests.

---

## File Structure

- `internal/model/paper.go`: add global highlight list response models.
- `internal/repository/inputs.go`: add repository filter for global annotation listing.
- `internal/repository/pdf_annotation_repo.go`: add joined global list/count query and scanner.
- `internal/repository/pdf_annotation_repo_test.go`: add repository coverage for joined fields, search, sort, and pagination.
- `internal/service/library_service_pdf_annotations.go`: add service-level validation, pagination defaults, and PDF URL decoration.
- `internal/service/library_service_pdf_annotations_test.go`: add service tests for defaults, sort validation, page size cap, and PDF URL output.
- `internal/handler/paper.go`: add handler for `GET /api/pdf-annotations`.
- `internal/handler/paper_pdf_annotations_test.go`: add handler test for global listing.
- `internal/app/server.go`: register `/api/pdf-annotations` and `/highlights`.
- `web/highlights.html`: new global highlight library page.
- `web/static/js/api.js`: add `API.listPDFAnnotationsGlobal`.
- `web/static/js/utils.js`: pass optional `annotation_id` through viewer URLs.
- `web/static/js/viewer.js`: parse `annotation_id`, scroll to the rendered highlighter marker, and emphasize it.
- `web/viewer.html`: add target highlight CSS.
- `web/static/js/highlights.js`: new page controller for query, sort, pagination, open, and delete.
- `web/static/css/style.css`: import page stylesheet.
- `web/static/css/pages/highlights.css`: page-specific list and filter styles.
- `web/static/locales/zh-CN/common.json` and `web/static/locales/en/common.json`: add title and nav label.
- `web/static/locales/zh-CN/highlights.json` and `web/static/locales/en/highlights.json`: new page strings.
- Existing page nav markup in `web/*.html`: add the highlights navigation link consistently.
- Frontend tests:
  - `web/static/js/__tests__/utils-resource-viewer.test.cjs`
  - `web/static/js/__tests__/api-library-paper-id.test.cjs`
  - `web/static/js/__tests__/viewer-pdf-selection.test.cjs`
  - `web/static/js/__tests__/highlights-page.test.cjs`

---

### Task 1: Repository Global Annotation Listing

**Files:**
- Modify: `internal/model/paper.go`
- Modify: `internal/repository/inputs.go`
- Modify: `internal/repository/pdf_annotation_repo.go`
- Test: `internal/repository/pdf_annotation_repo_test.go`

- [ ] **Step 1: Write failing repository tests**

Append these tests to `internal/repository/pdf_annotation_repo_test.go`:

```go
func TestPDFAnnotationRepositoryListGlobalIncludesPaperFieldsAndSorts(t *testing.T) {
	repo := newTestRepository(t)
	first, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Alpha Signaling",
		DOI:              "10.1000/alpha",
		OriginalFilename: "alpha-original.pdf",
		StoredPDFName:    "alpha stored.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper(first) error = %v", err)
	}
	second, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Beta Metabolism",
		DOI:              "10.1000/beta",
		OriginalFilename: "beta-original.pdf",
		StoredPDFName:    "beta.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper(second) error = %v", err)
	}
	firstAnnotation, err := repo.PDFAnnotation.Create(first.ID, PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "alpha selected text",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 4, Left: 0.10, Top: 0.20, Width: 0.30, Height: 0.04},
		},
	})
	if err != nil {
		t.Fatalf("Create(first annotation) error = %v", err)
	}
	secondAnnotation, err := repo.PDFAnnotation.Create(second.ID, PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "beta selected text",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 2, Left: 0.11, Top: 0.21, Width: 0.31, Height: 0.04},
		},
	})
	if err != nil {
		t.Fatalf("Create(second annotation) error = %v", err)
	}

	items, total, err := repo.PDFAnnotation.ListGlobal(PDFAnnotationListFilter{
		Sort:     "updated_desc",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListGlobal() error = %v", err)
	}

	if total != 2 || len(items) != 2 {
		t.Fatalf("total/items = %d/%d, want 2/2", total, len(items))
	}
	if items[0].ID != secondAnnotation.ID || items[1].ID != firstAnnotation.ID {
		t.Fatalf("ids = %d,%d want %d,%d", items[0].ID, items[1].ID, secondAnnotation.ID, firstAnnotation.ID)
	}
	if items[0].PaperTitle != "Beta Metabolism" || items[0].PaperOriginalFilename != "beta-original.pdf" || items[0].PaperStoredPDFName != "beta.pdf" {
		t.Fatalf("paper fields = %+v", items[0])
	}
	if items[1].PageStart != 4 || items[1].QuoteText != "alpha selected text" {
		t.Fatalf("annotation fields = %+v", items[1])
	}
}

func TestPDFAnnotationRepositoryListGlobalSearchesQuoteAndPaper(t *testing.T) {
	repo := newTestRepository(t)
	first, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Immune Atlas",
		DOI:              "10.1000/immune",
		OriginalFilename: "immune.pdf",
		StoredPDFName:    "immune.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper(first) error = %v", err)
	}
	second, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Metabolic Paper",
		DOI:              "10.1000/metabolic",
		OriginalFilename: "metabolic.pdf",
		StoredPDFName:    "metabolic.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper(second) error = %v", err)
	}
	if _, err := repo.PDFAnnotation.Create(first.ID, PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "selected immune evidence",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{{Page: 1, Left: 0.1, Top: 0.2, Width: 0.3, Height: 0.04}},
	}); err != nil {
		t.Fatalf("Create(first annotation) error = %v", err)
	}
	if _, err := repo.PDFAnnotation.Create(second.ID, PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "selected metabolic evidence",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{{Page: 2, Left: 0.1, Top: 0.2, Width: 0.3, Height: 0.04}},
	}); err != nil {
		t.Fatalf("Create(second annotation) error = %v", err)
	}

	items, total, err := repo.PDFAnnotation.ListGlobal(PDFAnnotationListFilter{
		Query:    "immune",
		Sort:     "created_desc",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListGlobal(query) error = %v", err)
	}

	if total != 1 || len(items) != 1 || items[0].PaperID != first.ID {
		t.Fatalf("query result total/items = %d/%+v, want first paper only", total, items)
	}

	items, total, err = repo.PDFAnnotation.ListGlobal(PDFAnnotationListFilter{
		Query:    "10.1000/metabolic",
		Sort:     "created_desc",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListGlobal(doi query) error = %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].PaperID != second.ID {
		t.Fatalf("doi query result total/items = %d/%+v, want second paper only", total, items)
	}
}
```

- [ ] **Step 2: Run repository tests and verify the new methods are missing**

Run:

```bash
go test ./internal/repository -run 'TestPDFAnnotationRepositoryListGlobal' -count=1
```

Expected: FAIL with compiler errors mentioning `ListGlobal` and `PDFAnnotationListFilter` are undefined.

- [ ] **Step 3: Add global annotation response models**

In `internal/model/paper.go`, add after `type PDFAnnotation struct`:

```go
type PDFAnnotationListItem struct {
	ID                    int64                   `json:"id"`
	PaperID               int64                   `json:"paper_id"`
	PaperTitle            string                  `json:"paper_title"`
	PaperOriginalFilename string                  `json:"paper_original_filename"`
	PaperStoredPDFName    string                  `json:"-"`
	PaperPDFURL           string                  `json:"paper_pdf_url,omitempty"`
	Type                  string                  `json:"type"`
	PageStart             int                     `json:"page_start"`
	PageEnd               int                     `json:"page_end"`
	QuoteText             string                  `json:"quote_text"`
	Color                 string                  `json:"color"`
	Fragments             []PDFAnnotationFragment `json:"fragments"`
	NoteText              string                  `json:"note_text"`
	CreatedAt             time.Time               `json:"created_at"`
	UpdatedAt             time.Time               `json:"updated_at"`
}

type PDFAnnotationListPagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type PDFAnnotationListResponse struct {
	Annotations []PDFAnnotationListItem     `json:"annotations"`
	Pagination  PDFAnnotationListPagination `json:"pagination"`
}
```

- [ ] **Step 4: Add repository filter type**

In `internal/repository/inputs.go`, add after `type PDFAnnotationCreateInput struct`:

```go
type PDFAnnotationListFilter struct {
	Query    string
	Sort     string
	Page     int
	PageSize int
}
```

- [ ] **Step 5: Implement the repository query**

In `internal/repository/pdf_annotation_repo.go`, add imports:

```go
import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)
```

Add this method after `ListByPaperID`:

```go
func (r *PDFAnnotationRepository) ListGlobal(filter PDFAnnotationListFilter) ([]model.PDFAnnotationListItem, int, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	whereSQL, args := pdfAnnotationGlobalWhere(filter.Query)
	countSQL := `
		SELECT COUNT(*)
		FROM pdf_annotations a
		JOIN papers p ON p.id = a.paper_id
	` + whereSQL
	var total int
	if err := r.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, wrapDBError(err, "统计 PDF 标注失败")
	}

	orderSQL := pdfAnnotationGlobalOrder(filter.Sort)
	queryArgs := append([]interface{}{}, args...)
	queryArgs = append(queryArgs, pageSize, offset)
	rows, err := r.db.Query(fmt.Sprintf(`
		SELECT
			a.id, a.paper_id, p.title, p.original_filename, p.stored_pdf_name,
			a.type, a.page_start, a.page_end, a.quote_text, a.color,
			a.fragments_json, a.note_text, a.created_at, a.updated_at
		FROM pdf_annotations a
		JOIN papers p ON p.id = a.paper_id
		%s
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, whereSQL, orderSQL), queryArgs...)
	if err != nil {
		return nil, 0, wrapDBError(err, "查询 PDF 标注库失败")
	}
	defer rows.Close()

	items := []model.PDFAnnotationListItem{}
	for rows.Next() {
		item, err := scanPDFAnnotationListItem(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, wrapDBError(err, "读取 PDF 标注库失败")
	}
	return items, total, nil
}

func pdfAnnotationGlobalWhere(query string) (string, []interface{}) {
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return "", nil
	}
	like := "%" + normalized + "%"
	return `
		WHERE lower(a.quote_text) LIKE ?
			OR lower(p.title) LIKE ?
			OR lower(p.original_filename) LIKE ?
			OR lower(COALESCE(p.doi, '')) LIKE ?
	`, []interface{}{like, like, like, like}
}

func pdfAnnotationGlobalOrder(sort string) string {
	switch strings.TrimSpace(sort) {
	case "updated_asc":
		return "a.updated_at ASC, a.id ASC"
	case "created_desc":
		return "a.created_at DESC, a.id DESC"
	case "created_asc":
		return "a.created_at ASC, a.id ASC"
	default:
		return "a.updated_at DESC, a.id DESC"
	}
}
```

Add this scanner after `scanPDFAnnotation`:

```go
func scanPDFAnnotationListItem(scanner interface {
	Scan(dest ...interface{}) error
}) (*model.PDFAnnotationListItem, error) {
	var item model.PDFAnnotationListItem
	var fragmentsJSON string
	if err := scanner.Scan(
		&item.ID,
		&item.PaperID,
		&item.PaperTitle,
		&item.PaperOriginalFilename,
		&item.PaperStoredPDFName,
		&item.Type,
		&item.PageStart,
		&item.PageEnd,
		&item.QuoteText,
		&item.Color,
		&fragmentsJSON,
		&item.NoteText,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(fragmentsJSON), &item.Fragments); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "解析 PDF 标注位置失败", err)
	}
	if item.Fragments == nil {
		item.Fragments = []model.PDFAnnotationFragment{}
	}
	return &item, nil
}
```

- [ ] **Step 6: Run repository tests**

Run:

```bash
go test ./internal/repository -run 'TestPDFAnnotationRepositoryListGlobal|TestPDFAnnotationRepositoryCreateAndList|TestPDFAnnotationRepositoryDeleteRequiresPaperOwnership|TestPDFAnnotationRepositoryCascadesWhenPaperDeleted' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit repository slice**

```bash
git add internal/model/paper.go internal/repository/inputs.go internal/repository/pdf_annotation_repo.go internal/repository/pdf_annotation_repo_test.go
git commit -m "feat: list PDF highlights globally"
```

---

### Task 2: Service, Handler, and API Route

**Files:**
- Modify: `internal/service/library_service_pdf_annotations.go`
- Test: `internal/service/library_service_pdf_annotations_test.go`
- Modify: `internal/handler/paper.go`
- Test: `internal/handler/paper_pdf_annotations_test.go`
- Modify: `internal/app/server.go`

- [ ] **Step 1: Write failing service tests**

Append this test to `internal/service/library_service_pdf_annotations_test.go`:

```go
func TestListPDFAnnotationsGlobalDefaultsAndDecoratesPDFURL(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)
	if _, err := svc.CreatePDFAnnotation(paper.ID, CreatePDFAnnotationParams{
		QuoteText: "global selected text",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 5, Left: 0.1, Top: 0.2, Width: 0.3, Height: 0.04},
		},
	}); err != nil {
		t.Fatalf("CreatePDFAnnotation() error = %v", err)
	}

	result, err := svc.ListPDFAnnotationsGlobal(ListPDFAnnotationsParams{})
	if err != nil {
		t.Fatalf("ListPDFAnnotationsGlobal() error = %v", err)
	}

	if result.Pagination.Page != 1 || result.Pagination.PageSize != 50 || result.Pagination.Total != 1 || result.Pagination.TotalPages != 1 {
		t.Fatalf("pagination = %+v, want page 1 size 50 total 1 total_pages 1", result.Pagination)
	}
	if len(result.Annotations) != 1 {
		t.Fatalf("annotations = %d, want 1", len(result.Annotations))
	}
	got := result.Annotations[0]
	if got.PaperTitle != paper.Title || got.PaperPDFURL != "/files/papers/test.pdf" || got.PageStart != 5 {
		t.Fatalf("annotation item = %+v", got)
	}
}

func TestListPDFAnnotationsGlobalValidationAndPageSizeCap(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.ListPDFAnnotationsGlobal(ListPDFAnnotationsParams{Sort: "bad"}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("bad sort code = %q, want %q (err=%v)", apperr.CodeOf(err), apperr.CodeInvalidArgument, err)
	}

	result, err := svc.ListPDFAnnotationsGlobal(ListPDFAnnotationsParams{Page: -3, PageSize: 1000, Sort: "created_asc"})
	if err != nil {
		t.Fatalf("ListPDFAnnotationsGlobal(capped) error = %v", err)
	}
	if result.Pagination.Page != 1 || result.Pagination.PageSize != 100 {
		t.Fatalf("pagination = %+v, want page 1 size 100", result.Pagination)
	}
}
```

- [ ] **Step 2: Run service tests and verify they fail**

Run:

```bash
go test ./internal/service -run 'TestListPDFAnnotationsGlobal' -count=1
```

Expected: FAIL with compiler errors mentioning `ListPDFAnnotationsGlobal` and `ListPDFAnnotationsParams` are undefined.

- [ ] **Step 3: Implement the service method**

In `internal/service/library_service_pdf_annotations.go`, update imports:

```go
import (
	"math"
	"net/url"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)
```

Add after `type CreatePDFAnnotationParams struct`:

```go
type ListPDFAnnotationsParams struct {
	Query    string
	Sort     string
	Page     int
	PageSize int
}
```

Add after `ListPDFAnnotations`:

```go
func (s *LibraryService) ListPDFAnnotationsGlobal(params ListPDFAnnotationsParams) (*model.PDFAnnotationListResponse, error) {
	filter, err := normalizePDFAnnotationListFilter(params)
	if err != nil {
		return nil, err
	}
	items, total, err := s.repo.PDFAnnotation.ListGlobal(filter)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].PaperStoredPDFName != "" {
			items[i].PaperPDFURL = "/files/papers/" + url.PathEscape(items[i].PaperStoredPDFName)
		}
	}
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(filter.PageSize)))
	}
	return &model.PDFAnnotationListResponse{
		Annotations: items,
		Pagination: model.PDFAnnotationListPagination{
			Page:       filter.Page,
			PageSize:   filter.PageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	}, nil
}

func normalizePDFAnnotationListFilter(params ListPDFAnnotationsParams) (repository.PDFAnnotationListFilter, error) {
	sort := strings.TrimSpace(params.Sort)
	if sort == "" {
		sort = "updated_desc"
	}
	switch sort {
	case "updated_desc", "updated_asc", "created_desc", "created_asc":
	default:
		return repository.PDFAnnotationListFilter{}, apperr.New(apperr.CodeInvalidArgument, "PDF 标注排序方式无效")
	}

	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	return repository.PDFAnnotationListFilter{
		Query:    strings.TrimSpace(params.Query),
		Sort:     sort,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
```

- [ ] **Step 4: Run service tests**

Run:

```bash
go test ./internal/service -run 'TestListPDFAnnotationsGlobal|TestCreatePDFAnnotation|TestPDFAnnotationServiceRequiresExistingPaper|TestDeletePDFAnnotationRequiresPaperOwnership' -count=1
```

Expected: PASS.

- [ ] **Step 5: Write failing handler test**

Append this test to `internal/handler/paper_pdf_annotations_test.go`:

```go
func TestPaperHandlerListPDFAnnotationsGlobal(t *testing.T) {
	h, repo := newPaperHandlerForTest(t)
	paper := createHandlerTestPaper(t, repo)
	if _, err := repo.PDFAnnotation.Create(paper.ID, repository.PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "handler global text",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 6, Left: 0.12, Top: 0.34, Width: 0.28, Height: 0.018},
		},
	}); err != nil {
		t.Fatalf("Create annotation error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/pdf-annotations?query=global&page=1&page_size=25&sort=created_desc", nil)
	rec := httptest.NewRecorder()
	h.ListPDFAnnotationsGlobal(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Success     bool                            `json:"success"`
		Annotations []model.PDFAnnotationListItem   `json:"annotations"`
		Pagination  model.PDFAnnotationListPagination `json:"pagination"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !body.Success || len(body.Annotations) != 1 {
		t.Fatalf("body = %+v", body)
	}
	if body.Annotations[0].PaperTitle != "Handler Paper" || body.Annotations[0].PaperPDFURL != "/files/papers/handler.pdf" {
		t.Fatalf("annotation paper fields = %+v", body.Annotations[0])
	}
	if body.Pagination.Page != 1 || body.Pagination.PageSize != 25 || body.Pagination.Total != 1 {
		t.Fatalf("pagination = %+v", body.Pagination)
	}
}
```

Run:

```bash
go test ./internal/handler -run 'TestPaperHandlerListPDFAnnotationsGlobal' -count=1
```

Expected: FAIL with `h.ListPDFAnnotationsGlobal undefined`.

- [ ] **Step 6: Implement the handler**

In `internal/handler/paper.go`, add this method before `ListPDFAnnotations`:

```go
func (h *PaperHandler) ListPDFAnnotationsGlobal(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	result, err := h.service.ListPDFAnnotationsGlobal(service.ListPDFAnnotationsParams{
		Query:    r.URL.Query().Get("query"),
		Sort:     r.URL.Query().Get("sort"),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"annotations": result.Annotations,
		"pagination":  result.Pagination,
	})
}
```

`internal/handler/paper.go` already imports `strconv` for other handlers. If it does not after local changes, add it to the import block.

- [ ] **Step 7: Register the global API route**

In `internal/app/server.go`, insert this route after the `/api/papers/purge` route and before `/api/papers/`:

```go
	mux.HandleFunc("/api/pdf-annotations", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			paperHandler.ListPDFAnnotationsGlobal(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
```

- [ ] **Step 8: Run handler and app package tests**

Run:

```bash
go test ./internal/handler ./internal/app -run 'TestPaperHandlerListPDFAnnotationsGlobal|TestPaperHandlerPDFAnnotationsCreateListDelete' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit API slice**

```bash
git add internal/service/library_service_pdf_annotations.go internal/service/library_service_pdf_annotations_test.go internal/handler/paper.go internal/handler/paper_pdf_annotations_test.go internal/app/server.go
git commit -m "feat: expose global PDF highlight API"
```

---

### Task 3: Viewer URL Metadata and PDF Targeting

**Files:**
- Modify: `web/static/js/utils.js`
- Test: `web/static/js/__tests__/utils-resource-viewer.test.cjs`
- Modify: `web/static/js/viewer.js`
- Modify: `web/viewer.html`
- Test: `web/static/js/__tests__/viewer-pdf-selection.test.cjs`

- [ ] **Step 1: Write failing utility tests**

In `web/static/js/__tests__/utils-resource-viewer.test.cjs`, add this test after `resourceViewerURL includes paper metadata for PDF reader links`:

```js
test('resourceViewerURL includes target annotation metadata for PDF reader links', () => {
    const Utils = loadUtils();
    const href = Utils.resourceViewerURL('pdf', '/files/papers/a.pdf', '/highlights', {
        paperId: 42,
        annotationId: 99,
        page: 7,
    });
    const url = new URL(href, 'http://localhost');

    assert.equal(url.searchParams.get('paper_id'), '42');
    assert.equal(url.searchParams.get('annotation_id'), '99');
    assert.equal(url.searchParams.get('page'), '7');
});
```

In the existing `parseResourceViewerNavigationURL returns paper metadata` test, change the URL and expected object:

```js
const parsed = Utils.parseResourceViewerNavigationURL('/viewer?kind=pdf&src=%2Ffiles%2Fpapers%2Fa.pdf&back=%2Flibrary&paper_id=42&page=7&annotation_id=99');

assert.deepEqual(JSON.parse(JSON.stringify(parsed)), {
    kind: 'pdf',
    src: '/files/papers/a.pdf',
    back: '/library',
    paperId: '42',
    page: '7',
    annotationId: '99',
});
```

Run:

```bash
node --test web/static/js/__tests__/utils-resource-viewer.test.cjs
```

Expected: FAIL because `annotation_id` is missing from URL creation and parsing.

- [ ] **Step 2: Pass annotation IDs through utility URLs**

In `web/static/js/utils.js`, update `resourceViewerURL` after the `paper_id` handling:

```js
const page = String(options.page ?? '').trim();
if (page && page !== '0') {
    params.set('page', page);
}
const annotationId = String(options.annotationId ?? options.annotation_id ?? '').trim();
if (annotationId && annotationId !== '0') {
    params.set('annotation_id', annotationId);
}
```

Update `parseResourceViewerNavigationURL` return value:

```js
return {
    kind,
    src,
    back: String(url.searchParams.get('back') || window.location.href).trim(),
    paperId: String(url.searchParams.get('paper_id') || '').trim(),
    page: String(url.searchParams.get('page') || '').trim(),
    annotationId: String(url.searchParams.get('annotation_id') || '').trim()
};
```

Update `bindResourceViewerLinks` where it calls `openResourceViewer`:

```js
this.openResourceViewer(resource.kind, resource.src, resource.back, {
    paperId: resource.paperId,
    page: resource.page,
    annotationId: resource.annotationId
});
```

- [ ] **Step 3: Run utility tests**

Run:

```bash
node --test web/static/js/__tests__/utils-resource-viewer.test.cjs
```

Expected: PASS.

- [ ] **Step 4: Write failing viewer targeting test**

In `web/static/js/__tests__/viewer-pdf-selection.test.cjs`, update `createElement.querySelector` to support target marker lookup:

```js
if (selector.startsWith('[data-highlight-id="')) {
    const id = selector.match(/\[data-highlight-id="([^"]+)"\]/)?.[1] || '';
    const stack = Array.from(this.children);
    while (stack.length) {
        const node = stack.shift();
        if (String(node?.dataset?.highlightId || '') === id) return node;
        stack.push(...Array.from(node?.children || []));
    }
    return null;
}
```

Then append this test:

```js
test('PDF target annotation scrolls matching rendered highlight into view', () => {
    const { viewer } = loadViewerPage(null);
    const page = createElement('page');
    page.classList.add('page');
    page.dataset.pageNumber = '3';
    page.getBoundingClientRect = () => rect(0, 0, 1000, 1200);
    const pdfViewer = createElement('pdfViewer');
    pdfViewer.appendChild(page);
    const stage = createElement('stage');
    stage.pdfViewer = pdfViewer;

    let scrolled = false;
    viewer.stage = stage;
    viewer.pdfState = {
        ...viewer.defaultPDFState(),
        targetAnnotationId: '11',
        targetAnnotationApplied: false,
        highlights: [{
            id: 11,
            quote_text: 'target highlight',
            fragments: [{ page: 3, left: 0.1, top: 0.2, width: 0.3, height: 0.04 }],
        }],
    };
    page.appendChild = function appendChild(child) {
        child.parentElement = this;
        this.children.add(child);
        child.scrollIntoView = function scrollIntoView(options) {
            scrolled = options?.block === 'center';
        };
    };

    viewer.renderPDFHighlights();

    assert.equal(scrolled, true);
    const marker = stage.querySelector('[data-highlight-id="11"]');
    assert.equal(marker.classList.contains('is-target-highlight'), true);
    assert.equal(viewer.pdfState.targetAnnotationApplied, true);
});
```

Run:

```bash
node --test web/static/js/__tests__/viewer-pdf-selection.test.cjs
```

Expected: FAIL because `targetAnnotationId` and targeting behavior are not implemented.

- [ ] **Step 5: Implement viewer target state and behavior**

In `web/static/js/viewer.js`, add fields to `defaultPDFState()`:

```js
targetAnnotationId: '',
targetAnnotationApplied: false,
targetAnnotationTimer: null,
```

In `renderPDFResource(resource)`, add these fields in the new `pdfState` object:

```js
targetAnnotationId: this.initialPDFAnnotationID(resource),
targetAnnotationApplied: false,
targetAnnotationTimer: null,
```

Add this method after `initialPDFPageNumber()`:

```js
initialPDFAnnotationID(resource = null) {
    const fromResource = String(resource?.annotationId || resource?.annotation_id || '').trim();
    if (fromResource && fromResource !== '0') return fromResource;
    const fromURL = String(new URLSearchParams(window.location.search).get('annotation_id') || '').trim();
    return fromURL && fromURL !== '0' ? fromURL : '';
}
```

In `parseResource()`, include the parsed annotation ID in the PDF resource object:

```js
annotationId: String(params.get('annotation_id') || '').trim()
```

Add this method after `renderPDFHighlights()`:

```js
applyPDFTargetAnnotation() {
    const targetID = String(this.pdfState?.targetAnnotationId || '').trim();
    if (!targetID || this.pdfState?.targetAnnotationApplied) return false;
    const marker = this.stage?.querySelector?.(`[data-highlight-id="${CSS.escape(targetID)}"]`);
    if (!marker) return false;

    this.pdfState.targetAnnotationApplied = true;
    marker.scrollIntoView?.({ block: 'center', inline: 'nearest', behavior: 'smooth' });
    marker.classList?.add('is-target-highlight');
    window.clearTimeout(this.pdfState.targetAnnotationTimer);
    this.pdfState.targetAnnotationTimer = window.setTimeout(() => {
        marker.classList?.remove('is-target-highlight');
        if (this.pdfState) {
            this.pdfState.targetAnnotationTimer = null;
        }
    }, 2200);
    return true;
}
```

If the test context does not provide `CSS.escape`, make the production code robust:

```js
const escapedID = typeof CSS !== 'undefined' && typeof CSS.escape === 'function'
    ? CSS.escape(targetID)
    : targetID.replace(/["\\]/g, '\\$&');
const marker = this.stage?.querySelector?.(`[data-highlight-id="${escapedID}"]`);
```

At the end of `renderPDFHighlights()`, after all markers are appended, call:

```js
this.applyPDFTargetAnnotation();
```

- [ ] **Step 6: Add target highlight CSS**

In `web/viewer.html`, add after `.viewer-pdf-highlight-fragment`:

```css
.viewer-pdf-highlight-fragment.is-target-highlight {
    background: rgba(255, 199, 0, 0.42);
    box-shadow: 0 0 0 2px rgba(181, 121, 0, 0.36), 0 0 18px rgba(255, 199, 0, 0.38);
}
```

- [ ] **Step 7: Run viewer tests and syntax checks**

Run:

```bash
node --test web/static/js/__tests__/viewer-pdf-selection.test.cjs
node --check web/static/js/viewer.js
```

Expected: PASS.

- [ ] **Step 8: Commit viewer targeting slice**

```bash
git add web/static/js/utils.js web/static/js/viewer.js web/viewer.html web/static/js/__tests__/utils-resource-viewer.test.cjs web/static/js/__tests__/viewer-pdf-selection.test.cjs
git commit -m "feat: target PDF highlights from viewer links"
```

---

### Task 4: Global Highlight Page

**Files:**
- Create: `web/highlights.html`
- Create: `web/static/js/highlights.js`
- Create: `web/static/css/pages/highlights.css`
- Modify: `web/static/css/style.css`
- Modify: `web/static/js/api.js`
- Modify: `web/static/js/main.js`
- Modify: `internal/app/server.go`
- Create: `web/static/locales/zh-CN/highlights.json`
- Create: `web/static/locales/en/highlights.json`
- Modify: `web/static/locales/zh-CN/common.json`
- Modify: `web/static/locales/en/common.json`
- Modify nav markup in: `web/ai.html`, `web/figures.html`, `web/groups.html`, `web/guide.html`, `web/index.html`, `web/library.html`, `web/manual.html`, `web/notes.html`, `web/overview.html`, `web/palettes.html`, `web/research.html`, `web/settings.html`, `web/tags.html`, `web/upload.html`
- Test: `web/static/js/__tests__/api-library-paper-id.test.cjs`
- Test: `web/static/js/__tests__/highlights-page.test.cjs`

- [ ] **Step 1: Write failing API helper test**

In `web/static/js/__tests__/api-library-paper-id.test.cjs`, add this test after the existing PDF annotation API test:

```js
test('global PDF annotation list API builds query string', async () => {
    const { API, requests } = loadAPI();

    await API.listPDFAnnotationsGlobal({
        query: 'immune',
        sort: 'created_desc',
        page: 2,
        page_size: 25,
    });

    assert.equal(requests[0], '/api/pdf-annotations?query=immune&sort=created_desc&page=2&page_size=25');
});
```

Run:

```bash
node --test web/static/js/__tests__/api-library-paper-id.test.cjs
```

Expected: FAIL because `API.listPDFAnnotationsGlobal` is undefined.

- [ ] **Step 2: Add API helper**

In `web/static/js/api.js`, add after `listPDFAnnotations(paperId)`:

```js
listPDFAnnotationsGlobal(params = {}) {
    const query = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
        if (value !== undefined && value !== null && value !== '') {
            query.set(key, value);
        }
    });
    const suffix = query.toString() ? `?${query.toString()}` : '';
    return requestJSON(`${API_BASE}/pdf-annotations${suffix}`);
},
```

Run:

```bash
node --test web/static/js/__tests__/api-library-paper-id.test.cjs
node --check web/static/js/api.js
```

Expected: PASS.

- [ ] **Step 3: Create page HTML**

Create `web/highlights.html`:

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>高亮库 - CiteBox</title>
    <link rel="icon" type="image/svg+xml" href="/static/img/citebox-icon.svg">
    <script>(function(){var d=document.documentElement;try{var t=localStorage.getItem("citebox_theme");if(t&&t!=="warm")d.setAttribute("data-theme",t);var l=localStorage.getItem("citebox_lang")||"zh-CN";d.lang=l;if(l!=="zh-CN")d.setAttribute("data-lang-loading","1");}catch(e){}})()</script>
    <link rel="stylesheet" href="/static/css/style.css">
</head>
<body>
    <nav class="navbar">
        <div class="navbar-content">
            <a href="/" class="logo">CiteBox</a>
            <ul class="nav-links">
                <li><a href="/" data-i18n="nav.overview">总览</a></li>
                <li><a href="/library" data-i18n="nav.library">文献库</a></li>
                <li><a href="/figures" data-i18n="nav.figures">图片库</a></li>
                <li><a href="/palettes" data-i18n="nav.palettes">配色库</a></li>
                <li><a href="/groups" data-i18n="nav.groups">分组</a></li>
                <li><a href="/tags" data-i18n="nav.tags">标签</a></li>
                <li><a href="/notes" data-i18n="nav.notes">笔记</a></li>
                <li><a href="/highlights" class="active" data-i18n="nav.highlights">高亮库</a></li>
                <li><a href="/research" data-i18n="nav.research">文献搜索</a></li>
                <li><a href="/ai" data-i18n="nav.ai">AI 助手</a></li>
                <li><a href="/settings" data-i18n="nav.settings">配置</a></li>
            </ul>
            <div class="nav-actions">
                <a href="/upload" class="nav-cta" data-i18n="nav.upload_pdf">上传 PDF</a>
            </div>
        </div>
    </nav>

    <main class="page-shell">
        <section class="hero compact">
            <div>
                <p class="eyebrow" data-i18n="highlights.hero_eyebrow">Highlights</p>
                <h1 data-i18n="highlights.hero_title">高亮库</h1>
                <p class="hero-text" data-i18n="highlights.hero_text">集中查看所有 PDF 阅读高亮，点击回到原文位置。</p>
            </div>
        </section>

        <section class="filters-card highlights-filter-card">
            <div class="filters-grid highlights-filters-grid">
                <label class="field">
                    <span data-i18n="highlights.search_label">搜索高亮</span>
                    <input id="highlightQuery" class="form-input" type="text" data-i18n-placeholder="highlights.search_placeholder" placeholder="高亮文本、文献标题、文件名或 DOI">
                </label>
                <label class="field">
                    <span data-i18n="highlights.sort_label">排序</span>
                    <select id="highlightSort" class="form-input">
                        <option value="updated_desc" data-i18n="highlights.sort_updated_desc">最近更新优先</option>
                        <option value="updated_asc" data-i18n="highlights.sort_updated_asc">最早更新优先</option>
                        <option value="created_desc" data-i18n="highlights.sort_created_desc">最近创建优先</option>
                        <option value="created_asc" data-i18n="highlights.sort_created_asc">最早创建优先</option>
                    </select>
                </label>
            </div>
        </section>

        <section class="library-shell highlights-shell">
            <div id="highlightResultMeta" class="library-result-meta"></div>
            <div id="highlightList" class="highlight-list"></div>
            <div id="highlightPagination" class="pagination"></div>
        </section>
    </main>

    <script src="/static/js/theme.js"></script>
    <script src="/static/js/i18n.js"></script>
    <script src="/static/js/utils.js"></script>
    <script src="/static/js/api.js"></script>
    <script src="/static/js/highlights.js"></script>
    <script src="/static/js/translate.js"></script>
    <script src="/static/js/main.js"></script>
</body>
</html>
```

- [ ] **Step 4: Add page route and navigation dropdown membership**

In `internal/app/server.go`, add to the static page switch near other page routes:

```go
		case "/highlights", "/highlights.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "highlights.html"))
```

In `web/static/js/main.js`, change `SECONDARY_HREFS`:

```js
SECONDARY_HREFS: ['/highlights', '/palettes', '/groups', '/tags', '/settings'],
```

In every existing `web/*.html` page with the shared nav, add this nav item immediately after the notes link:

```html
<li><a href="/highlights" data-i18n="nav.highlights">高亮库</a></li>
```

Do not add it to `web/login.html`, because the login page does not use the main application navigation.

- [ ] **Step 5: Add locales**

In `web/static/locales/zh-CN/common.json`, add:

```json
"_title.highlights": "高亮库 - CiteBox",
"nav.highlights": "高亮库",
```

In `web/static/locales/en/common.json`, add:

```json
"_title.highlights": "Highlight Library - CiteBox",
"nav.highlights": "Highlights",
```

Create `web/static/locales/zh-CN/highlights.json`:

```json
{
    "_title": "高亮库 - CiteBox",
    "highlights.hero_eyebrow": "Highlights",
    "highlights.hero_title": "高亮库",
    "highlights.hero_text": "集中查看所有 PDF 阅读高亮，点击回到原文位置。",
    "highlights.search_label": "搜索高亮",
    "highlights.search_placeholder": "高亮文本、文献标题、文件名或 DOI",
    "highlights.sort_label": "排序",
    "highlights.sort_updated_desc": "最近更新优先",
    "highlights.sort_updated_asc": "最早更新优先",
    "highlights.sort_created_desc": "最近创建优先",
    "highlights.sort_created_asc": "最早创建优先",
    "highlights.open_pdf": "打开 PDF",
    "highlights.delete": "删除",
    "highlights.page_label": "第 {page} 页",
    "highlights.page_range_label": "第 {start}-{end} 页",
    "highlights.result_meta": "共 {count} 条高亮",
    "highlights.empty_title": "还没有 PDF 高亮",
    "highlights.empty_text": "打开文献 PDF，划选文本后点击高亮，这里会集中展示。",
    "highlights.no_results_title": "没有匹配的高亮",
    "highlights.no_results_text": "换一个关键词，或清空搜索条件后再试。",
    "highlights.load_failed": "高亮库加载失败",
    "highlights.delete_confirm": "删除这条高亮？",
    "highlights.delete_failed": "高亮删除失败",
    "highlights.deleted": "高亮已删除"
}
```

Create `web/static/locales/en/highlights.json`:

```json
{
    "_title": "Highlight Library - CiteBox",
    "highlights.hero_eyebrow": "Highlights",
    "highlights.hero_title": "Highlight Library",
    "highlights.hero_text": "Review every saved PDF highlight and jump back to its original location.",
    "highlights.search_label": "Search highlights",
    "highlights.search_placeholder": "Highlight text, paper title, filename, or DOI",
    "highlights.sort_label": "Sort",
    "highlights.sort_updated_desc": "Recently updated",
    "highlights.sort_updated_asc": "Oldest updated",
    "highlights.sort_created_desc": "Recently created",
    "highlights.sort_created_asc": "Oldest created",
    "highlights.open_pdf": "Open PDF",
    "highlights.delete": "Delete",
    "highlights.page_label": "Page {page}",
    "highlights.page_range_label": "Pages {start}-{end}",
    "highlights.result_meta": "{count} highlights",
    "highlights.empty_title": "No PDF highlights yet",
    "highlights.empty_text": "Open a paper PDF, select text, and click Highlight to collect it here.",
    "highlights.no_results_title": "No matching highlights",
    "highlights.no_results_text": "Try another keyword or clear the search.",
    "highlights.load_failed": "Failed to load highlights",
    "highlights.delete_confirm": "Delete this highlight?",
    "highlights.delete_failed": "Failed to delete highlight",
    "highlights.deleted": "Highlight deleted"
}
```

- [ ] **Step 6: Add page styles**

Create `web/static/css/pages/highlights.css`:

```css
.highlights-filter-card {
    margin-bottom: 1rem;
}

.highlights-filters-grid {
    grid-template-columns: minmax(16rem, 1fr) minmax(12rem, 16rem);
}

.highlight-list {
    display: grid;
    gap: 0.75rem;
}

.highlight-card {
    display: grid;
    gap: 0.75rem;
    padding: 1rem;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    box-shadow: var(--shadow-sm);
}

.highlight-card-main {
    display: grid;
    gap: 0.45rem;
    min-width: 0;
}

.highlight-quote {
    margin: 0;
    color: var(--text);
    font-size: 0.98rem;
    line-height: 1.55;
}

.highlight-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem 0.85rem;
    color: var(--text-muted);
    font-size: 0.85rem;
}

.highlight-paper-title {
    color: var(--text);
    font-weight: 700;
}

.highlight-card-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    justify-content: flex-end;
}

@media (min-width: 780px) {
    .highlight-card {
        grid-template-columns: minmax(0, 1fr) auto;
        align-items: center;
    }
}

@media (max-width: 680px) {
    .highlights-filters-grid {
        grid-template-columns: 1fr;
    }

    .highlight-card-actions {
        justify-content: flex-start;
    }
}
```

In `web/static/css/style.css`, add after the notes import:

```css
@import url('pages/highlights.css');
```

- [ ] **Step 7: Implement page controller**

Create `web/static/js/highlights.js`:

```js
if (typeof window.t !== 'function') window.t = function(k,f){return f||k};

const HighlightLibraryPage = {
    state: {
        query: '',
        sort: 'updated_desc',
        page: 1,
        pageSize: 50,
        annotations: [],
        pagination: { page: 1, page_size: 50, total: 0, total_pages: 0 },
        loading: false,
    },

    init() {
        this.queryInput = document.getElementById('highlightQuery');
        this.sortInput = document.getElementById('highlightSort');
        this.resultMeta = document.getElementById('highlightResultMeta');
        this.list = document.getElementById('highlightList');
        this.pagination = document.getElementById('highlightPagination');
        if (!this.list) return;

        this.queryInput?.addEventListener('input', Utils.debounce ? Utils.debounce(() => {
            this.state.query = this.queryInput.value.trim();
            this.state.page = 1;
            this.load();
        }, 250) : () => {
            this.state.query = this.queryInput.value.trim();
            this.state.page = 1;
            this.load();
        });
        this.sortInput?.addEventListener('change', () => {
            this.state.sort = this.sortInput.value || 'updated_desc';
            this.state.page = 1;
            this.load();
        });
        this.list.addEventListener('click', async (event) => {
            const deleteButton = event.target.closest('[data-highlight-action="delete"]');
            if (deleteButton) {
                event.preventDefault();
                await this.deleteHighlight(deleteButton.dataset.paperId, deleteButton.dataset.annotationId);
                return;
            }
            const openButton = event.target.closest('[data-highlight-action="open"]');
            if (openButton) {
                event.preventDefault();
                this.openHighlight(openButton.dataset.annotationId);
                return;
            }
            const card = event.target.closest('[data-highlight-id]');
            if (card) {
                this.openHighlight(card.dataset.highlightId);
            }
        });
        Utils.bindPagination?.(this.pagination, async (page) => {
            this.state.page = page;
            await this.load();
        });
        this.load();
    },

    async load() {
        if (typeof API === 'undefined' || typeof API.listPDFAnnotationsGlobal !== 'function') return;
        this.state.loading = true;
        try {
            const payload = await API.listPDFAnnotationsGlobal({
                query: this.state.query,
                sort: this.state.sort,
                page: this.state.page,
                page_size: this.state.pageSize,
            });
            this.state.annotations = Array.isArray(payload.annotations) ? payload.annotations : [];
            this.state.pagination = payload.pagination || { page: 1, page_size: this.state.pageSize, total: 0, total_pages: 0 };
            this.state.page = Number(this.state.pagination.page) || this.state.page;
            this.render();
        } catch (error) {
            Utils.showToast?.(t('highlights.load_failed', '高亮库加载失败'), 'error');
            this.render();
        } finally {
            this.state.loading = false;
        }
    },

    render() {
        const total = Number(this.state.pagination?.total || 0);
        if (this.resultMeta) {
            this.resultMeta.textContent = t('highlights.result_meta', '共 {count} 条高亮').replace('{count}', total);
        }
        if (!this.list) return;
        if (!this.state.annotations.length) {
            const hasQuery = Boolean(this.state.query);
            this.list.innerHTML = `<div class="empty-state"><h3>${Utils.escapeHTML(t(hasQuery ? 'highlights.no_results_title' : 'highlights.empty_title', hasQuery ? '没有匹配的高亮' : '还没有 PDF 高亮'))}</h3><p>${Utils.escapeHTML(t(hasQuery ? 'highlights.no_results_text' : 'highlights.empty_text', hasQuery ? '换一个关键词，或清空搜索条件后再试。' : '打开文献 PDF，划选文本后点击高亮，这里会集中展示。'))}</p></div>`;
        } else {
            this.list.innerHTML = this.state.annotations.map((item) => this.renderHighlight(item)).join('');
        }
        Utils.renderPagination?.(this.pagination, this.state.page, Number(this.state.pagination?.total_pages || 0));
    },

    renderHighlight(item) {
        const id = String(item.id || '');
        const paperId = String(item.paper_id || '');
        const pageLabel = this.pageLabel(item);
        const dateText = Utils.formatDate(item.updated_at || item.created_at);
        return `
            <article class="highlight-card" data-highlight-id="${Utils.escapeHTML(id)}">
                <div class="highlight-card-main">
                    <p class="highlight-quote">${Utils.escapeHTML(item.quote_text || '')}</p>
                    <div class="highlight-meta">
                        <span class="highlight-paper-title">${Utils.escapeHTML(item.paper_title || item.paper_original_filename || '')}</span>
                        <span>${Utils.escapeHTML(pageLabel)}</span>
                        <span>${Utils.escapeHTML(dateText)}</span>
                    </div>
                </div>
                <div class="highlight-card-actions">
                    <button class="btn btn-outline" type="button" data-highlight-action="open" data-annotation-id="${Utils.escapeHTML(id)}">${t('highlights.open_pdf', '打开 PDF')}</button>
                    <button class="btn btn-outline" type="button" data-highlight-action="delete" data-paper-id="${Utils.escapeHTML(paperId)}" data-annotation-id="${Utils.escapeHTML(id)}">${t('highlights.delete', '删除')}</button>
                </div>
            </article>
        `;
    },

    pageLabel(item) {
        const start = Math.max(1, Math.floor(Number(item.page_start) || 1));
        const end = Math.max(start, Math.floor(Number(item.page_end) || start));
        if (start === end) {
            return t('highlights.page_label', '第 {page} 页').replace('{page}', start);
        }
        return t('highlights.page_range_label', '第 {start}-{end} 页')
            .replace('{start}', start)
            .replace('{end}', end);
    },

    findHighlight(annotationId) {
        const id = String(annotationId || '');
        return this.state.annotations.find((item) => String(item.id || '') === id) || null;
    },

    openHighlight(annotationId) {
        const item = this.findHighlight(annotationId);
        if (!item) return;
        if (!item.paper_pdf_url) {
            Utils.showToast?.(t('shared.paper.no_pdf_url', '当前文献缺少 PDF 文件地址'), 'error');
            return;
        }
        window.location.href = Utils.resourceViewerURL('pdf', item.paper_pdf_url, window.location.href, {
            paperId: item.paper_id,
            annotationId: item.id,
            page: item.page_start,
        });
    },

    async deleteHighlight(paperId, annotationId) {
        if (!paperId || !annotationId || typeof API === 'undefined') return;
        const confirmed = await Utils.confirm(t('highlights.delete_confirm', '删除这条高亮？'));
        if (!confirmed) return;
        try {
            await API.deletePDFAnnotation(paperId, annotationId);
            Utils.showToast?.(t('highlights.deleted', '高亮已删除'));
            await this.load();
        } catch (error) {
            Utils.showToast?.(t('highlights.delete_failed', '高亮删除失败'), 'error');
        }
    },
};

document.addEventListener('DOMContentLoaded', () => {
    if (window.CiteBoxI18n && typeof CiteBoxI18n.init === 'function') {
        CiteBoxI18n.init().then(() => HighlightLibraryPage.init());
    } else {
        HighlightLibraryPage.init();
    }
});
```

- [ ] **Step 8: Write page controller test**

Create `web/static/js/__tests__/highlights-page.test.cjs`:

```js
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const modulePath = path.resolve(__dirname, '..', 'highlights.js');

function createElement(id) {
    return {
        id,
        value: '',
        innerHTML: '',
        textContent: '',
        dataset: {},
        listeners: {},
        addEventListener(type, handler) { this.listeners[type] = handler; },
        closest() { return null; },
    };
}

function loadPage() {
    const elements = {
        highlightQuery: createElement('highlightQuery'),
        highlightSort: createElement('highlightSort'),
        highlightResultMeta: createElement('highlightResultMeta'),
        highlightList: createElement('highlightList'),
        highlightPagination: createElement('highlightPagination'),
    };
    const viewerURLs = [];
    const code = fs.readFileSync(modulePath, 'utf8') + '\nglobalThis.__TEST_HIGHLIGHTS__ = HighlightLibraryPage;';
    const context = {
        console,
        URL,
        URLSearchParams,
        window: {
            location: { href: 'http://localhost/highlights' },
        },
        document: {
            addEventListener(type, handler) {
                if (type === 'DOMContentLoaded') handler();
            },
            getElementById(id) {
                return elements[id] || null;
            },
        },
        API: {
            listPDFAnnotationsGlobal() {
                return Promise.resolve({
                    annotations: [{
                        id: 11,
                        paper_id: 42,
                        paper_title: 'Example Paper',
                        paper_pdf_url: '/files/papers/example.pdf',
                        page_start: 3,
                        page_end: 3,
                        quote_text: 'selected highlight text',
                        updated_at: '2026-05-22T10:00:00Z',
                    }],
                    pagination: { page: 1, page_size: 50, total: 1, total_pages: 1 },
                });
            },
            deletePDFAnnotation() {
                return Promise.resolve({ success: true });
            },
        },
        Utils: {
            escapeHTML(value) { return String(value || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;'); },
            formatDate() { return '2026/05/22 10:00'; },
            renderPagination() {},
            bindPagination() {},
            showToast() {},
            confirm() { return Promise.resolve(true); },
            resourceViewerURL(kind, src, back, options) {
                viewerURLs.push({ kind, src, back, options });
                return `/viewer?kind=${kind}&src=${encodeURIComponent(src)}&paper_id=${options.paperId}&page=${options.page}&annotation_id=${options.annotationId}`;
            },
        },
        t(key, fallback) { return fallback || key; },
    };
    context.globalThis = context;
    vm.runInNewContext(code, context, { filename: modulePath });
    return { page: context.__TEST_HIGHLIGHTS__, elements, context, viewerURLs };
}

test('highlight library renders annotations and opens target PDF URL', async () => {
    const { page, elements, context, viewerURLs } = loadPage();
    await new Promise((resolve) => setImmediate(resolve));

    assert.match(elements.highlightList.innerHTML, /selected highlight text/);
    assert.match(elements.highlightList.innerHTML, /Example Paper/);
    page.openHighlight(11);

    assert.equal(context.window.location.href, '/viewer?kind=pdf&src=%2Ffiles%2Fpapers%2Fexample.pdf&paper_id=42&page=3&annotation_id=11');
    assert.deepEqual(viewerURLs[0].options, {
        paperId: 42,
        annotationId: 11,
        page: 3,
    });
});
```

Run:

```bash
node --test web/static/js/__tests__/highlights-page.test.cjs
node --check web/static/js/highlights.js
```

Expected: PASS.

- [ ] **Step 9: Run frontend syntax checks**

Run:

```bash
node --check web/static/js/highlights.js
node --check web/static/js/api.js
node --check web/static/js/main.js
```

Expected: PASS.

- [ ] **Step 10: Commit page slice**

```bash
git add internal/app/server.go web/highlights.html web/static/js/highlights.js web/static/js/api.js web/static/js/main.js web/static/css/style.css web/static/css/pages/highlights.css web/static/locales/zh-CN/common.json web/static/locales/en/common.json web/static/locales/zh-CN/highlights.json web/static/locales/en/highlights.json web/*.html web/static/js/__tests__/api-library-paper-id.test.cjs web/static/js/__tests__/highlights-page.test.cjs
git commit -m "feat: add global PDF highlight library page"
```

---

### Task 5: Final Verification

**Files:**
- Verify all touched Go and frontend files.
- Update no additional docs unless implementation behavior diverges from the approved spec.

- [ ] **Step 1: Run focused Go tests**

Run:

```bash
go test ./internal/repository ./internal/service ./internal/handler ./internal/app -count=1
```

Expected: PASS.

- [ ] **Step 2: Run focused frontend tests**

Run:

```bash
node --test web/static/js/__tests__/utils-resource-viewer.test.cjs
node --test web/static/js/__tests__/api-library-paper-id.test.cjs
node --test web/static/js/__tests__/viewer-pdf-selection.test.cjs
node --test web/static/js/__tests__/highlights-page.test.cjs
```

Expected: PASS.

- [ ] **Step 3: Run syntax checks for touched JavaScript**

Run:

```bash
node --check web/static/js/highlights.js
node --check web/static/js/api.js
node --check web/static/js/utils.js
node --check web/static/js/viewer.js
node --check web/static/js/main.js
```

Expected: PASS.

- [ ] **Step 4: Run full test suite**

Run:

```bash
make test
```

Expected: PASS.

- [ ] **Step 5: Manual browser verification**

Run the app:

```bash
make run
```

Open `http://localhost:8080/highlights` and verify:

- The page appears under the top navigation "More" menu.
- Highlights from at least two PDFs are listed.
- Searching by quote text narrows results.
- Searching by paper title narrows results.
- Clicking a result opens `/viewer` with `paper_id`, `page`, and `annotation_id`.
- The PDF viewer lands on the requested page and scrolls to the matching highlight.
- Deleting a highlight from `/highlights` removes it from the list and it no longer appears after reopening the PDF.

- [ ] **Step 6: Commit any verification fixes**

If verification required changes, commit only those changes:

```bash
git status --short
git add <files changed during verification>
git commit -m "fix: polish global PDF highlight library"
```

If no changes were required, skip the commit command and record the clean status in the final implementation report.
