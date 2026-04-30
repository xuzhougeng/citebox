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
	lastQuery       string
	last            research.SnippetSearchOpts
	res             research.SnippetList
	err             error
	lastSearchQuery string
	lastSearch      research.SearchOpts
	searchRes       research.PaperList
	searchErr       error
}

func (s *stubSnippetSearcher) SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error) {
	s.lastQuery = query
	s.last = opts
	if s.err != nil {
		return research.SnippetList{}, s.err
	}
	return s.res, nil
}

func (s *stubSnippetSearcher) Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error) {
	s.lastSearchQuery = query
	s.lastSearch = opts
	if s.searchErr != nil {
		return research.PaperList{}, s.searchErr
	}
	return s.searchRes, nil
}

type stubPaperGetter map[int64]*model.Paper

func (s stubPaperGetter) GetPaperDetail(id int64) (*model.Paper, error) {
	return s[id], nil
}

type stubEvidenceLibrary struct {
	stubPaperGetter
	ids []int64
}

func (s stubEvidenceLibrary) ListEvidenceCandidatePaperIDs(terms []string, limit int) ([]int64, error) {
	return append([]int64(nil), s.ids...), nil
}

func TestEvidenceSearchTermsExpandATACData(t *testing.T) {
	terms := evidenceSearchTerms("帮我查找包括 ATAC 数据的文章")
	joined := strings.ToLower(strings.Join(terms, "\n"))
	for _, want := range []string{"atac-seq", "assay for transposase-accessible chromatin", "chromatin accessibility", "scatac-seq"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("terms missing %q: %v", want, terms)
		}
	}
}

func TestEvidenceInjectUsesLocalLibraryWhenNoPinnedPapers(t *testing.T) {
	papers := stubEvidenceLibrary{
		stubPaperGetter: stubPaperGetter{
			7: {
				ID:      7,
				Title:   "Chromatin Atlas",
				PDFText: "This paper profiles chromatin accessibility during cell differentiation.",
			},
		},
		ids: []int64{7},
	}

	prompt, citations, err := injectEvidence(context.Background(), papers, nil, "帮我查找包括 ATAC 数据的文章", nil, EvidenceOptions{})
	if err != nil {
		t.Fatalf("injectEvidence: %v", err)
	}
	if !strings.Contains(prompt, "本地文献库") || !strings.Contains(prompt, "Chromatin Atlas") || !strings.Contains(prompt, "chromatin accessibility") {
		t.Fatalf("prompt missing local library evidence: %s", prompt)
	}
	if len(citations) != 1 || citations[0].PaperID != 7 || citations[0].Source != evidenceSourceLocal {
		t.Fatalf("citations = %+v", citations)
	}
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
	if len(searcher.last.PaperIDs) != 0 {
		t.Fatalf("paperIDs sent to S2 = %v, want broad external search", searcher.last.PaperIDs)
	}
}

func TestEvidenceExternalSearchesBroadlyWithoutPinnedIDs(t *testing.T) {
	searcher := &stubSnippetSearcher{
		res: research.SnippetList{
			Items: []research.SnippetMatch{
				{
					PaperID: "s2-atac",
					Paper:   research.Paper{PaperID: "s2-atac", Title: "ATAC Atlas", ExternalIDs: research.IDs{DOI: "10.1/atac"}},
					Snippet: research.Snippet{Text: "ATAC-seq identifies chromatin accessibility changes.", SnippetKind: "abstract"},
					Score:   0.91,
				},
			},
		},
	}

	prompt, citations, err := injectEvidence(context.Background(), stubPaperGetter{}, searcher, "帮我查找包括 ATAC 数据的文章", nil, EvidenceOptions{
		IncludeExternal: true,
		DisableLocal:    true,
	})
	if err != nil {
		t.Fatalf("injectEvidence: %v", err)
	}
	if len(searcher.last.PaperIDs) != 0 {
		t.Fatalf("paper IDs = %v, want broad external search", searcher.last.PaperIDs)
	}
	if !strings.Contains(strings.ToLower(searcher.lastQuery), "atac-seq") {
		t.Fatalf("query = %q, want ATAC expansion", searcher.lastQuery)
	}
	if !strings.Contains(prompt, "外部 Semantic Scholar") || !strings.Contains(prompt, "ATAC-seq identifies") {
		t.Fatalf("prompt missing broad external evidence: %s", prompt)
	}
	if len(citations) != 1 || citations[0].S2PaperID != "s2-atac" || citations[0].ExternalID != "DOI:10.1/atac" {
		t.Fatalf("citations = %+v", citations)
	}
}

func TestEvidenceExternalFallsBackToPaperSearchWhenSnippetSearchFails(t *testing.T) {
	searcher := &stubSnippetSearcher{
		err: research.ErrRateLimited,
		searchRes: research.PaperList{
			Items: []research.Paper{
				{
					PaperID:     "s2-atac-search",
					Title:       "ATAC-seq maps chromatin accessibility",
					Abstract:    "ATAC-seq identifies accessible chromatin regions using sequencing.",
					TLDR:        "ATAC-seq profiles chromatin accessibility genome-wide.",
					ExternalIDs: research.IDs{DOI: "10.1/search-atac"},
					Year:        2024,
					Venue:       "Genome Biology",
				},
			},
		},
	}

	prompt, citations, err := injectEvidence(context.Background(), stubPaperGetter{}, searcher, "帮我查找包括 ATAC 数据的文章", nil, EvidenceOptions{
		IncludeExternal: true,
		DisableLocal:    true,
	})
	if err != nil {
		t.Fatalf("injectEvidence: %v", err)
	}
	if searcher.lastSearchQuery == "" {
		t.Fatalf("expected paper search fallback after snippet failure")
	}
	if strings.Contains(prompt, "外部搜索失败") {
		t.Fatalf("prompt should use fallback evidence instead of declaring external search failure: %s", prompt)
	}
	if !strings.Contains(prompt, "ATAC-seq maps chromatin accessibility") || !strings.Contains(prompt, "accessible chromatin regions") {
		t.Fatalf("prompt missing fallback search evidence: %s", prompt)
	}
	if len(citations) != 1 || citations[0].S2PaperID != "s2-atac-search" || citations[0].ExternalID != "DOI:10.1/search-atac" {
		t.Fatalf("citations = %+v", citations)
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
	searcher := &stubSnippetSearcher{err: research.ErrRateLimited, searchErr: research.ErrRateLimited}
	pinned := []repository.AIPinnedPaper{{PaperID: 1, Title: "x", DOI: "10.1/x"}}
	prompt, citations, err := injectEvidence(context.Background(), stubPaperGetter{}, searcher, "Q", pinned, EvidenceOptions{IncludeExternal: true})
	if err != nil {
		t.Fatalf("injectEvidence: %v", err)
	}
	if len(citations) != 0 {
		t.Fatalf("citations = %+v, want none", citations)
	}
	if !strings.Contains(prompt, "外部搜索失败") || !strings.Contains(prompt, "证据不足") {
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
