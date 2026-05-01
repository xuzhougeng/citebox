package ai_assistant

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

type stubExternalSearch struct {
	search research.PaperList
	snips  research.SnippetList
	err    error
}

func (s stubExternalSearch) Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error) {
	if s.err != nil {
		return research.PaperList{}, s.err
	}
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
			Abstract: "The full abstract states the pace of gene discovery has slowed.",
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
	if !strings.Contains(res.AnswerContext, "full abstract") {
		t.Fatalf("answer context = %s, want abstract evidence", res.AnswerContext)
	}
	if got := res.Citations[0].Snippet.Text; !strings.Contains(got, "pace of gene discovery has slowed") {
		t.Fatalf("citation snippet = %q, want abstract evidence", got)
	}
}

type limitCapturingExternalSearch struct {
	limit int
	query string
}

func (s *limitCapturingExternalSearch) Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error) {
	s.limit = opts.Limit
	s.query = query
	return research.PaperList{}, nil
}

func (s *limitCapturingExternalSearch) SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error) {
	return research.SnippetList{}, nil
}

type stubExternalPlanner struct {
	plan    ExternalSearchPlan
	err     error
	queries []string
}

func (p *stubExternalPlanner) PlanExternalSearch(ctx context.Context, query string) (ExternalSearchPlan, error) {
	p.queries = append(p.queries, query)
	if p.err != nil {
		return ExternalSearchPlan{}, p.err
	}
	return p.plan, nil
}

func TestExternalSearchToolNilSearcherReturnsFailedResult(t *testing.T) {
	res, err := NewExternalSearchTool(nil).Run(context.Background(), ToolInput{Query: "ATAC"})
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Status != "failed" {
		t.Fatalf("tool calls = %+v", res.ToolCalls)
	}
	if len(res.Process.Stages) != 1 || res.Process.Stages[0].Status != "failed" {
		t.Fatalf("process = %+v", res.Process)
	}
}

func TestExternalSearchToolClampsLimit(t *testing.T) {
	searcher := &limitCapturingExternalSearch{}
	_, err := NewExternalSearchTool(searcher).Run(context.Background(), ToolInput{Query: "ATAC", Limit: 1000})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if searcher.limit != 100 {
		t.Fatalf("limit = %d, want 100", searcher.limit)
	}
}

