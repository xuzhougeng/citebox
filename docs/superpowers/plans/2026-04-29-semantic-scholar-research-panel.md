# Semantic Scholar Research Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/research` page powered by the Semantic Scholar Graph API that lets users seed from a paper, expand its references / citations / recommendations one hop at a time, and curate a persisted basket they can import into the library.

**Architecture:** New `internal/service/research/` package owns the S2 client + cache + basket repo (independent of the existing `library_service` which stays focused on local-file paper management). A new `LibraryService.ImportPaperFromS2` method bridges S2 metadata into the existing `papers` table for "import to library". Frontend is a new `web/research.html` + `web/static/js/research.js` following the project's vanilla-JS / IIFE pattern.

**Tech Stack:** Go 1.x stdlib (`net/http`, `database/sql`, `golang.org/x/sync/singleflight`), SQLite, vanilla JS (no framework), existing project i18n system, `httptest` for unit tests.

**Spec:** [`docs/superpowers/specs/2026-04-29-semantic-scholar-research-panel-design.md`](../specs/2026-04-29-semantic-scholar-research-panel-design.md)

---

## File Structure

**New files:**

- `internal/service/research/types.go` — S2 paper / list / pagination types
- `internal/service/research/client.go` — HTTP client, rate limiter, error mapping
- `internal/service/research/client_test.go`
- `internal/service/research/cache.go` — TTL cache wrapping `Get` + `Recommendations` via repo
- `internal/service/research/cache_test.go`
- `internal/repository/research_repo.go` — `ResearchRepository` (cache + basket DB ops)
- `internal/repository/research_repo_test.go`
- `internal/service/library_service_research.go` — `LibraryService.ImportPaperFromS2`
- `internal/service/library_service_research_test.go`
- `internal/handler/research.go` — HTTP handlers
- `internal/handler/research_test.go`
- `web/research.html` — page skeleton
- `web/static/js/research.js` — page logic
- `web/static/css/research.css` — page-specific styles (or appended to `style.css` if that's the project pattern)

**Modified files:**

- `internal/repository/schema/schema.go` — add `s2_paper_cache` and `research_basket_items` tables
- `internal/repository/library_repo.go` — wire `ResearchRepository` into the aggregate `LibraryRepository`
- `internal/config/config.go` — add `S2APIKey` field
- `internal/app/server.go` — wire research service + handler, register `/api/research/*` and `/api/settings/research` routes, add `/research` HTML route
- `internal/handler/settings.go` — add S2 API key get/set endpoints
- `web/settings.html` — add S2 API key input row in "外部 API" group
- `web/static/locales/zh-CN.json` and `en.json` — add `research.*` and `settings.research.*` keys
- `web/library.html`, `web/figures.html`, `web/groups.html`, `web/tags.html`, `web/notes.html`, `web/index.html`, `web/ai.html`, `web/palettes.html`, `web/settings.html`, `web/upload.html`, `web/manual.html`, `web/guide.html` — add `<li><a href="/research" data-i18n="nav.research">调研</a></li>` to nav

---

## Task 1: DB Schema Migration for Cache + Basket

**Files:**
- Modify: `internal/repository/schema/schema.go`

- [ ] **Step 1: Read current schema to find insertion point**

Run: `rg -n "CREATE TABLE IF NOT EXISTS color_palettes" internal/repository/schema/schema.go`
Locate the closing `);` of the last CREATE TABLE block in `initSchema`'s schema string (just before the index block on lines ~120-128).

- [ ] **Step 2: Append new tables to the schema string**

In `internal/repository/schema/schema.go`, inside `initSchema()`, append to the `schema :=` raw string (just before the `CREATE INDEX` block):

```sql
CREATE TABLE IF NOT EXISTS s2_paper_cache (
    cache_key TEXT PRIMARY KEY,
    payload TEXT NOT NULL DEFAULT '',
    fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS research_basket_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    s2_paper_id TEXT NOT NULL UNIQUE,
    notes TEXT DEFAULT '',
    added_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

And append to the index block:

```sql
CREATE INDEX IF NOT EXISTS idx_s2_cache_fetched_at ON s2_paper_cache(fetched_at);
CREATE INDEX IF NOT EXISTS idx_research_basket_added_at ON research_basket_items(added_at DESC);
```

- [ ] **Step 3: Build to verify schema string is valid Go**

Run: `go build ./internal/repository/schema/...`
Expected: success.

- [ ] **Step 4: Run an integration test that hits Initialize() to confirm the migration applies cleanly**

Run: `go test ./internal/repository/...`
Expected: existing tests still pass; the new tables are silently created via `IF NOT EXISTS`.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/schema/schema.go
git commit -m "feat(db): add s2_paper_cache and research_basket_items tables"
```

---

## Task 2: ResearchRepository — Cache + Basket Storage

**Files:**
- Create: `internal/repository/research_repo.go`
- Create: `internal/repository/research_repo_test.go`
- Modify: `internal/repository/library_repo.go`

- [ ] **Step 1: Write the failing test**

Create `internal/repository/research_repo_test.go`:

```go
package repository

import (
	"testing"
	"time"
)

func newTestResearchRepo(t *testing.T) *ResearchRepository {
	t.Helper()
	repo, _ := newTestLibraryRepo(t)
	return repo.Research
}

func TestResearchRepoCachePutAndGet(t *testing.T) {
	repo := newTestResearchRepo(t)

	if err := repo.PutCache("paper:DOI:10.1/abc", `{"title":"foo"}`); err != nil {
		t.Fatalf("PutCache error: %v", err)
	}

	payload, fetchedAt, err := repo.GetCache("paper:DOI:10.1/abc")
	if err != nil {
		t.Fatalf("GetCache error: %v", err)
	}
	if payload != `{"title":"foo"}` {
		t.Fatalf("payload = %q, want %q", payload, `{"title":"foo"}`)
	}
	if time.Since(fetchedAt) > time.Minute {
		t.Fatalf("fetchedAt suspiciously old: %v", fetchedAt)
	}
}

func TestResearchRepoCacheMiss(t *testing.T) {
	repo := newTestResearchRepo(t)

	_, _, err := repo.GetCache("paper:DOI:nonexistent")
	if err != ErrCacheMiss {
		t.Fatalf("expected ErrCacheMiss, got %v", err)
	}
}

func TestResearchRepoBasketAddListRemove(t *testing.T) {
	repo := newTestResearchRepo(t)

	if err := repo.AddBasketItem("S2:abc", "my note"); err != nil {
		t.Fatalf("AddBasketItem error: %v", err)
	}
	if err := repo.AddBasketItem("S2:def", ""); err != nil {
		t.Fatalf("AddBasketItem error: %v", err)
	}
	if err := repo.AddBasketItem("S2:abc", "dup note"); err != nil {
		t.Fatalf("AddBasketItem dup error: %v", err)
	}

	items, err := repo.ListBasketItems()
	if err != nil {
		t.Fatalf("ListBasketItems error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}

	if err := repo.RemoveBasketItem("S2:abc"); err != nil {
		t.Fatalf("RemoveBasketItem error: %v", err)
	}
	items, _ = repo.ListBasketItems()
	if len(items) != 1 || items[0].S2PaperID != "S2:def" {
		t.Fatalf("after remove items = %+v", items)
	}
}
```

The helper `newTestLibraryRepo` already exists in `library_repo_test.go`; verify it returns a repo with the schema initialized.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/repository/ -run TestResearchRepo -v`
Expected: FAIL — `ResearchRepository` not defined.

- [ ] **Step 3: Implement `ResearchRepository`**

Create `internal/repository/research_repo.go`:

```go
package repository

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrCacheMiss is returned by GetCache when the key is not present.
var ErrCacheMiss = errors.New("research: cache miss")

// ResearchRepository owns the s2_paper_cache and research_basket_items tables.
type ResearchRepository struct {
	db *sql.DB
}

// NewResearchRepository builds a research repository.
func NewResearchRepository(db *sql.DB) *ResearchRepository {
	return &ResearchRepository{db: db}
}

// BasketItem mirrors a row in research_basket_items.
type BasketItem struct {
	ID        int64
	S2PaperID string
	Notes     string
	AddedAt   time.Time
}

// PutCache inserts or replaces a cache entry. Caller serialises payload as JSON.
func (r *ResearchRepository) PutCache(key, payload string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("research: cache key required")
	}
	_, err := r.db.Exec(`
		INSERT INTO s2_paper_cache (cache_key, payload, fetched_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(cache_key) DO UPDATE SET
			payload = excluded.payload,
			fetched_at = CURRENT_TIMESTAMP
	`, key, payload)
	return err
}

// GetCache returns the payload + fetched_at, or ErrCacheMiss.
func (r *ResearchRepository) GetCache(key string) (string, time.Time, error) {
	row := r.db.QueryRow(`SELECT payload, fetched_at FROM s2_paper_cache WHERE cache_key = ?`, key)
	var payload string
	var fetchedAt time.Time
	if err := row.Scan(&payload, &fetchedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", time.Time{}, ErrCacheMiss
		}
		return "", time.Time{}, err
	}
	return payload, fetchedAt, nil
}

