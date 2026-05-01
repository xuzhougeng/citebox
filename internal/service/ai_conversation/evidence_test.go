package ai_conversation

import (
	"context"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/ai_external"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

type stubSnippetSearcher struct {
	lastQueries ai_external.SourceQueries
	lastOpts    ai_external.SearchOptions
	searchRes   ai_external.SearchResult
	searchErr   error

	res research.SnippetList
	err error
}

func (s *stubSnippetSearcher) Search(ctx context.Context, queries ai_external.SourceQueries, opts ai_external.SearchOptions) (ai_external.SearchResult, error) {
	s.lastQueries = cloneSourceQueries(queries)
	s.lastOpts = opts
	if s.searchErr != nil {
		return s.searchRes, s.searchErr
	}
	if s.err != nil {
		return ai_external.SearchResult{}, s.err
	}
	if len(s.searchRes.Papers) > 0 || len(s.searchRes.Sources) > 0 || len(s.searchRes.Results) > 0 || len(s.searchRes.Failures) > 0 {
		return s.searchRes, nil
	}
	return aiExternalResultFromSnippetList(s.res), nil
}

func cloneSourceQueries(queries ai_external.SourceQueries) ai_external.SourceQueries {
	out := make(ai_external.SourceQueries, len(queries))
	for source, sourceQueries := range queries {
		out[source] = append([]string(nil), sourceQueries...)
	}
	return out
}

func aiExternalResultFromSnippetList(list research.SnippetList) ai_external.SearchResult {
	papers := make([]ai_external.Paper, 0, len(list.Items))
	for _, item := range list.Items {
		papers = append(papers, ai_external.Paper{
			Source:        ai_external.SourceSemanticScholar,
			SourcePaperID: item.PaperID,
			SourcePaperIDs: map[ai_external.SourceID]string{
				ai_external.SourceSemanticScholar: item.PaperID,
			},
			Sources:  []ai_external.SourceID{ai_external.SourceSemanticScholar},
			DOI:      item.Paper.ExternalIDs.DOI,
			ArXiv:    item.Paper.ExternalIDs.ArXiv,
			PMID:     item.Paper.ExternalIDs.PubMed,
			Title:    item.Paper.Title,
			Abstract: item.Snippet.Text,
		})
	}
	return ai_external.SearchResult{
		Sources: []ai_external.SourceID{ai_external.SourceSemanticScholar},
		Papers:  papers,
	}
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
	if !strings.Contains(prompt, "外部学术搜索") || !strings.Contains(prompt, "Semantic Scholar") || !strings.Contains(prompt, "external snippet text") {
		t.Fatalf("prompt missing external evidence: %s", prompt)
	}
	if len(citations) != 1 || citations[0].PaperID != 42 || citations[0].ExternalID != "DOI:10.1/abc" || citations[0].Source != "external:Semantic Scholar" {
		t.Fatalf("citations = %+v", citations)
	}
	if searcher.lastOpts.Limit != 8 {
		t.Fatalf("limit = %d, want 8", searcher.lastOpts.Limit)
	}
}

func TestExternalEvidenceUsesConfiguredSourceLabels(t *testing.T) {
	searcher := &stubSnippetSearcher{
		searchRes: ai_external.SearchResult{
			Sources: []ai_external.SourceID{ai_external.SourcePubMed},
			Papers: []ai_external.Paper{{
				Source:        ai_external.SourcePubMed,
				SourcePaperID: "12345",
				SourcePaperIDs: map[ai_external.SourceID]string{
					ai_external.SourcePubMed: "12345",
				},
				Sources:  []ai_external.SourceID{ai_external.SourcePubMed},
				PMID:     "12345",
				Title:    "PubMed Evidence",
				Abstract: "PubMed abstract evidence.",
			}},
		},
	}

	prompt, citations, err := injectEvidence(context.Background(), stubPaperGetter{}, searcher, "用户问题", nil, EvidenceOptions{
		IncludeExternal: true,
		DisableLocal:    true,
	})
	if err != nil {
		t.Fatalf("injectEvidence: %v", err)
	}
	if len(citations) != 1 || citations[0].Source != "external:PubMed" || citations[0].ExternalID != "PMID:12345" {
		t.Fatalf("citations = %+v", citations)
	}
	if !strings.Contains(prompt, "外部学术搜索") || !strings.Contains(prompt, "PubMed") {
		t.Fatalf("prompt missing PubMed source label: %s", prompt)
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
	if gotPubMed, gotS2 := strings.Join(searcher.lastQueries[ai_external.SourcePubMed], "\n"), strings.Join(searcher.lastQueries[ai_external.SourceSemanticScholar], "\n"); gotPubMed != gotS2 {
		t.Fatalf("source queries differ: pubmed=%q s2=%q", gotPubMed, gotS2)
	}
	if !strings.Contains(strings.ToLower(strings.Join(searcher.lastQueries[ai_external.SourceSemanticScholar], "\n")), "atac-seq") {
		t.Fatalf("queries = %v, want ATAC expansion", searcher.lastQueries)
	}
	if !strings.Contains(prompt, "外部学术搜索") || !strings.Contains(prompt, "Semantic Scholar") || !strings.Contains(prompt, "ATAC-seq identifies") {
		t.Fatalf("prompt missing broad external evidence: %s", prompt)
	}
	if len(citations) != 1 || citations[0].S2PaperID != "s2-atac" || citations[0].ExternalID != "DOI:10.1/atac" {
		t.Fatalf("citations = %+v", citations)
	}
}

func TestEvidenceExternalKeepsPartialPapersWhenSearchReturnsError(t *testing.T) {
	searcher := &stubSnippetSearcher{
		searchErr: research.ErrRateLimited,
		searchRes: ai_external.SearchResult{
			Sources: []ai_external.SourceID{ai_external.SourceSemanticScholar},
			Papers: []ai_external.Paper{
				{
					Source:        ai_external.SourceSemanticScholar,
					SourcePaperID: "s2-atac-search",
					SourcePaperIDs: map[ai_external.SourceID]string{
						ai_external.SourceSemanticScholar: "s2-atac-search",
					},
					Sources:  []ai_external.SourceID{ai_external.SourceSemanticScholar},
					Title:    "ATAC-seq maps chromatin accessibility",
					Abstract: "ATAC-seq identifies accessible chromatin regions using sequencing.",
					TLDR:     "ATAC-seq profiles chromatin accessibility genome-wide.",
					DOI:      "10.1/search-atac",
					Year:     2024,
					Venue:    "Genome Biology",
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
	if strings.Contains(prompt, "外部搜索失败") {
		t.Fatalf("prompt should use partial evidence instead of declaring external search failure: %s", prompt)
	}
	if !strings.Contains(prompt, "ATAC-seq maps chromatin accessibility") || !strings.Contains(prompt, "accessible chromatin regions") {
		t.Fatalf("prompt missing partial search evidence: %s", prompt)
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
	searcher := &stubSnippetSearcher{searchErr: research.ErrRateLimited}
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
