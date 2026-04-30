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
	getCalls   int32
	getFn      func(ctx context.Context, id string, fields []string) (Paper, error)
	recCalls   int32
	recFn      func(ctx context.Context, id string) ([]Paper, error)
	batchCalls int32
	batchFn    func(ctx context.Context, ids []string, fields []string) ([]Paper, error)
}

func (s *stubClient) Get(ctx context.Context, id string, fields []string) (Paper, error) {
	atomic.AddInt32(&s.getCalls, 1)
	return s.getFn(ctx, id, fields)
}

func (s *stubClient) GetBatch(ctx context.Context, ids []string, fields []string) ([]Paper, error) {
	atomic.AddInt32(&s.batchCalls, 1)
	if s.batchFn != nil {
		return s.batchFn(ctx, ids, fields)
	}
	out := make([]Paper, 0, len(ids))
	for _, id := range ids {
		p, err := s.getFn(ctx, id, fields)
		if err != nil {
			out = append(out, Paper{})
			continue
		}
		out = append(out, p)
	}
	return out, nil
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
func (s *stubClient) Autocomplete(ctx context.Context, q string) ([]AutocompleteItem, error) {
	return nil, nil
}
func (s *stubClient) SnippetSearch(ctx context.Context, q string, o SnippetSearchOpts) (SnippetList, error) {
	return SnippetList{}, nil
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

// putFresh seeds the cache with a paper that GetPapers will treat as a hit.
func putFresh(repo *stubCacheRepo, id string, p Paper) {
	buf, _ := json.Marshal(p)
	repo.store[paperCacheKey(id)] = struct {
		payload   string
		fetchedAt time.Time
	}{string(buf), time.Now()}
}

// putStale seeds an entry whose fetchedAt is older than the configured TTL.
func putStale(repo *stubCacheRepo, id string, p Paper) {
	buf, _ := json.Marshal(p)
	repo.store[paperCacheKey(id)] = struct {
		payload   string
		fetchedAt time.Time
	}{string(buf), time.Now().Add(-48 * time.Hour)}
}

func TestServiceGetPapersAllCached(t *testing.T) {
	repo := newStubCacheRepo()
	putFresh(repo, "p1", Paper{PaperID: "p1", Title: "One"})
	putFresh(repo, "p2", Paper{PaperID: "p2", Title: "Two"})

	cli := &stubClient{
		batchFn: func(ctx context.Context, ids []string, fields []string) ([]Paper, error) {
			t.Fatalf("GetBatch must not be called when every id is fresh-cached; got ids=%v", ids)
			return nil, nil
		},
	}
	svc := NewService(cli, repo, ServiceConfig{PaperTTL: time.Hour})

	papers, err := svc.GetPapers(context.Background(), []string{"p1", "p2"})
	if err != nil {
		t.Fatalf("GetPapers: %v", err)
	}
	if len(papers) != 2 || papers[0].Title != "One" || papers[1].Title != "Two" {
		t.Fatalf("got %+v", papers)
	}
	if cli.batchCalls != 0 {
		t.Fatalf("batchCalls = %d, want 0", cli.batchCalls)
	}
}

func TestServiceGetPapersAllMissingFetchesAndCaches(t *testing.T) {
	repo := newStubCacheRepo()
	cli := &stubClient{
		batchFn: func(ctx context.Context, ids []string, fields []string) ([]Paper, error) {
			out := make([]Paper, len(ids))
			for i, id := range ids {
				out[i] = Paper{PaperID: id, Title: "Fetched-" + id}
			}
			return out, nil
		},
	}
	svc := NewService(cli, repo, ServiceConfig{PaperTTL: time.Hour})

	papers, err := svc.GetPapers(context.Background(), []string{"p1", "p2", "p3"})
	if err != nil {
		t.Fatalf("GetPapers: %v", err)
	}
	if len(papers) != 3 || papers[0].Title != "Fetched-p1" || papers[2].Title != "Fetched-p3" {
		t.Fatalf("got %+v", papers)
	}
	if cli.batchCalls != 1 {
		t.Fatalf("batchCalls = %d, want 1", cli.batchCalls)
	}
	// All three should now be in the cache.
	for _, id := range []string{"p1", "p2", "p3"} {
		if _, _, err := repo.GetCache(paperCacheKey(id)); err != nil {
			t.Fatalf("expected cache hit for %s, got %v", id, err)
		}
	}
}

func TestServiceGetPapersPartialCacheOnlyFetchesMissing(t *testing.T) {
	repo := newStubCacheRepo()
	putFresh(repo, "p1", Paper{PaperID: "p1", Title: "Cached-1"})

	var requestedIDs []string
	cli := &stubClient{
		batchFn: func(ctx context.Context, ids []string, fields []string) ([]Paper, error) {
			requestedIDs = append([]string(nil), ids...)
			out := make([]Paper, len(ids))
			for i, id := range ids {
				out[i] = Paper{PaperID: id, Title: "Fetched-" + id}
			}
			return out, nil
		},
	}
	svc := NewService(cli, repo, ServiceConfig{PaperTTL: time.Hour})

	papers, err := svc.GetPapers(context.Background(), []string{"p1", "p2", "p3"})
	if err != nil {
		t.Fatalf("GetPapers: %v", err)
	}
	if len(papers) != 3 {
		t.Fatalf("len = %d", len(papers))
	}
	if papers[0].Title != "Cached-1" || papers[1].Title != "Fetched-p2" || papers[2].Title != "Fetched-p3" {
		t.Fatalf("output order/content wrong: %+v", papers)
	}
	if cli.batchCalls != 1 {
		t.Fatalf("batchCalls = %d, want 1", cli.batchCalls)
	}
	if len(requestedIDs) != 2 || requestedIDs[0] != "p2" || requestedIDs[1] != "p3" {
		t.Fatalf("upstream should only see uncached ids; got %v", requestedIDs)
	}
}

func TestServiceGetPapersUpstreamErrorFallsBackToStale(t *testing.T) {
	repo := newStubCacheRepo()
	putStale(repo, "p1", Paper{PaperID: "p1", Title: "Stale-1"})
	// p2 has no cache entry at all, should be dropped from output.

	cli := &stubClient{
		batchFn: func(ctx context.Context, ids []string, fields []string) ([]Paper, error) {
			return nil, errors.New("network down")
		},
	}
	svc := NewService(cli, repo, ServiceConfig{PaperTTL: time.Hour})

	papers, err := svc.GetPapers(context.Background(), []string{"p1", "p2"})
	if err != nil {
		t.Fatalf("expected stale fallback, got error: %v", err)
	}
	if len(papers) != 1 || papers[0].Title != "Stale-1" {
		t.Fatalf("got %+v", papers)
	}
}

func TestServiceGetPapersUpstreamErrorKeepsFreshSubset(t *testing.T) {
	// Reviewer-flagged regression: if an upstream batch fails and there's no
	// stale cache for the missing ids, we must still return the fresh-cache
	// hits we already had instead of blanking the whole list.
	repo := newStubCacheRepo()
	putFresh(repo, "p1", Paper{PaperID: "p1", Title: "Fresh-1"})

	cli := &stubClient{
		batchFn: func(ctx context.Context, ids []string, fields []string) ([]Paper, error) {
			return nil, errors.New("upstream 502")
		},
	}
	svc := NewService(cli, repo, ServiceConfig{PaperTTL: time.Hour})

	papers, err := svc.GetPapers(context.Background(), []string{"p1", "p2"})
	if err != nil {
		t.Fatalf("expected nil error when fresh cache covers part of the request, got %v", err)
	}
	if len(papers) != 1 || papers[0].Title != "Fresh-1" {
		t.Fatalf("got %+v", papers)
	}
}