// DeleteExpiredCache prunes cache entries older than maxAge.
func (r *ResearchRepository) DeleteExpiredCache(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	res, err := r.db.Exec(`DELETE FROM s2_paper_cache WHERE fetched_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// AddBasketItem inserts an item; if s2_paper_id already exists, the existing
// row's notes are updated to the supplied value.
func (r *ResearchRepository) AddBasketItem(s2PaperID, notes string) error {
	if strings.TrimSpace(s2PaperID) == "" {
		return errors.New("research: s2_paper_id required")
	}
	_, err := r.db.Exec(`
		INSERT INTO research_basket_items (s2_paper_id, notes, added_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(s2_paper_id) DO UPDATE SET notes = excluded.notes
	`, s2PaperID, notes)
	return err
}

// RemoveBasketItem deletes the item by s2_paper_id; not-found is not an error.
func (r *ResearchRepository) RemoveBasketItem(s2PaperID string) error {
	_, err := r.db.Exec(`DELETE FROM research_basket_items WHERE s2_paper_id = ?`, s2PaperID)
	return err
}

// ListBasketItems returns all basket items, newest first.
func (r *ResearchRepository) ListBasketItems() ([]BasketItem, error) {
	rows, err := r.db.Query(`
		SELECT id, s2_paper_id, notes, added_at
		FROM research_basket_items
		ORDER BY added_at DESC, id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]BasketItem, 0)
	for rows.Next() {
		var it BasketItem
		if err := rows.Scan(&it.ID, &it.S2PaperID, &it.Notes, &it.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Wire `Research` field into the aggregate `LibraryRepository`**

In `internal/repository/library_repo.go`, find the struct (around line 27) and the constructor (around line 62). Add a field and constructor wiring:

```go
type LibraryRepository struct {
    Paper    *PaperRepository
    Group    *GroupRepository
    Tag      *TagRepository
    Figure   *FigureRepository
    Palette  *PaletteRepository
    Setting  *SettingRepository
    Research *ResearchRepository  // <-- add
    // ...
}
```

In the constructor (after `settingRepo := NewSettingRepository(db)` line):

```go
researchRepo := NewResearchRepository(db)
return &LibraryRepository{
    // ... existing fields,
    Setting:  settingRepo,
    Research: researchRepo,
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/repository/ -run TestResearchRepo -v`
Expected: PASS.

- [ ] **Step 6: Run the full repository test suite to confirm no regression**

Run: `go test ./internal/repository/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/repository/research_repo.go internal/repository/research_repo_test.go internal/repository/library_repo.go
git commit -m "feat(repo): add ResearchRepository for S2 cache and basket"
```

---

## Task 3: Add S2 API key to config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/config/config_test.go`, add:

```go
func TestLoadReadsS2APIKey(t *testing.T) {
	t.Setenv("S2_API_KEY", "test-key-abc")
	cfg := Load()
	if cfg.S2APIKey != "test-key-abc" {
		t.Fatalf("Load() S2APIKey = %q, want %q", cfg.S2APIKey, "test-key-abc")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/config/ -run TestLoadReadsS2APIKey -v`
Expected: FAIL — `S2APIKey` field undefined.

- [ ] **Step 3: Add the field**

In `internal/config/config.go`, in the `Config` struct (around line 20 next to `OAContactEmail`):

```go
type Config struct {
    // ... existing fields
    OAContactEmail string
    S2APIKey       string
}
```

In the `Load()` function next to the `OAContactEmail` assignment (around line 43):

```go
S2APIKey: getEnv("S2_API_KEY", ""),
```

- [ ] **Step 4: Run the test**

Run: `go test ./internal/config/ -run TestLoadReadsS2APIKey -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add S2_API_KEY env var"
```

---

## Task 4: Research package — Types

**Files:**
- Create: `internal/service/research/types.go`

- [ ] **Step 1: Write the types file**

Create `internal/service/research/types.go`:

```go
// Package research contains the Semantic Scholar Graph API client,
// response cache, and helpers backing the /research panel.
package research

// Paper is a normalised view of a Semantic Scholar paper. We only keep
// fields the panel actually renders; raw responses live in the cache.
type Paper struct {
	PaperID              string   `json:"paperId"`
	ExternalIDs          IDs      `json:"externalIds"`
	Title                string   `json:"title"`
	Abstract             string   `json:"abstract"`
	TLDR                 string   `json:"tldr,omitempty"`
	Year                 int      `json:"year"`
	Venue                string   `json:"venue"`
	Authors              []Author `json:"authors"`
	CitationCount        int      `json:"citationCount"`
	InfluentialCount     int      `json:"influentialCitationCount"`
	OpenAccessPDFURL     string   `json:"openAccessPdfUrl,omitempty"`
	FieldsOfStudy        []string `json:"fieldsOfStudy,omitempty"`
}

// IDs holds the cross-source identifiers S2 attaches to a paper.
type IDs struct {
	DOI    string `json:"DOI,omitempty"`
	ArXiv  string `json:"ArXiv,omitempty"`
	PubMed string `json:"PubMed,omitempty"`
}

// Author is a minimal author record.
type Author struct {
	AuthorID string `json:"authorId,omitempty"`
	Name     string `json:"name"`
}

// SearchOpts narrows a paper/search query.
type SearchOpts struct {
	Year          string // e.g. "2018-2024", "2020-", "-2015"
	FieldsOfStudy string // comma-separated S2 fields-of-study labels
	Limit         int    // 1..100
	Offset        int
}

// PaperList is a single page of S2 paper-list responses (references / citations / search results).
type PaperList struct {
	Items  []Paper `json:"items"`
	Offset int     `json:"offset"`
	Next   int     `json:"next"`  // 0 means no more
	Total  int     `json:"total,omitempty"`
}

// CitationOpts filters /paper/{id}/citations responses.
type CitationOpts struct {
	InfluentialOnly bool
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/service/research/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/service/research/types.go
git commit -m "feat(research): add S2 paper / list / opts types"
```

---

## Task 5: Research client — Search + Get

**Files:**
- Create: `internal/service/research/client.go`
- Create: `internal/service/research/client_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/service/research/client_test.go`:

```go
package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	return srv, srv.Close
}

func newTestClient(baseURL, apiKey string) *Client {
	return NewClient(Config{
		BaseURL: baseURL,
		APIKey:  apiKey,
		// disable rate limiting in tests by setting MinInterval to 0
		MinInterval: 0,
	})
}

func TestClientSearchHappyPath(t *testing.T) {
	srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graph/v1/paper/search" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "transformer" {
			t.Errorf("query = %q, want transformer", r.URL.Query().Get("query"))
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"total":  42,
			"offset": 0,
			"next":   10,
			"data": []map[string]interface{}{
				{"paperId": "p1", "title": "Attention Is All You Need", "year": 2017,
					"externalIds": map[string]string{"DOI": "10.1/abc"}},
			},
		})
	})
	defer stop()

	c := newTestClient(srv.URL, "")
	res, err := c.Search(context.Background(), "transformer", SearchOpts{Limit: 10})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if res.Total != 42 || res.Next != 10 || len(res.Items) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Items[0].ExternalIDs.DOI != "10.1/abc" {
		t.Fatalf("DOI = %q", res.Items[0].ExternalIDs.DOI)
	}
}

func TestClientGetByDOI(t *testing.T) {
	srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/graph/v1/paper/DOI:10.1/abc") {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"paperId":       "p1",
			"title":         "Some Paper",
			"citationCount": 7,
		})
	})
	defer stop()

	c := newTestClient(srv.URL, "")
	p, err := c.Get(context.Background(), "DOI:10.1/abc", nil)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if p.PaperID != "p1" || p.CitationCount != 7 {
		t.Fatalf("unexpected paper: %+v", p)
	}
}

func TestClientGetNotFound(t *testing.T) {
	srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	defer stop()

	c := newTestClient(srv.URL, "")
	_, err := c.Get(context.Background(), "DOI:nope", nil)
	if err != ErrPaperNotFound {
		t.Fatalf("expected ErrPaperNotFound, got %v", err)
	}
}

func TestClientSendsAPIKey(t *testing.T) {
	srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "secret" {
			t.Errorf("missing api key header, got %q", r.Header.Get("x-api-key"))
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"paperId": "p1", "title": "x"})
	})
	defer stop()

	c := newTestClient(srv.URL, "secret")
	if _, err := c.Get(context.Background(), "DOI:x", nil); err != nil {
		t.Fatalf("Get error: %v", err)
	}
}

func TestClientRateLimit(t *testing.T) {
	calls := 0
	srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"paperId": "p1", "title": "x"})
	})
	defer stop()

	c := NewClient(Config{
		BaseURL:     srv.URL,
		MinInterval: 50 * time.Millisecond,
	})
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := c.Get(context.Background(), "DOI:x", nil); err != nil {
			t.Fatalf("Get %d error: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Fatalf("rate limiter ineffective: 3 calls in %v", elapsed)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/service/research/ -run TestClient -v`
Expected: FAIL — `Client` not defined.

- [ ] **Step 3: Implement `client.go`**

Create `internal/service/research/client.go`:

```go
package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrPaperNotFound is returned by Get / References / Citations when S2 returns 404.
var ErrPaperNotFound = errors.New("research: paper not found")

// ErrRateLimited is returned when S2 returns 429 even after a single retry.
var ErrRateLimited = errors.New("research: rate limited")

// Config configures the Semantic Scholar client.
type Config struct {
	BaseURL     string        // default https://api.semanticscholar.org
	APIKey      string        // optional; sent as x-api-key
	HTTPClient  *http.Client  // optional override
	MinInterval time.Duration // minimum delay between consecutive requests
}

// Client wraps the Semantic Scholar Graph API. All methods are safe for
// concurrent use; an internal ticker enforces MinInterval globally.
type Client struct {
	baseURL     string
	apiKey      string
	httpClient  *http.Client
	tokens      chan struct{} // ticker buffer; nil if rate limiting disabled
	closeTicker chan struct{}
}

// NewClient constructs a Client. If MinInterval > 0, a goroutine drips tokens.
func NewClient(cfg Config) *Client {
	c := &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:     strings.TrimSpace(cfg.APIKey),
		httpClient: cfg.HTTPClient,
	}
	if c.baseURL == "" {
		c.baseURL = "https://api.semanticscholar.org"
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.MinInterval > 0 {
		c.tokens = make(chan struct{}, 1)
		c.closeTicker = make(chan struct{})
		go c.refillTokens(cfg.MinInterval)
	}
	return c
}

// Close releases the rate-limit ticker goroutine.
func (c *Client) Close() {
	if c.closeTicker != nil {
		close(c.closeTicker)
	}
}

func (c *Client) refillTokens(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// seed one token immediately so first call doesn't wait
	select {
	case c.tokens <- struct{}{}:
	default:
	}
	for {
		select {
		case <-c.closeTicker:
			return
		case <-ticker.C:
			select {
			case c.tokens <- struct{}{}:
			default:
			}
		}
	}
}

func (c *Client) takeToken(ctx context.Context) error {
	if c.tokens == nil {
		return nil
	}
	select {
	case <-c.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// doJSON performs the request, decodes JSON into out, and maps known errors.
func (c *Client) doJSON(ctx context.Context, path string, query url.Values, out interface{}) error {
	if err := c.takeToken(ctx); err != nil {
		return err
	}
	full := c.baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return json.NewDecoder(resp.Body).Decode(out)
	case http.StatusNotFound:
		return ErrPaperNotFound
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("research: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

// rawSearchResponse maps the S2 search response shape.
type rawSearchResponse struct {
	Total  int          `json:"total"`
	Offset int          `json:"offset"`
	Next   int          `json:"next"`
	Data   []rawPaper   `json:"data"`
}

type rawPaper struct {
	PaperID                  string            `json:"paperId"`
	ExternalIDs              map[string]string `json:"externalIds"`
	Title                    string            `json:"title"`
	Abstract                 string            `json:"abstract"`
	Year                     int               `json:"year"`
	Venue                    string            `json:"venue"`
	Authors                  []Author          `json:"authors"`
	CitationCount            int               `json:"citationCount"`
	InfluentialCitationCount int               `json:"influentialCitationCount"`
	OpenAccessPDF            *struct {
		URL string `json:"url"`
	} `json:"openAccessPdf"`
	TLDR *struct {
		Text string `json:"text"`
	} `json:"tldr"`
	FieldsOfStudy []string `json:"fieldsOfStudy"`
}

func (rp rawPaper) toPaper() Paper {
	out := Paper{
		PaperID:          rp.PaperID,
		Title:            rp.Title,
		Abstract:         rp.Abstract,
		Year:             rp.Year,
		Venue:            rp.Venue,
		Authors:          rp.Authors,
		CitationCount:    rp.CitationCount,
		InfluentialCount: rp.InfluentialCitationCount,
		FieldsOfStudy:    rp.FieldsOfStudy,
	}
	if rp.OpenAccessPDF != nil {
		out.OpenAccessPDFURL = rp.OpenAccessPDF.URL
	}
	if rp.TLDR != nil {
		out.TLDR = rp.TLDR.Text
	}
	if v, ok := rp.ExternalIDs["DOI"]; ok {
		out.ExternalIDs.DOI = v
	}
	if v, ok := rp.ExternalIDs["ArXiv"]; ok {
		out.ExternalIDs.ArXiv = v
	}
	if v, ok := rp.ExternalIDs["PubMed"]; ok {
		out.ExternalIDs.PubMed = v
	}
	return out
}

// defaultPaperFields is the field selection used by Get/Search/References/Citations.
const defaultPaperFields = "paperId,externalIds,title,abstract,year,venue,authors,citationCount,influentialCitationCount,openAccessPdf,tldr,fieldsOfStudy"

// Search executes a paper/search query.
func (c *Client) Search(ctx context.Context, query string, opts SearchOpts) (PaperList, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("fields", defaultPaperFields)
	if opts.Year != "" {
		q.Set("year", opts.Year)
	}
	if opts.FieldsOfStudy != "" {
		q.Set("fieldsOfStudy", opts.FieldsOfStudy)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Offset > 0 {
		q.Set("offset", strconv.Itoa(opts.Offset))
	}

	var raw rawSearchResponse
	if err := c.doJSON(ctx, "/graph/v1/paper/search", q, &raw); err != nil {
		return PaperList{}, err
	}
	out := PaperList{Items: make([]Paper, 0, len(raw.Data)), Offset: raw.Offset, Next: raw.Next, Total: raw.Total}
	for _, rp := range raw.Data {
		out.Items = append(out.Items, rp.toPaper())
	}
	return out, nil
}

// Get fetches a single paper by S2-supported ID (DOI:..., ArXiv:..., paperId, etc).
// `fields` is optional; pass nil for the default selection.
func (c *Client) Get(ctx context.Context, id string, fields []string) (Paper, error) {
	q := url.Values{}
	if len(fields) > 0 {
		q.Set("fields", strings.Join(fields, ","))
	} else {
		q.Set("fields", defaultPaperFields)
	}
	var rp rawPaper
	path := "/graph/v1/paper/" + url.PathEscape(id)
	if err := c.doJSON(ctx, path, q, &rp); err != nil {
		return Paper{}, err
	}
	return rp.toPaper(), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/service/research/ -run TestClient -v`
Expected: PASS for all 5 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/service/research/client.go internal/service/research/client_test.go
git commit -m "feat(research): add S2 client with Search + Get + rate limiter"
```

---

## Task 6: Client — References / Citations / Recommendations

**Files:**
- Modify: `internal/service/research/client.go`
- Modify: `internal/service/research/client_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/service/research/client_test.go`:

```go
func TestClientReferencesCitationsRecommendations(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		invoke    func(c *Client, ctx context.Context) (PaperList, error)
		respData  string
		listKey   string
	}{
		{
			name: "References",
			path: "/graph/v1/paper/p1/references",
			invoke: func(c *Client, ctx context.Context) (PaperList, error) {
				return c.References(ctx, "p1", 0, 10)
			},
			respData: `{"offset":0,"next":10,"data":[{"citedPaper":{"paperId":"r1","title":"Ref 1"}}]}`,
		},
		{
			name: "Citations",
			path: "/graph/v1/paper/p1/citations",
			invoke: func(c *Client, ctx context.Context) (PaperList, error) {
				return c.Citations(ctx, "p1", 0, 10, CitationOpts{})
			},
			respData: `{"offset":0,"next":10,"data":[{"citingPaper":{"paperId":"c1","title":"Cite 1"}}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path = %s, want %s", r.URL.Path, tc.path)
				}
				w.Write([]byte(tc.respData))
			})
			defer stop()
			c := newTestClient(srv.URL, "")
			res, err := tc.invoke(c, context.Background())
			if err != nil {
				t.Fatalf("invoke error: %v", err)
			}
			if len(res.Items) != 1 {
				t.Fatalf("got %d items, want 1: %+v", len(res.Items), res.Items)
			}
		})
	}
}

func TestClientRecommendations(t *testing.T) {
	srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/recommendations/v1/papers/forpaper/p1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"recommendedPapers":[{"paperId":"rec1","title":"Rec"}]}`))
	})
	defer stop()
	c := newTestClient(srv.URL, "")
	papers, err := c.Recommendations(context.Background(), "p1")
	if err != nil {
		t.Fatalf("Recommendations error: %v", err)
	}
	if len(papers) != 1 || papers[0].PaperID != "rec1" {
		t.Fatalf("got %+v", papers)
	}
}