func TestExternalSearchToolNormalizesChineseNaturalLanguageQuery(t *testing.T) {
	searcher := &limitCapturingExternalSearch{}
	_, err := NewExternalSearchTool(searcher).Run(context.Background(), ToolInput{
		Query: "P0验收：查一下外部有没有 single-cell ATAC 综述",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if searcher.query != "single-cell ATAC-seq review" {
		t.Fatalf("query = %q, want normalized Semantic Scholar query", searcher.query)
	}
}

func TestExternalSearchToolNormalizesForwardGeneticsSourceQuery(t *testing.T) {
	searcher := &limitCapturingExternalSearch{}
	_, err := NewExternalSearchTool(searcher).Run(context.Background(), ToolInput{
		Query: "帮我给这句话找个出处：目前的正向遗传学筛选基因速度变慢了",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if searcher.query != "forward genetic screens gene discovery slowed" {
		t.Fatalf("query = %q, want forward genetics source query", searcher.query)
	}
}

func TestExternalSearchToolUsesPlannerSearchQuery(t *testing.T) {
	searcher := &limitCapturingExternalSearch{}
	planner := &stubExternalPlanner{plan: ExternalSearchPlan{
		SearchQuery: "forward genetic screens gene discovery slowed",
		Rationale:   "source request rewritten for external search",
	}}
	res, err := NewExternalSearchToolWithPlanner(searcher, planner).Run(context.Background(), ToolInput{
		Query: "帮我给这句话找个出处：遗传发现速度变慢",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if searcher.query != "forward genetic screens gene discovery slowed" {
		t.Fatalf("query = %q, want planner search query", searcher.query)
	}
	if len(planner.queries) != 1 {
		t.Fatalf("planner queries = %+v, want one call", planner.queries)
	}
	if !strings.Contains(res.ToolCalls[0].InputJSON, `"search_query":"forward genetic screens gene discovery slowed"`) {
		t.Fatalf("input json = %s, want planner search query", res.ToolCalls[0].InputJSON)
	}
}

func TestExternalSearchToolFallsBackWhenPlannerFails(t *testing.T) {
	searcher := &limitCapturingExternalSearch{}
	planner := &stubExternalPlanner{err: errors.New("planner unavailable")}
	_, err := NewExternalSearchToolWithPlanner(searcher, planner).Run(context.Background(), ToolInput{
		Query: "查一下外部有没有 single-cell ATAC 综述",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if searcher.query != "single-cell ATAC-seq review" {
		t.Fatalf("query = %q, want fallback query", searcher.query)
	}
}

func TestExternalSearchToolReportsSourceAndPlannedQuery(t *testing.T) {
	planner := &stubExternalPlanner{plan: ExternalSearchPlan{
		SearchQuery: "single-cell ATAC-seq review",
		Rationale:   "external review query",
	}}
	tool := NewExternalSearchToolWithPlanner(stubExternalSearch{
		search: research.PaperList{Items: []research.Paper{{
			PaperID: "s2-review", Title: "Single-cell ATAC review", Abstract: "Review evidence.",
		}}},
	}, planner)

	res, err := tool.Run(context.Background(), ToolInput{Query: "查一下外部有没有 single-cell ATAC 综述"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	labels := processLabels(res.Process.Stages)
	for _, want := range []string{"Master规划", "外部搜索", "命中"} {
		if !containsTestTerm(labels, want) {
			t.Fatalf("process labels = %v, missing %s", labels, want)
		}
	}
	if !strings.Contains(res.Process.Note, "Semantic Scholar") || !strings.Contains(res.Process.Note, "single-cell ATAC-seq review") {
		t.Fatalf("process note = %q, want source and planned query", res.Process.Note)
	}
	if got := stageByLabel(res.Process.Stages, "外部搜索").Detail; !strings.Contains(got, "Semantic Scholar") {
		t.Fatalf("external search stage detail = %q, want source detail", got)
	}
	if !strings.Contains(res.ToolCalls[0].OutputSummaryJSON, `"source":"Semantic Scholar"`) ||
		!strings.Contains(res.ToolCalls[0].OutputSummaryJSON, `"search_query":"single-cell ATAC-seq review"`) {
		t.Fatalf("output summary = %s, want source and search query", res.ToolCalls[0].OutputSummaryJSON)
	}
}

func TestExternalSearchToolExplainsFailureInProcessAndAnswerContext(t *testing.T) {
	tool := NewExternalSearchTool(stubExternalSearch{err: errors.New("semantic scholar timeout")})

	res, err := tool.Run(context.Background(), ToolInput{Query: "ATAC review"})
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if !strings.Contains(res.Process.Note, "外部搜索失败") || !strings.Contains(res.Process.Note, "semantic scholar timeout") {
		t.Fatalf("process note = %q, want explicit failure reason", res.Process.Note)
	}
	if !strings.Contains(res.AnswerContext, "外部搜索失败") || !strings.Contains(res.AnswerContext, "semantic scholar timeout") {
		t.Fatalf("answer context = %q, want explicit failure reason", res.AnswerContext)
	}
	if got := stageByLabel(res.Process.Stages, "外部搜索").Detail; !strings.Contains(got, "semantic scholar timeout") {
		t.Fatalf("failed stage detail = %q, want error detail", got)
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

func TestPaperReadToolNilGetterReturnsSkippedResult(t *testing.T) {
	res, err := NewPaperReadTool(nil).Run(context.Background(), ToolInput{
		Query:   "读这篇",
		Context: RequestContext{PaperID: 1},
	})
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Status != "skipped" {
		t.Fatalf("tool calls = %+v", res.ToolCalls)
	}
	if len(res.Cards) != 0 {
		t.Fatalf("cards = %+v, want empty", res.Cards)
	}
}

func TestPaperReadToolAllMissingReturnsSkippedResult(t *testing.T) {
	res, err := NewPaperReadTool(stubPaperStore{papers: map[int64]*model.Paper{}}).Run(context.Background(), ToolInput{
		Query:   "读这篇",
		Context: RequestContext{PaperIDs: []int64{1, 2}},
	})
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Status != "skipped" {
		t.Fatalf("tool calls = %+v", res.ToolCalls)
	}
	if len(res.Cards) != 0 {
		t.Fatalf("cards = %+v, want empty", res.Cards)
	}
}

func TestPaperReadToolCapsLoadedPapersAndIncludesCompareNote(t *testing.T) {
	store := stubPaperStore{
		papers: map[int64]*model.Paper{
			1: {ID: 1, Title: "Paper A", PDFText: "ATAC-seq measures chromatin accessibility."},
			2: {ID: 2, Title: "Paper B", PDFText: "scRNA-seq measures gene expression."},
			3: {ID: 3, Title: "Paper C", PDFText: "Third paper should not be loaded."},
		},
	}
	res, err := NewPaperReadTool(store).Run(context.Background(), ToolInput{
		Query:   "对比这些文献",
		Context: RequestContext{PaperIDs: []int64{1, 2, 3}},
	})
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if len(res.Cards) != 1 || res.Cards[0].Type != "paper_compare" {
		t.Fatalf("cards = %+v", res.Cards)
	}
	card, ok := res.Cards[0].Payload.(PaperCompareCard)
	if !ok {
		t.Fatalf("card payload = %T, want PaperCompareCard", res.Cards[0].Payload)
	}
	if len(card.Papers) != 2 {
		t.Fatalf("loaded papers = %d, want 2", len(card.Papers))
	}
	if card.Note == "" {
		t.Fatalf("compare note is empty")
	}
	if !strings.Contains(res.ToolCalls[0].OutputSummaryJSON, `"requested":3`) || !strings.Contains(res.ToolCalls[0].OutputSummaryJSON, `"skipped":1`) {
		t.Fatalf("output summary = %s", res.ToolCalls[0].OutputSummaryJSON)
	}
	if len(res.Process.Stages) == 0 || res.Process.Stages[0].Count != 2 {
		t.Fatalf("process = %+v", res.Process)
	}
}

func TestPaperReadToolUsesStableHitStage(t *testing.T) {
	store := stubPaperStore{
		papers: map[int64]*model.Paper{
			1: {ID: 1, Title: "Paper A", PDFText: "ATAC-seq measures chromatin accessibility."},
		},
	}
	res, err := NewPaperReadTool(store).Run(context.Background(), ToolInput{
		Query:   "读这篇 ATAC 文章",
		Context: RequestContext{PaperID: 1},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	labels := processLabels(res.Process.Stages)
	if !containsTestTerm(labels, "全文扫描") || !containsTestTerm(labels, "命中") {
		t.Fatalf("process labels = %v, want stable scan and hit labels", labels)
	}
}

type stubFigureStore struct {
	figures []FigureRecord
	limit   int
}

func (s stubFigureStore) SearchFigures(query string, paperID int64, limit int) ([]FigureRecord, int, error) {
	return s.figures, len(s.figures), nil
}

type limitCapturingFigureStore struct {
	limit int
}

func (s *limitCapturingFigureStore) SearchFigures(query string, paperID int64, limit int) ([]FigureRecord, int, error) {
	s.limit = limit
	return nil, 0, nil
}

type termSensitiveFigureStore struct {
	queries []string
}

func (s *termSensitiveFigureStore) SearchFigures(query string, paperID int64, limit int) ([]FigureRecord, int, error) {
	s.queries = append(s.queries, query)
	if strings.EqualFold(strings.TrimSpace(query), "ChIP-seq") {
		figures := []FigureRecord{{
			FigureID: 8, PaperID: 2, PaperTitle: "ChIP Paper", DisplayLabel: "Fig 3",
			ImageURL: "/api/figures/8/image", Caption: "H3K27ac ChIP-seq tracks.",
		}}
		return figures, len(figures), nil
	}
	return nil, 0, nil
}

type figureSearchCall struct {
	query   string
	paperID int64
}

type paperFallbackFigureStore struct {
	calls []figureSearchCall
}

func (s *paperFallbackFigureStore) SearchFigures(query string, paperID int64, limit int) ([]FigureRecord, int, error) {
	s.calls = append(s.calls, figureSearchCall{query: query, paperID: paperID})
	if strings.TrimSpace(query) == "" && paperID == 5 {
		figures := []FigureRecord{{
			FigureID: 11, PaperID: 5, PaperTitle: "Enhancer Paper", DisplayLabel: "Fig 2",
			ImageURL: "/api/figures/11/image", Caption: "Genome browser tracks around enhancer loci.",
		}}
		return figures, len(figures), nil
	}
	return nil, 0, nil
}

type figurePaperFallbackStore struct {
	terms []string
}

func (s *figurePaperFallbackStore) GetPaperDetail(id int64) (*model.Paper, error) {
	if id != 5 {
		return nil, nil
	}
	return &model.Paper{
		ID:      5,
		Title:   "Enhancer Paper",
		PDFText: "The results include H3K27ac ChIP-seq signal tracks across enhancer loci.",
	}, nil
}

func (s *figurePaperFallbackStore) ListEvidenceCandidatePaperIDs(terms []string, limit int) ([]int64, error) {
	s.terms = append([]string(nil), terms...)
	if containsStringFold(terms, "ChIP-seq") {
		return []int64{5}, nil
	}
	return nil, nil
}

func containsStringFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func containsFigureSearchCall(calls []figureSearchCall, query string, paperID int64) bool {
	for _, call := range calls {
		if strings.TrimSpace(call.query) == strings.TrimSpace(query) && call.paperID == paperID {
			return true
		}
	}
	return false
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

func TestFigureLookupToolExtractsSearchTermFromNaturalLanguage(t *testing.T) {
	store := &termSensitiveFigureStore{}
	tool := NewFigureLookupTool(store)

	res, err := tool.Run(context.Background(), ToolInput{Query: "在文献库中找一张ChIP-seq相关的图"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 1 || res.Cards[0].Type != "figure_result" {
		t.Fatalf("cards = %+v, queries = %v, want ChIP-seq figure hit", res.Cards, store.queries)
	}
	if !containsStringFold(store.queries, "ChIP-seq") {
		t.Fatalf("queries = %v, want extracted ChIP-seq search", store.queries)
	}
	if !strings.Contains(res.AnswerContext, "H3K27ac ChIP-seq tracks") {
		t.Fatalf("answer context = %s", res.AnswerContext)
	}
}

func TestFigureLookupToolFallsBackToFullTextCandidatePapers(t *testing.T) {
	figures := &paperFallbackFigureStore{}
	papers := &figurePaperFallbackStore{}
	tool := NewFigureLookupToolWithPapers(figures, papers)

	res, err := tool.Run(context.Background(), ToolInput{Query: "在文献库中找一张ChIP-seq相关的图"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 1 || res.Cards[0].Type != "figure_result" {
		t.Fatalf("cards = %+v, figure calls = %+v, terms = %v", res.Cards, figures.calls, papers.terms)
	}
	card, ok := res.Cards[0].Payload.(FigureResultCard)
	if !ok {
		t.Fatalf("payload = %T, want FigureResultCard", res.Cards[0].Payload)
	}
	if card.FigureID != 11 || card.PaperID != 5 {
		t.Fatalf("card = %+v, want candidate paper figure", card)
	}
	if card.EvidenceText == "" || !strings.Contains(card.EvidenceText, "H3K27ac ChIP-seq") {
		t.Fatalf("card evidence = %+v, want full-text ChIP-seq evidence", card)
	}
	if !containsFigureSearchCall(figures.calls, "", 5) {
		t.Fatalf("figure calls = %+v, want unfiltered figure listing for candidate paper", figures.calls)
	}
	if !strings.Contains(res.AnswerContext, "Full-text evidence") || !strings.Contains(res.AnswerContext, "H3K27ac ChIP-seq") {
		t.Fatalf("answer context = %s", res.AnswerContext)
	}
	if !strings.Contains(res.Process.Note, "全文候选文献") {
		t.Fatalf("process note = %q, want fallback path explanation", res.Process.Note)
	}
}

func TestFigureLookupToolNilSearcherReturnsFailedResult(t *testing.T) {
	res, err := NewFigureLookupTool(nil).Run(context.Background(), ToolInput{Query: "看图"})
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Status != "failed" {
		t.Fatalf("tool calls = %+v", res.ToolCalls)
	}
	if len(res.Process.Stages) != 1 || res.Process.Stages[0].Status != "failed" {
		t.Fatalf("process = %+v", res.Process)
	}
}

func TestFigureLookupToolClampsLimit(t *testing.T) {
	figures := &limitCapturingFigureStore{}
	_, err := NewFigureLookupTool(figures).Run(context.Background(), ToolInput{Query: "看图", Limit: 1000})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if figures.limit != 50 {
		t.Fatalf("limit = %d, want 50", figures.limit)
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
		Filename:     "figure 2.png",
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
	if figures[0].ImageURL != "/files/figures/figure%202.png" {
		t.Fatalf("image_url = %q, want filename fallback URL", figures[0].ImageURL)
	}
}

func TestRepositoryFigureSearcherNilRepoReturnsError(t *testing.T) {
	_, _, err := NewRepositoryFigureSearcher(nil).SearchFigures("ATAC", 1, 5)
	if err == nil {
		t.Fatalf("error = nil, want error")
	}
}

func processLabels(stages []ProcessStage) []string {
	labels := make([]string, 0, len(stages))
	for _, stage := range stages {
		labels = append(labels, stage.Label)
	}
	return labels
}

func stageByLabel(stages []ProcessStage, label string) ProcessStage {
	for _, stage := range stages {
		if stage.Label == label {
			return stage
		}
	}
	return ProcessStage{}
}
