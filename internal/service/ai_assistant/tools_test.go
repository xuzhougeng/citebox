package ai_assistant

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/ai_external"
)

type stubExternalSearch struct {
	search ai_external.SearchResult
	err    error
}

func (s stubExternalSearch) Search(ctx context.Context, queries ai_external.SourceQueries, opts ai_external.SearchOptions) (ai_external.SearchResult, error) {
	return s.search, s.err
}

func cloneTestSourceQueries(queries ai_external.SourceQueries) ai_external.SourceQueries {
	out := make(ai_external.SourceQueries, len(queries))
	for source, sourceQueries := range queries {
		out[source] = append([]string(nil), sourceQueries...)
	}
	return out
}

func TestExternalSearchToolReturnsExternalPaperCards(t *testing.T) {
	tool := NewExternalSearchTool(stubExternalSearch{
		search: ai_external.SearchResult{Sources: []ai_external.SourceID{ai_external.SourceSemanticScholar}, Papers: []ai_external.Paper{{
			Source: ai_external.SourceSemanticScholar, SourcePaperID: "s2-1",
			SourcePaperIDs: map[ai_external.SourceID]string{ai_external.SourceSemanticScholar: "s2-1"},
			Sources:        []ai_external.SourceID{ai_external.SourceSemanticScholar},
			Title:          "ATAC Review", Year: 2024, Venue: "Genome Biology",
			DOI: "10.1/ext", TLDR: "ATAC review summary.",
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

type sourceQueryCapturingExternalSearch struct {
	mu      sync.Mutex
	queries ai_external.SourceQueries
}

func (s *sourceQueryCapturingExternalSearch) Search(ctx context.Context, queries ai_external.SourceQueries, opts ai_external.SearchOptions) (ai_external.SearchResult, error) {
	s.mu.Lock()
	s.queries = cloneTestSourceQueries(queries)
	s.mu.Unlock()
	return ai_external.SearchResult{
		Sources: []ai_external.SourceID{ai_external.SourcePubMed, ai_external.SourceSemanticScholar},
		Papers: []ai_external.Paper{{
			Source:        ai_external.SourcePubMed,
			SourcePaperID: "12345",
			SourcePaperIDs: map[ai_external.SourceID]string{
				ai_external.SourcePubMed: "12345",
			},
			Sources:      []ai_external.SourceID{ai_external.SourcePubMed},
			PMID:         "12345",
			PMCID:        "PMC12345",
			URL:          "https://pubmed.ncbi.nlm.nih.gov/12345/",
			Title:        "PubMed indexed gene discovery paper",
			Abstract:     "PubMed abstract evidence.",
			MatchedQuery: queries[ai_external.SourcePubMed][0],
			Year:         2024,
			Venue:        "PubMed Journal",
		}},
	}, nil
}

type enabledSourceQueryCapturingExternalSearch struct {
	mu      sync.Mutex
	queries ai_external.SourceQueries
	sources []ai_external.SourceID
}

func (s *enabledSourceQueryCapturingExternalSearch) EnabledExternalSources(ctx context.Context) ([]ai_external.SourceID, error) {
	return append([]ai_external.SourceID(nil), s.sources...), nil
}

func (s *enabledSourceQueryCapturingExternalSearch) Search(ctx context.Context, queries ai_external.SourceQueries, opts ai_external.SearchOptions) (ai_external.SearchResult, error) {
	s.mu.Lock()
	s.queries = cloneTestSourceQueries(queries)
	s.mu.Unlock()

	return ai_external.SearchResult{
		Sources: append([]ai_external.SourceID(nil), s.sources...),
		Papers: []ai_external.Paper{{
			Source:        ai_external.SourcePubMed,
			SourcePaperID: "12345",
			SourcePaperIDs: map[ai_external.SourceID]string{
				ai_external.SourcePubMed: "12345",
			},
			Sources:      []ai_external.SourceID{ai_external.SourcePubMed},
			PMID:         "12345",
			Title:        "PubMed indexed gene discovery paper",
			Abstract:     "PubMed abstract evidence.",
			MatchedQuery: queries[ai_external.SourcePubMed][0],
		}},
	}, nil
}

func TestExternalSearchToolReturnsPubMedSourceMetadata(t *testing.T) {
	searcher := &sourceQueryCapturingExternalSearch{}
	planner := &stubExternalPlanner{plan: ExternalSearchPlan{
		QueriesBySource: map[string][]string{
			"pubmed":           {"gene discovery PubMed query"},
			"semantic_scholar": {"gene discovery broad query"},
		},
		Rationale: "source-specific query planning",
	}}

	res, err := NewExternalSearchToolWithPlanner(searcher, planner).Run(context.Background(), ToolInput{
		Query: "找 PubMed 里的 gene discovery 证据",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := searcher.queries[ai_external.SourcePubMed], []string{"gene discovery PubMed query"}; !slices.Equal(got, want) {
		t.Fatalf("pubmed queries = %+v, want %+v", got, want)
	}
	if got, want := searcher.queries[ai_external.SourceSemanticScholar], []string{"gene discovery broad query"}; !slices.Equal(got, want) {
		t.Fatalf("semantic scholar queries = %+v, want %+v", got, want)
	}
	if len(res.Cards) != 1 {
		t.Fatalf("cards = %+v, want one PubMed card", res.Cards)
	}
	card, ok := res.Cards[0].Payload.(ExternalPaperCard)
	if !ok {
		t.Fatalf("payload = %#v", res.Cards[0].Payload)
	}
	if card.PMID != "12345" || card.URL != "https://pubmed.ncbi.nlm.nih.gov/12345/" {
		t.Fatalf("card metadata = %+v, want PMID and URL", card)
	}
	if !slices.Equal(card.Sources, []string{"PubMed"}) {
		t.Fatalf("card sources = %+v, want PubMed", card.Sources)
	}
	if got := card.SourceIDs["pubmed"]; got != "12345" {
		t.Fatalf("source ids = %+v, want pubmed id", card.SourceIDs)
	}
	if len(res.Citations) != 1 || res.Citations[0].Source != "external:PubMed" {
		t.Fatalf("citations = %+v, want PubMed citation source", res.Citations)
	}
	if res.Citations[0].ExternalID != "PMID:12345" {
		t.Fatalf("external id = %q, want PMID", res.Citations[0].ExternalID)
	}
	if !strings.Contains(res.Citations[0].Snippet.Section, "外部学术搜索: PubMed") {
		t.Fatalf("snippet section = %q", res.Citations[0].Snippet.Section)
	}
	if !strings.Contains(res.Process.Note, "PubMed") {
		t.Fatalf("process note = %q, want PubMed", res.Process.Note)
	}
}

func TestExternalSearchToolUsesOnlyEnabledSources(t *testing.T) {
	searcher := &enabledSourceQueryCapturingExternalSearch{
		sources: []ai_external.SourceID{ai_external.SourcePubMed},
	}
	planner := &stubExternalPlanner{plan: ExternalSearchPlan{
		QueriesBySource: map[string][]string{
			"pubmed":           {"gene discovery PubMed query"},
			"semantic_scholar": {"gene discovery broad query"},
		},
	}}

	res, err := NewExternalSearchToolWithPlanner(searcher, planner).Run(context.Background(), ToolInput{
		Query: "找 PubMed 里的 gene discovery 证据",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := searcher.queries[ai_external.SourcePubMed], []string{"gene discovery PubMed query"}; !slices.Equal(got, want) {
		t.Fatalf("pubmed queries = %+v, want %+v", got, want)
	}
	if _, ok := searcher.queries[ai_external.SourceSemanticScholar]; ok {
		t.Fatalf("semantic scholar should not be queried when disabled: %+v", searcher.queries)
	}
	if strings.Contains(res.Process.Note, "Semantic Scholar") {
		t.Fatalf("process note = %q, should not mention disabled Semantic Scholar", res.Process.Note)
	}
	if strings.Contains(res.ToolCalls[0].InputJSON, "semantic_scholar") || strings.Contains(res.ToolCalls[0].OutputSummaryJSON, "Semantic Scholar") {
		t.Fatalf("tool call should not mention disabled source: input=%s output=%s", res.ToolCalls[0].InputJSON, res.ToolCalls[0].OutputSummaryJSON)
	}
}

type limitCapturingExternalSearch struct {
	mu       sync.Mutex
	limit    int
	query    string
	queries  []string
	bySource ai_external.SourceQueries
}

func (s *limitCapturingExternalSearch) Search(ctx context.Context, queries ai_external.SourceQueries, opts ai_external.SearchOptions) (ai_external.SearchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limit = opts.Limit
	s.bySource = cloneTestSourceQueries(queries)
	s.queries = flattenExternalSourceQueries(queries)
	if len(s.queries) > 0 {
		s.query = s.queries[0]
	}
	return ai_external.SearchResult{Sources: sourceIDsInQueries(queries)}, nil
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

type stubAISettingsProvider struct {
	settings model.AISettings
	err      error
}

func (p stubAISettingsProvider) GetSettings() (*model.AISettings, error) {
	if p.err != nil {
		return nil, p.err
	}
	settings := p.settings
	return &settings, nil
}

type stubNonStreamCaller struct {
	output       string
	settingsSeen model.AISettings
	systemPrompt string
	userPrompt   string
}

func (c *stubNonStreamCaller) CallProviderGeneric(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string) (string, string, error) {
	c.settingsSeen = settings
	c.systemPrompt = systemPrompt
	c.userPrompt = userPrompt
	return c.output, "", nil
}

type queryRoutingExternalSearch struct {
	mu       sync.Mutex
	queries  []string
	bySource ai_external.SourceQueries
	results  map[string][]ai_external.Paper
}

func (s *queryRoutingExternalSearch) Search(ctx context.Context, queries ai_external.SourceQueries, opts ai_external.SearchOptions) (ai_external.SearchResult, error) {
	s.mu.Lock()
	s.bySource = cloneTestSourceQueries(queries)
	s.queries = flattenExternalSourceQueries(queries)
	s.mu.Unlock()

	results := make([]ai_external.SourceResult, 0)
	for _, source := range orderedExternalSearchSources(queries) {
		for _, query := range queries[source] {
			results = append(results, ai_external.SourceResult{
				Source: source,
				Query:  query,
				Papers: s.results[query],
			})
		}
	}
	return ai_external.SearchResult{
		Sources: sourceIDsInQueries(queries),
		Results: results,
		Papers:  ai_external.MergePapers(results, sourceIDsInQueries(queries), opts.Limit),
	}, nil
}

type stubExternalClassifier struct {
	mu      sync.Mutex
	inputs  []ExternalPaperClassificationInput
	results map[string]ExternalPaperClassificationResult
	err     error
}

func (s *stubExternalClassifier) ClassifyExternalPaper(ctx context.Context, in ExternalPaperClassificationInput) (ExternalPaperClassificationResult, error) {
	s.mu.Lock()
	s.inputs = append(s.inputs, in)
	s.mu.Unlock()
	if s.err != nil {
		return ExternalPaperClassificationResult{}, s.err
	}
	if res, ok := s.results[in.Paper.PaperID]; ok {
		return res, nil
	}
	return ExternalPaperClassificationResult{Relevant: false, Reason: "unrelated"}, nil
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
	if !containsTestTerm(searcher.queries, "forward genetic screens gene discovery slowed") {
		t.Fatalf("queries = %q, want forward genetics source query", searcher.queries)
	}
	if !containsTestTerm(searcher.queries, "gene discovery slowed forward genetic screens saturation") {
		t.Fatalf("queries = %q, want source-oriented fallback query", searcher.queries)
	}
}

func TestExternalSearchQueriesForLongForwardGeneticsClaimIncludeRecallVariants(t *testing.T) {
	queries := ExternalSearchQueries("寻找出处：然而，由于模式物种中正向遗传筛选的饱和以及基因家族内普遍存在的冗余，基因发现——尤其是那些影响细胞命运的基因——在过去十年中显著减少。")
	for _, want := range []string{
		"forward genetic screens gene discovery slowed",
		"gene discovery slowed forward genetic screens saturation",
		"plant gene discovery slowed forward genetic screens",
	} {
		if !containsTestTerm(queries, want) {
			t.Fatalf("queries = %+v, missing %q", queries, want)
		}
	}
}

func TestExternalSearchPlanQueriesForSource(t *testing.T) {
	fallback := []string{"fallback external query"}
	plan := ExternalSearchPlan{
		QueriesBySource: map[string][]string{
			"pubmed": {
				"CRISPR therapy MeSH",
				" ",
				"CRISPR therapy MeSH",
			},
			"semantic_scholar": {
				"CRISPR gene editing therapy",
				"genome editing clinical trials",
			},
		},
	}

	if got, want := plan.QueriesForSource("pubmed", fallback), []string{"CRISPR therapy MeSH"}; !slices.Equal(got, want) {
		t.Fatalf("pubmed queries = %+v, want %+v", got, want)
	}
	if got, want := plan.QueriesForSource("semantic_scholar", fallback), []string{"CRISPR gene editing therapy", "genome editing clinical trials"}; !slices.Equal(got, want) {
		t.Fatalf("semantic scholar queries = %+v, want %+v", got, want)
	}
	if got := plan.QueriesForSource("missing_source", fallback); !slices.Equal(got, fallback) {
		t.Fatalf("missing source queries = %+v, want fallback %+v", got, fallback)
	}
}

func TestExternalPlannerReturnsQueriesBySource(t *testing.T) {
	caller := &stubNonStreamCaller{output: `{
		"queries_by_source": {
			" PubMed ": [" CRISPR therapy MeSH ", "gene editing adverse effects", "CRISPR therapy MeSH"],
			"semantic_scholar": ["CRISPR gene editing therapy", "genome editing clinical trials"]
		},
		"rationale": "source-specific biomedical and broad academic coverage"
	}`}
	planner := NewLLMExternalSearchPlanner(stubAISettingsProvider{settings: model.DefaultAISettings()}, caller)

	plan, err := planner.PlanExternalSearch(context.Background(), "查找 CRISPR 治疗副作用的出处")
	if err != nil {
		t.Fatalf("PlanExternalSearch: %v", err)
	}
	if got, want := plan.QueriesBySource["pubmed"], []string{"CRISPR therapy MeSH", "gene editing adverse effects"}; !slices.Equal(got, want) {
		t.Fatalf("pubmed queries = %+v, want %+v", got, want)
	}
	if got, want := plan.QueriesBySource["semantic_scholar"], []string{"CRISPR gene editing therapy", "genome editing clinical trials"}; !slices.Equal(got, want) {
		t.Fatalf("semantic scholar queries = %+v, want %+v", got, want)
	}
	if len(plan.SearchQueries) != 0 || plan.SearchQuery != "" {
		t.Fatalf("global queries = %q/%+v, want empty when source-specific queries are present", plan.SearchQuery, plan.SearchQueries)
	}
	if !strings.Contains(caller.systemPrompt, `"queries_by_source"`) ||
		!strings.Contains(caller.systemPrompt, "pubmed") ||
		!strings.Contains(caller.systemPrompt, "semantic_scholar") {
		t.Fatalf("system prompt does not describe source-specific JSON: %s", caller.systemPrompt)
	}
}

func TestExternalPlannerReturnsGoalConstraintsAndTargetYear(t *testing.T) {
	caller := &stubNonStreamCaller{output: `{
		"search_goal": " Evidence ",
		"must_match": ["CRISPR", " retinal degeneration ", "CRISPR"],
		"soft_preferences": ["AAV", "mouse model", "AAV"],
		"target_year": 2024,
		"queries_by_source": {
			"pubmed": ["CRISPR retinal degeneration AAV"],
			"semantic_scholar": ["CRISPR retinal degeneration mouse model"]
		},
		"rationale": "preserve the core claim while keeping expansion terms soft"
	}`}
	planner := NewLLMExternalSearchPlanner(stubAISettingsProvider{settings: model.DefaultAISettings()}, caller)

	plan, err := planner.PlanExternalSearch(context.Background(), "Find evidence for CRISPR retinal degeneration therapy in 2024")
	if err != nil {
		t.Fatalf("PlanExternalSearch: %v", err)
	}
	if got, want := plan.SearchGoal, ExternalSearchGoalEvidence; got != want {
		t.Fatalf("search goal = %q, want %q", got, want)
	}
	if got, want := plan.MustMatch, []string{"CRISPR", "retinal degeneration"}; !slices.Equal(got, want) {
		t.Fatalf("must match = %+v, want %+v", got, want)
	}
	if got, want := plan.SoftPreferences, []string{"AAV", "mouse model"}; !slices.Equal(got, want) {
		t.Fatalf("soft preferences = %+v, want %+v", got, want)
	}
	if got, want := plan.TargetYear, 2024; got != want {
		t.Fatalf("target year = %d, want %d", got, want)
	}
	if got, want := plan.QueriesBySource["pubmed"], []string{"CRISPR retinal degeneration AAV"}; !slices.Equal(got, want) {
		t.Fatalf("pubmed queries = %+v, want %+v", got, want)
	}
	if got, want := plan.QueriesBySource["semantic_scholar"], []string{"CRISPR retinal degeneration mouse model"}; !slices.Equal(got, want) {
		t.Fatalf("semantic scholar queries = %+v, want %+v", got, want)
	}
	if !strings.Contains(caller.systemPrompt, `"search_goal"`) ||
		!strings.Contains(caller.systemPrompt, `"must_match"`) ||
		!strings.Contains(caller.systemPrompt, `"soft_preferences"`) ||
		!strings.Contains(caller.systemPrompt, `"target_year"`) {
		t.Fatalf("system prompt does not describe richer planner JSON: %s", caller.systemPrompt)
	}
}

func TestNormalizeExternalSearchGoalDefaultsToDiscovery(t *testing.T) {
	for _, raw := range []string{"", " ", "unknown", "DISCOVERY", " discovery "} {
		got := normalizeExternalSearchGoal(raw)
		want := ExternalSearchGoalDiscovery
		if raw == "DISCOVERY" || raw == " discovery " {
			want = ExternalSearchGoalDiscovery
		}
		if got != want {
			t.Fatalf("normalizeExternalSearchGoal(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestFallbackExternalSearchGoalTreatsQuotedClaimAsEvidence(t *testing.T) {
	for _, query := range []string{
		`Find a source for "gene discovery has slowed over the past decade"`,
		`10.1038/nature12373`,
		`PMID: 12345678`,
		`arXiv:2401.01234`,
	} {
		if got, want := fallbackExternalSearchGoal(query), ExternalSearchGoalEvidence; got != want {
			t.Fatalf("fallbackExternalSearchGoal(%q) = %q, want %q", query, got, want)
		}
	}
	if got, want := fallbackExternalSearchGoal("recent CRISPR delivery review"), ExternalSearchGoalDiscovery; got != want {
		t.Fatalf("fallbackExternalSearchGoal(discovery query) = %q, want %q", got, want)
	}
}

func TestExternalSearchQueriesPlannerFailureReturnsFallbackGoalPlan(t *testing.T) {
	query := `Find a source for "gene discovery has slowed over the past decade"`
	tool := &ExternalSearchTool{
		planner: &stubExternalPlanner{err: errors.New("planner unavailable")},
	}

	sourceQueries, plan, err := tool.searchQueries(context.Background(), query)
	if err == nil {
		t.Fatal("searchQueries error = nil, want planner failure")
	}
	if got, want := plan.SearchGoal, fallbackExternalSearchGoal(query); got != want {
		t.Fatalf("search goal = %q, want fallback %q", got, want)
	}
	if got, want := plan.SearchQuery, ""; got != want {
		t.Fatalf("search query = %q, want %q on planner failure fallback plan", got, want)
	}
	if got, want := sourceQueries[ai_external.SourcePubMed], ExternalSearchQueries(query); !slices.Equal(got, want) {
		t.Fatalf("pubmed fallback queries = %+v, want %+v", got, want)
	}
	if got, want := sourceQueries[ai_external.SourceSemanticScholar], ExternalSearchQueries(query); !slices.Equal(got, want) {
		t.Fatalf("semantic scholar fallback queries = %+v, want %+v", got, want)
	}
}

func TestExternalSearchQueriesInvalidSearchGoalFallsBackToQueryHeuristic(t *testing.T) {
	query := `Find a source for "gene discovery has slowed over the past decade"`
	tool := &ExternalSearchTool{
		planner: &stubExternalPlanner{plan: ExternalSearchPlan{
			SearchGoal: "unsupported",
			QueriesBySource: map[string][]string{
				"pubmed":           {"gene discovery slowed forward genetic screens"},
				"semantic_scholar": {"gene discovery slowed model organisms"},
			},
		}},
	}

	sourceQueries, plan, err := tool.searchQueries(context.Background(), query)
	if err != nil {
		t.Fatalf("searchQueries: %v", err)
	}
	if got, want := plan.SearchGoal, fallbackExternalSearchGoal(query); got != want {
		t.Fatalf("search goal = %q, want fallback %q", got, want)
	}
	if got, want := sourceQueries[ai_external.SourcePubMed], []string{"gene discovery slowed forward genetic screens"}; !slices.Equal(got, want) {
		t.Fatalf("pubmed queries = %+v, want %+v", got, want)
	}
	if got, want := sourceQueries[ai_external.SourceSemanticScholar], []string{"gene discovery slowed model organisms"}; !slices.Equal(got, want) {
		t.Fatalf("semantic scholar queries = %+v, want %+v", got, want)
	}
}

func TestNormalizeExternalQueriesBySourceMergesDuplicateAliasesDeterministically(t *testing.T) {
	in := map[string][]string{
		"PubMed": {
			"mixed tertiary",
			"mixed fourth",
			"mixed fifth",
		},
		" pubmed ": {
			" spaced secondary ",
			"duplicate term",
		},
		"pubmed": {
			"exact primary",
			"duplicate term",
		},
		" Semantic_Scholar ": {
			"broad academic query",
		},
	}
	wantPubMed := []string{"exact primary", "duplicate term", "spaced secondary", "mixed tertiary"}
	wantSemanticScholar := []string{"broad academic query"}

	for i := 0; i < 20; i++ {
		got := normalizeExternalQueriesBySource(in)
		if !slices.Equal(got["pubmed"], wantPubMed) {
			t.Fatalf("run %d pubmed queries = %+v, want %+v", i, got["pubmed"], wantPubMed)
		}
		if !slices.Equal(got["semantic_scholar"], wantSemanticScholar) {
			t.Fatalf("run %d semantic scholar queries = %+v, want %+v", i, got["semantic_scholar"], wantSemanticScholar)
		}
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
	if !containsTestTerm(searcher.queries, "forward genetic screens gene discovery slowed") {
		t.Fatalf("queries = %+v, want planner search query", searcher.queries)
	}
	if len(planner.queries) != 1 {
		t.Fatalf("planner queries = %+v, want one call", planner.queries)
	}
	if !strings.Contains(res.ToolCalls[0].InputJSON, `"search_query":"forward genetic screens gene discovery slowed"`) {
		t.Fatalf("input json = %s, want planner search query", res.ToolCalls[0].InputJSON)
	}
}

func TestExternalSearchToolMergesPlannerAndFallbackQueries(t *testing.T) {
	searcher := &limitCapturingExternalSearch{}
	planner := &stubExternalPlanner{plan: ExternalSearchPlan{
		SearchQuery: "cell fate gene discovery decline forward genetic screens redundancy",
		SearchQueries: []string{
			"model organisms forward genetics saturation gene discovery",
			"gene family redundancy forward genetic screens developmental genes",
		},
		Rationale: "planner query is specific but may be too narrow",
	}}
	_, err := NewExternalSearchToolWithPlanner(searcher, planner).Run(context.Background(), ToolInput{
		Query: "寻找出处：然而，由于模式物种中正向遗传筛选的饱和以及基因家族内普遍存在的冗余，基因发现——尤其是那些影响细胞命运的基因——在过去十年中显著减少。",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !containsTestTerm(searcher.queries, "cell fate gene discovery decline forward genetic screens redundancy") {
		t.Fatalf("queries = %+v, missing planner primary", searcher.queries)
	}
	if !containsTestTerm(searcher.queries, "gene discovery slowed forward genetic screens saturation") {
		t.Fatalf("queries = %+v, missing fallback recall query", searcher.queries)
	}
}

func TestExternalSearchToolAddsRecallQueriesFromPlannerTerms(t *testing.T) {
	searcher := &limitCapturingExternalSearch{}
	planner := &stubExternalPlanner{plan: ExternalSearchPlan{
		SearchQuery: "model organisms forward genetic screens gene discovery decline",
		SearchQueries: []string{
			"functional redundancy gene families forward genetics screens",
		},
		Rationale: "translated source query",
	}}
	_, err := NewExternalSearchToolWithPlanner(searcher, planner).Run(context.Background(), ToolInput{
		Query: "source request in another language",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !containsTestTerm(searcher.queries, "gene discovery slowed forward genetic screens saturation") {
		t.Fatalf("queries = %+v, missing recall query derived from planner terms", searcher.queries)
	}
}

func TestExternalSearchToolRunsMultipleQueriesAndFiltersWithClassifier(t *testing.T) {
	correct := ai_external.Paper{
		Source:        ai_external.SourceSemanticScholar,
		SourcePaperID: "cell-2025",
		SourcePaperIDs: map[ai_external.SourceID]string{
			ai_external.SourceSemanticScholar: "cell-2025",
		},
		Sources:  []ai_external.SourceID{ai_external.SourceSemanticScholar},
		Title:    "A unified cell atlas of vascular plants reveals cell-type foundational genes and accelerates gene discovery.",
		Year:     2025,
		Venue:    "Cell",
		Abstract: "The pace of gene discovery in plants has slowed as forward genetic screens reach saturation.",
	}
	searcher := &queryRoutingExternalSearch{results: map[string][]ai_external.Paper{
		"forward genetic screens saturation gene discovery decline": {
			{Source: ai_external.SourceSemanticScholar, SourcePaperID: "pamir", Title: "pamiR", TLDR: "Forward genetics without functional genetic redundancy."},
		},
		"gene discovery slowed forward genetic screens saturation": {
			correct,
			{Source: ai_external.SourceSemanticScholar, SourcePaperID: "root-fate", Title: "Regulation of cell fate", TLDR: "Cell fate segregation."},
		},
	}}
	planner := &stubExternalPlanner{plan: ExternalSearchPlan{
		SearchQuery: "forward genetic screens saturation gene discovery decline",
		SearchQueries: []string{
			"forward genetic screens saturation gene discovery decline",
			"gene discovery slowed forward genetic screens saturation",
		},
		Rationale: "split precise and recall queries",
	}}
	classifier := &stubExternalClassifier{results: map[string]ExternalPaperClassificationResult{
		"cell-2025": {
			Relevant: true,
			Reason:   "摘要直接说明 gene discovery slowed and forward genetic screens reach saturation.",
			Annotations: []ExternalEvidenceAnnotation{{
				Claim:     "基因发现速度变慢",
				Evidence:  "The pace of gene discovery in plants has slowed as forward genetic screens reach saturation.",
				Verdict:   "supported",
				Rationale: "该句直接对应原句核心断言。",
			}},
		},
	}}

	res, err := NewExternalSearchToolWithAgents(searcher, planner, classifier).Run(context.Background(), ToolInput{
		Query: "寻找出处：正向遗传筛选饱和导致基因发现减少",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !containsTestTerm(searcher.queries, "forward genetic screens saturation gene discovery decline") ||
		!containsTestTerm(searcher.queries, "gene discovery slowed forward genetic screens saturation") {
		t.Fatalf("queries = %+v", searcher.queries)
	}
	if len(res.Cards) != 1 {
		t.Fatalf("cards = %+v, want one classified hit", res.Cards)
	}
	card, ok := res.Cards[0].Payload.(ExternalPaperCard)
	if !ok {
		t.Fatalf("payload = %#v", res.Cards[0].Payload)
	}
	if card.S2PaperID != "cell-2025" || len(card.EvidenceAnnotations) == 0 {
		t.Fatalf("card = %+v, want annotated Cell hit", card)
	}
	if !strings.Contains(res.AnswerContext, "Evidence annotations") ||
		!strings.Contains(res.AnswerContext, "gene discovery in plants has slowed") {
		t.Fatalf("answer context = %s", res.AnswerContext)
	}
	if got := stageByLabel(res.Process.Stages, "Sub-Agent判定").Count; got != 3 {
		t.Fatalf("classified count = %d, want 3", got)
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
		search: ai_external.SearchResult{Sources: []ai_external.SourceID{ai_external.SourcePubMed, ai_external.SourceSemanticScholar}, Papers: []ai_external.Paper{{
			Source: ai_external.SourceSemanticScholar, SourcePaperID: "s2-review",
			SourcePaperIDs: map[ai_external.SourceID]string{ai_external.SourceSemanticScholar: "s2-review"},
			Sources:        []ai_external.SourceID{ai_external.SourceSemanticScholar},
			Title:          "Single-cell ATAC review", Abstract: "Review evidence.",
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
	if !strings.Contains(res.Process.Note, "PubMed") || !strings.Contains(res.Process.Note, "Semantic Scholar") || !strings.Contains(res.Process.Note, "single-cell ATAC-seq review") {
		t.Fatalf("process note = %q, want source and planned query", res.Process.Note)
	}
	if got := stageByLabel(res.Process.Stages, "外部搜索").Detail; !strings.Contains(got, "PubMed") || !strings.Contains(got, "Semantic Scholar") {
		t.Fatalf("external search stage detail = %q, want source detail", got)
	}
	if !strings.Contains(res.ToolCalls[0].OutputSummaryJSON, `"sources":["PubMed","Semantic Scholar"]`) ||
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

func TestExternalSearchToolHonoursUserSourcesOverride(t *testing.T) {
	searcher := &enabledSourceQueryCapturingExternalSearch{
		sources: []ai_external.SourceID{ai_external.SourcePubMed, ai_external.SourceSemanticScholar},
	}
	planner := &stubExternalPlanner{plan: ExternalSearchPlan{
		QueriesBySource: map[string][]string{
			"pubmed":           {"q1"},
			"semantic_scholar": {"q2"},
		},
	}}
	tool := NewExternalSearchToolWithPlanner(searcher, planner)

	res, err := tool.Run(context.Background(), ToolInput{
		Query:   "找一下",
		Sources: []string{"pubmed"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := searcher.queries[ai_external.SourcePubMed]; !ok {
		t.Fatalf("expected PubMed in queries, got %+v", searcher.queries)
	}
	if _, ok := searcher.queries[ai_external.SourceSemanticScholar]; ok {
		t.Fatalf("Semantic Scholar should be excluded by user override, got %+v", searcher.queries)
	}
	// User did not request a disabled source, so no disabled detail should leak.
	if strings.Contains(res.Process.Note, "未启用") {
		t.Fatalf("process note should not mention 未启用, got %q", res.Process.Note)
	}
}

func TestExternalSearchToolReportsDisabledSourceWhenExplicitlyRequested(t *testing.T) {
	searcher := &enabledSourceQueryCapturingExternalSearch{
		sources: []ai_external.SourceID{ai_external.SourcePubMed}, // S2 NOT enabled
	}
	planner := &stubExternalPlanner{plan: ExternalSearchPlan{
		QueriesBySource: map[string][]string{
			"pubmed":           {"q1"},
			"semantic_scholar": {"q2"},
		},
	}}
	tool := NewExternalSearchToolWithPlanner(searcher, planner)

	res, err := tool.Run(context.Background(), ToolInput{
		Query:   "找一下",
		Sources: []string{"pubmed", "semantic_scholar"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// PubMed should still be queried.
	if _, ok := searcher.queries[ai_external.SourcePubMed]; !ok {
		t.Fatalf("expected PubMed in queries, got %+v", searcher.queries)
	}
	// Process note must mention that Semantic Scholar was disabled.
	if !strings.Contains(res.Process.Note, "Semantic Scholar") || !strings.Contains(res.Process.Note, "未启用") {
		t.Fatalf("expected note to mention disabled Semantic Scholar, got %q", res.Process.Note)
	}
	if strings.Contains(res.Process.Note, "ai_assistant:") {
		t.Fatalf("internal sentinel leaked into note: %q", res.Process.Note)
	}
}

func TestExternalSearchToolEmptyUserSourcesPreservesDefault(t *testing.T) {
	searcher := &enabledSourceQueryCapturingExternalSearch{
		sources: []ai_external.SourceID{ai_external.SourcePubMed, ai_external.SourceSemanticScholar},
	}
	planner := &stubExternalPlanner{plan: ExternalSearchPlan{
		QueriesBySource: map[string][]string{
			"pubmed":           {"q1"},
			"semantic_scholar": {"q2"},
		},
	}}
	tool := NewExternalSearchToolWithPlanner(searcher, planner)

	_, err := tool.Run(context.Background(), ToolInput{Query: "找一下"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Both sources should appear in queries when user gave no explicit Sources.
	if _, ok := searcher.queries[ai_external.SourcePubMed]; !ok {
		t.Fatalf("expected PubMed in queries when Sources is nil, got %+v", searcher.queries)
	}
	if _, ok := searcher.queries[ai_external.SourceSemanticScholar]; !ok {
		t.Fatalf("expected Semantic Scholar in queries when Sources is nil, got %+v", searcher.queries)
	}
}

type onlyEnabledStubExternalSearch struct {
	enabled []ai_external.SourceID
}

func (s *onlyEnabledStubExternalSearch) EnabledExternalSources(ctx context.Context) ([]ai_external.SourceID, error) {
	return append([]ai_external.SourceID(nil), s.enabled...), nil
}

func (s *onlyEnabledStubExternalSearch) Search(ctx context.Context, queries ai_external.SourceQueries, opts ai_external.SearchOptions) (ai_external.SearchResult, error) {
	// Mimic the real service's failure when no queries are present.
	hasQueries := false
	for _, qs := range queries {
		if len(qs) > 0 {
			hasQueries = true
			break
		}
	}
	if !hasQueries {
		return ai_external.SearchResult{}, ai_external.ErrNoSourcesEnabled
	}
	return ai_external.SearchResult{
		Sources: append([]ai_external.SourceID(nil), s.enabled...),
		Papers: []ai_external.Paper{{
			Source:        ai_external.SourcePubMed,
			SourcePaperID: "1",
			Title:         "P",
		}},
	}, nil
}

func TestExternalSearchToolOnlyDisabledRequestedSurfacesError(t *testing.T) {
	searcher := &onlyEnabledStubExternalSearch{
		enabled: []ai_external.SourceID{ai_external.SourcePubMed},
	}
	tool := NewExternalSearchTool(searcher)
	res, err := tool.Run(context.Background(), ToolInput{
		Query:   "找一下",
		Sources: []string{"semantic_scholar"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The disabled detail must be visible somewhere reachable by the user.
	combined := res.Process.Note + " " + res.AnswerContext
	if !strings.Contains(combined, "Semantic Scholar") {
		t.Fatalf("expected Semantic Scholar in note/answer, got note=%q answer=%q", res.Process.Note, res.AnswerContext)
	}
	// And the internal sentinel must NOT leak into either user-facing string.
	if strings.Contains(combined, "ai_assistant:") {
		t.Fatalf("internal sentinel leaked: note=%q answer=%q", res.Process.Note, res.AnswerContext)
	}
}
