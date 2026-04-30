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

func TestClientSearchHandlesNumericExternalIDs(t *testing.T) {
	srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// S2 mixes strings and numbers in externalIds (CorpusId, PubMed often arrive
		// as JSON numbers). Send a raw JSON body that exercises both shapes.
		body := `{"total":1,"offset":0,"next":0,"data":[{
			"paperId":"p1",
			"title":"Numeric ExternalIDs",
			"externalIds":{"DOI":"10.1/abc","PubMed":12345,"CorpusId":67890}
		}]}`
		_, _ = w.Write([]byte(body))
	})
	defer stop()

	c := newTestClient(srv.URL, "")
	res, err := c.Search(context.Background(), "x", SearchOpts{Limit: 1})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("got %d items", len(res.Items))
	}
	if res.Items[0].ExternalIDs.DOI != "10.1/abc" {
		t.Fatalf("DOI = %q", res.Items[0].ExternalIDs.DOI)
	}
	if res.Items[0].ExternalIDs.PubMed != "12345" {
		t.Fatalf("PubMed (numeric) = %q, want %q", res.Items[0].ExternalIDs.PubMed, "12345")
	}
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
					"referenceCount": 31,
					"externalIds":    map[string]string{"DOI": "10.1/abc"}},
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
	if res.Items[0].ReferenceCount != 31 {
		t.Fatalf("ReferenceCount = %d, want 31", res.Items[0].ReferenceCount)
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
				fields := r.URL.Query().Get("fields")
				if !strings.Contains(fields, "isInfluential") || !strings.Contains(fields, "intents") {
					t.Errorf("fields = %q, want edge metadata", fields)
				}
				if strings.Contains(fields, ".tldr") || strings.Contains(fields, ",tldr") {
					t.Errorf("fields = %q, must not request nested tldr", fields)
				}
				if tc.name == "References" {
					if !strings.Contains(fields, "citedPaper.paperId") {
						t.Errorf("fields = %q, want citedPaper fields", fields)
					}
					if strings.Contains(fields, "citingPaper.") {
						t.Errorf("fields = %q, references must not request citingPaper fields", fields)
					}
				}
				if tc.name == "Citations" {
					if !strings.Contains(fields, "citingPaper.paperId") {
						t.Errorf("fields = %q, want citingPaper fields", fields)
					}
					if strings.Contains(fields, "citedPaper.") {
						t.Errorf("fields = %q, citations must not request citedPaper fields", fields)
					}
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
		fields := r.URL.Query().Get("fields")
		if !strings.Contains(fields, "paperId") || !strings.Contains(fields, "title") {
			t.Errorf("fields = %q, want paper metadata", fields)
		}
		if strings.Contains(fields, "tldr") {
			t.Errorf("fields = %q, recommendations must not request tldr", fields)
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

func TestClientGetBatch(t *testing.T) {
	srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/graph/v1/paper/batch" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if !strings.Contains(r.URL.Query().Get("fields"), "title") {
			t.Errorf("fields query missing")
		}
		var body struct {
			IDs []string `json:"ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.IDs) != 3 {
			t.Errorf("ids = %v", body.IDs)
		}
		// middle entry is missing → null
		_, _ = w.Write([]byte(`[
			{"paperId":"p1","title":"First"},
			null,
			{"paperId":"p3","title":"Third"}
		]`))
	})
	defer stop()
	c := newTestClient(srv.URL, "")
	papers, err := c.GetBatch(context.Background(), []string{"p1", "p2", "p3"}, nil)
	if err != nil {
		t.Fatalf("GetBatch: %v", err)
	}
	if len(papers) != 3 {
		t.Fatalf("len = %d", len(papers))
	}
	if papers[0].PaperID != "p1" || papers[1].PaperID != "" || papers[2].PaperID != "p3" {
		t.Fatalf("got %+v", papers)
	}
}

func TestClientAutocomplete(t *testing.T) {
	srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graph/v1/paper/autocomplete" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "scib" {
			t.Errorf("query = %q", r.URL.Query().Get("query"))
		}
		_, _ = w.Write([]byte(`{"matches":[
			{"id":"p1","title":"SciBERT","authorsYear":"Beltagy et al., 2019"}
		]}`))
	})
	defer stop()
	c := newTestClient(srv.URL, "")
	items, err := c.Autocomplete(context.Background(), "scib")
	if err != nil {
		t.Fatalf("Autocomplete: %v", err)
	}
	if len(items) != 1 || items[0].PaperID != "p1" || items[0].Title != "SciBERT" || items[0].AuthorsYear == "" {
		t.Fatalf("items = %+v", items)
	}
}

func TestClientSnippetSearch(t *testing.T) {
	srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graph/v1/snippet/search" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "graph neural network" {
			t.Errorf("query = %q", r.URL.Query().Get("query"))
		}
		if r.URL.Query().Get("paperIds") != "p1,p2" {
			t.Errorf("paperIds = %q", r.URL.Query().Get("paperIds"))
		}
		_, _ = w.Write([]byte(`{
			"data":[{
				"paper":{"paperId":"p1","title":"GNN paper"},
				"snippet":{"text":"GNNs work...","snippetKind":"body","section":"Intro","snippetOffset":{"start":10,"end":42}},
				"score":0.87
			}],
			"retrievalVersion":"v3"
		}`))
	})
	defer stop()
	c := newTestClient(srv.URL, "")
	res, err := c.SnippetSearch(context.Background(), "graph neural network", SnippetSearchOpts{
		PaperIDs: []string{"p1", "p2"},
	})
	if err != nil {
		t.Fatalf("SnippetSearch: %v", err)
	}
	if res.RetrievalVersion != "v3" || len(res.Matches) != 1 {
		t.Fatalf("res = %+v", res)
	}
	m := res.Matches[0]
	if m.PaperID != "p1" || m.Snippet.Text != "GNNs work..." || m.Snippet.SnippetOffset.End != 42 || m.Score < 0.86 {
		t.Fatalf("match = %+v", m)
	}
}

func TestClientRetriesOn429(t *testing.T) {
	calls := 0
	srv, stop := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"paperId":"p1","title":"x"}`))
	})
	defer stop()
	c := newTestClient(srv.URL, "")
	if _, err := c.Get(context.Background(), "DOI:x", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls (1 + retry), got %d", calls)
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
		fields := r.URL.Query().Get("fields")
		if !strings.Contains(fields, "paperId") || !strings.Contains(fields, "title") {
			t.Errorf("fields = %q, want paper metadata", fields)
		}
		if strings.Contains(fields, "tldr") {
			t.Errorf("fields = %q, recommendations must not request tldr", fields)
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