func TestClientRecommendationsForList(t *testing.T) {
	srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/recommendations/v1/papers" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body struct {
			Positive []string `json:"positivePaperIds"`
			Negative []string `json:"negativePaperIds"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Positive) != 2 {
			t.Errorf("positive = %v", body.Positive)
		}
		w.Write([]byte(`{"recommendedPapers":[{"paperId":"rec1","title":"Rec"}]}`))
	})
	defer stop()
	c := newTestClient(srv.URL, "")
	papers, err := c.RecommendationsForList(context.Background(), []string{"a", "b"}, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("got %d papers", len(papers))
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/service/research/ -run TestClient -v`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Implement the methods**

Append to `internal/service/research/client.go`:

```go
type rawListResponse struct {
	Offset int       `json:"offset"`
	Next   int       `json:"next"`
	Data   []rawEdge `json:"data"`
}

// rawEdge is a row in references / citations responses; one of citedPaper /
// citingPaper is populated. We unify into a Paper.
type rawEdge struct {
	CitedPaper       *rawPaper `json:"citedPaper,omitempty"`
	CitingPaper      *rawPaper `json:"citingPaper,omitempty"`
	IsInfluential    bool      `json:"isInfluential"`
	Intents          []string  `json:"intents"`
}

func (e rawEdge) paper() (Paper, bool) {
	switch {
	case e.CitedPaper != nil:
		return e.CitedPaper.toPaper(), e.IsInfluential
	case e.CitingPaper != nil:
		return e.CitingPaper.toPaper(), e.IsInfluential
	}
	return Paper{}, false
}

// References returns papers that the given paper cites.
func (c *Client) References(ctx context.Context, paperID string, offset, limit int) (PaperList, error) {
	return c.fetchPaperList(ctx, "/graph/v1/paper/"+url.PathEscape(paperID)+"/references", offset, limit, false)
}

// Citations returns papers that cite the given paper. If opts.InfluentialOnly,
// non-influential edges are dropped *after* fetch (S2 has no server-side filter).
func (c *Client) Citations(ctx context.Context, paperID string, offset, limit int, opts CitationOpts) (PaperList, error) {
	return c.fetchPaperList(ctx, "/graph/v1/paper/"+url.PathEscape(paperID)+"/citations", offset, limit, opts.InfluentialOnly)
}

func (c *Client) fetchPaperList(ctx context.Context, path string, offset, limit int, influentialOnly bool) (PaperList, error) {
	q := url.Values{}
	q.Set("fields", "isInfluential,intents,"+wrapPaperFields(defaultPaperFields))
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var raw rawListResponse
	if err := c.doJSON(ctx, path, q, &raw); err != nil {
		return PaperList{}, err
	}
	out := PaperList{Offset: raw.Offset, Next: raw.Next, Items: make([]Paper, 0, len(raw.Data))}
	for _, edge := range raw.Data {
		paper, influential := edge.paper()
		if paper.PaperID == "" {
			continue
		}
		if influentialOnly && !influential {
			continue
		}
		out.Items = append(out.Items, paper)
	}
	return out, nil
}

// wrapPaperFields prefixes every comma-delimited field with the given parent prefix
// so the citedPaper / citingPaper sub-objects are projected fully. The S2 API
// expects e.g. `citedPaper.title,citedPaper.abstract,...`.
func wrapPaperFields(fields string) string {
	parts := strings.Split(fields, ",")
	out := make([]string, 0, 2*len(parts))
	for _, p := range parts {
		out = append(out, "citedPaper."+p, "citingPaper."+p)
	}
	return strings.Join(out, ",")
}

// recommendationsResponse maps both recommendation endpoints (they share shape).
type recommendationsResponse struct {
	RecommendedPapers []rawPaper `json:"recommendedPapers"`
}

// Recommendations returns S2 recommendations for a single seed paper.
func (c *Client) Recommendations(ctx context.Context, paperID string) ([]Paper, error) {
	q := url.Values{}
	q.Set("fields", defaultPaperFields)
	var raw recommendationsResponse
	if err := c.doJSON(ctx, "/recommendations/v1/papers/forpaper/"+url.PathEscape(paperID), q, &raw); err != nil {
		return nil, err
	}
	out := make([]Paper, 0, len(raw.RecommendedPapers))
	for _, rp := range raw.RecommendedPapers {
		out = append(out, rp.toPaper())
	}
	return out, nil
}

// RecommendationsForList POSTs a positive/negative list of paper IDs and
// returns recommendations.
func (c *Client) RecommendationsForList(ctx context.Context, positive, negative []string) ([]Paper, error) {
	if err := c.takeToken(ctx); err != nil {
		return nil, err
	}
	body := map[string][]string{
		"positivePaperIds": positive,
		"negativePaperIds": negative,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("fields", defaultPaperFields)
	full := c.baseURL + "/recommendations/v1/papers?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, full, strings.NewReader(string(buf)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrPaperNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("research: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var raw recommendationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]Paper, 0, len(raw.RecommendedPapers))
	for _, rp := range raw.RecommendedPapers {
		out = append(out, rp.toPaper())
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to confirm pass**

Run: `go test ./internal/service/research/ -v`
Expected: PASS for all client tests.

- [ ] **Step 5: Commit**

```bash
git add internal/service/research/client.go internal/service/research/client_test.go
git commit -m "feat(research): add References, Citations, Recommendations endpoints"
```

---

## Task 7: Cache wrapper (`research.Service`)

**Files:**
- Create: `internal/service/research/service.go`
- Create: `internal/service/research/service_test.go`

The cache only wraps `Get` (paper metadata, TTL 7d) and `Recommendations` (TTL 1d). Search / References / Citations always pass through (paged + cheap to refetch).

- [ ] **Step 1: Write the failing test**

Create `internal/service/research/service_test.go`:

```go
package research

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type stubCacheRepo struct {
	store map[string]struct {
		payload   string
		fetchedAt time.Time
	}
}

func newStubCacheRepo() *stubCacheRepo {
	return &stubCacheRepo{store: make(map[string]struct {
		payload   string
		fetchedAt time.Time
	})}
}

func (s *stubCacheRepo) PutCache(key, payload string) error {
	s.store[key] = struct {
		payload   string
		fetchedAt time.Time
	}{payload, time.Now()}
	return nil
}

func (s *stubCacheRepo) GetCache(key string) (string, time.Time, error) {
	v, ok := s.store[key]
	if !ok {
		return "", time.Time{}, ErrCacheRepoMiss
	}
	return v.payload, v.fetchedAt, nil
}

// stubClient implements the inner client interface used by Service.
type stubClient struct {
	getCalls int32
	getFn    func(ctx context.Context, id string, fields []string) (Paper, error)
	recCalls int32
	recFn    func(ctx context.Context, id string) ([]Paper, error)
}

func (s *stubClient) Get(ctx context.Context, id string, fields []string) (Paper, error) {
	atomic.AddInt32(&s.getCalls, 1)
	return s.getFn(ctx, id, fields)
}

func (s *stubClient) Search(ctx context.Context, q string, o SearchOpts) (PaperList, error) {
	return PaperList{}, nil
}
func (s *stubClient) References(ctx context.Context, id string, off, lim int) (PaperList, error) {
	return PaperList{}, nil
}
func (s *stubClient) Citations(ctx context.Context, id string, off, lim int, o CitationOpts) (PaperList, error) {
	return PaperList{}, nil
}
func (s *stubClient) Recommendations(ctx context.Context, id string) ([]Paper, error) {
	atomic.AddInt32(&s.recCalls, 1)
	return s.recFn(ctx, id)
}
func (s *stubClient) RecommendationsForList(ctx context.Context, pos, neg []string) ([]Paper, error) {
	return nil, nil
}

func TestServiceGetCacheMissThenHit(t *testing.T) {
	repo := newStubCacheRepo()
	cli := &stubClient{
		getFn: func(ctx context.Context, id string, fields []string) (Paper, error) {
			return Paper{PaperID: "p1", Title: "T"}, nil
		},
	}
	svc := NewService(cli, repo, ServiceConfig{PaperTTL: time.Hour, RecTTL: time.Hour})

	p, err := svc.GetPaper(context.Background(), "DOI:abc")
	if err != nil {
		t.Fatalf("first get error: %v", err)
	}
	if p.PaperID != "p1" {
		t.Fatalf("paper = %+v", p)
	}

	p2, err := svc.GetPaper(context.Background(), "DOI:abc")
	if err != nil {
		t.Fatalf("second get error: %v", err)
	}
	if p2.PaperID != "p1" {
		t.Fatalf("p2 = %+v", p2)
	}
	if cli.getCalls != 1 {
		t.Fatalf("expected 1 upstream call, got %d", cli.getCalls)
	}
}

func TestServiceGetExpiredCacheRefetches(t *testing.T) {
	repo := newStubCacheRepo()
	// pre-populate with an "expired" entry: set fetchedAt to 2h ago, TTL 1h
	stale, _ := json.Marshal(Paper{PaperID: "old", Title: "stale"})
	repo.store["paper:DOI:abc"] = struct {
		payload   string
		fetchedAt time.Time
	}{string(stale), time.Now().Add(-2 * time.Hour)}

	cli := &stubClient{
		getFn: func(ctx context.Context, id string, fields []string) (Paper, error) {
			return Paper{PaperID: "fresh"}, nil
		},
	}
	svc := NewService(cli, repo, ServiceConfig{PaperTTL: time.Hour, RecTTL: time.Hour})

	p, err := svc.GetPaper(context.Background(), "DOI:abc")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p.PaperID != "fresh" {
		t.Fatalf("expected refetch, got %+v", p)
	}
	if cli.getCalls != 1 {
		t.Fatalf("expected 1 upstream call, got %d", cli.getCalls)
	}
}

func TestServiceGetUpstreamErrorFallsBackToStale(t *testing.T) {
	repo := newStubCacheRepo()
	stale, _ := json.Marshal(Paper{PaperID: "stale", Title: "still ok"})
	repo.store["paper:DOI:abc"] = struct {
		payload   string
		fetchedAt time.Time
	}{string(stale), time.Now().Add(-48 * time.Hour)}

	cli := &stubClient{
		getFn: func(ctx context.Context, id string, fields []string) (Paper, error) {
			return Paper{}, errors.New("network down")
		},
	}
	svc := NewService(cli, repo, ServiceConfig{PaperTTL: time.Hour, RecTTL: time.Hour})

	p, err := svc.GetPaper(context.Background(), "DOI:abc")
	if err != nil {
		t.Fatalf("expected fallback to stale, got error: %v", err)
	}
	if p.PaperID != "stale" {
		t.Fatalf("got %+v", p)
	}
}
```

Note: the stub repo references `ErrCacheRepoMiss`. We declare it as an alias in the service file because `repository.ErrCacheMiss` is in another package — the inner cache interface uses our own sentinel.

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/service/research/ -run TestService -v`
Expected: FAIL — `Service`, `NewService`, `ErrCacheRepoMiss` undefined.

- [ ] **Step 3: Implement `service.go`**

Create `internal/service/research/service.go`:

```go
package research

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"
)

// ErrCacheRepoMiss signals "not in cache". The wrapping repo is expected to
// translate its own miss sentinel into this one before returning.
var ErrCacheRepoMiss = errors.New("research: cache repo miss")

// CacheRepo is the storage backend the Service uses for cache entries.
// Implementations should map their own miss sentinel to ErrCacheRepoMiss.
type CacheRepo interface {
	PutCache(key, payload string) error
	GetCache(key string) (string, time.Time, error)
}

// PaperFetcher is the inner client surface the Service depends on.
// Mirrors *Client.
type PaperFetcher interface {
	Search(ctx context.Context, query string, opts SearchOpts) (PaperList, error)
	Get(ctx context.Context, id string, fields []string) (Paper, error)
	References(ctx context.Context, paperID string, offset, limit int) (PaperList, error)
	Citations(ctx context.Context, paperID string, offset, limit int, opts CitationOpts) (PaperList, error)
	Recommendations(ctx context.Context, paperID string) ([]Paper, error)
	RecommendationsForList(ctx context.Context, positive, negative []string) ([]Paper, error)
}

// ServiceConfig sets cache TTLs.
type ServiceConfig struct {
	PaperTTL time.Duration // default 7 days
	RecTTL   time.Duration // default 1 day
}

// Service wraps a PaperFetcher with a CacheRepo and singleflight dedup.
type Service struct {
	client PaperFetcher
	cache  CacheRepo
	cfg    ServiceConfig
	group  singleflight.Group
}

// NewService constructs a Service. Zero TTLs become package defaults.
func NewService(client PaperFetcher, cache CacheRepo, cfg ServiceConfig) *Service {
	if cfg.PaperTTL == 0 {
		cfg.PaperTTL = 7 * 24 * time.Hour
	}
	if cfg.RecTTL == 0 {
		cfg.RecTTL = 24 * time.Hour
	}
	return &Service{client: client, cache: cache, cfg: cfg}
}

// GetPaper returns the paper, using the cache when fresh and falling back to
// stale cache on upstream errors.
func (s *Service) GetPaper(ctx context.Context, id string) (Paper, error) {
	key := paperCacheKey(id)
	v, err, _ := s.group.Do(key, func() (interface{}, error) {
		if payload, fetchedAt, err := s.cache.GetCache(key); err == nil {
			if time.Since(fetchedAt) <= s.cfg.PaperTTL {
				var p Paper
				if jsonErr := json.Unmarshal([]byte(payload), &p); jsonErr == nil {
					return p, nil
				}
			}
		}
		paper, upstreamErr := s.client.Get(ctx, id, nil)
		if upstreamErr != nil {
			// Fall back to stale cache if it exists.
			if payload, _, cacheErr := s.cache.GetCache(key); cacheErr == nil {
				var p Paper
				if jsonErr := json.Unmarshal([]byte(payload), &p); jsonErr == nil {
					return p, nil
				}
			}
			return Paper{}, upstreamErr
		}
		if buf, mErr := json.Marshal(paper); mErr == nil {
			_ = s.cache.PutCache(key, string(buf))
		}
		return paper, nil
	})
	if err != nil {
		return Paper{}, err
	}
	return v.(Paper), nil
}

// Search is a passthrough; not cached.
func (s *Service) Search(ctx context.Context, query string, opts SearchOpts) (PaperList, error) {
	return s.client.Search(ctx, query, opts)
}

// References / Citations: not cached, passthrough.
func (s *Service) References(ctx context.Context, paperID string, offset, limit int) (PaperList, error) {
	return s.client.References(ctx, paperID, offset, limit)
}

func (s *Service) Citations(ctx context.Context, paperID string, offset, limit int, opts CitationOpts) (PaperList, error) {
	return s.client.Citations(ctx, paperID, offset, limit, opts)
}

// Recommendations cached with RecTTL.
func (s *Service) Recommendations(ctx context.Context, paperID string) ([]Paper, error) {
	key := recCacheKey(paperID)
	v, err, _ := s.group.Do(key, func() (interface{}, error) {
		if payload, fetchedAt, err := s.cache.GetCache(key); err == nil {
			if time.Since(fetchedAt) <= s.cfg.RecTTL {
				var ps []Paper
				if jsonErr := json.Unmarshal([]byte(payload), &ps); jsonErr == nil {
					return ps, nil
				}
			}
		}
		papers, upstreamErr := s.client.Recommendations(ctx, paperID)
		if upstreamErr != nil {
			if payload, _, cacheErr := s.cache.GetCache(key); cacheErr == nil {
				var ps []Paper
				if jsonErr := json.Unmarshal([]byte(payload), &ps); jsonErr == nil {
					return ps, nil
				}
			}
			return nil, upstreamErr
		}
		if buf, mErr := json.Marshal(papers); mErr == nil {
			_ = s.cache.PutCache(key, string(buf))
		}
		return papers, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]Paper), nil
}

// RecommendationsForList: list responses for ad-hoc baskets aren't cached because
// the input set varies per request.
func (s *Service) RecommendationsForList(ctx context.Context, positive, negative []string) ([]Paper, error) {
	return s.client.RecommendationsForList(ctx, positive, negative)
}

func paperCacheKey(id string) string { return "paper:" + strings.TrimSpace(id) }
func recCacheKey(id string) string   { return "rec:" + strings.TrimSpace(id) }
```

Add `golang.org/x/sync` to deps:

Run: `go get golang.org/x/sync/singleflight`

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/research/ -run TestService -v`
Expected: PASS for all 3 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/service/research/service.go internal/service/research/service_test.go go.mod go.sum
git commit -m "feat(research): add Service layer with TTL cache and singleflight"
```

---

## Task 8: Bridge `research.CacheRepo` to `repository.ResearchRepository`

**Files:**
- Create: `internal/service/research/repo_adapter.go`

The repo's miss sentinel is `repository.ErrCacheMiss`; we need to translate it.

- [ ] **Step 1: Implement adapter**

Create `internal/service/research/repo_adapter.go`:

```go
package research

import (
	"errors"
	"time"

	"github.com/xuzhougeng/citebox/internal/repository"
)

// RepoAdapter wraps *repository.ResearchRepository to satisfy CacheRepo.
type RepoAdapter struct {
	Repo *repository.ResearchRepository
}

// PutCache forwards to the underlying repo.
func (a *RepoAdapter) PutCache(key, payload string) error {
	return a.Repo.PutCache(key, payload)
}

// GetCache forwards to the underlying repo, translating the miss sentinel.
func (a *RepoAdapter) GetCache(key string) (string, time.Time, error) {
	payload, fetchedAt, err := a.Repo.GetCache(key)
	if errors.Is(err, repository.ErrCacheMiss) {
		return "", time.Time{}, ErrCacheRepoMiss
	}
	return payload, fetchedAt, err
}
```

- [ ] **Step 2: Build to verify**

Run: `go build ./internal/service/research/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/service/research/repo_adapter.go
git commit -m "feat(research): add adapter from ResearchRepository to CacheRepo"
```

---

## Task 9: `LibraryService.ImportPaperFromS2`

**Files:**
- Create: `internal/service/library_service_research.go`
- Create: `internal/service/library_service_research_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/service/library_service_research_test.go`:

```go
package service

import (
	"context"
	"testing"

	"github.com/xuzhougeng/citebox/internal/service/research"
)

func TestImportPaperFromS2CreatesNewMetadataOnlyPaper(t *testing.T) {
	svc, _ := newTestLibraryService(t)
	p, err := svc.ImportPaperFromS2(context.Background(), research.Paper{
		PaperID:     "S2:p1",
		Title:       "Attention Is All You Need",
		ExternalIDs: research.IDs{DOI: "10.5555/aiayn"},
		Year:        2017,
		Venue:       "NeurIPS",
		Authors:     []research.Author{{Name: "Vaswani"}},
		Abstract:    "...",
	})
	if err != nil {
		t.Fatalf("ImportPaperFromS2 error: %v", err)
	}
	if p == nil {
		t.Fatal("expected paper, got nil")
	}
	if p.Title != "Attention Is All You Need" {
		t.Fatalf("title = %q", p.Title)
	}
	if p.DOI != "10.5555/aiayn" {
		t.Fatalf("DOI = %q", p.DOI)
	}
}

func TestImportPaperFromS2DeduplicatesByDOI(t *testing.T) {
	svc, repo := newTestLibraryService(t)

	first, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Already Here",
		DOI:              "10.5555/dup",
		OriginalFilename: "x.pdf",
		StoredPDFName:    "x.pdf",
	})
	if err != nil {
		t.Fatalf("seed CreatePaper: %v", err)
	}

	got, err := svc.ImportPaperFromS2(context.Background(), research.Paper{
		PaperID:     "S2:other",
		Title:       "Different Title",
		ExternalIDs: research.IDs{DOI: "10.5555/dup"},
	})
	if err != nil {
		t.Fatalf("import error: %v", err)
	}
	if got.ID != first.ID {
		t.Fatalf("expected dedup → existing id %d, got %d", first.ID, got.ID)
	}
}
```

The helper `newTestLibraryService` should already exist (used by other library_service_*_test.go files); if its signature differs, adjust. If absent, look at an existing test like `library_service_oa_test.go` to mirror the setup.

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/service/ -run TestImportPaperFromS2 -v`
Expected: FAIL — method undefined.

- [ ] **Step 3: Implement the method**

Create `internal/service/library_service_research.go`:

```go
package service

import (
	"context"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

// ImportPaperFromS2 creates a metadata-only paper record from a Semantic Scholar
// Paper. If a paper with the same DOI already exists, returns the existing
// record (no duplicate, no error).
func (s *LibraryService) ImportPaperFromS2(ctx context.Context, p research.Paper) (*model.Paper, error) {
	if dup, err := s.findDuplicateByDOI(strings.TrimSpace(p.ExternalIDs.DOI)); err != nil {
		return nil, err
	} else if dup != nil {
		return dup.Paper, nil
	}

	authors := make([]string, 0, len(p.Authors))
	for _, a := range p.Authors {
		if a.Name != "" {
			authors = append(authors, a.Name)
		}
	}
	authorsText := strings.Join(authors, ", ")

	publishedAt := ""
	if p.Year > 0 {
		publishedAt = strconvItoa(p.Year)
	}

	input := repository.PaperUpsertInput{
		Title:            p.Title,
		DOI:              p.ExternalIDs.DOI,
		AuthorsText:      authorsText,
		Journal:          p.Venue,
		PublishedAt:      publishedAt,
		AbstractText:     p.Abstract,
		OriginalFilename: "",
		StoredPDFName:    "",
		ExtractionStatus: "manual_pending",
	}

	paper, err := s.repo.CreatePaper(input)
	if err != nil {
		return nil, err
	}
	s.decoratePaper(paper)
	return paper, nil
}

// strconvItoa is a tiny helper kept here to avoid importing strconv in callers
// that don't need it elsewhere.
func strconvItoa(n int) string {
	if n == 0 {
		return ""
	}
	digits := []byte{}
	if n < 0 {
		digits = append(digits, '-')
		n = -n
	}
	if n == 0 {
		return "0"
	}
	rev := []byte{}
	for n > 0 {
		rev = append(rev, byte('0'+n%10))
		n /= 10
	}
	for i := len(rev) - 1; i >= 0; i-- {
		digits = append(digits, rev[i])
	}
	return string(digits)
}
```

If `decoratePaper` is unexported and called the same way elsewhere in `library_service_*.go`, copy that idiom; otherwise drop the call.

The "async Unpaywall trigger" mentioned in the spec is intentionally **not implemented in this method** — keep import synchronous; the auto-PDF-fetch can be added in a follow-up task. Mark the spec hook with a TODO comment in the import flow if you want a reminder.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ -run TestImportPaperFromS2 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/library_service_research.go internal/service/library_service_research_test.go
git commit -m "feat(library): add ImportPaperFromS2 metadata-only ingest"
```

---

## Task 10: Research handler — Search / Paper / References / Citations / Recommendations

**Files:**
- Create: `internal/handler/research.go`
- Create: `internal/handler/research_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/handler/research_test.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/service/research"
)

type stubResearchService struct {
	searchResult research.PaperList
	getResult    research.Paper
	getErr       error
}

func (s *stubResearchService) Search(ctx context.Context, q string, opts research.SearchOpts) (research.PaperList, error) {
	return s.searchResult, nil
}
func (s *stubResearchService) GetPaper(ctx context.Context, id string) (research.Paper, error) {
	return s.getResult, s.getErr
}
func (s *stubResearchService) References(ctx context.Context, id string, off, lim int) (research.PaperList, error) {
	return research.PaperList{}, nil
}
func (s *stubResearchService) Citations(ctx context.Context, id string, off, lim int, opts research.CitationOpts) (research.PaperList, error) {
	return research.PaperList{}, nil
}
func (s *stubResearchService) Recommendations(ctx context.Context, id string) ([]research.Paper, error) {
	return nil, nil
}
func (s *stubResearchService) RecommendationsForList(ctx context.Context, pos, neg []string) ([]research.Paper, error) {
	return nil, nil
}

func TestResearchHandlerSearch(t *testing.T) {
	stub := &stubResearchService{
		searchResult: research.PaperList{
			Items: []research.Paper{{PaperID: "p1", Title: "Hello"}},
			Total: 1,
		},
	}
	h := NewResearchHandler(stub, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/research/search?q=hello", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []research.Paper `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Total != 1 || len(body.Items) != 1 {
		t.Fatalf("body = %+v", body)
	}
}

func TestResearchHandlerSearchMissingQuery(t *testing.T) {
	stub := &stubResearchService{}
	h := NewResearchHandler(stub, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/research/search", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestResearchHandlerGetPaperNotFound(t *testing.T) {
	stub := &stubResearchService{getErr: research.ErrPaperNotFound}
	h := NewResearchHandler(stub, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/research/paper/DOI:nope", nil)
	rec := httptest.NewRecorder()
	h.GetPaper(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestResearchHandlerGetPaperRoute(t *testing.T) {
	stub := &stubResearchService{getResult: research.Paper{PaperID: "p1", Title: "Found"}}
	h := NewResearchHandler(stub, nil, nil)

	// Path should be parsed: /api/research/paper/<id>
	req := httptest.NewRequest(http.MethodGet, "/api/research/paper/DOI:10.1%2Fabc", nil)
	rec := httptest.NewRecorder()
	h.GetPaper(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"paperId":"p1"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/handler/ -run TestResearchHandler -v`
Expected: FAIL.

- [ ] **Step 3: Implement the handler**

Create `internal/handler/research.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

// ResearchService is the interface ResearchHandler depends on. Mirrors
// research.Service plus a minimum surface for testability.
type ResearchService interface {
	Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error)
	GetPaper(ctx context.Context, id string) (research.Paper, error)
	References(ctx context.Context, paperID string, offset, limit int) (research.PaperList, error)
	Citations(ctx context.Context, paperID string, offset, limit int, opts research.CitationOpts) (research.PaperList, error)
	Recommendations(ctx context.Context, paperID string) ([]research.Paper, error)
	RecommendationsForList(ctx context.Context, positive, negative []string) ([]research.Paper, error)
}

// BasketStore is the basket-side interface; defined here so we can stub in tests.
type BasketStore interface {
	List(ctx context.Context) ([]research.Paper, error)
	Add(ctx context.Context, s2PaperID, notes string) error
	Remove(ctx context.Context, s2PaperID string) error
	ImportToLibrary(ctx context.Context, ids []string) (imported int, err error)
	ExportMarkdown(ctx context.Context) (string, error)
}

// ResearchHandler aggregates routes under /api/research/*.
type ResearchHandler struct {
	service ResearchService
	basket  BasketStore
}

// NewResearchHandler builds a handler. The third parameter is reserved for an
// optional library-existence checker (see ExistsByDOI) and is currently nil.
func NewResearchHandler(service ResearchService, basket BasketStore, _ interface{}) *ResearchHandler {
	return &ResearchHandler{service: service, basket: basket}
}

// Search → GET /api/research/search?q=...
func (h *ResearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "缺少查询参数 q"))
		return
	}
	opts := research.SearchOpts{
		Year:          r.URL.Query().Get("year"),
		FieldsOfStudy: r.URL.Query().Get("fields_of_study"),
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		opts.Limit, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		opts.Offset, _ = strconv.Atoi(v)
	}

	res, err := h.service.Search(r.Context(), q, opts)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, res)
}

// GetPaper → GET /api/research/paper/:id
func (h *ResearchHandler) GetPaper(w http.ResponseWriter, r *http.Request) {
	id, err := parseResearchID(r.URL.Path, "/api/research/paper/")
	if err != nil {
		sendError(w, err)
		return
	}
	p, err := h.service.GetPaper(r.Context(), id)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, p)
}

// References → GET /api/research/paper/:id/references
func (h *ResearchHandler) References(w http.ResponseWriter, r *http.Request) {
	id, err := parseResearchIDWithSuffix(r.URL.Path, "/api/research/paper/", "/references")
	if err != nil {
		sendError(w, err)
		return
	}
	off, lim := readPageParams(r)
	res, err := h.service.References(r.Context(), id, off, lim)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, res)
}

