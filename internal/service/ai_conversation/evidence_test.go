package ai_conversation

import (
	"context"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
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

type stubPaperGetter map[int64]*model.Paper

func (s stubPaperGetter) GetPaperDetail(id int64) (*model.Paper, error) {
	return s[id], nil
}

func TestEvidenceInjectUsesLocalPinnedFullText(t *testing.T) {
	papers := stubPaperGetter{
		42: {
			ID:      42,
			Title:   "Plant Cell Atlas",
			PDFText: "We integrated 11 public single-cell RNA-seq datasets of Arabidopsis shoots and leaves.",
		},
	}
	pinned := []repository.AIPinnedPaper{{PaperID: 42, Title: "Plant Cell Atlas"}}

	prompt, citations, err := injectEvidence(context.Background(), papers, nil, "查找单细胞测序相关证据", pinned, EvidenceOptions{})
	if err != nil {
		t.Fatalf("injectEvidence: %v", err)
	}
	if !strings.Contains(prompt, "本地已钉文献全文") || !strings.Contains(prompt, "single-cell RNA-seq") {
		t.Fatalf("prompt missing local evidence: %s", prompt)
	}
	if len(citations) != 1 || citations[0].PaperID != 42 || citations[0].Source != evidenceSourceLocal {
		t.Fatalf("citations = %+v", citations)
	}
}

func TestEvidenceCanIncludeExternalSnippets(t *testing.T) {
	searcher := &stubSnippetSearcher{
		res: research.SnippetList{
			Items: []research.SnippetMatch{
				{
					PaperID: "abc",
					Paper:   research.Paper{PaperID: "abc", Title: "Pinned", ExternalIDs: research.IDs{DOI: "10.1/abc"}},
					Snippet: research.Snippet{Text: "external snippet text", SnippetKind: "body", Section: "Intro"},
					Score:   0.9,
				},
			},
		},
	}
	pinned := []repository.AIPinnedPaper{{PaperID: 42, Title: "Pinned", DOI: "10.1/abc"}}
	prompt, citations, err := injectEvidence(context.Background(), stubPaperGetter{}, searcher, "用户问题", pinned, EvidenceOptions{IncludeExternal: true})
	if err != nil {
		t.Fatalf("injectEvidence: %v", err)
	}
	if !strings.Contains(prompt, "外部 Semantic Scholar") || !strings.Contains(prompt, "external snippet text") {
		t.Fatalf("prompt missing external evidence: %s", prompt)
	}
	if len(citations) != 1 || citations[0].PaperID != 42 || citations[0].ExternalID != "DOI:10.1/abc" || citations[0].Source != evidenceSourceExternal {
		t.Fatalf("citations = %+v", citations)
	}
	if searcher.last.PaperIDs[0] != "DOI:10.1/abc" {
		t.Fatalf("paperIDs sent to S2 = %v", searcher.last.PaperIDs)
	}
}

func TestEvidenceDoesNotRequireExternalIDsForLocalEvidence(t *testing.T) {
	papers := stubPaperGetter{
		1: {
			ID:      1,
			Title:   "Local Only",
			PDFText: "The method uses scRNA-seq data for developmental trajectory analysis.",
		},
	}
	pinned := []repository.AIPinnedPaper{{PaperID: 1, Title: "Local Only", DOI: ""}}
	_, citations, err := injectEvidence(context.Background(), papers, &stubSnippetSearcher{}, "单细胞发育轨迹", pinned, EvidenceOptions{IncludeExternal: true})
	if err != nil {
		t.Fatalf("injectEvidence: %v", err)
	}
	if len(citations) == 0 || citations[0].Source != evidenceSourceLocal {
		t.Fatalf("citations = %+v, want local citation", citations)
	}
}

func TestEvidenceExternalSearchErrorKeepsStrictLocalPrompt(t *testing.T) {
	searcher := &stubSnippetSearcher{err: research.ErrRateLimited}
	pinned := []repository.AIPinnedPaper{{PaperID: 1, Title: "x", DOI: "10.1/x"}}
	prompt, citations, err := injectEvidence(context.Background(), stubPaperGetter{}, searcher, "Q", pinned, EvidenceOptions{IncludeExternal: true})
	if err != nil {
		t.Fatalf("injectEvidence: %v", err)
	}
	if len(citations) != 0 {
		t.Fatalf("citations = %+v, want none", citations)
	}
	if !strings.Contains(prompt, "外部证据检索失败") || !strings.Contains(prompt, "证据不足") {
		t.Fatalf("prompt missing strict fallback: %s", prompt)
	}
}

func TestEvidenceExternalSkipsWhenNoExternalIDs(t *testing.T) {
	searcher := &stubSnippetSearcher{}
	pinned := []repository.AIPinnedPaper{{PaperID: 1, Title: "x", DOI: ""}}
	prompt, citations, err := injectEvidence(context.Background(), stubPaperGetter{}, searcher, "Q", pinned, EvidenceOptions{IncludeExternal: true})
	if err != nil {
		t.Fatalf("injectEvidence: %v", err)
	}
	if len(citations) != 0 || !strings.Contains(prompt, "证据不足") {
		t.Fatalf("prompt=%s citations=%+v", prompt, citations)
	}
}
