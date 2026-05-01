# AI External Search Sources Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add configurable multi-source AI external search, defaulting to PubMed while keeping Semantic Scholar available as an explicit source.

**Architecture:** Add `internal/service/pubmed` for NCBI E-utilities and `internal/service/ai_external` for AI-only source orchestration, adapters, and merge logic. Wire AI assistant external search and AI conversation external evidence through `ai_external.Service`; keep `/research` and the existing Semantic Scholar research basket untouched.

**Tech Stack:** Go stdlib (`net/http`, `encoding/xml`, `encoding/json`, `httptest`, `sync`, `time`), SQLite app settings, existing vanilla JS settings page, existing CiteBox i18n JSON files.

---

## File Structure

**Create:**

- `internal/model/ai_external_settings.go` — persisted AI external source settings and normalizers.
- `internal/service/ai_external_settings.go` — `LibraryService` getters/updaters for AI external source settings.
- `internal/service/pubmed/types.go` — PubMed client types and normalized paper shape.
- `internal/service/pubmed/client.go` — NCBI E-utilities client, rate limiter, XML parsing.
- `internal/service/pubmed/client_test.go` — PubMed client tests using `httptest.Server`.
- `internal/service/ai_external/types.go` — source IDs, paper/snippet/result types, searcher interfaces.
- `internal/service/ai_external/merge.go` — DOI/PMID/title normalization and cross-source merge.
- `internal/service/ai_external/merge_test.go` — merge behavior tests.
- `internal/service/ai_external/service.go` — source orchestration, partial failure handling, per-source query execution.
- `internal/service/ai_external/service_test.go` — orchestration tests.
- `internal/service/ai_external/s2_adapter.go` — Semantic Scholar adapter over existing `research.Service`.
- `internal/service/ai_external/pubmed_adapter.go` — PubMed adapter over `pubmed.Client`.

**Modify:**

- `internal/config/config.go` — add `PubMedAPIKey`, `PubMedEmail`, `PubMedTool` env config.
- `internal/handler/settings.go` — add AI external settings endpoints and hot reload hook.
- `internal/app/server.go` — construct PubMed client, `ai_external.Service`, and route settings.
- `internal/service/ai_assistant/external_search_tool.go` — consume `ai_external.Service` results instead of `research.Service` directly.
- `internal/service/ai_assistant/library_agents.go` — update planner JSON shape and prompt for source-specific queries.
- `internal/service/ai_assistant/types.go` — extend citations/cards for source metadata as needed.
- `internal/service/ai_assistant/tools_test.go` — update external search tests for source-aware behavior.
- `internal/service/ai_conversation/evidence.go` — route external evidence through `ai_external.Service` and update source labels.
- `internal/service/ai_conversation/evidence_test.go` — update evidence tests for source-aware behavior.
- `web/settings.html` — add AI external source checkboxes and PubMed settings.
- `web/static/js/settings.js` — load/save new settings.
- `web/static/locales/zh-CN/settings.json` — Chinese settings strings.
- `web/static/locales/en/settings.json` — English settings strings.
- `docs/api.md` — document `GET/PUT /api/settings/ai-external-search`.

---

## Task 1: Add AI External Settings Model And Service

**Files:**

- Create: `internal/model/ai_external_settings.go`
- Create: `internal/service/ai_external_settings.go`
- Modify: `internal/config/config.go`
- Test: `internal/service/library_service_integration_test.go`

- [ ] **Step 1: Write failing service tests**

Append these tests to `internal/service/library_service_integration_test.go`:

```go
func TestAIExternalSearchSettingsDefaultToPubMed(t *testing.T) {
	svc, repo, _ := newTestLibraryService(t)

	settings, err := svc.GetAIExternalSearchSettings()
	if err != nil {
		t.Fatalf("GetAIExternalSearchSettings() error = %v", err)
	}
	if len(settings.Sources) != 1 || settings.Sources[0] != "pubmed" {
		t.Fatalf("Sources = %#v, want [pubmed]", settings.Sources)
	}

	raw, err := repo.GetAppSetting("ai_external_search_settings")
	if err != nil {
		t.Fatalf("GetAppSetting() error = %v", err)
	}
	if raw != "" {
		t.Fatalf("raw default setting = %q, want empty until saved", raw)
	}
}

func TestUpdateAIExternalSearchSettingsNormalizesSourcesAndPubMedFields(t *testing.T) {
	svc, repo, _ := newTestLibraryService(t)

	updated, err := svc.UpdateAIExternalSearchSettings(model.AIExternalSearchSettings{
		Sources:      []string{"semantic_scholar", "pubmed", "pubmed", "bad"},
		PubMedAPIKey: " key ",
		PubMedEmail:  " user@example.org ",
		PubMedTool:   " CiteBox Desktop ",
	})
	if err != nil {
		t.Fatalf("UpdateAIExternalSearchSettings() error = %v", err)
	}
	if got, want := strings.Join(updated.Sources, ","), "semantic_scholar,pubmed"; got != want {
		t.Fatalf("Sources = %q, want %q", got, want)
	}
	if updated.PubMedAPIKey != "key" || updated.PubMedEmail != "user@example.org" || updated.PubMedTool != "CiteBox Desktop" {
		t.Fatalf("settings not trimmed: %+v", updated)
	}

	raw, err := repo.GetAppSetting("ai_external_search_settings")
	if err != nil {
		t.Fatalf("GetAppSetting() error = %v", err)
	}
	if !strings.Contains(raw, `"sources":["semantic_scholar","pubmed"]`) {
		t.Fatalf("persisted raw = %s", raw)
	}
}
```

Add imports if missing:

```go
import (
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
)
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/service -run TestAIExternalSearchSettings -v
```

Expected: FAIL because `GetAIExternalSearchSettings`, `UpdateAIExternalSearchSettings`, and `model.AIExternalSearchSettings` do not exist.

- [ ] **Step 3: Add model settings type**

Create `internal/model/ai_external_settings.go`:

```go
package model

import "strings"

const (
	AIExternalSourcePubMed          = "pubmed"
	AIExternalSourceSemanticScholar = "semantic_scholar"
)

type AIExternalSearchSettings struct {
	Sources      []string `json:"sources"`
	PubMedAPIKey string   `json:"pubmed_api_key,omitempty"`
	PubMedEmail  string   `json:"pubmed_email,omitempty"`
	PubMedTool   string   `json:"pubmed_tool,omitempty"`
}

func NormalizeAIExternalSearchSettings(input AIExternalSearchSettings) AIExternalSearchSettings {
	seen := map[string]bool{}
	sources := make([]string, 0, len(input.Sources))
	for _, source := range input.Sources {
		source = strings.TrimSpace(strings.ToLower(source))
		switch source {
		case AIExternalSourcePubMed, AIExternalSourceSemanticScholar:
		default:
			continue
		}
		if seen[source] {
			continue
		}
		seen[source] = true
		sources = append(sources, source)
	}
	if input.Sources == nil && len(sources) == 0 {
		sources = []string{AIExternalSourcePubMed}
	}
	return AIExternalSearchSettings{
		Sources:      sources,
		PubMedAPIKey: strings.TrimSpace(input.PubMedAPIKey),
		PubMedEmail:  strings.TrimSpace(input.PubMedEmail),
		PubMedTool:   strings.TrimSpace(input.PubMedTool),
	}
}
```

- [ ] **Step 4: Add service getters and updaters**

Create `internal/service/ai_external_settings.go`:

```go
package service

import (
	"encoding/json"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

const aiExternalSearchSettingsKey = "ai_external_search_settings"

func (s *LibraryService) GetAIExternalSearchSettings() (*model.AIExternalSearchSettings, error) {
	raw, err := s.repo.GetAppSetting(aiExternalSearchSettingsKey)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取 AI 外部搜索配置失败", err)
	}
	settings := model.NormalizeAIExternalSearchSettings(model.AIExternalSearchSettings{})
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "解析 AI 外部搜索配置失败", err)
		}
	}
	normalized := model.NormalizeAIExternalSearchSettings(settings)
	return &normalized, nil
}

func (s *LibraryService) UpdateAIExternalSearchSettings(input model.AIExternalSearchSettings) (*model.AIExternalSearchSettings, error) {
	settings := model.NormalizeAIExternalSearchSettings(input)
	payload, err := json.Marshal(settings)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化 AI 外部搜索配置失败", err)
	}
	if err := s.repo.UpsertAppSetting(aiExternalSearchSettingsKey, string(payload)); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "保存 AI 外部搜索配置失败", err)
	}
	return &settings, nil
}
```

- [ ] **Step 5: Add PubMed env config fields**

In `internal/config/config.go`, add fields to `Config`:

```go
PubMedAPIKey string
PubMedEmail  string
PubMedTool   string
```

Add defaults in `Load()` near `S2APIKey`:

```go
PubMedAPIKey: getEnv("PUBMED_API_KEY", ""),
PubMedEmail:  getEnv("PUBMED_EMAIL", ""),
PubMedTool:   getEnv("PUBMED_TOOL", "citebox"),
```

- [ ] **Step 6: Run tests**

Run:

```bash
gofmt -w internal/model/ai_external_settings.go internal/service/ai_external_settings.go internal/config/config.go internal/service/library_service_integration_test.go
go test ./internal/service -run TestAIExternalSearchSettings -v
go test ./internal/config ./internal/service
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/model/ai_external_settings.go internal/service/ai_external_settings.go internal/config/config.go internal/service/library_service_integration_test.go
git commit -m "Add AI external search settings"
```

---

## Task 2: Add Settings API For AI External Search

**Files:**

- Modify: `internal/handler/settings.go`
- Modify: `internal/app/server.go`
- Test: `internal/handler/settings_test.go`

- [ ] **Step 1: Write failing handler tests**

Create `internal/handler/settings_test.go` if it does not exist. If it exists, append:

