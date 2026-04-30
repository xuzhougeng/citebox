package ai_assistant

import (
	"context"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

type stubExternalSearch struct {
	search research.PaperList
	snips  research.SnippetList
}

func (s stubExternalSearch) Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error) {
	return s.search, nil
}

func (s stubExternalSearch) SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error) {
	return s.snips, nil
}

func TestExternalSearchToolReturnsExternalPaperCards(t *testing.T) {
	tool := NewExternalSearchTool(stubExternalSearch{
		search: research.PaperList{Items: []research.Paper{{
			PaperID: "s2-1", Title: "ATAC Review", Year: 2024, Venue: "Genome Biology",
			ExternalIDs: research.IDs{DOI: "10.1/ext"}, TLDR: "ATAC review summary.",
		}}},
	})
	res, err := tool.Run(context.Background(), ToolInput{Query: "single-cell ATAC 综述"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 1 || res.Cards[0].Type != "external_paper" {
		t.Fatalf("cards = %+v", res.Cards)
	}
	if !strings.Contains(res.AnswerContext, "ATAC Review") {
		t.Fatalf("answer context = %s", res.AnswerContext)
	}
}

func TestPaperReadToolComparesFullText(t *testing.T) {
	store := stubPaperStore{
		ids: []int64{1, 2},
		papers: map[int64]*model.Paper{
			1: {ID: 1, Title: "Paper A", PDFText: "ATAC-seq measures chromatin accessibility."},
			2: {ID: 2, Title: "Paper B", PDFText: "scRNA-seq measures gene expression."},
		},
	}
	tool := NewPaperReadTool(store)
	res, err := tool.Run(context.Background(), ToolInput{
		Query:   "对比这两篇文献",
		Context: RequestContext{PaperIDs: []int64{1, 2}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 1 || res.Cards[0].Type != "paper_compare" {
		t.Fatalf("cards = %+v", res.Cards)
	}
	if len(res.Citations) == 0 {
		t.Fatalf("citations empty")
	}
}

type stubFigureStore struct {
	figures []FigureRecord
}

func (s stubFigureStore) SearchFigures(query string, paperID int64, limit int) ([]FigureRecord, int, error) {
	return s.figures, len(s.figures), nil
}

func TestFigureLookupToolReturnsFigureCards(t *testing.T) {
	tool := NewFigureLookupTool(stubFigureStore{
		figures: []FigureRecord{{
			FigureID: 7, PaperID: 1, PaperTitle: "ATAC Paper", DisplayLabel: "Fig 1",
			ImageURL: "/api/figures/7/image", Caption: "ATAC-seq overview", NotesText: "Important panel.",
		}},
	})
	res, err := tool.Run(context.Background(), ToolInput{Query: "看图 1", Context: RequestContext{PaperID: 1}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 1 || res.Cards[0].Type != "figure_result" {
		t.Fatalf("cards = %+v", res.Cards)
	}
	if !strings.Contains(res.AnswerContext, "ATAC-seq overview") {
		t.Fatalf("answer context = %s", res.AnswerContext)
	}
}

type capturingFigureListProvider struct {
	filter model.FigureFilter
}

func (p *capturingFigureListProvider) ListFigures(filter model.FigureFilter) ([]model.FigureListItem, int, error) {
	p.filter = filter
	return []model.FigureListItem{{
		ID:           9,
		PaperID:      42,
		PaperTitle:   "Scoped Paper",
		DisplayLabel: "Fig 2",
		Caption:      "Scoped caption",
	}}, 1, nil
}

func TestRepositoryFigureSearcherPassesPaperFilter(t *testing.T) {
	provider := &capturingFigureListProvider{}
	searcher := NewRepositoryFigureSearcher(provider)

	figures, total, err := searcher.SearchFigures("chromatin", 42, 7)
	if err != nil {
		t.Fatalf("SearchFigures: %v", err)
	}
	if total != 1 || len(figures) != 1 {
		t.Fatalf("total=%d len=%d, want 1/1", total, len(figures))
	}
	if provider.filter.Keyword != "chromatin" {
		t.Fatalf("keyword = %q, want chromatin", provider.filter.Keyword)
	}
	if provider.filter.Page != 1 || provider.filter.PageSize != 7 {
		t.Fatalf("page=%d page_size=%d, want 1/7", provider.filter.Page, provider.filter.PageSize)
	}
	if provider.filter.PaperID == nil || *provider.filter.PaperID != 42 {
		t.Fatalf("paper_id = %v, want 42", provider.filter.PaperID)
	}
}