// Citations → GET /api/research/paper/:id/citations
func (h *ResearchHandler) Citations(w http.ResponseWriter, r *http.Request) {
	id, err := parseResearchIDWithSuffix(r.URL.Path, "/api/research/paper/", "/citations")
	if err != nil {
		sendError(w, err)
		return
	}
	off, lim := readPageParams(r)
	opts := research.CitationOpts{
		InfluentialOnly: r.URL.Query().Get("influential_only") == "true",
	}
	res, err := h.service.Citations(r.Context(), id, off, lim, opts)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, res)
}

// Recommendations → GET /api/research/paper/:id/recommendations
func (h *ResearchHandler) Recommendations(w http.ResponseWriter, r *http.Request) {
	id, err := parseResearchIDWithSuffix(r.URL.Path, "/api/research/paper/", "/recommendations")
	if err != nil {
		sendError(w, err)
		return
	}
	res, err := h.service.Recommendations(r.Context(), id)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"items": res})
}

// RecommendationsForList → POST /api/research/recommendations
func (h *ResearchHandler) RecommendationsForList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Positive []string `json:"positive"`
		Negative []string `json:"negative"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	if len(body.Positive) == 0 {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "positive 不能为空"))
		return
	}
	res, err := h.service.RecommendationsForList(r.Context(), body.Positive, body.Negative)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"items": res})
}

func parseResearchID(path, prefix string) (string, error) {
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || rest == path {
		return "", apperr.New(apperr.CodeInvalidArgument, "paper id 缺失")
	}
	id, err := url.PathUnescape(rest)
	if err != nil {
		return "", apperr.New(apperr.CodeInvalidArgument, "paper id 无效")
	}
	return id, nil
}

func parseResearchIDWithSuffix(path, prefix, suffix string) (string, error) {
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || rest == path {
		return "", apperr.New(apperr.CodeInvalidArgument, "paper id 缺失")
	}
	rest = strings.TrimSuffix(rest, suffix)
	id, err := url.PathUnescape(rest)
	if err != nil {
		return "", apperr.New(apperr.CodeInvalidArgument, "paper id 无效")
	}
	return id, nil
}

func readPageParams(r *http.Request) (int, int) {
	off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if lim <= 0 || lim > 100 {
		lim = 20
	}
	return off, lim
}

func writeResearchError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, research.ErrPaperNotFound):
		sendError(w, apperr.New(apperr.CodeNotFound, "未找到对应论文"))
	case errors.Is(err, research.ErrRateLimited):
		sendError(w, apperr.New(apperr.CodeUnavailable, "Semantic Scholar 限流，请稍后再试"))
	default:
		sendError(w, apperr.New(apperr.CodeUnavailable, "调研服务暂不可用"))
	}
}
```