```go
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

func newSettingsHandlerForTest(t *testing.T) (*SettingsHandler, *repository.LibraryRepository) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		StorageDir:              filepath.Join(root, "storage"),
		DatabasePath:            filepath.Join(root, "library.db"),
		AdminUsername:           "citebox",
		AdminPassword:           "citebox123",
		ExtractorTimeoutSeconds: 1,
		ExtractorPollInterval:   1,
		ExtractorFileField:      "file",
	}
	repo, err := repository.NewLibraryRepository(cfg.DatabasePath)
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	librarySvc, err := service.NewLibraryService(repo, cfg, service.WithoutBackgroundJobs())
	if err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}
	return NewSettingsHandler(librarySvc, service.NewVersionService()), repo
}

func TestGetAIExternalSearchSettingsDefaultsToPubMed(t *testing.T) {
	h, _ := newSettingsHandlerForTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/settings/ai-external-search", nil)
	rec := httptest.NewRecorder()

	h.GetAIExternalSearchSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body model.AIExternalSearchSettings
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(body.Sources) != 1 || body.Sources[0] != model.AIExternalSourcePubMed {
		t.Fatalf("Sources = %#v, want [pubmed]", body.Sources)
	}
}

func TestPutAIExternalSearchSettingsPersistsNormalizedValues(t *testing.T) {
	h, repo := newSettingsHandlerForTest(t)
	req := httptest.NewRequest(http.MethodPut, "/api/settings/ai-external-search", bytes.NewBufferString(`{
		"sources":["semantic_scholar","pubmed","pubmed"],
		"pubmed_api_key":" key ",
		"pubmed_email":" user@example.org ",
		"pubmed_tool":" CiteBox "
	}`))
	rec := httptest.NewRecorder()

	h.PutAIExternalSearchSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	raw, err := repo.GetAppSetting("ai_external_search_settings")
	if err != nil {
		t.Fatalf("GetAppSetting() error = %v", err)
	}
	if !strings.Contains(raw, `"sources":["semantic_scholar","pubmed"]`) {
		t.Fatalf("raw = %s", raw)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/handler -run 'Test(Get|Put)AIExternalSearchSettings' -v
```

Expected: FAIL because handler methods are missing.

- [ ] **Step 3: Add handler methods**

In `internal/handler/settings.go`, add:

```go
// GetAIExternalSearchSettings -> GET /api/settings/ai-external-search
func (h *SettingsHandler) GetAIExternalSearchSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.libraryService.GetAIExternalSearchSettings()
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, settings)
}

// PutAIExternalSearchSettings -> PUT /api/settings/ai-external-search
func (h *SettingsHandler) PutAIExternalSearchSettings(w http.ResponseWriter, r *http.Request) {
	var body model.AIExternalSearchSettings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	settings, err := h.libraryService.UpdateAIExternalSearchSettings(body)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, settings)
}
```

- [ ] **Step 4: Add route**

In `internal/app/server.go`, register route near `/api/settings/research`:

```go
mux.HandleFunc("/api/settings/ai-external-search", func(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settingsHandler.GetAIExternalSearchSettings(w, r)
	case http.MethodPut:
		settingsHandler.PutAIExternalSearchSettings(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
})
```

- [ ] **Step 5: Run tests**

Run:

```bash
gofmt -w internal/handler/settings.go internal/handler/settings_test.go internal/app/server.go
go test ./internal/handler -run 'Test(Get|Put)AIExternalSearchSettings' -v
go test ./internal/app ./internal/handler
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/handler/settings.go internal/handler/settings_test.go internal/app/server.go
git commit -m "Add AI external search settings API"
```

---

## Task 3: Add PubMed Client

**Files:**

- Create: `internal/service/pubmed/types.go`
- Create: `internal/service/pubmed/client.go`
- Create: `internal/service/pubmed/client_test.go`

- [ ] **Step 1: Write failing PubMed client tests**

Create `internal/service/pubmed/client_test.go`:

```go
package pubmed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientSearchHydratesPubMedArticle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/entrez/eutils/esearch.fcgi":
			if got := r.URL.Query().Get("term"); got != "cell fate" {
				t.Fatalf("term = %q, want cell fate", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"esearchresult":{"idlist":["12345"]}}`))
		case "/entrez/eutils/efetch.fcgi":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(pubmedFetchXML))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, MinInterval: 0})
	res, err := client.Search(context.Background(), "cell fate", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("len = %d, want 1", len(res.Items))
	}
	p := res.Items[0]
	if p.PMID != "12345" || p.Title != "Cell fate control" || p.DOI != "10.1/example" || p.PMCID != "PMC123" {
		t.Fatalf("paper = %+v", p)
	}
	if p.Year != 2024 || p.Journal != "Nature Medicine" {
		t.Fatalf("paper = %+v", p)
	}
	if !strings.Contains(p.Abstract, "cell fate decisions") {
		t.Fatalf("abstract = %q", p.Abstract)
	}
	if len(p.Authors) != 2 || p.Authors[0] != "Ada Lovelace" || p.Authors[1] != "Alan Turing" {
		t.Fatalf("authors = %#v", p.Authors)
	}
}

func TestClientSearchReturnsEmptyWithoutFetching(t *testing.T) {
	fetchCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/entrez/eutils/efetch.fcgi" {
			fetchCalled = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"esearchresult":{"idlist":[]}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, MinInterval: 0})
	res, err := client.Search(context.Background(), "nothing", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(res.Items) != 0 || fetchCalled {
		t.Fatalf("items = %#v fetchCalled = %v", res.Items, fetchCalled)
	}
}

