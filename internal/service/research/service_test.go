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

func (s *stubClient) GetBatch(ctx context.Context, ids []string, fields []string) ([]Paper, error) {
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