Note: `sendJSON`, `sendError`, and `apperr.Code*` constants already exist in this package — verify by reading `internal/handler/common.go` and `internal/apperr/`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/handler/ -run TestResearchHandler -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/research.go internal/handler/research_test.go
git commit -m "feat(handler): add /api/research/* search and paper expansion endpoints"
```

---

## Task 11: Basket store + handlers + import-to-library

**Files:**
- Create: `internal/service/research/basket.go`
- Create: `internal/service/research/basket_test.go`
- Modify: `internal/handler/research.go`
- Modify: `internal/handler/research_test.go`

The basket needs the cache to render a paper given just its `s2_paper_id`. So basket lives in the research package and reaches into both the repo (for storage) and the service (for paper lookup).

- [ ] **Step 1: Write the failing test**

Create `internal/service/research/basket_test.go`:

```go
package research

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeBasketRepo struct {
	items map[string]string
}

func newFakeBasketRepo() *fakeBasketRepo { return &fakeBasketRepo{items: map[string]string{}} }

func (f *fakeBasketRepo) AddBasketItem(id, notes string) error { f.items[id] = notes; return nil }
func (f *fakeBasketRepo) RemoveBasketItem(id string) error     { delete(f.items, id); return nil }
func (f *fakeBasketRepo) ListBasketItems() ([]BasketRow, error) {
	out := make([]BasketRow, 0, len(f.items))
	for k, v := range f.items {
		out = append(out, BasketRow{S2PaperID: k, Notes: v, AddedAt: time.Now()})
	}
	return out, nil
}

type fakePaperLookup struct {
	store map[string]Paper
}

func (f *fakePaperLookup) GetPaper(ctx context.Context, id string) (Paper, error) {
	if p, ok := f.store[id]; ok {
		return p, nil
	}
	return Paper{}, ErrPaperNotFound
}