func TestClientRetriesRateLimitOnce(t *testing.T) {
	calls := 0
	oldBackoff := retryBackoff
	retryBackoff = time.Millisecond
	t.Cleanup(func() { retryBackoff = oldBackoff })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, "rate", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"esearchresult":{"idlist":[]}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, MinInterval: 0})
	if _, err := client.Search(context.Background(), "retry", SearchOptions{Limit: 1}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

const pubmedFetchXML = `<?xml version="1.0"?>
<PubmedArticleSet>
  <PubmedArticle>
    <MedlineCitation>
      <PMID>12345</PMID>
      <Article>
        <ArticleTitle>Cell fate control</ArticleTitle>
        <Journal>
          <Title>Nature Medicine</Title>
          <JournalIssue><PubDate><Year>2024</Year></PubDate></JournalIssue>
        </Journal>
        <Abstract><AbstractText>Signals control cell fate decisions.</AbstractText></Abstract>
        <AuthorList>
          <Author><LastName>Lovelace</LastName><ForeName>Ada</ForeName></Author>
          <Author><LastName>Turing</LastName><ForeName>Alan</ForeName></Author>
        </AuthorList>
        <ELocationID EIdType="doi">10.1/example</ELocationID>
      </Article>
    </MedlineCitation>
    <PubmedData>
      <ArticleIdList>
        <ArticleId IdType="doi">10.1/example</ArticleId>
        <ArticleId IdType="pmc">PMC123</ArticleId>
      </ArticleIdList>
    </PubmedData>
  </PubmedArticle>
</PubmedArticleSet>`
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/service/pubmed -v
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Add PubMed types**

Create `internal/service/pubmed/types.go`:

```go
package pubmed

type Paper struct {
	PMID     string
	PMCID    string
	DOI      string
	Title    string
	Abstract string
	Journal  string
	Year     int
	Authors  []string
	URL      string
}

type SearchOptions struct {
	Limit int
}

type SearchResult struct {
	Items []Paper
}
```

- [ ] **Step 4: Add PubMed client**

Create `internal/service/pubmed/client.go`:

```go
package pubmed

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrRateLimited = errors.New("pubmed: rate limited")
var retryBackoff = 500 * time.Millisecond

type Config struct {
	BaseURL     string
	APIKey      string
	Email       string
	Tool        string
	HTTPClient  *http.Client
	MinInterval time.Duration
}

type Client struct {
	baseURL     string
	apiKeyMu    sync.RWMutex
	apiKey      string
	email       string
	tool        string
	httpClient  *http.Client
	tokens      chan struct{}
	closeTicker chan struct{}
}

func NewClient(cfg Config) *Client {
	c := &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:     strings.TrimSpace(cfg.APIKey),
		email:      strings.TrimSpace(cfg.Email),
		tool:       strings.TrimSpace(cfg.Tool),
		httpClient: cfg.HTTPClient,
	}
	if c.baseURL == "" {
		c.baseURL = "https://eutils.ncbi.nlm.nih.gov"
	}
	if c.tool == "" {
		c.tool = "citebox"
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

func RateInterval(apiKey string) time.Duration {
	if strings.TrimSpace(apiKey) != "" {
		return 100 * time.Millisecond
	}
	return 350 * time.Millisecond
}

func (c *Client) Close() {
	if c.closeTicker != nil {
		close(c.closeTicker)
	}
}

func (c *Client) SetSettings(apiKey, email, tool string) {
	c.apiKeyMu.Lock()
	defer c.apiKeyMu.Unlock()
	c.apiKey = strings.TrimSpace(apiKey)
	c.email = strings.TrimSpace(email)
	c.tool = strings.TrimSpace(tool)
	if c.tool == "" {
		c.tool = "citebox"
	}
}

func (c *Client) refillTokens(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
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

func (c *Client) Search(ctx context.Context, query string, opts SearchOptions) (SearchResult, error) {
	ids, err := c.esearch(ctx, query, opts)
	if err != nil || len(ids) == 0 {
		return SearchResult{}, err
	}
	items, err := c.efetch(ctx, ids)
	if err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Items: items}, nil
}

func (c *Client) esearch(ctx context.Context, query string, opts SearchOptions) ([]string, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := url.Values{}
	q.Set("db", "pubmed")
	q.Set("retmode", "json")
	q.Set("term", query)
	q.Set("retmax", strconv.Itoa(limit))
	var raw struct {
		Result struct {
			IDList []string `json:"idlist"`
		} `json:"esearchresult"`
	}
	if err := c.doJSON(ctx, "/entrez/eutils/esearch.fcgi", q, &raw); err != nil {
		return nil, err
	}
	return raw.Result.IDList, nil
}

func (c *Client) efetch(ctx context.Context, ids []string) ([]Paper, error) {
	q := url.Values{}
	q.Set("db", "pubmed")
	q.Set("retmode", "xml")
	q.Set("id", strings.Join(ids, ","))
	var raw pubmedArticleSet
	if err := c.doXML(ctx, "/entrez/eutils/efetch.fcgi", q, &raw); err != nil {
		return nil, err
	}
	out := make([]Paper, 0, len(raw.Articles))
	for _, article := range raw.Articles {
		p := article.paper()
		if p.PMID == "" {
			continue
		}
		p.URL = "https://pubmed.ncbi.nlm.nih.gov/" + p.PMID + "/"
		out = append(out, p)
	}
	return out, nil
}

func (c *Client) doJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, path, query, func(body io.Reader) error {
		return json.NewDecoder(body).Decode(out)
	})
}

func (c *Client) doXML(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, path, query, func(body io.Reader) error {
		return xml.NewDecoder(body).Decode(out)
	})
}

func (c *Client) do(ctx context.Context, path string, query url.Values, decode func(io.Reader) error) error {
	err := c.doOnce(ctx, path, query, decode)
	if !errors.Is(err, ErrRateLimited) {
		return err
	}
	select {
	case <-time.After(retryBackoff):
	case <-ctx.Done():
		return ctx.Err()
	}
	return c.doOnce(ctx, path, query, decode)
}

func (c *Client) doOnce(ctx context.Context, path string, query url.Values, decode func(io.Reader) error) error {
	if err := c.takeToken(ctx); err != nil {
		return err
	}
	c.apiKeyMu.RLock()
	apiKey, email, tool := c.apiKey, c.email, c.tool
	c.apiKeyMu.RUnlock()
	if apiKey != "" {
		query.Set("api_key", apiKey)
	}
	if email != "" {
		query.Set("email", email)
	}
	if tool != "" {
		query.Set("tool", tool)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return decode(resp.Body)
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pubmed: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

type pubmedArticleSet struct {
	Articles []pubmedArticle `xml:"PubmedArticle"`
}

type pubmedArticle struct {
	Medline struct {
		PMID    string `xml:"PMID"`
		Article struct {
			Title       string `xml:"ArticleTitle"`
			ELocations []struct {
				Type  string `xml:"EIdType,attr"`
				Value string `xml:",chardata"`
			} `xml:"ELocationID"`
			Journal struct {
				Title string `xml:"Title"`
				Issue struct {
					PubDate struct {
						Year string `xml:"Year"`
					} `xml:"PubDate"`
				} `xml:"JournalIssue"`
			} `xml:"Journal"`
			Abstract struct {
				Texts []string `xml:"AbstractText"`
			} `xml:"Abstract"`
			Authors []struct {
				LastName string `xml:"LastName"`
				ForeName string `xml:"ForeName"`
			} `xml:"AuthorList>Author"`
		} `xml:"Article"`
	} `xml:"MedlineCitation"`
	PubmedData struct {
		IDs []struct {
			Type  string `xml:"IdType,attr"`
			Value string `xml:",chardata"`
		} `xml:"ArticleIdList>ArticleId"`
	} `xml:"PubmedData"`
}

func (a pubmedArticle) paper() Paper {
	p := Paper{
		PMID:     strings.TrimSpace(a.Medline.PMID),
		Title:    normalizeSpace(a.Medline.Article.Title),
		Abstract: normalizeSpace(strings.Join(a.Medline.Article.Abstract.Texts, " ")),
		Journal:  normalizeSpace(a.Medline.Article.Journal.Title),
	}
	p.Year, _ = strconv.Atoi(strings.TrimSpace(a.Medline.Article.Journal.Issue.PubDate.Year))
	for _, id := range a.PubmedData.IDs {
		switch strings.ToLower(strings.TrimSpace(id.Type)) {
		case "doi":
			p.DOI = strings.TrimSpace(id.Value)
		case "pmc":
			p.PMCID = strings.TrimSpace(id.Value)
		}
	}
	for _, loc := range a.Medline.Article.ELocations {
		if strings.EqualFold(strings.TrimSpace(loc.Type), "doi") && p.DOI == "" {
			p.DOI = strings.TrimSpace(loc.Value)
		}
	}
	for _, author := range a.Medline.Article.Authors {
		name := strings.TrimSpace(strings.TrimSpace(author.ForeName) + " " + strings.TrimSpace(author.LastName))
		if name != "" {
			p.Authors = append(p.Authors, name)
		}
	}
	return p
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
```

- [ ] **Step 5: Run tests**

Run:

```bash
gofmt -w internal/service/pubmed
go test ./internal/service/pubmed -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/service/pubmed
git commit -m "Add PubMed E-utilities client"
```

---

## Task 4: Add AI External Types And Merge Logic

**Files:**

- Create: `internal/service/ai_external/types.go`
- Create: `internal/service/ai_external/merge.go`
- Create: `internal/service/ai_external/merge_test.go`

- [ ] **Step 1: Write failing merge tests**

Create `internal/service/ai_external/merge_test.go`:

```go
package ai_external

import "testing"

func TestMergePapersDedupesByDOIAndMergesSources(t *testing.T) {
	in := []SourceResult{
		{Source: SourcePubMed, Papers: []Paper{{Source: SourcePubMed, SourcePaperID: "pmid-1", PMID: "1", DOI: "https://doi.org/10.1/ABC", Title: "Short", Abstract: "short"}}},
		{Source: SourceSemanticScholar, Papers: []Paper{{Source: SourceSemanticScholar, SourcePaperID: "s2-1", DOI: "10.1/abc", Title: "Short", Abstract: "a much longer abstract"}}},
	}
	out := MergePapers(in, []SourceID{SourcePubMed, SourceSemanticScholar}, 10)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(out), out)
	}
	p := out[0]
	if len(p.Sources) != 2 || p.SourcePaperIDs[SourcePubMed] != "pmid-1" || p.SourcePaperIDs[SourceSemanticScholar] != "s2-1" {
		t.Fatalf("merged source metadata = %+v", p)
	}
	if p.Abstract != "a much longer abstract" {
		t.Fatalf("abstract = %q", p.Abstract)
	}
}

func TestMergePapersDedupesByPMID(t *testing.T) {
	out := MergePapers([]SourceResult{
		{Source: SourcePubMed, Papers: []Paper{{Source: SourcePubMed, SourcePaperID: "123", PMID: "123", Title: "A"}}},
		{Source: SourceSemanticScholar, Papers: []Paper{{Source: SourceSemanticScholar, SourcePaperID: "s2", PMID: "123", Title: "A"}}},
	}, []SourceID{SourcePubMed, SourceSemanticScholar}, 10)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
}

func TestMergePapersDedupesByNormalizedTitleAndPreservesSourceOrder(t *testing.T) {
	out := MergePapers([]SourceResult{
		{Source: SourceSemanticScholar, Papers: []Paper{{Source: SourceSemanticScholar, SourcePaperID: "s2", Title: "Cell Fate Control!"}}},
		{Source: SourcePubMed, Papers: []Paper{{Source: SourcePubMed, SourcePaperID: "pmid", Title: "cell fate control"}}},
	}, []SourceID{SourcePubMed, SourceSemanticScholar}, 10)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	if out[0].SourcePaperIDs[SourcePubMed] != "pmid" || out[0].SourcePaperIDs[SourceSemanticScholar] != "s2" {
		t.Fatalf("source ids = %+v", out[0].SourcePaperIDs)
	}
}

func TestMergePapersTruncatesAfterMerge(t *testing.T) {
	out := MergePapers([]SourceResult{
		{Source: SourcePubMed, Papers: []Paper{
			{Source: SourcePubMed, SourcePaperID: "1", PMID: "1", Title: "One"},
			{Source: SourcePubMed, SourcePaperID: "2", PMID: "2", Title: "Two"},
		}},
	}, []SourceID{SourcePubMed}, 1)
	if len(out) != 1 || out[0].PMID != "1" {
		t.Fatalf("out = %+v", out)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/service/ai_external -run TestMergePapers -v
```

Expected: FAIL because package does not exist.

- [ ] **Step 3: Add AI external types**

Create `internal/service/ai_external/types.go`:

```go
package ai_external

import "context"

type SourceID string

const (
	SourcePubMed          SourceID = "pubmed"
	SourceSemanticScholar SourceID = "semantic_scholar"
)

type Paper struct {
	Source         SourceID
	SourcePaperID  string
	Sources        []SourceID
	SourcePaperIDs map[SourceID]string
	PMID           string
	PMCID          string
	DOI            string
	ArXiv          string
	Title          string
	Abstract       string
	TLDR           string
	Venue          string
	Year           int
	Authors        []string
	URL            string
	OpenAccessURL  string
	CitationCount  int
	MatchedQuery   string
}

type SearchOptions struct {
	Limit int
}

type Searcher interface {
	Search(ctx context.Context, query string, opts SearchOptions) ([]Paper, error)
}

type SourceResult struct {
	Source SourceID
	Query  string
	Papers []Paper
	Err    error
}

type SourceQueries map[SourceID][]string
```

- [ ] **Step 4: Add merge implementation**

Create `internal/service/ai_external/merge.go`:

```go
package ai_external

import (
	"regexp"
	"strings"
)

var nonTitleWordRe = regexp.MustCompile(`[^a-z0-9]+`)

func MergePapers(results []SourceResult, sourceOrder []SourceID, limit int) []Paper {
	if limit <= 0 {
		limit = 20
	}
	bySource := map[SourceID][]Paper{}
	for _, result := range results {
		if result.Err != nil {
			continue
		}
		for _, paper := range result.Papers {
			paper.Source = result.Source
			if paper.MatchedQuery == "" {
				paper.MatchedQuery = result.Query
			}
			bySource[result.Source] = append(bySource[result.Source], paper)
		}
	}
	seen := map[string]int{}
	out := make([]Paper, 0, limit)
	addOrMerge := func(p Paper) {
		keys := paperKeys(p)
		for _, key := range keys {
			if idx, ok := seen[key]; ok {
				out[idx] = mergePaper(out[idx], p)
				for _, nextKey := range keys {
					seen[nextKey] = idx
				}
				return
			}
		}
		p = ensureSourceMetadata(p)
		out = append(out, p)
		idx := len(out) - 1
		for _, key := range keys {
			seen[key] = idx
		}
	}
	maxRows := 0
	for _, source := range sourceOrder {
		if len(bySource[source]) > maxRows {
			maxRows = len(bySource[source])
		}
	}
	for row := 0; row < maxRows; row++ {
		for _, source := range sourceOrder {
			papers := bySource[source]
			if row >= len(papers) {
				continue
			}
			addOrMerge(papers[row])
		}
	}
	if len(out) > limit {
		return out[:limit]
	}
	return out
}

func mergePaper(a, b Paper) Paper {
	a = ensureSourceMetadata(a)
	b = ensureSourceMetadata(b)
	for _, source := range b.Sources {
		if !hasSource(a.Sources, source) {
			a.Sources = append(a.Sources, source)
		}
	}
	for source, id := range b.SourcePaperIDs {
		if id != "" {
			a.SourcePaperIDs[source] = id
		}
	}
	if a.PMID == "" {
		a.PMID = b.PMID
	}
	if a.PMCID == "" {
		a.PMCID = b.PMCID
	}
	if a.DOI == "" {
		a.DOI = b.DOI
	}
	if a.ArXiv == "" {
		a.ArXiv = b.ArXiv
	}
	if a.Title == "" {
		a.Title = b.Title
	}
	if len([]rune(b.Abstract)) > len([]rune(a.Abstract)) {
		a.Abstract = b.Abstract
	}
	if a.TLDR == "" {
		a.TLDR = b.TLDR
	}
	if a.Venue == "" {
		a.Venue = b.Venue
	}
	if a.Year == 0 {
		a.Year = b.Year
	}
	if len(a.Authors) == 0 {
		a.Authors = b.Authors
	}
	if a.URL == "" {
		a.URL = b.URL
	}
	if a.OpenAccessURL == "" {
		a.OpenAccessURL = b.OpenAccessURL
	}
	if a.CitationCount == 0 {
		a.CitationCount = b.CitationCount
	}
	return a
}

func ensureSourceMetadata(p Paper) Paper {
	if len(p.Sources) == 0 && p.Source != "" {
		p.Sources = []SourceID{p.Source}
	}
	if p.SourcePaperIDs == nil {
		p.SourcePaperIDs = map[SourceID]string{}
	}
	if p.Source != "" && p.SourcePaperID != "" {
		p.SourcePaperIDs[p.Source] = p.SourcePaperID
	}
	return p
}

func paperKeys(p Paper) []string {
	keys := make([]string, 0, 3)
	if doi := normalizeDOI(p.DOI); doi != "" {
		keys = append(keys, "doi:"+doi)
	}
	if pmid := strings.TrimSpace(p.PMID); pmid != "" {
		keys = append(keys, "pmid:"+pmid)
	}
	if title := normalizeTitle(p.Title); title != "" {
		keys = append(keys, "title:"+title)
	}
	return keys
}

func normalizeDOI(doi string) string {
	doi = strings.TrimSpace(strings.ToLower(doi))
	doi = strings.TrimPrefix(doi, "https://doi.org/")
	doi = strings.TrimPrefix(doi, "http://doi.org/")
	doi = strings.TrimPrefix(doi, "doi:")
	return strings.TrimSpace(doi)
}

func normalizeTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	title = nonTitleWordRe.ReplaceAllString(title, " ")
	return strings.Join(strings.Fields(title), " ")
}

func hasSource(sources []SourceID, source SourceID) bool {
	for _, existing := range sources {
		if existing == source {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Run tests**

Run:

```bash
gofmt -w internal/service/ai_external
go test ./internal/service/ai_external -run TestMergePapers -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/service/ai_external
git commit -m "Add AI external merge types"
```

---

## Task 5: Add AI External Service And Adapters

**Files:**

- Create: `internal/service/ai_external/service.go`
- Create: `internal/service/ai_external/service_test.go`
- Create: `internal/service/ai_external/s2_adapter.go`
- Create: `internal/service/ai_external/pubmed_adapter.go`

- [ ] **Step 1: Write failing service tests**

Create `internal/service/ai_external/service_test.go`:

```go
package ai_external

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubSettings struct {
	sources []SourceID
}

func (s stubSettings) EnabledExternalSources(ctx context.Context) ([]SourceID, error) {
	return s.sources, nil
}

type stubSearcher struct {
	papers  map[string][]Paper
	err     error
	queries []string
}

func (s *stubSearcher) Search(ctx context.Context, query string, opts SearchOptions) ([]Paper, error) {
	s.queries = append(s.queries, query)
	if s.err != nil {
		return nil, s.err
	}
	return s.papers[query], nil
}

func TestServiceSearchUsesEnabledSourcesAndMergesResults(t *testing.T) {
	pubmed := &stubSearcher{papers: map[string][]Paper{"pub query": {{SourcePaperID: "pmid-1", PMID: "1", DOI: "10.1/a", Title: "A"}}}}
	s2 := &stubSearcher{papers: map[string][]Paper{"s2 query": {{SourcePaperID: "s2-1", DOI: "10.1/a", Title: "A", Abstract: "long abstract"}}}}
	svc := NewService(stubSettings{sources: []SourceID{SourcePubMed, SourceSemanticScholar}}, map[SourceID]Searcher{
		SourcePubMed:          pubmed,
		SourceSemanticScholar: s2,
	})

	res, err := svc.Search(context.Background(), SourceQueries{
		SourcePubMed:          {"pub query"},
		SourceSemanticScholar: {"s2 query"},
	}, SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(res.Papers) != 1 {
		t.Fatalf("papers = %+v", res.Papers)
	}
	if len(res.Failures) != 0 {
		t.Fatalf("failures = %+v", res.Failures)
	}
	if got := strings.Join([]string{pubmed.queries[0], s2.queries[0]}, "|"); got != "pub query|s2 query" {
		t.Fatalf("queries = %s", got)
	}
}

func TestServiceSearchKeepsPartialSuccess(t *testing.T) {
	pubmed := &stubSearcher{papers: map[string][]Paper{"pub query": {{SourcePaperID: "pmid-1", PMID: "1", Title: "A"}}}}
	s2 := &stubSearcher{err: errors.New("s2 rate limited")}
	svc := NewService(stubSettings{sources: []SourceID{SourcePubMed, SourceSemanticScholar}}, map[SourceID]Searcher{
		SourcePubMed:          pubmed,
		SourceSemanticScholar: s2,
	})

	res, err := svc.Search(context.Background(), SourceQueries{
		SourcePubMed:          {"pub query"},
		SourceSemanticScholar: {"s2 query"},
	}, SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(res.Papers) != 1 || len(res.Failures) != 1 || res.Failures[0].Source != SourceSemanticScholar {
		t.Fatalf("res = %+v", res)
	}
}

func TestServiceSearchReturnsErrorWhenNoSourcesEnabled(t *testing.T) {
	svc := NewService(stubSettings{sources: []SourceID{}}, nil)
	_, err := svc.Search(context.Background(), SourceQueries{}, SearchOptions{Limit: 5})
	if !errors.Is(err, ErrNoSourcesEnabled) {
		t.Fatalf("err = %v, want ErrNoSourcesEnabled", err)
	}
}

func TestServiceSearchReturnsErrorWhenAllSourcesFail(t *testing.T) {
	svc := NewService(stubSettings{sources: []SourceID{SourcePubMed}}, map[SourceID]Searcher{
		SourcePubMed: &stubSearcher{err: errors.New("down")},
	})
	_, err := svc.Search(context.Background(), SourceQueries{SourcePubMed: {"pub query"}}, SearchOptions{Limit: 5})
	if err == nil || !strings.Contains(err.Error(), "down") {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/service/ai_external -run 'TestServiceSearch' -v
```

Expected: FAIL because `NewService`, `ErrNoSourcesEnabled`, and service result types do not exist.

- [ ] **Step 3: Add service orchestration**

Create `internal/service/ai_external/service.go`:

```go
package ai_external

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var ErrNoSourcesEnabled = errors.New("ai_external: no external search sources enabled")

type SettingsProvider interface {
	EnabledExternalSources(ctx context.Context) ([]SourceID, error)
}

type SourceFailure struct {
	Source SourceID
	Err    error
}

type SearchResult struct {
	Papers   []Paper
	Results  []SourceResult
	Failures []SourceFailure
	Sources  []SourceID
}

type Service struct {
	settings  SettingsProvider
	searchers map[SourceID]Searcher
}

func NewService(settings SettingsProvider, searchers map[SourceID]Searcher) *Service {
	return &Service{settings: settings, searchers: searchers}
}

func (s *Service) Search(ctx context.Context, queries SourceQueries, opts SearchOptions) (SearchResult, error) {
	sources, err := s.enabledSources(ctx)
	if err != nil {
		return SearchResult{}, err
	}
	if len(sources) == 0 {
		return SearchResult{}, ErrNoSourcesEnabled
	}
	results := s.searchSources(ctx, sources, queries, opts)
	failures := make([]SourceFailure, 0)
	successes := 0
	for _, result := range results {
		if result.Err != nil {
			failures = append(failures, SourceFailure{Source: result.Source, Err: result.Err})
			continue
		}
		successes++
	}
	if successes == 0 && len(failures) > 0 {
		return SearchResult{Results: results, Failures: failures, Sources: sources}, combineFailures(failures)
	}
	return SearchResult{
		Papers:   MergePapers(results, sources, opts.Limit),
		Results:  results,
		Failures: failures,
		Sources:  sources,
	}, nil
}

func (s *Service) enabledSources(ctx context.Context) ([]SourceID, error) {
	if s == nil || s.settings == nil {
		return []SourceID{SourcePubMed}, nil
	}
	return s.settings.EnabledExternalSources(ctx)
}

func (s *Service) searchSources(ctx context.Context, sources []SourceID, queries SourceQueries, opts SearchOptions) []SourceResult {
	out := make(chan SourceResult, len(sources)*4)
	var wg sync.WaitGroup
	for _, source := range sources {
		searcher := s.searchers[source]
		if searcher == nil {
			out <- SourceResult{Source: source, Err: fmt.Errorf("source %s is not configured", source)}
			continue
		}
		sourceQueries := queries[source]
		if len(sourceQueries) == 0 {
			out <- SourceResult{Source: source, Err: fmt.Errorf("source %s has no queries", source)}
			continue
		}
		for _, query := range sourceQueries {
			query := strings.TrimSpace(query)
			if query == "" {
				continue
			}
			wg.Add(1)
			go func(source SourceID, query string, searcher Searcher) {
				defer wg.Done()
				papers, err := searcher.Search(ctx, query, opts)
				out <- SourceResult{Source: source, Query: query, Papers: papers, Err: err}
			}(source, query, searcher)
		}
	}
	wg.Wait()
	close(out)
	results := make([]SourceResult, 0)
	for result := range out {
		results = append(results, result)
	}
	return results
}

func combineFailures(failures []SourceFailure) error {
	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		if failure.Err != nil {
			parts = append(parts, fmt.Sprintf("%s: %s", failure.Source, failure.Err.Error()))
		}
	}
	return errors.New(strings.Join(parts, "; "))
}
```

- [ ] **Step 4: Add Semantic Scholar adapter**

Create `internal/service/ai_external/s2_adapter.go`:

```go
package ai_external

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/service/research"
)

type SemanticScholarSearcher interface {
	Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error)
}

type SemanticScholarAdapter struct {
	SearchService SemanticScholarSearcher
}

func (a SemanticScholarAdapter) Search(ctx context.Context, query string, opts SearchOptions) ([]Paper, error) {
	list, err := a.SearchService.Search(ctx, query, research.SearchOpts{Limit: opts.Limit})
	if err != nil {
		return nil, err
	}
	out := make([]Paper, 0, len(list.Items))
	for _, p := range list.Items {
		authors := make([]string, 0, len(p.Authors))
		for _, author := range p.Authors {
			if author.Name != "" {
				authors = append(authors, author.Name)
			}
		}
		out = append(out, Paper{
			Source:        SourceSemanticScholar,
			SourcePaperID: p.PaperID,
			DOI:           p.ExternalIDs.DOI,
			ArXiv:         p.ExternalIDs.ArXiv,
			PMID:          p.ExternalIDs.PubMed,
			Title:         p.Title,
			Abstract:      p.Abstract,
			TLDR:          p.TLDR,
			Venue:         p.Venue,
			Year:          p.Year,
			Authors:       authors,
			OpenAccessURL: p.OpenAccessPDFURL,
			CitationCount: p.CitationCount,
		})
	}
	return out, nil
}
```

- [ ] **Step 5: Add PubMed adapter**

Create `internal/service/ai_external/pubmed_adapter.go`:

```go
package ai_external

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/service/pubmed"
)

type PubMedSearcher interface {
	Search(ctx context.Context, query string, opts pubmed.SearchOptions) (pubmed.SearchResult, error)
}

type PubMedAdapter struct {
	Client PubMedSearcher
}

func (a PubMedAdapter) Search(ctx context.Context, query string, opts SearchOptions) ([]Paper, error) {
	res, err := a.Client.Search(ctx, query, pubmed.SearchOptions{Limit: opts.Limit})
	if err != nil {
		return nil, err
	}
	out := make([]Paper, 0, len(res.Items))
	for _, p := range res.Items {
		out = append(out, Paper{
			Source:        SourcePubMed,
			SourcePaperID: p.PMID,
			PMID:          p.PMID,
			PMCID:         p.PMCID,
			DOI:           p.DOI,
			Title:         p.Title,
			Abstract:      p.Abstract,
			Venue:         p.Journal,
			Year:          p.Year,
			Authors:       p.Authors,
			URL:           p.URL,
		})
	}
	return out, nil
}
```

- [ ] **Step 6: Run tests**

Run:

```bash
gofmt -w internal/service/ai_external
go test ./internal/service/ai_external -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/service/ai_external
git commit -m "Add AI external search orchestration"
```

---

## Task 6: Update Planner To Source-Specific Queries

**Files:**

- Modify: `internal/service/ai_assistant/external_search_tool.go`
- Modify: `internal/service/ai_assistant/library_agents.go`
- Test: `internal/service/ai_assistant/tools_test.go`

- [ ] **Step 1: Write failing planner tests**

Append to `internal/service/ai_assistant/tools_test.go`:

```go
func TestExternalPlannerReturnsQueriesBySource(t *testing.T) {
	plan := ExternalSearchPlan{
		QueriesBySource: map[string][]string{
			"pubmed":           {"pub query"},
			"semantic_scholar": {"s2 query"},
		},
		Rationale: "source-specific",
	}
	got := plan.QueriesForSource("pubmed", []string{"fallback"})
	if len(got) != 1 || got[0] != "pub query" {
		t.Fatalf("pubmed queries = %#v", got)
	}
	got = plan.QueriesForSource("semantic_scholar", []string{"fallback"})
	if len(got) != 1 || got[0] != "s2 query" {
		t.Fatalf("s2 queries = %#v", got)
	}
	got = plan.QueriesForSource("missing", []string{"fallback"})
	if len(got) != 1 || got[0] != "fallback" {
		t.Fatalf("missing queries = %#v", got)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/service/ai_assistant -run TestExternalPlannerReturnsQueriesBySource -v
```

Expected: FAIL because `QueriesBySource` and `QueriesForSource` do not exist.

- [ ] **Step 3: Extend `ExternalSearchPlan`**

In `internal/service/ai_assistant/external_search_tool.go`, replace `ExternalSearchPlan` with:

```go
type ExternalSearchPlan struct {
	SearchQuery     string              `json:"search_query,omitempty"`
	SearchQueries   []string            `json:"search_queries,omitempty"`
	QueriesBySource map[string][]string `json:"queries_by_source,omitempty"`
	Rationale       string              `json:"rationale,omitempty"`
}

func (p ExternalSearchPlan) QueriesForSource(source string, fallback []string) []string {
	if len(p.QueriesBySource) > 0 {
		if queries := sanitizeExternalQueries(p.QueriesBySource[source]); len(queries) > 0 {
			return queries
		}
	}
	if len(p.SearchQueries) > 0 || p.SearchQuery != "" {
		return mergeExternalQueries(append([]string{p.SearchQuery}, p.SearchQueries...), fallback)
	}
	return fallback
}
```

- [ ] **Step 4: Update LLM planner normalization**

In `internal/service/ai_assistant/library_agents.go`, update `PlanExternalSearch` after JSON decode:

```go
fallback := ExternalSearchQueries(query)
plan.SearchQuery = strings.Join(strings.Fields(plan.SearchQuery), " ")
plan.SearchQueries = sanitizeExternalQueries(append([]string{plan.SearchQuery}, plan.SearchQueries...))
if len(plan.SearchQueries) > 0 {
	plan.SearchQuery = plan.SearchQueries[0]
}
normalizedBySource := map[string][]string{}
for source, queries := range plan.QueriesBySource {
	clean := sanitizeExternalQueries(queries)
	if len(clean) > 0 {
		normalizedBySource[strings.TrimSpace(strings.ToLower(source))] = clean
	}
}
plan.QueriesBySource = normalizedBySource
plan.Rationale = strings.TrimSpace(plan.Rationale)
if len(plan.SearchQueries) == 0 && len(plan.QueriesBySource) == 0 {
	plan.SearchQueries = fallback
	if len(plan.SearchQueries) > 0 {
		plan.SearchQuery = plan.SearchQueries[0]
	}
}
if len(plan.SearchQueries) == 0 && len(plan.QueriesBySource) == 0 {
	return ExternalSearchPlan{}, fmt.Errorf("empty external search query")
}
return plan, nil
```

- [ ] **Step 5: Update external planner prompt**

Replace `externalPlannerSystemPrompt` in `internal/service/ai_assistant/library_agents.go` with:

```go
const externalPlannerSystemPrompt = `你是 CiteBox AI 助手的 Master Agent。你的任务是把用户的外部调研或出处查找请求改写成适合不同学术搜索源的英文检索式。
只输出 JSON，不要输出 Markdown，不要解释思考过程。
JSON 格式：
{"queries_by_source":{"pubmed":["query 1","query 2"],"semantic_scholar":["query 1","query 2"]},"rationale":"一句话说明检索式如何覆盖用户需求"}
规则：
- 只为系统启用的搜索源输出 queries；如果用户提示里没有启用源列表，则可以同时输出 pubmed 和 semantic_scholar。
- 用户可能使用任意语言提问；理解其真实学术需求，并改写为简短英文关键词检索式。
- PubMed 查询应偏向生物医学术语、疾病、基因、方法、MeSH 风格短语和必要的 Boolean 组合。
- Semantic Scholar 查询应偏向更宽泛的自然语言学术关键词，适合跨学科召回。
- 保留用户明确给出的技术名词、实验类型、测序类型、物种、疾病和缩写，例如 ChIP-seq、ATAC-seq、single-cell RNA-seq。
- 用户要求找出处、引用或证据时，检索核心断言本身，不要保留 source、citation、reference、出处、引用等操作词。
- 不要加入 article、paper、literature、data、search、find、about、external 等泛词。
- 每个搜索源生成 2 到 4 个互补查询：第一个高精度，后续用于召回同义表达；不要把所有限定词塞进同一个长查询。
- 每个查询优先输出 3 到 8 个关键词；必要时加入一个能提高召回的同义技术短语。`
```

- [ ] **Step 6: Run planner tests**

Run:

```bash
gofmt -w internal/service/ai_assistant/external_search_tool.go internal/service/ai_assistant/library_agents.go internal/service/ai_assistant/tools_test.go
go test ./internal/service/ai_assistant -run 'TestExternalPlannerReturnsQueriesBySource|TestExternalSearch' -v
```

Expected: PASS after updating existing assertions that still assume a single Semantic Scholar query.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/service/ai_assistant/external_search_tool.go internal/service/ai_assistant/library_agents.go internal/service/ai_assistant/tools_test.go
git commit -m "Add source-specific external search planning"
```

---

## Task 7: Wire AI Assistant ExternalSearchTool To ai_external

**Files:**

- Modify: `internal/service/ai_assistant/external_search_tool.go`
- Modify: `internal/service/ai_assistant/types.go`
- Test: `internal/service/ai_assistant/tools_test.go`

- [ ] **Step 1: Write failing tool test for multiple sources**

Append to `internal/service/ai_assistant/tools_test.go`:

```go
func TestExternalSearchToolIncludesSourceMetadata(t *testing.T) {
	searcher := &stubAIExternalSearch{
		result: ai_external.SearchResult{
			Sources: []ai_external.SourceID{ai_external.SourcePubMed},
			Papers: []ai_external.Paper{{
				Sources:        []ai_external.SourceID{ai_external.SourcePubMed},
				SourcePaperIDs: map[ai_external.SourceID]string{ai_external.SourcePubMed: "12345"},
				SourcePaperID:  "12345",
				PMID:           "12345",
				DOI:            "10.1/a",
				Title:          "PubMed Hit",
				Abstract:       "PubMed abstract evidence.",
				URL:            "https://pubmed.ncbi.nlm.nih.gov/12345/",
			}},
		},
	}
	tool := NewExternalSearchToolWithAgents(searcher, nil, nil)
	res, err := tool.Run(context.Background(), ToolInput{Query: "cell fate", Limit: 5})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(res.Cards) != 1 {
		t.Fatalf("cards = %+v", res.Cards)
	}
	payload := res.Cards[0].Payload.(ExternalPaperCard)
	if payload.PMID != "12345" || payload.URL == "" || len(payload.Sources) != 1 || payload.Sources[0] != "PubMed" {
		t.Fatalf("payload = %+v", payload)
	}
	if res.Citations[0].Source != "external:PubMed" {
		t.Fatalf("citation source = %q", res.Citations[0].Source)
	}
	if !strings.Contains(res.Process.Note, "PubMed") {
		t.Fatalf("note = %q", res.Process.Note)
	}
}
```

Add the stub near existing stubs:

```go
type stubAIExternalSearch struct {
	result ai_external.SearchResult
	err    error
	inputs []ai_external.SourceQueries
}

func (s *stubAIExternalSearch) Search(ctx context.Context, queries ai_external.SourceQueries, opts ai_external.SearchOptions) (ai_external.SearchResult, error) {
	s.inputs = append(s.inputs, queries)
	return s.result, s.err
}
```

Add import:

```go
"github.com/xuzhougeng/citebox/internal/service/ai_external"
```

- [ ] **Step 2: Run test and verify failure**

Run:

```bash
go test ./internal/service/ai_assistant -run TestExternalSearchToolIncludesSourceMetadata -v
```

Expected: FAIL because the tool still expects the old `ExternalSearcher`.

- [ ] **Step 3: Replace searcher interface**

In `internal/service/ai_assistant/external_search_tool.go`, replace `ExternalSearcher` with:

```go
type ExternalSearcher interface {
	Search(ctx context.Context, queries ai_external.SourceQueries, opts ai_external.SearchOptions) (ai_external.SearchResult, error)
}
```

Add import:

```go
"github.com/xuzhougeng/citebox/internal/service/ai_external"
```

- [ ] **Step 4: Extend external paper card**

Replace `ExternalPaperCard` with:

```go
type ExternalPaperCard struct {
	S2PaperID           string                       `json:"s2_paper_id,omitempty"`
	SourceIDs           map[string]string            `json:"source_ids,omitempty"`
	Sources             []string                     `json:"sources,omitempty"`
	PMID                string                       `json:"pmid,omitempty"`
	PMCID               string                       `json:"pmcid,omitempty"`
	URL                 string                       `json:"url,omitempty"`
	Title               string                       `json:"title"`
	Year                int                          `json:"year,omitempty"`
	Venue               string                       `json:"venue,omitempty"`
	DOI                 string                       `json:"doi,omitempty"`
	TLDR                string                       `json:"tldr,omitempty"`
	Abstract            string                       `json:"abstract,omitempty"`
	MatchedQuery        string                       `json:"matched_query,omitempty"`
	Reason              string                       `json:"reason,omitempty"`
	CitationIndex       int                          `json:"citation_index,omitempty"`
	HighlightTerms      []string                     `json:"highlight_terms,omitempty"`
	EvidenceAnnotations []ExternalEvidenceAnnotation `json:"evidence_annotations,omitempty"`
}
```

- [ ] **Step 5: Update `Run` data flow**

In `Run`, build source queries from the plan:

```go
fallback := ExternalSearchQueries(in.Query)
plan := ExternalSearchPlan{}
var planErr error
if t != nil && t.planner != nil {
	plan, planErr = t.planner.PlanExternalSearch(ctx, in.Query)
}
queries := ai_external.SourceQueries{}
for _, source := range []ai_external.SourceID{ai_external.SourcePubMed, ai_external.SourceSemanticScholar} {
	queries[source] = plan.QueriesForSource(string(source), fallback)
}
```

Replace `t.searchMany(...)` with:

```go
searchRes, searchErr := t.searcher.Search(ctx, queries, ai_external.SearchOptions{Limit: limit})
if searchErr != nil && len(searchRes.Papers) == 0 {
	return externalSearchFailedResult(inputJSON, searchErr, processStages, flattenSourceQueries(queries)), nil
}
candidates := externalCandidatesFromPapers(searchRes.Papers)
rawReturned := len(candidates)
```

Add helpers:

```go
func externalCandidatesFromPapers(papers []ai_external.Paper) []externalSearchCandidate {
	out := make([]externalSearchCandidate, 0, len(papers))
	for _, p := range papers {
		out = append(out, externalSearchCandidate{Paper: p, MatchedQuery: p.MatchedQuery})
	}
	return out
}

func flattenSourceQueries(queries ai_external.SourceQueries) []string {
	out := make([]string, 0)
	for _, source := range []ai_external.SourceID{ai_external.SourcePubMed, ai_external.SourceSemanticScholar} {
		out = append(out, queries[source]...)
	}
	return sanitizeExternalQueries(out)
}
```

Change `externalSearchCandidate.Paper` to `ai_external.Paper`.

- [ ] **Step 6: Update classifier input conversion**

Keep `ExternalPaperClassificationInput.Paper` as `research.Paper` for the classifier prompt, and add a converter:

```go
func researchPaperFromExternal(p ai_external.Paper) research.Paper {
	authors := make([]research.Author, 0, len(p.Authors))
	for _, name := range p.Authors {
		authors = append(authors, research.Author{Name: name})
	}
	return research.Paper{
		PaperID:  p.SourcePaperID,
		Title:    p.Title,
		Abstract: p.Abstract,
		TLDR:     p.TLDR,
		Year:     p.Year,
		Venue:    p.Venue,
		Authors:  authors,
		ExternalIDs: research.IDs{
			DOI:    p.DOI,
			ArXiv:  p.ArXiv,
			PubMed: p.PMID,
		},
		OpenAccessPDFURL: p.OpenAccessURL,
		CitationCount:    p.CitationCount,
	}
}
```

In `classifyCandidates`, call:

```go
rp := researchPaperFromExternal(cand.Paper)
if res, ok := classifyExternalPaperHeuristic(query, rp); ok {
	out <- result{index: index, ok: true, res: res}
	return
}
res, err := t.classifier.ClassifyExternalPaper(ctx, ExternalPaperClassificationInput{
	Query:         query,
	SearchQueries: searchQueries,
	MatchedQuery:  cand.MatchedQuery,
	Paper:         rp,
	EvidenceText:  externalEvidenceText(rp),
})
```

- [ ] **Step 7: Update citations and cards**

In the card loop, use:

```go
sourceLabels := externalSourceLabels(p.Sources)
citation := Citation{
	I:          len(citations) + 1,
	S2PaperID:  p.SourcePaperIDs[ai_external.SourceSemanticScholar],
	ExternalID: externalIDFromAIPaper(p),
	Title:      p.Title,
	Source:     "external:" + strings.Join(sourceLabels, "+"),
	Snippet: research.Snippet{
		Text:        firstNonEmpty(p.Abstract, p.TLDR, p.Title),
		SnippetKind: "abstract",
		Section:     "外部学术搜索: " + strings.Join(sourceLabels, " + "),
	},
}
```

Add helpers:

```go
func externalSourceLabels(sources []ai_external.SourceID) []string {
	labels := make([]string, 0, len(sources))
	for _, source := range sources {
		switch source {
		case ai_external.SourcePubMed:
			labels = append(labels, "PubMed")
		case ai_external.SourceSemanticScholar:
			labels = append(labels, "Semantic Scholar")
		default:
			labels = append(labels, string(source))
		}
	}
	return labels
}

func externalIDFromAIPaper(p ai_external.Paper) string {
	if p.DOI != "" {
		return "DOI:" + p.DOI
	}
	if p.PMID != "" {
		return "PMID:" + p.PMID
	}
	if p.ArXiv != "" {
		return "ArXiv:" + p.ArXiv
	}
	return string(p.Source) + ":" + p.SourcePaperID
}
```

- [ ] **Step 8: Remove old `searchMany` path**

Delete the old `searchMany` method that calls `research.Service.Search` directly. Keep query helper functions such as `ExternalSearchQueries`, `sanitizeExternalQueries`, and classification helpers.

- [ ] **Step 9: Run tests**

Run:

```bash
gofmt -w internal/service/ai_assistant
go test ./internal/service/ai_assistant -run TestExternalSearch -v
go test ./internal/service/ai_assistant
```

Expected: PASS after updating old tests that still use `stubExternalSearch` to use `stubAIExternalSearch`.

- [ ] **Step 10: Commit**

Run:

```bash
git add internal/service/ai_assistant
git commit -m "Wire AI assistant to external source service"
```

---

## Task 8: Wire AI Conversation External Evidence Through ai_external

**Files:**

- Modify: `internal/service/ai_conversation/evidence.go`
- Modify: `internal/service/ai_conversation/service.go`
- Modify: `internal/service/ai_conversation/evidence_test.go`

- [ ] **Step 1: Write failing evidence test**

Append to `internal/service/ai_conversation/evidence_test.go`:

```go
func TestExternalEvidenceUsesConfiguredSourceLabels(t *testing.T) {
	searcher := &stubAIExternalEvidenceSearcher{
		result: ai_external.SearchResult{
			Sources: []ai_external.SourceID{ai_external.SourcePubMed},
			Papers: []ai_external.Paper{{
				Source:        ai_external.SourcePubMed,
				SourcePaperID: "12345",
				Sources:       []ai_external.SourceID{ai_external.SourcePubMed},
				PMID:          "12345",
				Title:         "PubMed Evidence",
				Abstract:      "PubMed evidence text.",
			}},
		},
	}
	prompt, citations, err := injectEvidence(context.Background(), nil, searcher, "find source", nil, EvidenceOptions{IncludeExternal: true, DisableLocal: true})
	if err != nil {
		t.Fatalf("injectEvidence() error = %v", err)
	}
	if len(citations) != 1 || citations[0].Source != "external:PubMed" {
		t.Fatalf("citations = %+v", citations)
	}
	if !strings.Contains(prompt, "外部学术搜索") || !strings.Contains(prompt, "PubMed") {
		t.Fatalf("prompt = %s", prompt)
	}
}
```

Add stub:

```go
type stubAIExternalEvidenceSearcher struct {
	result ai_external.SearchResult
	err    error
}

func (s *stubAIExternalEvidenceSearcher) Search(ctx context.Context, queries ai_external.SourceQueries, opts ai_external.SearchOptions) (ai_external.SearchResult, error) {
	return s.result, s.err
}
```

Add import:

```go
"github.com/xuzhougeng/citebox/internal/service/ai_external"
```

- [ ] **Step 2: Run test and verify failure**

Run:

```bash
go test ./internal/service/ai_conversation -run TestExternalEvidenceUsesConfiguredSourceLabels -v
```

Expected: FAIL because `injectEvidence` expects the old Semantic Scholar searcher.

- [ ] **Step 3: Replace external evidence interface**

In `internal/service/ai_conversation/evidence.go`, replace `ExternalEvidenceSearcher` with:

```go
type ExternalEvidenceSearcher interface {
	Search(ctx context.Context, queries ai_external.SourceQueries, opts ai_external.SearchOptions) (ai_external.SearchResult, error)
}
```

Add import:

```go
"github.com/xuzhougeng/citebox/internal/service/ai_external"
```

- [ ] **Step 4: Update external evidence search**

Do not import `ai_assistant` into `ai_conversation`; that would create a package cycle. Add a small local fallback query helper and replace `externalEvidence`, `citationsFromSnippetMatches`, and `externalPaperSearchEvidence` with a single `externalEvidence` implementation:

```go
func externalEvidence(ctx context.Context, searcher ExternalEvidenceSearcher, userText string, pinned []repository.AIPinnedPaper, limit int) ([]Citation, error) {
	if searcher == nil || limit <= 0 {
		return nil, nil
	}
	fallbackQueries := externalEvidenceQueries(userText)
	queries := ai_external.SourceQueries{
		ai_external.SourcePubMed:          fallbackQueries,
		ai_external.SourceSemanticScholar: fallbackQueries,
	}
	res, err := searcher.Search(ctx, queries, ai_external.SearchOptions{Limit: limit})
	if err != nil && len(res.Papers) == 0 {
		return nil, err
	}
	out := make([]Citation, 0, len(res.Papers))
	for i, p := range res.Papers {
		snippet := evidenceSnippetFromAIPaper(p)
		if snippet.Text == "" {
			continue
		}
		out = append(out, Citation{
			ExternalID: externalIDForAIPaper(p),
			S2PaperID: p.SourcePaperIDs[ai_external.SourceSemanticScholar],
			Title:     p.Title,
			Source:    "external:" + strings.Join(aiExternalSourceLabels(p.Sources), "+"),
			Snippet:   snippet,
			Score:     0.7 - float64(i)*0.03,
		})
	}
	return out, nil
}
```

Add helper functions in the same file:

```go
func externalEvidenceQueries(userText string) []string {
	q := strings.TrimSpace(userText)
	if q == "" {
		return nil
	}
	return []string{q}
}

func evidenceSnippetFromAIPaper(p ai_external.Paper) research.Snippet {
	parts := make([]string, 0, 3)
	if p.Title != "" {
		parts = append(parts, "Title: "+p.Title)
	}
	kind := "title"
	if p.TLDR != "" {
		parts = append(parts, "TLDR: "+p.TLDR)
		kind = "tldr"
	}
	if p.Abstract != "" {
		parts = append(parts, "Abstract: "+p.Abstract)
		kind = "abstract"
	}
	text := normalizeEvidenceWhitespace(strings.Join(parts, " "))
	if len([]rune(text)) > 900 {
		text = string([]rune(text)[:900]) + "..."
	}
	return research.Snippet{
		Text:        text,
		SnippetKind: kind,
		Section:     "外部学术搜索: " + strings.Join(aiExternalSourceLabels(p.Sources), " + "),
	}
}

func externalIDForAIPaper(p ai_external.Paper) string {
	if p.DOI != "" {
		return "DOI:" + p.DOI
	}
	if p.PMID != "" {
		return "PMID:" + p.PMID
	}
	if p.ArXiv != "" {
		return "ArXiv:" + p.ArXiv
	}
	return string(p.Source) + ":" + p.SourcePaperID
}

func aiExternalSourceLabels(sources []ai_external.SourceID) []string {
	out := make([]string, 0, len(sources))
	for _, source := range sources {
		switch source {
		case ai_external.SourcePubMed:
			out = append(out, "PubMed")
		case ai_external.SourceSemanticScholar:
			out = append(out, "Semantic Scholar")
		default:
			out = append(out, string(source))
		}
	}
	return out
}
```

- [ ] **Step 5: Update prompt source labels**

In `buildEvidencePrompt`, change:

```go
sources = append(sources, "外部 Semantic Scholar 片段")
```

to:

```go
sources = append(sources, "外部学术搜索")
```

Change the per-citation label:

```go
if strings.HasPrefix(c.Source, evidenceSourceExternal) {
	source = strings.TrimPrefix(c.Source, "external:")
	if source == "" {
		source = "外部学术搜索"
	}
}
```

- [ ] **Step 6: Run tests**

Run:

```bash
gofmt -w internal/service/ai_conversation
go test ./internal/service/ai_conversation -run TestExternalEvidence -v
go test ./internal/service/ai_conversation
```

Expected: PASS after updating older test expectations that hardcode `Semantic Scholar`.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/service/ai_conversation
git commit -m "Use configured sources for AI external evidence"
```

---

## Task 9: Wire App Server And Client Lifecycle

**Files:**

- Modify: `internal/app/server.go`
- Modify: `internal/handler/settings.go`
- Test: `internal/app/server_test.go` if present, otherwise `go test ./internal/app`

- [ ] **Step 1: Add settings provider adapter**

In `internal/app/research_wire.go`, add:

```go
type aiExternalSettingsShim struct {
	svc *service.LibraryService
}

func (s aiExternalSettingsShim) EnabledExternalSources(ctx context.Context) ([]ai_external.SourceID, error) {
	settings, err := s.svc.GetAIExternalSearchSettings()
	if err != nil {
		return nil, err
	}
	out := make([]ai_external.SourceID, 0, len(settings.Sources))
	for _, source := range settings.Sources {
		out = append(out, ai_external.SourceID(source))
	}
	return out, nil
}
```

Add import:

```go
"github.com/xuzhougeng/citebox/internal/service/ai_external"
```

- [ ] **Step 2: Add PubMed client to server**

In `internal/app/server.go`, import:

```go
"github.com/xuzhougeng/citebox/internal/service/ai_external"
"github.com/xuzhougeng/citebox/internal/service/pubmed"
```

Add field to `Server`:

```go
pubmedClient *pubmed.Client
```

Construct the client in `NewServer`:

```go
pubMedAPIKey := strings.TrimSpace(cfg.PubMedAPIKey)
pubMedEmail := strings.TrimSpace(cfg.PubMedEmail)
pubMedTool := strings.TrimSpace(cfg.PubMedTool)
if settings, _ := librarySvc.GetAIExternalSearchSettings(); settings != nil {
	if pubMedAPIKey == "" {
		pubMedAPIKey = settings.PubMedAPIKey
	}
	if pubMedEmail == "" {
		pubMedEmail = settings.PubMedEmail
	}
	if pubMedTool == "" {
		pubMedTool = settings.PubMedTool
	}
}
pubmedClient := pubmed.NewClient(pubmed.Config{
	APIKey:      pubMedAPIKey,
	Email:       pubMedEmail,
	Tool:        pubMedTool,
	MinInterval: pubmed.RateInterval(pubMedAPIKey),
})
```

Pass it to `buildHandler`.

- [ ] **Step 3: Update `buildHandler` signature and wiring**

Add parameter:

```go
pubmedClient *pubmed.Client
```

Create `aiExternalSvc`:

```go
aiExternalSvc := ai_external.NewService(aiExternalSettingsShim{librarySvc}, map[ai_external.SourceID]ai_external.Searcher{
	ai_external.SourcePubMed:          ai_external.PubMedAdapter{Client: pubmedClient},
	ai_external.SourceSemanticScholar: ai_external.SemanticScholarAdapter{SearchService: researchSvc},
})
```

Wire:

```go
ExternalSearch: ai_assistant.NewExternalSearchToolWithAgents(aiExternalSvc, externalPlanner, externalClassifier),
```

and:

```go
aiConvService := ai_conversation.New(repo.AIConversation, repo.Paper, aiSvc, aiSvc, aiExternalSvc, logger.With("component", "ai_conversation"), assistantOrchestrator)
```

- [ ] **Step 4: Add handler hot reload hook**

In `SettingsHandler`, add field and setter:

```go
pubmedClient interface {
	SetSettings(apiKey, email, tool string)
}

func (h *SettingsHandler) SetPubMedClient(client interface{ SetSettings(apiKey, email, tool string) }) {
	h.pubmedClient = client
}
```

At the end of `PutAIExternalSearchSettings`, after save:

```go
if h.pubmedClient != nil {
	h.pubmedClient.SetSettings(settings.PubMedAPIKey, settings.PubMedEmail, settings.PubMedTool)
}
```

In `buildHandler`, call:

```go
settingsHandler.SetPubMedClient(pubmedClient)
```

- [ ] **Step 5: Close PubMed client**

In `Server.Close()`:

```go
if s.pubmedClient != nil {
	s.pubmedClient.Close()
}
```

Set `pubmedClient` in `server := &Server{...}`.

- [ ] **Step 6: Run tests**

Run:

```bash
gofmt -w internal/app/server.go internal/app/research_wire.go internal/handler/settings.go
go test ./internal/app ./internal/handler ./internal/service/ai_assistant ./internal/service/ai_conversation
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/app/server.go internal/app/research_wire.go internal/handler/settings.go
git commit -m "Wire configured AI external sources"
```

---

## Task 10: Add Settings UI And i18n

**Files:**

- Modify: `web/settings.html`
- Modify: `web/static/js/settings.js`
- Modify: `web/static/locales/zh-CN/settings.json`
- Modify: `web/static/locales/en/settings.json`

- [ ] **Step 1: Add HTML controls**

In `web/settings.html`, inside the AI settings section near assistant model binding, add:

```html
<div class="settings-form-section">
    <h3 data-i18n="settings.ai.external_search.title">AI 外部搜索源</h3>
    <p class="muted" data-i18n="settings.ai.external_search.desc">选择 AI 助手执行外部检索时使用的学术搜索源。可启用多个来源，AI 会按来源生成检索词并合并结果。</p>
    <div class="checkbox-list">
        <label class="field checkbox-field">
            <input id="aiExternalSourcePubMedInput" type="checkbox">
            <span data-i18n-html="settings.ai.external_search.pubmed"><strong>PubMed</strong><br>默认来源；支持匿名访问，也可配置 NCBI API key。</span>
        </label>
        <label class="field checkbox-field">
            <input id="aiExternalSourceS2Input" type="checkbox">
            <span data-i18n-html="settings.ai.external_search.s2"><strong>Semantic Scholar</strong><br>适合跨学科调研；未配置 API key 时容易遇到匿名限流。</span>
        </label>
    </div>
    <div class="form-grid">
        <label class="field">
            <span data-i18n="settings.ai.external_search.pubmed_api_key">PubMed / NCBI API key</span>
            <input id="pubmedAPIKeyInput" class="form-input" type="password" autocomplete="off">
        </label>
        <label class="field">
            <span data-i18n="settings.ai.external_search.pubmed_email">NCBI email</span>
            <input id="pubmedEmailInput" class="form-input" type="email" autocomplete="off">
        </label>
        <label class="field">
            <span data-i18n="settings.ai.external_search.pubmed_tool">NCBI tool</span>
            <input id="pubmedToolInput" class="form-input" type="text" placeholder="citebox">
        </label>
    </div>
    <div class="submit-row">
        <button id="saveAIExternalSearchSettingsButton" class="btn btn-secondary" type="button" data-i18n="settings.ai.external_search.save_btn">保存外部搜索源</button>
        <span id="aiExternalSearchSaveStatus" class="settings-inline-status"></span>
    </div>
</div>
```

- [ ] **Step 2: Bind controls in settings JS**

In `web/static/js/settings.js` `init()`, add:

```js
this.aiExternalSourcePubMedInput = document.getElementById('aiExternalSourcePubMedInput');
this.aiExternalSourceS2Input = document.getElementById('aiExternalSourceS2Input');
this.pubmedAPIKeyInput = document.getElementById('pubmedAPIKeyInput');
this.pubmedEmailInput = document.getElementById('pubmedEmailInput');
this.pubmedToolInput = document.getElementById('pubmedToolInput');
this.saveAIExternalSearchSettingsButton = document.getElementById('saveAIExternalSearchSettingsButton');
this.aiExternalSearchSaveStatus = document.getElementById('aiExternalSearchSaveStatus');
```

In `bindEvents()`, add:

```js
this.saveAIExternalSearchSettingsButton?.addEventListener('click', async () => {
    await this.saveAIExternalSearchSettings();
});
```

In `bootstrap()` or the same area that loads research settings, call:

```js
await this.loadAIExternalSearchSettings();
```

Add methods:

```js
async loadAIExternalSearchSettings() {
    const res = await fetch('/api/settings/ai-external-search');
    if (!res.ok) return;
    const data = await res.json();
    const sources = Array.isArray(data.sources) ? data.sources : ['pubmed'];
    if (this.aiExternalSourcePubMedInput) {
        this.aiExternalSourcePubMedInput.checked = sources.includes('pubmed');
    }
    if (this.aiExternalSourceS2Input) {
        this.aiExternalSourceS2Input.checked = sources.includes('semantic_scholar');
    }
    if (this.pubmedAPIKeyInput) this.pubmedAPIKeyInput.value = data.pubmed_api_key || '';
    if (this.pubmedEmailInput) this.pubmedEmailInput.value = data.pubmed_email || '';
    if (this.pubmedToolInput) this.pubmedToolInput.value = data.pubmed_tool || 'citebox';
}

async saveAIExternalSearchSettings() {
    const sources = [];
    if (this.aiExternalSourcePubMedInput?.checked) sources.push('pubmed');
    if (this.aiExternalSourceS2Input?.checked) sources.push('semantic_scholar');
    const res = await fetch('/api/settings/ai-external-search', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            sources,
            pubmed_api_key: this.pubmedAPIKeyInput?.value || '',
            pubmed_email: this.pubmedEmailInput?.value || '',
            pubmed_tool: this.pubmedToolInput?.value || 'citebox',
        }),
    });
    if (!res.ok) {
        if (this.aiExternalSearchSaveStatus) {
            this.aiExternalSearchSaveStatus.textContent = t('settings.ai.external_search.save_failed', '保存失败');
        }
        return;
    }
    if (this.aiExternalSearchSaveStatus) {
        this.aiExternalSearchSaveStatus.textContent = t('settings.ai.external_search.saved', '外部搜索源已保存');
    }
    Utils.showToast(t('settings.ai.external_search.saved', '外部搜索源已保存'));
}
```

- [ ] **Step 3: Add i18n keys**

Add to both locale files.

Chinese:

```json
"settings.ai.external_search.title": "AI 外部搜索源",
"settings.ai.external_search.desc": "选择 AI 助手执行外部检索时使用的学术搜索源。可启用多个来源，AI 会按来源生成检索词并合并结果。",
"settings.ai.external_search.pubmed": "<strong>PubMed</strong><br>默认来源；支持匿名访问，也可配置 NCBI API key。",
"settings.ai.external_search.s2": "<strong>Semantic Scholar</strong><br>适合跨学科调研；未配置 API key 时容易遇到匿名限流。",
"settings.ai.external_search.pubmed_api_key": "PubMed / NCBI API key",
"settings.ai.external_search.pubmed_email": "NCBI email",
"settings.ai.external_search.pubmed_tool": "NCBI tool",
"settings.ai.external_search.save_btn": "保存外部搜索源",
"settings.ai.external_search.saved": "外部搜索源已保存",
"settings.ai.external_search.save_failed": "保存失败"
```

English:

```json
"settings.ai.external_search.title": "AI External Search Sources",
"settings.ai.external_search.desc": "Choose which academic search sources the AI assistant uses for external search. Multiple sources can be enabled; AI plans queries per source and merges results.",
"settings.ai.external_search.pubmed": "<strong>PubMed</strong><br>Default source; anonymous access is allowed, and an NCBI API key can be configured.",
"settings.ai.external_search.s2": "<strong>Semantic Scholar</strong><br>Useful for broad cross-discipline search; anonymous access can hit rate limits without an API key.",
"settings.ai.external_search.pubmed_api_key": "PubMed / NCBI API key",
"settings.ai.external_search.pubmed_email": "NCBI email",
"settings.ai.external_search.pubmed_tool": "NCBI tool",
"settings.ai.external_search.save_btn": "Save external search sources",
"settings.ai.external_search.saved": "External search sources saved",
"settings.ai.external_search.save_failed": "Save failed"
```

- [ ] **Step 4: Run frontend checks**

Run:

```bash
node --check web/static/js/settings.js
go test ./internal/app/i18n_assets_test.go
```

Expected: PASS.

- [ ] **Step 5: Commit**

Run:

```bash
git add web/settings.html web/static/js/settings.js web/static/locales/zh-CN/settings.json web/static/locales/en/settings.json
git commit -m "Add AI external search settings UI"
```

---

## Task 11: Update API Docs And Final Verification

**Files:**

- Modify: `docs/api.md`
- Review: `README.md`
- Review: `README.en.md`

- [ ] **Step 1: Document settings API**

In `docs/api.md`, add under settings APIs:

````md
#### `GET /api/settings/ai-external-search`

返回 AI 助手外部搜索源配置。默认 `sources` 为 `["pubmed"]`。

```json
{
  "sources": ["pubmed", "semantic_scholar"],
  "pubmed_api_key": "",
  "pubmed_email": "",
  "pubmed_tool": "citebox"
}
```

#### `PUT /api/settings/ai-external-search`

保存 AI 助手外部搜索源配置。`sources` 可包含 `pubmed`、`semantic_scholar`，也可以为空数组表示不启用外部搜索源。PubMed 配置允许留空，留空时使用 NCBI 匿名访问。

```json
{
  "sources": ["pubmed"],
  "pubmed_api_key": "",
  "pubmed_email": "user@example.org",
  "pubmed_tool": "citebox"
}
```
````

- [ ] **Step 2: Run targeted backend tests**

Run:

```bash
go test ./internal/service/pubmed ./internal/service/ai_external ./internal/service/ai_assistant ./internal/service/ai_conversation ./internal/handler ./internal/app
```

Expected: PASS.

- [ ] **Step 3: Run full test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Run frontend syntax check**

Run:

```bash
node --check web/static/js/settings.js
```

Expected: PASS.

- [ ] **Step 5: Manual verification**

Start the app:

```bash
make run
```

Verify in browser:

- Open `/settings`.
- Confirm AI external source section appears in the AI settings area.
- Confirm PubMed is checked by default.
- Save with PubMed only.
- Open `/ai`.
- Send an external-search request such as `查外部：cell fate forward genetic screen source`.
- Confirm process stages mention PubMed.
- Return to settings, enable Semantic Scholar too, save, and repeat.
- Confirm process stages mention both sources or report one source failure while still using the successful source.

- [ ] **Step 6: Commit docs**

Run:

```bash
git add docs/api.md README.md README.en.md
git commit -m "Document AI external search sources"
```

If `README.md` and `README.en.md` were not changed, run:

```bash
git add docs/api.md
git commit -m "Document AI external search sources"
```

---

## Task 12: Final Integration Sweep

**Files:**

- Review only unless tests reveal defects.

- [ ] **Step 1: Inspect final diff**

Run:

```bash
git status --short
git log --oneline -12
```

Expected: clean worktree and a task-by-task commit history.

- [ ] **Step 2: Search for stale hardcoded source labels**

Run:

```bash
rg -n "外部 Semantic Scholar|Semantic Scholar 片段|externalSearchSource|Source:\\s*\"Semantic Scholar\"" internal web docs
```

Expected: only `/research` docs/UI and Semantic Scholar adapter code should retain Semantic Scholar-specific labels. AI assistant evidence prompts should use source-aware labels.

- [ ] **Step 3: Search for old S2-only AI fields**

Run:

```bash
rg -n "s2_paper_id|S2PaperID" internal/service/ai_assistant internal/service/ai_conversation web/static/js/ai-result-cards.js
```

Expected: existing `s2_paper_id` may remain for backward compatibility, but PubMed cards must also expose `sources`, `source_ids`, and `pmid`.

- [ ] **Step 4: Run final verification**

Run:

```bash
go test ./...
node --check web/static/js/settings.js
```

Expected: PASS.

- [ ] **Step 5: Commit any fixes**

If Step 2, Step 3, or Step 4 required fixes, commit them:

```bash
git add <changed-files>
git commit -m "Fix AI external search integration"
```

If no fixes were needed, do not create an empty commit.
