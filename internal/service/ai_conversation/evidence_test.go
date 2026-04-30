package ai_conversation

import (
	"context"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

type stubSnippetSearcher struct {
	last research.SnippetSearchOpts
	res  research.SnippetList
	err  error
}

func (s *stubSnippetSearcher) SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error) {
	s.last = opts
	if s.err != nil {
		return research.SnippetList{}, s.err
	}
	return s.res, nil
}

func TestEvidenceInjectAddsBlockAndCitations(t *testing.T) {
	searcher := &stubSnippetSearcher{
		res: research.SnippetList{
			Items: []research.SnippetMatch{
				{
					PaperID: "abc",
					Paper:   research.Paper{PaperID: "abc", ExternalIDs: research.IDs{DOI: "10.1/abc"}},
					Snippet: research.Snippet{Text: "snippet text", SnippetKind: "body", Section: "Intro"},
					Score:   0.9,
				},
			},
		},
	}
	pinned := []repository.AIPinnedPaper{
		{PaperID: 42, Title: "Pinned", DOI: "10.1/abc"},
	}
	prompt, citations, err := injectEvidence(context.Background(), searcher, "用户问题", pinned)
	if err != nil {
		t.Fatalf("injectEvidence: %v", err)
	}
	if !strings.Contains(prompt, "[1]") || !strings.Contains(prompt, "snippet text") {
		t.Fatalf("prompt missing evidence: %s", prompt)
	}
	if len(citations) != 1 || citations[0].PaperID != 42 || citations[0].ExternalID != "DOI:10.1/abc" {
		t.Fatalf("citations = %+v", citations)
	}
	if searcher.last.PaperIDs[0] != "DOI:10.1/abc" {
		t.Fatalf("paperIDs sent to S2 = %v", searcher.last.PaperIDs)
	}
}

func TestEvidenceFallsBackOnSearcherError(t *testing.T) {
	searcher := &stubSnippetSearcher{err: research.ErrRateLimited}
	pinned := []repository.AIPinnedPaper{{PaperID: 1, Title: "x", DOI: "10.1/x"}}
	_, _, err := injectEvidence(context.Background(), searcher, "Q", pinned)
	if err == nil {
		t.Fatalf("expected error from rate-limited searcher")
	}
}

func TestEvidenceSkipsWhenNoExternalIDs(t *testing.T) {
	searcher := &stubSnippetSearcher{}
	pinned := []repository.AIPinnedPaper{{PaperID: 1, Title: "x", DOI: ""}}
	_, _, err := injectEvidence(context.Background(), searcher, "Q", pinned)
	if err != ErrNoExternalIDs {
		t.Fatalf("err = %v, want ErrNoExternalIDs", err)
	}
}