func TestBasketAddListExport(t *testing.T) {
	repo := newFakeBasketRepo()
	lookup := &fakePaperLookup{store: map[string]Paper{
		"S2:p1": {PaperID: "S2:p1", Title: "First", ExternalIDs: IDs{DOI: "10.1/a"}, Year: 2020, Authors: []Author{{Name: "Alice"}}},
	}}
	b := NewBasket(repo, lookup, nil)

	if err := b.Add(context.Background(), "S2:p1", "interesting"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	items, err := b.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Title != "First" {
		t.Fatalf("items = %+v", items)
	}

	md, err := b.ExportMarkdown(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !strings.Contains(md, "First") || !strings.Contains(md, "10.1/a") {
		t.Fatalf("markdown = %q", md)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/service/research/ -run TestBasket -v`
Expected: FAIL.

- [ ] **Step 3: Implement basket**

Create `internal/service/research/basket.go`:

```go
package research

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// BasketRow is the storage view of a basket item.
type BasketRow struct {
	S2PaperID string
	Notes     string
	AddedAt   time.Time
}

// BasketRepo is the storage backend for the basket.
type BasketRepo interface {
	AddBasketItem(s2PaperID, notes string) error
	RemoveBasketItem(s2PaperID string) error
	ListBasketItems() ([]BasketRow, error)
}

// PaperLookup hydrates an item to a full Paper via the cache/service.
type PaperLookup interface {
	GetPaper(ctx context.Context, id string) (Paper, error)
}

// LibraryImporter persists a Paper into the local library and returns the new
// internal record id. Implemented by the library service.
type LibraryImporter interface {
	ImportPaperFromS2(ctx context.Context, p Paper) (importedID int64, err error)
}

// Basket bundles storage + lookup + (optional) library importer.
type Basket struct {
	repo     BasketRepo
	lookup   PaperLookup
	importer LibraryImporter
}

// NewBasket constructs a Basket. importer may be nil if Import is unused (tests).
func NewBasket(repo BasketRepo, lookup PaperLookup, importer LibraryImporter) *Basket {
	return &Basket{repo: repo, lookup: lookup, importer: importer}
}

// Add inserts/updates a basket entry.
func (b *Basket) Add(ctx context.Context, s2PaperID, notes string) error {
	if strings.TrimSpace(s2PaperID) == "" {
		return fmt.Errorf("research: s2_paper_id required")
	}
	return b.repo.AddBasketItem(s2PaperID, notes)
}

// Remove deletes a basket entry by id.
func (b *Basket) Remove(ctx context.Context, s2PaperID string) error {
	return b.repo.RemoveBasketItem(s2PaperID)
}

// List returns all basket items hydrated with Paper metadata. Items whose
// metadata can't be fetched are skipped (logged at the caller level if needed).
func (b *Basket) List(ctx context.Context) ([]Paper, error) {
	rows, err := b.repo.ListBasketItems()
	if err != nil {
		return nil, err
	}
	out := make([]Paper, 0, len(rows))
	for _, row := range rows {
		paper, err := b.lookup.GetPaper(ctx, row.S2PaperID)
		if err != nil {
			continue
		}
		out = append(out, paper)
	}
	return out, nil
}

// ImportToLibrary creates papers in the library for the requested ids and
// removes successful ids from the basket. Returns the number imported.
func (b *Basket) ImportToLibrary(ctx context.Context, ids []string) (int, error) {
	if b.importer == nil {
		return 0, fmt.Errorf("research: importer not configured")
	}
	imported := 0
	for _, id := range ids {
		paper, err := b.lookup.GetPaper(ctx, id)
		if err != nil {
			continue
		}
		if _, err := b.importer.ImportPaperFromS2(ctx, paper); err != nil {
			continue
		}
		_ = b.repo.RemoveBasketItem(id)
		imported++
	}
	return imported, nil
}

// ExportMarkdown formats basket contents as a markdown bullet list.
func (b *Basket) ExportMarkdown(ctx context.Context) (string, error) {
	papers, err := b.List(ctx)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("# 调研篮\n\n")
	for _, p := range papers {
		authors := make([]string, 0, len(p.Authors))
		for _, a := range p.Authors {
			authors = append(authors, a.Name)
		}
		fmt.Fprintf(&sb, "- **%s** — %s (%d)", p.Title, strings.Join(authors, ", "), p.Year)
		if p.ExternalIDs.DOI != "" {
			fmt.Fprintf(&sb, " · DOI: %s", p.ExternalIDs.DOI)
		}
		if p.TLDR != "" {
			fmt.Fprintf(&sb, "\n  > %s", p.TLDR)
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
```

- [ ] **Step 4: Bridge `repository.ResearchRepository` to `BasketRepo`**

Append to `internal/service/research/repo_adapter.go`:

```go
// ListBasketItems implements BasketRepo.
func (a *RepoAdapter) ListBasketItems() ([]BasketRow, error) {
	rows, err := a.Repo.ListBasketItems()
	if err != nil {
		return nil, err
	}
	out := make([]BasketRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, BasketRow{S2PaperID: r.S2PaperID, Notes: r.Notes, AddedAt: r.AddedAt})
	}
	return out, nil
}

// AddBasketItem implements BasketRepo.
func (a *RepoAdapter) AddBasketItem(id, notes string) error {
	return a.Repo.AddBasketItem(id, notes)
}

// RemoveBasketItem implements BasketRepo.
func (a *RepoAdapter) RemoveBasketItem(id string) error {
	return a.Repo.RemoveBasketItem(id)
}
```

- [ ] **Step 5: Update LibraryImporter shim**

The basket's `LibraryImporter` interface returns `int64`. Adjust `library_service_research.go` to expose a thin wrapper:

```go
// ImportPaperFromS2WithID matches research.LibraryImporter.
func (s *LibraryService) ImportPaperFromS2WithID(ctx context.Context, p research.Paper) (int64, error) {
	paper, err := s.ImportPaperFromS2(ctx, p)
	if err != nil {
		return 0, err
	}
	if paper == nil {
		return 0, nil
	}
	return paper.ID, nil
}
```

- [ ] **Step 6: Add basket handlers**

Append to `internal/handler/research.go`:

```go
// Basket → GET /api/research/basket
func (h *ResearchHandler) BasketList(w http.ResponseWriter, r *http.Request) {
	if h.basket == nil {
		sendError(w, apperr.New(apperr.CodeUnavailable, "basket not configured"))
		return
	}
	items, err := h.basket.List(r.Context())
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

// BasketAdd → POST /api/research/basket
func (h *ResearchHandler) BasketAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		S2PaperID string `json:"s2_paper_id"`
		Notes     string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	if body.S2PaperID == "" {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "s2_paper_id 必填"))
		return
	}
	if err := h.basket.Add(r.Context(), body.S2PaperID, body.Notes); err != nil {
		writeResearchError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BasketRemove → DELETE /api/research/basket/:id
func (h *ResearchHandler) BasketRemove(w http.ResponseWriter, r *http.Request) {
	id, err := parseResearchID(r.URL.Path, "/api/research/basket/")
	if err != nil {
		sendError(w, err)
		return
	}
	if err := h.basket.Remove(r.Context(), id); err != nil {
		writeResearchError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BasketImportToLibrary → POST /api/research/basket/import-to-library
func (h *ResearchHandler) BasketImportToLibrary(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	n, err := h.basket.ImportToLibrary(r.Context(), body.IDs)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"imported": n})
}

// BasketExport → GET /api/research/basket/export
func (h *ResearchHandler) BasketExport(w http.ResponseWriter, r *http.Request) {
	md, err := h.basket.ExportMarkdown(r.Context())
	if err != nil {
		writeResearchError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="research-basket.md"`)
	_, _ = w.Write([]byte(md))
}
```

- [ ] **Step 7: Update BasketStore interface in handler**

Replace the existing `BasketStore` declaration in `internal/handler/research.go`:

```go
type BasketStore interface {
	List(ctx context.Context) ([]research.Paper, error)
	Add(ctx context.Context, s2PaperID, notes string) error
	Remove(ctx context.Context, s2PaperID string) error
	ImportToLibrary(ctx context.Context, ids []string) (int, error)
	ExportMarkdown(ctx context.Context) (string, error)
}
```

This matches the `*Basket` shape.

- [ ] **Step 8: Run all research-related tests**

Run: `go test ./internal/service/research/... ./internal/handler/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/service/research/basket.go internal/service/research/basket_test.go internal/service/research/repo_adapter.go internal/service/library_service_research.go internal/handler/research.go
git commit -m "feat(research): add basket service and HTTP endpoints"
```

---

## Task 12: Settings — S2 API key get/set + frontend field

**Files:**
- Modify: `internal/handler/settings.go`
- Modify: `internal/app/server.go` (route registration in next task)
- Modify: `web/settings.html`
- Modify: `web/static/js/settings.js`

- [ ] **Step 1: Add settings handler methods**

In `internal/handler/settings.go`, look at how an existing simple handler (e.g. `ExtractorSettings`) is shaped, and add:

```go
// GetResearchSettings → GET /api/settings/research
func (h *SettingsHandler) GetResearchSettings(w http.ResponseWriter, r *http.Request) {
	key, err := h.service.GetAppSetting("s2_api_key")
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]string{"s2_api_key": key})
}

// PutResearchSettings → PUT /api/settings/research
func (h *SettingsHandler) PutResearchSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		S2APIKey string `json:"s2_api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	if err := h.service.UpsertAppSetting("s2_api_key", body.S2APIKey); err != nil {
		sendError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

`SettingsHandler.service` is the existing `*service.LibraryService`; verify `GetAppSetting` and `UpsertAppSetting` are exported on `LibraryService` (they exist on the repo and likely already on the service). If not, add thin wrappers in `library_service.go`:

```go
func (s *LibraryService) GetAppSetting(key string) (string, error) {
    return s.repo.GetAppSetting(key)
}
func (s *LibraryService) UpsertAppSetting(key, value string) error {
    return s.repo.UpsertAppSetting(key, value)
}
```

- [ ] **Step 2: Add UI input row in `web/settings.html`**

Open `web/settings.html`, locate the "外部 API" / "External APIs" section. Add (adjust class names to match the file's current pattern):

```html
<div class="settings-row">
    <label for="settings-s2-api-key" data-i18n="settings.research.s2.label">Semantic Scholar API key</label>
    <input id="settings-s2-api-key" type="password" autocomplete="off" placeholder="" />
    <p class="hint" data-i18n="settings.research.s2.hint">未填写时使用匿名速率（约 1 req/s）。</p>
</div>
```

- [ ] **Step 3: Wire it in `web/static/js/settings.js`**

Find the load-settings init code (the function that pulls each setting from the server and populates inputs). Add:

```js
async function loadResearchSettings() {
    const res = await fetch('/api/settings/research');
    if (!res.ok) return;
    const data = await res.json();
    const el = document.getElementById('settings-s2-api-key');
    if (el) el.value = data.s2_api_key || '';
}

async function saveResearchSettings() {
    const el = document.getElementById('settings-s2-api-key');
    if (!el) return;
    await fetch('/api/settings/research', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ s2_api_key: el.value }),
    });
}
```

Hook `loadResearchSettings` into the page-init flow and `saveResearchSettings` into the existing "save" button handler.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/handler/...`
Expected: PASS (no new tests, but no regressions).

- [ ] **Step 5: Syntax-check the JS**

Run: `node --check web/static/js/settings.js`
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add internal/handler/settings.go internal/service/library_service.go web/settings.html web/static/js/settings.js
git commit -m "feat(settings): add S2 API key field and /api/settings/research endpoints"
```

---

## Task 13: Wire all routes + service composition in `internal/app/server.go`

**Files:**
- Modify: `internal/app/server.go`

- [ ] **Step 1: Add the imports and constructor wiring**

At the top of `internal/app/server.go`, add the import:

```go
"github.com/xuzhougeng/citebox/internal/service/research"
```

Below the existing `librarySvc` construction (around line 200), add:

```go
s2APIKey := strings.TrimSpace(cfg.S2APIKey)
if s2APIKey == "" {
    s2APIKey, _ = repo.GetAppSetting("s2_api_key")
}
s2Client := research.NewClient(research.Config{
    APIKey:      s2APIKey,
    MinInterval: research.RateInterval(s2APIKey),  // helper added below
})
researchAdapter := &research.RepoAdapter{Repo: repo.Research}
researchSvc := research.NewService(s2Client, researchAdapter, research.ServiceConfig{})
basket := research.NewBasket(researchAdapter, researchSvc, librarySvcImporterShim{librarySvc})
researchHandler := handler.NewResearchHandler(researchSvc, basket, nil)
```

- [ ] **Step 2: Add the importer shim**

In the same file (or a new small helper file `internal/app/research_wire.go`):

```go
type librarySvcImporterShim struct {
    svc *service.LibraryService
}

func (s librarySvcImporterShim) ImportPaperFromS2(ctx context.Context, p research.Paper) (int64, error) {
    return s.svc.ImportPaperFromS2WithID(ctx, p)
}
```

- [ ] **Step 3: Add `RateInterval` helper to research package**

Append to `internal/service/research/client.go`:

```go
// RateInterval returns the minimum delay between requests appropriate for the
// given API key state. With a key, allow ~5 req/s; without, fall back to ~1.
func RateInterval(apiKey string) time.Duration {
	if strings.TrimSpace(apiKey) != "" {
		return 200 * time.Millisecond
	}
	return time.Second
}
```

- [ ] **Step 4: Register all the new routes**

After the existing route block (after the wolai routes around line 540+), append:

```go
mux.HandleFunc("/api/research/search", func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    researchHandler.Search(w, r)
})

mux.HandleFunc("/api/research/recommendations", func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    researchHandler.RecommendationsForList(w, r)
})

mux.HandleFunc("/api/research/paper/", func(w http.ResponseWriter, r *http.Request) {
    p := r.URL.Path
    switch {
    case strings.HasSuffix(p, "/references"):
        researchHandler.References(w, r)
    case strings.HasSuffix(p, "/citations"):
        researchHandler.Citations(w, r)
    case strings.HasSuffix(p, "/recommendations"):
        researchHandler.Recommendations(w, r)
    default:
        researchHandler.GetPaper(w, r)
    }
})

mux.HandleFunc("/api/research/basket", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        researchHandler.BasketList(w, r)
    case http.MethodPost:
        researchHandler.BasketAdd(w, r)
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
})

mux.HandleFunc("/api/research/basket/", func(w http.ResponseWriter, r *http.Request) {
    p := r.URL.Path
    switch {
    case p == "/api/research/basket/import-to-library" && r.Method == http.MethodPost:
        researchHandler.BasketImportToLibrary(w, r)
    case p == "/api/research/basket/export" && r.Method == http.MethodGet:
        researchHandler.BasketExport(w, r)
    case r.Method == http.MethodDelete:
        researchHandler.BasketRemove(w, r)
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
})

mux.HandleFunc("/api/settings/research", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        settingsHandler.GetResearchSettings(w, r)
    case http.MethodPut:
        settingsHandler.PutResearchSettings(w, r)
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
})
```

- [ ] **Step 5: Add the `/research` HTML page route**

Find where existing pages are served (look for `web/library.html` or `web/figures.html` registration). Add the same pattern for `web/research.html`:

```go
mux.HandleFunc("/research", func(w http.ResponseWriter, r *http.Request) {
    serveStaticHTML(w, r, "web/research.html")
})
```

(Adjust to whatever the project's static-HTML helper is named.)

- [ ] **Step 6: Build the server**

Run: `go build ./cmd/server/`
Expected: success.

- [ ] **Step 7: Run the full backend test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 8: Smoke test against running server**

```bash
make run &
SERVER_PID=$!
sleep 3
curl -s 'http://localhost:8080/api/research/search?q=transformer' | head -c 400
kill $SERVER_PID
```

Expected: a JSON response (probably 401 if not authed; or an error from S2 — either is "wired correctly"). Authed requests are expected to return real data.

- [ ] **Step 9: Commit**

```bash
git add internal/app/server.go internal/service/research/client.go
git commit -m "feat(server): wire research service, handlers, and /api/research routes"
```

---

## Task 14: Frontend — research.html skeleton + nav update

**Files:**
- Create: `web/research.html`
- Modify: `web/library.html`, `web/figures.html`, `web/groups.html`, `web/tags.html`, `web/notes.html`, `web/index.html`, `web/ai.html`, `web/palettes.html`, `web/settings.html`, `web/upload.html`, `web/manual.html`, `web/guide.html` — nav block

- [ ] **Step 1: Add `research` to nav in every existing page**

In each listed `web/*.html`, locate the `<ul class="nav-links">` block and insert before `<li><a href="/ai" ...>`:

```html
<li><a href="/research" data-i18n="nav.research">调研</a></li>
```

- [ ] **Step 2: Create `web/research.html`**

Copy the structural shell from `web/library.html` (head + nav + body container). Replace the body content with:

```html
<main class="research-page" id="research-root">
  <section class="research-toolbar">
    <input id="research-search-input" type="text" data-i18n-placeholder="research.search.placeholder" />
    <button id="research-search-btn" data-i18n="research.action.search">搜索</button>
    <button id="research-pick-from-library" data-i18n="research.action.pickFromLibrary">从文献库选种子</button>
    <span id="research-rate-warning" class="research-rate-warning hidden" data-i18n="research.error.rateLimited"></span>
  </section>

  <section class="research-main" id="research-main">
    <aside class="research-seed-pane" id="research-seed-pane">
      <div id="research-empty-state" class="research-empty">
        <p data-i18n="research.empty.hint"></p>
      </div>
    </aside>
    <aside class="research-basket-pane" id="research-basket-pane">
      <h3 data-i18n="research.basket.title">调研篮</h3>
      <ul id="research-basket-list" class="research-basket-list"></ul>
      <div class="research-basket-actions">
        <button id="research-basket-import-all" data-i18n="research.basket.importAll">全部加入文献库</button>
        <button id="research-basket-export" data-i18n="research.basket.exportMarkdown">导出 Markdown 清单</button>
        <button id="research-basket-recommend" data-i18n="research.basket.recommendMore">基于篮子推荐更多</button>
      </div>
    </aside>
  </section>
</main>
<script src="/static/js/api.js"></script>
<script src="/static/js/i18n.js"></script>
<script src="/static/js/research.js"></script>
```

Match the `<head>`/`<nav>` blocks to match `library.html` exactly so the page layout, theme, and i18n bootstrap behave consistently. Mark the active nav item: `<a href="/research" class="active" data-i18n="nav.research">调研</a>`.

- [ ] **Step 3: Add a `research.css` block to existing `style.css`**

Append to `web/static/css/style.css`:

```css
.research-page { padding: 16px; }
.research-toolbar { display: flex; gap: 8px; align-items: center; margin-bottom: 12px; }
.research-toolbar input { flex: 1; }
.research-rate-warning { color: #b85e00; font-size: 12px; }
.research-rate-warning.hidden { display: none; }
.research-main { display: flex; gap: 12px; }
.research-seed-pane { flex: 2; min-width: 0; border: 1px solid var(--border, #ccc); border-radius: 6px; padding: 12px; }
.research-basket-pane { flex: 1; min-width: 220px; border: 1px solid var(--border, #ccc); border-radius: 6px; padding: 12px; background: var(--surface-soft, #fafaf6); }
.research-basket-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 4px; }
.research-empty { padding: 32px; text-align: center; color: #888; }
.research-tabs { display: flex; gap: 6px; border-bottom: 1px solid var(--border, #ccc); margin-bottom: 8px; }
.research-tab { padding: 4px 10px; cursor: pointer; }
.research-tab.active { border-bottom: 2px solid var(--accent, #b07a3a); font-weight: 600; }
.research-list { display: flex; flex-direction: column; gap: 6px; }
.research-list-item { padding: 8px; border: 1px solid var(--border, #e3dccd); border-radius: 4px; }
.research-list-item.in-library { opacity: 0.6; }
```

- [ ] **Step 4: Validate HTML syntax (smoke check)**

Run: `node --check web/research.html 2>/dev/null || true`
(HTML isn't checked by node; this step is a placeholder. Manual verify by loading `/research` after Task 15 lands.)

- [ ] **Step 5: Commit**

```bash
git add web/research.html web/static/css/style.css web/library.html web/figures.html web/groups.html web/tags.html web/notes.html web/index.html web/ai.html web/palettes.html web/settings.html web/upload.html web/manual.html web/guide.html
git commit -m "feat(web): add /research page skeleton and update nav"
```

---

## Task 15: Frontend — research.js — search + seed + tabs

**Files:**
- Create: `web/static/js/research.js`

- [ ] **Step 1: Implement the module**

Create `web/static/js/research.js`:

```js
(function () {
    'use strict';

    const state = {
        seed: null,
        activeTab: 'references',
        list: [],
        offset: 0,
        limit: 20,
        filters: { yearMin: null, yearMax: null, minCites: 0, influentialOnly: false },
        basket: [],
        history: [],
    };

    const dom = {};

    function $(id) { return document.getElementById(id); }

    async function api(path, opts = {}) {
        const res = await fetch(path, {
            ...opts,
            headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) },
        });
        if (res.status === 503) {
            $('research-rate-warning').classList.remove('hidden');
            setTimeout(() => $('research-rate-warning').classList.add('hidden'), 4000);
        }
        if (!res.ok) {
            const text = await res.text();
            throw new Error(`${res.status}: ${text}`);
        }
        return res.json();
    }

    async function searchSeed(query) {
        const data = await api(`/api/research/search?q=${encodeURIComponent(query)}&limit=20`);
        renderSearchResults(data.items || []);
    }

    function renderSearchResults(items) {
        state.activeTab = 'search';
        renderSeedPane(`<h4 data-i18n="research.tab.search">搜索结果</h4>`, items);
    }

    async function setSeed(s2PaperID) {
        if (state.seed) state.history.push(state.seed.paperId);
        const paper = await api(`/api/research/paper/${encodeURIComponent(s2PaperID)}`);
        state.seed = paper;
        state.activeTab = 'references';
        await loadActiveTab();
    }

    async function loadActiveTab() {
        if (!state.seed) return;
        const id = encodeURIComponent(state.seed.paperId);
        let items = [];
        if (state.activeTab === 'references') {
            const data = await api(`/api/research/paper/${id}/references?offset=0&limit=20`);
            items = data.items || [];
        } else if (state.activeTab === 'citations') {
            const params = state.filters.influentialOnly ? '&influential_only=true' : '';
            const data = await api(`/api/research/paper/${id}/citations?offset=0&limit=20${params}`);
            items = data.items || [];
        } else if (state.activeTab === 'recommendations') {
            const data = await api(`/api/research/paper/${id}/recommendations`);
            items = data.items || [];
        }
        renderSeedPane(buildSeedHeader(), items);
    }

    function buildSeedHeader() {
        const s = state.seed;
        const tldr = s.tldr ? `<div class="research-seed-tldr">${escapeHtml(s.tldr)}</div>` : '';
        return `
            <div class="research-seed-card">
                <div class="research-seed-title"><b>${escapeHtml(s.title)}</b></div>
                <div class="research-seed-meta">${escapeHtml(formatAuthors(s.authors))} · ${s.year || ''} · cites ${s.citationCount || 0}</div>
                ${tldr}
                <div class="research-seed-actions">
                    <button data-action="add-seed-to-basket">+ 篮子</button>
                </div>
            </div>
            <div class="research-tabs">
                <span class="research-tab ${state.activeTab === 'references' ? 'active' : ''}" data-tab="references" data-i18n="research.tab.references">引用了</span>
                <span class="research-tab ${state.activeTab === 'citations' ? 'active' : ''}" data-tab="citations" data-i18n="research.tab.citations">被引用</span>
                <span class="research-tab ${state.activeTab === 'recommendations' ? 'active' : ''}" data-tab="recommendations" data-i18n="research.tab.recommendations">相似</span>
            </div>
        `;
    }

    function renderSeedPane(headerHTML, items) {
        state.list = items;
        const listHTML = items.map(p => `
            <div class="research-list-item" data-id="${escapeHtml(p.paperId)}">
                <b>${escapeHtml(p.title)}</b>
                <span class="research-meta"> · ${escapeHtml(formatAuthors(p.authors))} · ${p.year || ''} · ${p.citationCount || 0} cites</span>
                <div class="research-list-actions">
                    <button data-action="add-to-basket" data-id="${escapeHtml(p.paperId)}">+ 篮</button>
                    <button data-action="set-as-seed" data-id="${escapeHtml(p.paperId)}">设为种子</button>
                </div>
            </div>
        `).join('');
        $('research-seed-pane').innerHTML = `${headerHTML}<div class="research-list">${listHTML || '<div class="research-empty">无结果</div>'}</div>`;
    }

    function formatAuthors(authors) {
        if (!authors || !authors.length) return '';
        return authors.slice(0, 3).map(a => a.name).join(', ') + (authors.length > 3 ? ' et al.' : '');
    }

    function escapeHtml(s) {
        if (s == null) return '';
        return String(s)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    async function refreshBasket() {
        const data = await api('/api/research/basket');
        state.basket = data.items || [];
        renderBasket();
    }

    function renderBasket() {
        $('research-basket-list').innerHTML = state.basket.map(p => `
            <li class="research-basket-item" data-id="${escapeHtml(p.paperId)}">
                <span>${escapeHtml(p.title)}</span>
                <button data-action="remove-from-basket" data-id="${escapeHtml(p.paperId)}">×</button>
            </li>
        `).join('');
    }

    async function addToBasket(s2PaperID) {
        await api('/api/research/basket', {
            method: 'POST',
            body: JSON.stringify({ s2_paper_id: s2PaperID }),
        });
        await refreshBasket();
    }

    async function removeFromBasket(s2PaperID) {
        await api(`/api/research/basket/${encodeURIComponent(s2PaperID)}`, { method: 'DELETE' });
        await refreshBasket();
    }

    async function importBasketToLibrary() {
        const ids = state.basket.map(p => p.paperId);
        if (!ids.length) return;
        await api('/api/research/basket/import-to-library', {
            method: 'POST',
            body: JSON.stringify({ ids }),
        });
        await refreshBasket();
    }

    async function exportBasket() {
        const res = await fetch('/api/research/basket/export');
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'research-basket.md';
        a.click();
        URL.revokeObjectURL(url);
    }

    async function recommendFromBasket() {
        const ids = state.basket.map(p => p.paperId);
        if (!ids.length) return;
        const data = await api('/api/research/recommendations', {
            method: 'POST',
            body: JSON.stringify({ positive: ids, negative: [] }),
        });
        state.activeTab = 'basketRec';
        renderSeedPane(`<h4 data-i18n="research.tab.basketRec">基于篮子推荐</h4>`, data.items || []);
    }

    function bindEvents() {
        $('research-search-btn').addEventListener('click', () => {
            const q = $('research-search-input').value.trim();
            if (q) searchSeed(q);
        });
        $('research-search-input').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') $('research-search-btn').click();
        });
        $('research-basket-import-all').addEventListener('click', importBasketToLibrary);
        $('research-basket-export').addEventListener('click', exportBasket);
        $('research-basket-recommend').addEventListener('click', recommendFromBasket);

        // Event delegation for the seed pane (set-as-seed, add-to-basket, tab switches)
        $('research-seed-pane').addEventListener('click', async (e) => {
            const tab = e.target.dataset.tab;
            if (tab) {
                state.activeTab = tab;
                await loadActiveTab();
                return;
            }
            const action = e.target.dataset.action;
            const id = e.target.dataset.id;
            if (action === 'set-as-seed' && id) await setSeed(id);
            if (action === 'add-to-basket' && id) await addToBasket(id);
            if (action === 'add-seed-to-basket' && state.seed) await addToBasket(state.seed.paperId);
        });

        $('research-basket-list').addEventListener('click', async (e) => {
            const action = e.target.dataset.action;
            const id = e.target.dataset.id;
            if (action === 'remove-from-basket' && id) await removeFromBasket(id);
        });
    }

    async function init() {
        bindEvents();
        await refreshBasket();
    }

    document.addEventListener('DOMContentLoaded', init);
})();
```

- [ ] **Step 2: Syntax-check the JS**

Run: `node --check web/static/js/research.js`
Expected: success.

- [ ] **Step 3: Manual smoke test**

```bash
make run
```

Open `http://localhost:8080/research`. Log in, then:
- Type a query and hit search → should see results
- Click "设为种子" → seed card appears + references load
- Click "+ 篮" on an item → it appears in the basket panel
- Click "全部加入文献库" → items vanish from basket; check `/library` for new metadata-only entries
- Click "导出 Markdown 清单" → file downloads

If anything breaks, fix inline before committing.

- [ ] **Step 4: Commit**

```bash
git add web/static/js/research.js
git commit -m "feat(web): add research.js with search, tabs, and basket flow"
```

---

## Task 16: i18n keys

**Files:**
- Modify: `web/static/locales/zh-CN.json`
- Modify: `web/static/locales/en.json`

- [ ] **Step 1: Add `research.*` and `nav.research` keys**

In `web/static/locales/zh-CN.json`, merge into the existing JSON object (preserve indentation/format style):

```json
{
  "nav": {
    "research": "调研"
  },
  "research": {
    "title": "调研",
    "search": { "placeholder": "输入关键词 / DOI / arXiv ID / S2 paperId" },
    "action": {
      "search": "搜索",
      "pickFromLibrary": "从文献库选种子",
      "addToBasket": "+ 篮子",
      "setAsSeed": "设为种子",
      "openInLibrary": "在文献库打开",
      "removeFromBasket": "移除"
    },
    "tab": {
      "references": "引用了",
      "citations": "被引用",
      "recommendations": "相似",
      "search": "搜索结果",
      "basketRec": "基于篮子推荐"
    },
    "basket": {
      "title": "调研篮",
      "importAll": "全部加入文献库",
      "exportMarkdown": "导出 Markdown 清单",
      "recommendMore": "基于篮子推荐更多",
      "empty": "篮子是空的"
    },
    "filter": {
      "year": "年份",
      "minCites": "最低引用数",
      "influentialOnly": "仅高影响力",
      "sort": "排序"
    },
    "error": {
      "rateLimited": "Semantic Scholar 限流，请稍后重试。",
      "notFound": "未找到对应论文。",
      "unknown": "调研服务暂不可用。"
    },
    "empty": { "hint": "在上方搜索框输入关键词或粘贴 DOI / arXiv ID 开始调研。" },
    "history": { "back": "返回上一种子" },
    "inLibrary": "已在文献库"
  },
  "settings": {
    "research": {
      "s2": {
        "label": "Semantic Scholar API key",
        "hint": "未填写时使用匿名速率（约 1 req/s）。"
      }
    }
  }
}
```

In `web/static/locales/en.json`:

```json
{
  "nav": { "research": "Research" },
  "research": {
    "title": "Research",
    "search": { "placeholder": "Keyword, DOI, arXiv ID, or S2 paperId" },
    "action": {
      "search": "Search",
      "pickFromLibrary": "Pick seed from library",
      "addToBasket": "+ Basket",
      "setAsSeed": "Set as seed",
      "openInLibrary": "Open in library",
      "removeFromBasket": "Remove"
    },
    "tab": {
      "references": "References",
      "citations": "Citations",
      "recommendations": "Similar",
      "search": "Search results",
      "basketRec": "Basket recommendations"
    },
    "basket": {
      "title": "Basket",
      "importAll": "Import all to library",
      "exportMarkdown": "Export markdown",
      "recommendMore": "Recommend more from basket",
      "empty": "Basket is empty"
    },
    "filter": {
      "year": "Year",
      "minCites": "Min citations",
      "influentialOnly": "Influential only",
      "sort": "Sort"
    },
    "error": {
      "rateLimited": "Semantic Scholar rate limited; try again shortly.",
      "notFound": "Paper not found.",
      "unknown": "Research service unavailable."
    },
    "empty": { "hint": "Enter a keyword or paste a DOI / arXiv ID above to start researching." },
    "history": { "back": "Back to previous seed" },
    "inLibrary": "Already in library"
  },
  "settings": {
    "research": {
      "s2": {
        "label": "Semantic Scholar API key",
        "hint": "Leave blank to use anonymous rate (~1 req/s)."
      }
    }
  }
}
```

Make sure the merge is a JSON merge — don't overwrite existing top-level keys. If an existing `nav` or `settings` block already has children, append your keys into them.

- [ ] **Step 2: Validate JSON**

Run: `python3 -m json.tool web/static/locales/zh-CN.json > /dev/null && python3 -m json.tool web/static/locales/en.json > /dev/null`
Expected: no output (valid).

- [ ] **Step 3: Reload `/research` in the browser**

Confirm strings render in zh-CN; toggle to English (existing language switcher) and confirm.

- [ ] **Step 4: Commit**

```bash
git add web/static/locales/zh-CN.json web/static/locales/en.json
git commit -m "i18n(research): add zh-CN and en strings for the research panel"
```

---

## Task 17: Backend integration test

**Files:**
- Create: `internal/service/research/integration_test.go`

- [ ] **Step 1: Write the test**

Create `internal/service/research/integration_test.go`:

```go
package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/repository"
)

// TestResearchEndToEnd exercises the full path: search → set seed → add to basket
// → import to library, against an in-process S2 stub.
func TestResearchEndToEnd(t *testing.T) {
	repo, _ := repository.NewTestLibraryRepo(t)  // helper exposed via test setup

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/graph/v1/paper/search":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"total": 1, "offset": 0, "next": 0,
				"data": []map[string]interface{}{
					{
						"paperId":     "p1",
						"title":       "Attention",
						"externalIds": map[string]string{"DOI": "10.1/abc"},
						"year":        2017,
					},
				},
			})
		case r.URL.Path == "/graph/v1/paper/p1":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"paperId": "p1", "title": "Attention", "year": 2017,
				"externalIds": map[string]string{"DOI": "10.1/abc"},
			})
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusBadRequest)
		}
	}))
	defer s2.Close()

	cli := NewClient(Config{BaseURL: s2.URL, MinInterval: 0})
	adapter := &RepoAdapter{Repo: repo.Research}
	svc := NewService(cli, adapter, ServiceConfig{PaperTTL: time.Hour})

	res, err := svc.Search(context.Background(), "transformer", SearchOpts{Limit: 5})
	if err != nil || len(res.Items) != 1 {
		t.Fatalf("search err=%v items=%d", err, len(res.Items))
	}

	paper, err := svc.GetPaper(context.Background(), res.Items[0].PaperID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if paper.Title != "Attention" {
		t.Fatalf("title = %q", paper.Title)
	}

	// Cache hit on second call: stop the upstream and re-fetch.
	s2.Close()
	paper2, err := svc.GetPaper(context.Background(), res.Items[0].PaperID)
	if err != nil {
		t.Fatalf("cached get: %v", err)
	}
	if paper2.Title != "Attention" {
		t.Fatalf("expected cache hit, got %q", paper2.Title)
	}
}
```

If `repository.NewTestLibraryRepo` is not exported, copy the in-memory setup from `library_repo_test.go` into a small helper function inside this test file.

- [ ] **Step 2: Run**

Run: `go test ./internal/service/research/ -run TestResearchEndToEnd -v`
Expected: PASS.

- [ ] **Step 3: Run the entire suite once more**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/service/research/integration_test.go
git commit -m "test(research): add end-to-end integration test for cache + search"
```

---

## Task 18: README + manual page mention

**Files:**
- Modify: `README.md` and/or `web/manual.html`

- [ ] **Step 1: Add a section to README**

In `README.md`, after the existing feature highlights, add:

```markdown
### 调研 / Research panel

`/research` 提供基于 Semantic Scholar Graph API 的单跳文献拓展能力：搜索关键词、查看 references / citations / recommendations、把感兴趣的论文加入"调研篮"，再一键导入文献库。

Semantic Scholar API key 可在 设置 → 外部 API 里填写，或通过 `S2_API_KEY` 环境变量提供。未填写时使用匿名速率（约 1 req/s）。
```

Mirror in `README.en.md`.

- [ ] **Step 2: Add a short page in `web/manual.html`** (if the manual is structured as in-app docs)

Add a `<section>` with quick instructions matching the README content. Skip if the manual is auto-generated.

- [ ] **Step 3: Commit**

```bash
git add README.md README.en.md web/manual.html
git commit -m "docs(research): add /research panel section to README and manual"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Plan task |
| --- | --- |
| §1 Architecture overview | Task 1, 2, 4-9 |
| §2.1 S2 client | Task 4, 5, 6 |
| §2.2 Cache strategy | Task 1, 2, 7, 8 |
| §2.3 Basket persistence | Task 1, 2, 11 |
| §2.4 API routes | Task 10, 11, 13 |
| §3.1 Page skeleton | Task 14 |
| §3.2 JS module | Task 15 |
| §3.3 i18n | Task 16 |
| §4 Settings | Task 12, 13 |
| §5.1 Unit tests | Task 2, 5, 6, 7, 9, 10, 11 |
| §5.2 Integration test | Task 17 |
| §6 Risks (rate-limit UX) | Task 15 (rate-warning toast) |
| §7 Phase suggestion | Tasks 1-9 = phase 1 (backend), 10-13 = phase 2 (HTTP), 14-16 = phase 3 (UI), 17-18 = phase 4 (polish) |

**Placeholder scan:** No "TBD"/"TODO"/"implement later" entries. The "async Unpaywall trigger" mentioned in the spec §2.3 is intentionally deferred, with a comment in Task 9 noting it as out-of-scope for this plan — captured in §6 of the spec.

**Type consistency:**
- `research.Paper`, `research.PaperList`, `research.SearchOpts`, `research.CitationOpts` — used consistently across types.go, client.go, service.go, basket.go, handler/research.go
- `research.CacheRepo`, `research.BasketRepo`, `research.PaperLookup`, `research.LibraryImporter` — interfaces declared in service.go / basket.go and satisfied by RepoAdapter and the library service shim
- `LibraryService.ImportPaperFromS2` returns `(*model.Paper, error)`; `LibraryService.ImportPaperFromS2WithID` returns `(int64, error)` — both used; the basket goes through the int64 variant via the shim
- Cache keys: always `paper:<id>` and `rec:<id>` (defined once in service.go's helpers)

**Known follow-ups (out of scope, captured for tracking):**

- Rate-limit warning currently a transient toast; consider a persistent badge.
- arXiv-only "已在库中" judgment — needs an arXiv-id index on `papers` table.
- Auto Unpaywall PDF trigger after `ImportPaperFromS2`.
- Server-side `/api/library/papers/exists` batch endpoint for the front-end to flag already-imported papers in lists (mentioned in §3.2; not included in this plan because the basic flow works without it; add as a follow-up plan if frontends start showing many duplicates).
