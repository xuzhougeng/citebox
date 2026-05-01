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
	pubmed := &stubSearcher{}
	s2 := &stubSearcher{}
	svc := NewService(stubSettings{sources: []SourceID{}}, map[SourceID]Searcher{
		SourcePubMed:          pubmed,
		SourceSemanticScholar: s2,
	})

	_, err := svc.Search(context.Background(), SourceQueries{}, SearchOptions{Limit: 5})
	if !errors.Is(err, ErrNoSourcesEnabled) {
		t.Fatalf("err = %v, want ErrNoSourcesEnabled", err)
	}
	if len(pubmed.queries) != 0 || len(s2.queries) != 0 {
		t.Fatalf("unexpected upstream calls: pubmed=%v s2=%v", pubmed.queries, s2.queries)
	}
}

func TestServiceSearchReturnsErrorWhenAllSourcesFail(t *testing.T) {
	svc := NewService(stubSettings{sources: []SourceID{SourcePubMed, SourceSemanticScholar}}, map[SourceID]Searcher{
		SourcePubMed:          &stubSearcher{err: errors.New("pubmed down")},
		SourceSemanticScholar: &stubSearcher{err: errors.New("s2 rate limited")},
	})
	_, err := svc.Search(context.Background(), SourceQueries{
		SourcePubMed:          {"pub query"},
		SourceSemanticScholar: {"s2 query"},
	}, SearchOptions{Limit: 5})
	if err == nil {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"pubmed", "semantic_scholar", "pubmed down", "s2 rate limited"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want it to contain %q", err, want)
		}
	}
}
