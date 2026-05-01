package ai_external

import (
	"context"
	"testing"

	"github.com/xuzhougeng/citebox/internal/service/pubmed"
)

type stubPubMedSearcher struct {
	result pubmed.SearchResult
	err    error
}

func (s stubPubMedSearcher) Search(ctx context.Context, query string, opts pubmed.SearchOptions) (pubmed.SearchResult, error) {
	if s.err != nil {
		return pubmed.SearchResult{}, s.err
	}
	return s.result, nil
}

func TestPubMedAdapterCarriesDualYearMetadata(t *testing.T) {
	adapter := PubMedAdapter{
		Client: stubPubMedSearcher{
			result: pubmed.SearchResult{
				Items: []pubmed.Paper{{
					PMID:       "12345",
					Title:      "Dual year article",
					Year:       2026,
					OnlineYear: 2025,
					IssueYear:  2026,
					YearLabel:  "2025 online / 2026 issue",
				}},
			},
		},
	}

	papers, err := adapter.Search(context.Background(), "dual year", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("len = %d, want 1", len(papers))
	}
	p := papers[0]
	if p.OnlineYear != 2025 {
		t.Fatalf("OnlineYear = %d, want 2025", p.OnlineYear)
	}
	if p.IssueYear != 2026 {
		t.Fatalf("IssueYear = %d, want 2026", p.IssueYear)
	}
	if p.YearLabel != "2025 online / 2026 issue" {
		t.Fatalf("YearLabel = %q, want %q", p.YearLabel, "2025 online / 2026 issue")
	}
}
