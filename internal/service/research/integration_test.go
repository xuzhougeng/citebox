package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/repository"
)

// newIntegrationRepo opens a file-backed SQLite DB in a temp dir, applies the
// schema via NewLibraryRepository, and returns the Research sub-repo.
// The DB file is cleaned up automatically when the test ends.
func newIntegrationRepo(t *testing.T) *repository.ResearchRepository {
	t.Helper()
	libRepo, err := repository.NewLibraryRepository(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("NewLibraryRepository: %v", err)
	}
	t.Cleanup(func() { _ = libRepo.Close() })
	return libRepo.Research
}

// TestResearchEndToEnd exercises the full path: search → cache miss → upstream
// fetch → cache write → cache hit (verified by stopping the upstream server).
func TestResearchEndToEnd(t *testing.T) {
	researchRepo := newIntegrationRepo(t)

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/graph/v1/paper/search":
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
		case "/graph/v1/paper/p1":
			q := r.URL.Query()
			_ = q // fields param present but not required by stub
			json.NewEncoder(w).Encode(map[string]interface{}{
				"paperId":     "p1",
				"title":       "Attention",
				"year":        2017,
				"externalIds": map[string]string{"DOI": "10.1/abc"},
			})
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusBadRequest)
		}
	}))
	// Note: we intentionally close s2 mid-test to verify cache fallback;
	// defer here is a safety net for early-exit paths.
	defer s2.Close()

	cli := NewClient(Config{BaseURL: s2.URL, MinInterval: 0})
	adapter := &RepoAdapter{Repo: researchRepo}
	svc := NewService(cli, adapter, ServiceConfig{PaperTTL: time.Hour})

	// -- Step 1: search (passthrough, no caching) --
	res, err := svc.Search(context.Background(), "transformer", SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("Search err: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("Search: want 1 item, got %d", len(res.Items))
	}

	paperID := res.Items[0].PaperID

	// -- Step 2: first GetPaper – cache miss, fetches from upstream, writes cache --
	paper, err := svc.GetPaper(context.Background(), paperID)
	if err != nil {
		t.Fatalf("GetPaper (first): %v", err)
	}
	if paper.Title != "Attention" {
		t.Fatalf("GetPaper (first): title = %q, want %q", paper.Title, "Attention")
	}

	// -- Step 3: stop the upstream; second call must be served from cache --
	s2.Close()

	paper2, err := svc.GetPaper(context.Background(), paperID)
	if err != nil {
		t.Fatalf("GetPaper (cached): %v", err)
	}
	if paper2.Title != "Attention" {
		t.Fatalf("GetPaper (cached): expected cache hit, got title %q", paper2.Title)
	}
}
