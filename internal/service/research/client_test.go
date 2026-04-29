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

func TestClientReferencesCitationsRecommendations(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		invoke   func(c *Client, ctx context.Context) (PaperList, error)
		respData string
		listKey  string
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
