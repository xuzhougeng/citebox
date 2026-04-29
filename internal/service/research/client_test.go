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
