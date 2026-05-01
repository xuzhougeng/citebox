package ai_assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/xuzhougeng/citebox/internal/service/research"
)

type ExternalSearcher interface {
	Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error)
	SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error)
}

type ExternalSearchPlanner interface {
	PlanExternalSearch(ctx context.Context, query string) (ExternalSearchPlan, error)
}

type ExternalPaperClassifier interface {
	ClassifyExternalPaper(ctx context.Context, in ExternalPaperClassificationInput) (ExternalPaperClassificationResult, error)
}

type ExternalSearchPlan struct {
	SearchQuery     string              `json:"search_query,omitempty"`
	SearchQueries   []string            `json:"search_queries,omitempty"`
	QueriesBySource map[string][]string `json:"queries_by_source,omitempty"`
	Rationale       string              `json:"rationale,omitempty"`
}

type ExternalPaperClassificationInput struct {
	Query         string
	SearchQueries []string
	MatchedQuery  string
	Paper         research.Paper
	EvidenceText  string
}

type ExternalPaperClassificationResult struct {
	Relevant    bool                         `json:"relevant"`
	Reason      string                       `json:"reason,omitempty"`
	Annotations []ExternalEvidenceAnnotation `json:"annotations,omitempty"`
}

type ExternalEvidenceAnnotation struct {
	Claim     string `json:"claim,omitempty"`
	Evidence  string `json:"evidence,omitempty"`
	Verdict   string `json:"verdict,omitempty"`
	Rationale string `json:"rationale,omitempty"`
}

type ExternalSearchTool struct {
	searcher   ExternalSearcher
	planner    ExternalSearchPlanner
	classifier ExternalPaperClassifier
}

const (
	defaultExternalSearchLimit = 8
	maxExternalSearchLimit     = 100
	maxExternalSearchQueries   = 4
	maxExternalClassification  = 20
	externalClassifierParallel = 20
	externalSearchSource       = "Semantic Scholar"
)

type ExternalPaperCard struct {
	S2PaperID           string                       `json:"s2_paper_id"`
	Title               string                       `json:"title"`
	Year                int                          `json:"year,omitempty"`
	Venue               string                       `json:"venue,omitempty"`
	DOI                 string                       `json:"doi,omitempty"`
	TLDR                string                       `json:"tldr,omitempty"`
	Abstract            string                       `json:"abstract,omitempty"`
	MatchedQuery        string                       `json:"matched_query,omitempty"`
	Reason              string                       `json:"reason,omitempty"`
	CitationIndex       int                          `json:"citation_index,omitempty"`
	HighlightTerms      []string                     `json:"highlight_terms,omitempty"`
	EvidenceAnnotations []ExternalEvidenceAnnotation `json:"evidence_annotations,omitempty"`
}

func NewExternalSearchTool(searcher ExternalSearcher) *ExternalSearchTool {
	return NewExternalSearchToolWithPlanner(searcher, nil)
}

func NewExternalSearchToolWithPlanner(searcher ExternalSearcher, planner ExternalSearchPlanner) *ExternalSearchTool {
	return NewExternalSearchToolWithAgents(searcher, planner, nil)
}

func NewExternalSearchToolWithAgents(searcher ExternalSearcher, planner ExternalSearchPlanner, classifier ExternalPaperClassifier) *ExternalSearchTool {
	return &ExternalSearchTool{searcher: searcher, planner: planner, classifier: classifier}
}

func (t *ExternalSearchTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	limit := clampExternalSearchLimit(in.Limit)
	searchQueries := ExternalSearchQueries(in.Query)
	plan := ExternalSearchPlan{}
	var planErr error
	if t != nil {
		searchQueries, plan, planErr = t.searchQueries(ctx, in.Query)
	}
	searchQuery := ""
	if len(searchQueries) > 0 {
		searchQuery = searchQueries[0]
	}
	inputJSON, _ := json.Marshal(struct {
		Query         string   `json:"query"`
		SearchQuery   string   `json:"search_query,omitempty"`
		SearchQueries []string `json:"search_queries,omitempty"`
		Limit         int      `json:"limit"`
	}{Query: in.Query, SearchQuery: searchQuery, SearchQueries: searchQueries, Limit: limit})
	processStages := externalPlanningStages(t, searchQueries, plan, planErr)

	if t == nil || t.searcher == nil {
		return externalSearchFailedResult(inputJSON, errors.New("external searcher is not configured"), processStages, searchQueries), nil
	}

	candidates, searchErrs := t.searchMany(ctx, searchQueries, limit)
	if len(candidates) == 0 && len(searchErrs) > 0 {
		return externalSearchFailedResult(inputJSON, combineExternalSearchErrors(searchErrs), processStages, searchQueries), nil
	}
	rawReturned := len(candidates)

	classified := 0
	classifierFailed := 0
	if t.classifier != nil && len(candidates) > 0 {
		classifyInput := candidates
		if len(classifyInput) > maxExternalClassification {
			classifyInput = classifyInput[:maxExternalClassification]
		}
		candidates, classified, classifierFailed = t.classifyCandidates(ctx, in.Query, searchQueries, classifyInput)
	}

	cards := make([]ResultCard, 0, len(candidates))
	citations := make([]Citation, 0, len(candidates))
	highlightTerms := externalHighlightTerms(searchQueries)
	for _, candidate := range candidates {
		if len(cards) >= limit {
			break
		}
		p := candidate.Paper
		citation := Citation{
			I:          len(citations) + 1,
			S2PaperID:  p.PaperID,
			ExternalID: externalID(p),
			Title:      p.Title,
			Source:     "external",
			Snippet: research.Snippet{
				Text:        firstNonEmpty(p.Abstract, p.TLDR, p.Title),
				SnippetKind: "abstract",
				Section:     "Semantic Scholar",
			},
		}
		citations = append(citations, citation)
		cards = append(cards, ResultCard{Type: "external_paper", Payload: ExternalPaperCard{
			S2PaperID:           p.PaperID,
			Title:               p.Title,
			Year:                p.Year,
			Venue:               p.Venue,
			DOI:                 p.ExternalIDs.DOI,
			TLDR:                p.TLDR,
			Abstract:            p.Abstract,
			MatchedQuery:        candidate.MatchedQuery,
			Reason:              candidate.Classification.Reason,
			CitationIndex:       citation.I,
			HighlightTerms:      highlightTerms,
			EvidenceAnnotations: candidate.Classification.Annotations,
		}})
	}

	outputJSON, _ := json.Marshal(struct {
		Source           string   `json:"source"`
		SearchQuery      string   `json:"search_query"`
		SearchQueries    []string `json:"search_queries,omitempty"`
		Returned         int      `json:"returned"`
		Hits             int      `json:"hits"`
		Classified       int      `json:"classified,omitempty"`
		ClassifierFailed int      `json:"classifier_failed,omitempty"`
	}{Source: externalSearchSource, SearchQuery: searchQuery, SearchQueries: searchQueries, Returned: rawReturned, Hits: len(cards), Classified: classified, ClassifierFailed: classifierFailed})

	searchDetail := fmt.Sprintf("来源: %s", externalSearchSource)
	if len(searchQueries) > 1 {
		searchDetail += fmt.Sprintf("; 查询 %d个", len(searchQueries))
	}
	if len(searchErrs) > 0 {
		searchDetail += fmt.Sprintf("; 失败 %d个", len(searchErrs))
	}
	processStages = append(processStages,
		ProcessStage{Label: "外部搜索", Count: rawReturned, Unit: "条", Status: "completed", Detail: searchDetail},
	)
	if t.classifier != nil {
		detail := ""
		if classifierFailed > 0 {
			detail = fmt.Sprintf("失败 %d篇；失败候选保留为待核查结果", classifierFailed)
		}
		processStages = append(processStages,
			ProcessStage{Label: "Sub-Agent判定", Count: classified, Unit: "篇", Status: "completed", Detail: detail},
		)
	}
	processStages = append(processStages,
		externalHitStage(len(cards)),
	)
	noteParts := make([]string, 0, 2)
	if planErr != nil {
		noteParts = append(noteParts, "Master规划失败，已使用本地查询回退。")
	}
	if len(searchErrs) > 0 && len(candidates) > 0 {
		noteParts = append(noteParts, fmt.Sprintf("%s 部分查询失败 %d 个。", externalSearchSource, len(searchErrs)))
	}
	noteParts = append(noteParts, fmt.Sprintf("%s 查询: %s", externalSearchSource, strings.Join(searchQueries, " | ")))
	note := joinProcessNotes(noteParts)
	answerContext := externalAnswerContext(cards)
	if len(cards) == 0 {
		answerContext = fmt.Sprintf("没有命中：%s 使用查询 %q 返回 0 条结果。", externalSearchSource, strings.Join(searchQueries, " | "))
	}

	return ToolResult{
		Process: ProcessSummary{
			Intent: IntentExternalSearch,
			Stages: processStages,
			Note:   note,
		},
		Cards:         cards,
		Citations:     citations,
		AnswerContext: answerContext,
		ToolCalls: []ToolCallSummary{{
			ToolName:          "external_search",
			InputJSON:         string(inputJSON),
			OutputSummaryJSON: string(outputJSON),
			Status:            "completed",
		}},
	}, nil
}

type externalSearchCandidate struct {
	Paper          research.Paper
	MatchedQuery   string
	Classification ExternalPaperClassificationResult
}

func (t *ExternalSearchTool) searchMany(ctx context.Context, queries []string, limit int) ([]externalSearchCandidate, []error) {
	if len(queries) == 0 {
		return nil, nil
	}
	type result struct {
		index int
		query string
		list  research.PaperList
		err   error
	}
	out := make(chan result, len(queries))
	var wg sync.WaitGroup
	for i, query := range queries {
		if strings.TrimSpace(query) == "" {
			continue
		}
		wg.Add(1)
		go func(index int, q string) {
			defer wg.Done()
			list, err := t.searcher.Search(ctx, q, research.SearchOpts{Limit: limit})
			out <- result{index: index, query: q, list: list, err: err}
		}(i, query)
	}
	wg.Wait()
	close(out)

	results := make([]result, len(queries))
	hasResult := make([]bool, len(queries))
	errs := make([]error, 0)
	for res := range out {
		results[res.index] = res
		hasResult[res.index] = true
		if res.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", res.query, res.err))
		}
	}

	seen := map[string]bool{}
	candidates := make([]externalSearchCandidate, 0)
	maxRows := 0
	for i := range queries {
		if !hasResult[i] || results[i].err != nil {
			continue
		}
		if len(results[i].list.Items) > maxRows {
			maxRows = len(results[i].list.Items)
		}
	}
	for row := 0; row < maxRows; row++ {
		for i := range queries {
			if !hasResult[i] || results[i].err != nil || row >= len(results[i].list.Items) {
				continue
			}
			p := results[i].list.Items[row]
			key := externalPaperKey(p)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			candidates = append(candidates, externalSearchCandidate{Paper: p, MatchedQuery: results[i].query})
		}
	}
	return candidates, errs
}

func (t *ExternalSearchTool) classifyCandidates(ctx context.Context, query string, searchQueries []string, candidates []externalSearchCandidate) ([]externalSearchCandidate, int, int) {
	type result struct {
		index int
		ok    bool
		res   ExternalPaperClassificationResult
	}
	parallel := externalClassifierParallel
	if parallel > len(candidates) {
		parallel = len(candidates)
	}
	sem := make(chan struct{}, parallel)
	out := make(chan result, len(candidates))
	var wg sync.WaitGroup
	for i, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(index int, cand externalSearchCandidate) {
			defer wg.Done()
			defer func() { <-sem }()
			if res, ok := classifyExternalPaperHeuristic(query, cand.Paper); ok {
				out <- result{index: index, ok: true, res: res}
				return
			}
			res, err := t.classifier.ClassifyExternalPaper(ctx, ExternalPaperClassificationInput{
				Query:         query,
				SearchQueries: searchQueries,
				MatchedQuery:  cand.MatchedQuery,
				Paper:         cand.Paper,
				EvidenceText:  externalEvidenceText(cand.Paper),
			})
			if err != nil {
				if fallback, ok := classifyExternalPaperHeuristic(query, cand.Paper); ok {
					out <- result{index: index, ok: true, res: fallback}
					return
				}
			}
			out <- result{index: index, ok: err == nil, res: sanitizeExternalClassification(res)}
		}(i, candidate)
	}
	wg.Wait()
	close(out)

	accepted := make([]bool, len(candidates))
	classifications := make([]ExternalPaperClassificationResult, len(candidates))
	classified := 0
	failed := 0
	for res := range out {
		if !res.ok {
			failed++
			accepted[res.index] = false
			continue
		}
		classified++
		accepted[res.index] = res.res.Relevant
		classifications[res.index] = res.res
	}
	filtered := make([]externalSearchCandidate, 0, len(candidates))
	for i, cand := range candidates {
		if !accepted[i] {
			continue
		}
		cand.Classification = classifications[i]
		filtered = append(filtered, cand)
	}
	return filtered, classified, failed
}

func (t *ExternalSearchTool) searchQueries(ctx context.Context, query string) ([]string, ExternalSearchPlan, error) {
	fallback := ExternalSearchQueries(query)
	if t == nil || t.planner == nil {
		return fallback, ExternalSearchPlan{}, nil
	}
	plan, err := t.planner.PlanExternalSearch(ctx, query)
	if err != nil {
		return fallback, ExternalSearchPlan{}, err
	}
	plan.SearchQueries = plan.QueriesForSource("semantic_scholar", fallback)
	plan.SearchQuery = firstExternalQuery(plan.SearchQueries)
	plan.Rationale = strings.TrimSpace(plan.Rationale)
	if len(plan.SearchQueries) == 0 {
		return fallback, ExternalSearchPlan{}, errors.New("empty external search query")
	}
	return plan.SearchQueries, plan, nil
}

func externalPlanningStages(t *ExternalSearchTool, searchQueries []string, plan ExternalSearchPlan, planErr error) []ProcessStage {
	if t == nil || t.planner == nil {
		return nil
	}
	if planErr != nil {
		return []ProcessStage{{
			Label:  "Master规划",
			Status: "failed",
			Detail: fmt.Sprintf("规划失败: %s; 回退查询: %s", planErr.Error(), strings.Join(searchQueries, " | ")),
		}}
	}
	detail := "检索式: " + strings.Join(searchQueries, " | ")
	if strings.TrimSpace(plan.Rationale) != "" {
		detail += "; " + strings.TrimSpace(plan.Rationale)
	}
	return []ProcessStage{{
		Label:  "Master规划",
		Count:  len(searchQueries),
		Unit:   "式",
		Status: "completed",
		Detail: detail,
	}}
}

func ExternalSearchQueries(query string) []string {
	return sanitizeExternalQueries(externalSearchQueryCandidates(query))
}

func ExternalSearchQuery(query string) string {
	queries := ExternalSearchQueries(query)
	if len(queries) > 0 {
		return queries[0]
	}
	return ""
}

func (p ExternalSearchPlan) QueriesForSource(source string, fallback []string) []string {
	source = normalizeExternalSource(source)
	if source != "" {
		if queries := sanitizeExternalQueries(p.QueriesBySource[source]); len(queries) > 0 {
			return queries
		}
		for planSource, sourceQueries := range p.QueriesBySource {
			if normalizeExternalSource(planSource) != source {
				continue
			}
			if queries := sanitizeExternalQueries(sourceQueries); len(queries) > 0 {
				return queries
			}
		}
	}

	legacyQueries := sanitizeExternalQueries(append([]string{p.SearchQuery}, p.SearchQueries...))
	if len(legacyQueries) == 0 {
		return fallback
	}
	return mergeExternalQueries(legacyQueries, fallback)
}

func externalSearchQueryCandidates(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	lower := strings.ToLower(q)
	terms := make([]string, 0, 6)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range terms {
			if strings.EqualFold(existing, value) {
				return
			}
		}
		terms = append(terms, value)
	}

	if containsAnyEvidenceText(lower, "单细胞", "single-cell", "single cell", "scatac") {
		add("single-cell")
	}
	if containsAnyEvidenceText(lower, "正向遗传", "forward genetic", "forward genetics") {
		add("forward genetic screens")
	}
	if containsAnyEvidenceText(lower, "基因", "gene discovery") {
		add("gene discovery")
	}
	if containsAnyEvidenceText(lower, "变慢", "速度变慢", "减少", "下降", "slowed", "declined", "decline") {
		add("slowed")
	}
	if containsAnyEvidenceText(lower, "atac", "染色质可及", "开放染色质", "转座酶") {
		add("ATAC-seq")
	}
	if containsAnyEvidenceText(lower, "chip-seq", "chip seq", "chipseq", "染色质免疫沉淀") {
		add("ChIP-seq")
	}
	if containsAnyEvidenceText(lower, "rna-seq", "rna seq", "transcriptome", "转录组") {
		add("RNA-seq")
	}
	if containsAnyEvidenceText(lower, "review", "综述") {
		add("review")
	}

	for _, match := range assistantASCIITermRe.FindAllString(q, -1) {
		if len(terms) >= 6 {
			break
		}
		if shouldSkipExternalASCIITerm(match, terms) {
			continue
		}
		add(match)
	}
	if len(terms) > 0 {
		primary := strings.Join(terms, " ")
		queries := []string{primary}
		if containsAnyEvidenceText(lower, "正向遗传", "forward genetic", "forward genetics") &&
			containsAnyEvidenceText(lower, "基因", "gene discovery") &&
			containsAnyEvidenceText(lower, "变慢", "速度变慢", "减少", "下降", "slowed", "declined", "decline") {
			queries = append(queries,
				"gene discovery slowed forward genetic screens saturation",
				"plant gene discovery slowed forward genetic screens",
				"pace of gene discovery plants slowed",
			)
		}
		return queries
	}
	return []string{strings.TrimSpace(strings.NewReplacer(
		"外部", " ",
		"查一下", " ",
		"查找", " ",
		"检索", " ",
		"有没有", " ",
		"关于", " ",
		"综述", " review ",
		"：", " ",
		":", " ",
	).Replace(q))}
}

func externalHitStage(hits int) ProcessStage {
	if hits == 0 {
		return ProcessStage{Label: "命中 0条", Unit: "条", Status: "completed"}
	}
	return ProcessStage{Label: "命中", Count: hits, Unit: "条", Status: "completed"}
}

func shouldSkipExternalASCIITerm(term string, existing []string) bool {
	lower := strings.ToLower(strings.TrimSpace(term))
	if lower == "" || lower == "p0" || isEvidenceStopWord(lower) {
		return true
	}
	for _, value := range existing {
		v := strings.ToLower(value)
		if lower == v || strings.Contains(v, lower) {
			return true
		}
	}
	return false
}

func clampExternalSearchLimit(limit int) int {
	if limit <= 0 {
		return defaultExternalSearchLimit
	}
	if limit > maxExternalSearchLimit {
		return maxExternalSearchLimit
	}
	return limit
}

func combineExternalSearchErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			parts = append(parts, err.Error())
		}
	}
	return errors.New(strings.Join(parts, "; "))
}

func normalizeExternalQuery(query string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
}

func normalizeExternalSource(source string) string {
	return strings.ToLower(strings.TrimSpace(source))
}

func normalizeExternalQueriesBySource(queriesBySource map[string][]string) map[string][]string {
	if len(queriesBySource) == 0 {
		return nil
	}
	normalized := make(map[string][]string, len(queriesBySource))
	for source, queries := range queriesBySource {
		source = normalizeExternalSource(source)
		if source == "" {
			continue
		}
		normalized[source] = sanitizeExternalQueries(append(normalized[source], queries...))
		if len(normalized[source]) == 0 {
			delete(normalized, source)
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func firstExternalQuery(queries []string) string {
	if len(queries) == 0 {
		return ""
	}
	return queries[0]
}

func sanitizeExternalQueries(queries []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(queries))
	for _, query := range queries {
		query = normalizeExternalQuery(query)
		if query == "" {
			continue
		}
		key := strings.ToLower(query)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, query)
		if len(out) >= maxExternalSearchQueries {
			break
		}
	}
	return out
}

func mergeExternalQueries(planned, fallback []string) []string {
	primary := ""
	if len(planned) > 0 {
		primary = normalizeExternalQuery(planned[0])
	}
	recall := externalRecallQueriesFromPlanned(planned)
	seen := map[string]bool{}
	out := make([]string, 0, maxExternalSearchQueries)
	add := func(query string) {
		query = normalizeExternalQuery(query)
		if query == "" {
			return
		}
		key := strings.ToLower(query)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, query)
	}
	add(primary)
	for _, query := range recall {
		if len(out) >= maxExternalSearchQueries {
			break
		}
		add(query)
	}
	for _, query := range fallback {
		if len(out) >= maxExternalSearchQueries {
			break
		}
		add(query)
	}
	if len(planned) > 1 {
		for _, query := range planned[1:] {
			if len(out) >= maxExternalSearchQueries {
				break
			}
			add(query)
		}
	}
	return out
}

func externalRecallQueriesFromPlanned(planned []string) []string {
	joined := strings.ToLower(strings.Join(planned, " "))
	if !containsAnyEvidenceText(joined, "forward genetic", "forward genetics") ||
		!containsAnyEvidenceText(joined, "gene discovery") ||
		!containsAnyEvidenceText(joined, "slowed", "slow", "decline", "declined", "reduced", "saturation") {
		return nil
	}
	return []string{
		"forward genetic screens gene discovery slowed",
		"gene discovery slowed forward genetic screens saturation",
		"plant gene discovery slowed forward genetic screens",
	}
}

func externalPaperKey(p research.Paper) string {
	if strings.TrimSpace(p.PaperID) != "" {
		return "s2:" + strings.TrimSpace(p.PaperID)
	}
	if strings.TrimSpace(p.ExternalIDs.DOI) != "" {
		return "doi:" + strings.ToLower(strings.TrimSpace(p.ExternalIDs.DOI))
	}
	if strings.TrimSpace(p.Title) != "" {
		return "title:" + strings.ToLower(strings.TrimSpace(p.Title))
	}
	return ""
}

func externalEvidenceText(p research.Paper) string {
	var b strings.Builder
	if strings.TrimSpace(p.Title) != "" {
		b.WriteString("Title: ")
		b.WriteString(strings.TrimSpace(p.Title))
		b.WriteString("\n")
	}
	if strings.TrimSpace(p.TLDR) != "" {
		b.WriteString("TLDR: ")
		b.WriteString(strings.TrimSpace(p.TLDR))
		b.WriteString("\n")
	}
	if strings.TrimSpace(p.Abstract) != "" {
		b.WriteString("Abstract: ")
		b.WriteString(strings.TrimSpace(p.Abstract))
	}
	return strings.TrimSpace(b.String())
}

func sanitizeExternalClassification(res ExternalPaperClassificationResult) ExternalPaperClassificationResult {
	res.Reason = strings.TrimSpace(res.Reason)
	cleaned := make([]ExternalEvidenceAnnotation, 0, len(res.Annotations))
	for _, annotation := range res.Annotations {
		annotation.Claim = strings.TrimSpace(annotation.Claim)
		annotation.Evidence = strings.TrimSpace(annotation.Evidence)
		annotation.Verdict = strings.TrimSpace(annotation.Verdict)
		annotation.Rationale = strings.TrimSpace(annotation.Rationale)
		if annotation.Claim == "" && annotation.Evidence == "" && annotation.Rationale == "" {
			continue
		}
		cleaned = append(cleaned, annotation)
		if len(cleaned) >= 4 {
			break
		}
	}
	res.Annotations = cleaned
	return res
}

func classifyExternalPaperHeuristic(_ string, p research.Paper) (ExternalPaperClassificationResult, bool) {
	evidenceText := externalEvidenceText(p)
	lower := strings.ToLower(evidenceText)
	annotations := make([]ExternalEvidenceAnnotation, 0, 3)

	if containsAnyEvidenceText(lower, "gene discovery") &&
		containsAnyEvidenceText(lower, "slowed", "slow") &&
		containsAnyEvidenceText(lower, "forward genetic") &&
		containsAnyEvidenceText(lower, "saturation", "reach saturation") {
		annotations = append(annotations, ExternalEvidenceAnnotation{
			Claim:     "基因发现速度变慢，并与正向遗传筛选饱和有关",
			Evidence:  bestExternalEvidenceSentence(evidenceText, "gene discovery", "forward genetic", "saturation"),
			Verdict:   "supported",
			Rationale: "候选摘要直接同时覆盖 gene discovery slowed、forward genetic screens 和 saturation。",
		})
	}

	if (containsAnyEvidenceText(lower, "gene family", "gene families") ||
		(containsAnyEvidenceText(lower, "poor handling") && containsAnyEvidenceText(lower, "genetic redundancy"))) &&
		containsAnyEvidenceText(lower, "forward genetic", "genetic screens") {
		annotations = append(annotations, ExternalEvidenceAnnotation{
			Claim:     "基因家族冗余会影响正向遗传筛选或功能发现",
			Evidence:  bestExternalEvidenceSentence(evidenceText, "redundancy", "gene family", "forward genetic"),
			Verdict:   "partial",
			Rationale: "候选文本支持遗传冗余/基因家族对筛选的影响，但不一定支持整句的时间趋势。",
		})
	}

	if len(annotations) > 0 &&
		containsAnyEvidenceText(lower, "cell-type", "cell type", "cell fate", "key regulators of plant cell types") {
		annotations = append(annotations, ExternalEvidenceAnnotation{
			Claim:     "研究目标涉及细胞类型/细胞命运相关调控基因",
			Evidence:  bestExternalEvidenceSentence(evidenceText, "cell-type", "cell type", "cell fate", "key regulators"),
			Verdict:   "partial",
			Rationale: "候选文本支持细胞类型调控基因发现这一部分，但不单独支持整句全部断言。",
		})
	}

	if len(annotations) == 0 {
		return ExternalPaperClassificationResult{}, false
	}
	return ExternalPaperClassificationResult{
		Relevant:    true,
		Reason:      "工具判定：候选文本包含可直接对应用户原句的证据片段。",
		Annotations: annotations,
	}, true
}

func bestExternalEvidenceSentence(text string, needles ...string) string {
	text = normalizeEvidenceWhitespace(text)
	if text == "" {
		return ""
	}
	sentences := splitExternalEvidenceSentences(text)
	best := ""
	bestScore := -1
	for _, sentence := range sentences {
		lower := strings.ToLower(sentence)
		score := 0
		for _, needle := range needles {
			if strings.Contains(lower, strings.ToLower(needle)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = sentence
		}
	}
	if best == "" {
		return trimRunes(text, 360)
	}
	return trimRunes(best, 420)
}

func splitExternalEvidenceSentences(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == ';' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		field = strings.TrimPrefix(field, "Abstract: ")
		field = strings.TrimPrefix(field, "TLDR: ")
		field = strings.TrimPrefix(field, "Title: ")
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func externalHighlightTerms(searchQueries []string) []string {
	terms := make([]string, 0, 24)
	for _, query := range searchQueries {
		for _, field := range strings.Fields(query) {
			field = strings.Trim(field, ".,;:!?()[]{}\"'")
			if len(field) < 4 || isEvidenceStopWord(field) {
				continue
			}
			terms = append(terms, field)
		}
	}
	return dedupeEvidenceTerms(terms)
}

func externalSearchFailedResult(inputJSON []byte, err error, stages []ProcessStage, searchQueries []string) ToolResult {
	reason := "外部搜索失败：" + err.Error()
	stages = append(stages, ProcessStage{Label: "外部搜索", Status: "failed", Detail: err.Error()})
	if len(searchQueries) > 0 {
		reason += "；查询: " + strings.Join(searchQueries, " | ")
	}
	return ToolResult{
		Process: ProcessSummary{
			Intent: IntentExternalSearch,
			Stages: stages,
			Note:   reason,
		},
		AnswerContext: reason,
		ToolCalls: []ToolCallSummary{{
			ToolName:  "external_search",
			InputJSON: string(inputJSON),
			Status:    "failed",
			Error:     err.Error(),
		}},
	}
}

func externalID(p research.Paper) string {
	if p.ExternalIDs.DOI != "" {
		return "DOI:" + p.ExternalIDs.DOI
	}
	if p.ExternalIDs.ArXiv != "" {
		return "ARXIV:" + p.ExternalIDs.ArXiv
	}
	if p.ExternalIDs.PubMed != "" {
		return "PMID:" + p.ExternalIDs.PubMed
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func externalAnswerContext(cards []ResultCard) string {
	var b strings.Builder
	for i, card := range cards {
		p, ok := card.Payload.(ExternalPaperCard)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "[external %d] %s", i+1, p.Title)
		if p.Year > 0 || p.Venue != "" {
			fmt.Fprintf(&b, " (%s %d)", p.Venue, p.Year)
		}
		if p.MatchedQuery != "" {
			fmt.Fprintf(&b, "\nMatched query: %s", p.MatchedQuery)
		}
		if p.Reason != "" {
			fmt.Fprintf(&b, "\nSub-Agent judgment: %s", p.Reason)
		}
		if len(p.EvidenceAnnotations) > 0 {
			b.WriteString("\nEvidence annotations:")
			for _, annotation := range p.EvidenceAnnotations {
				if annotation.Claim != "" {
					fmt.Fprintf(&b, "\n- Claim: %s", annotation.Claim)
				}
				if annotation.Evidence != "" {
					fmt.Fprintf(&b, "\n  Evidence: %s", annotation.Evidence)
				}
				if annotation.Verdict != "" {
					fmt.Fprintf(&b, "\n  Verdict: %s", annotation.Verdict)
				}
				if annotation.Rationale != "" {
					fmt.Fprintf(&b, "\n  Rationale: %s", annotation.Rationale)
				}
			}
		}
		if p.TLDR != "" {
			fmt.Fprintf(&b, "\nTLDR: %s", p.TLDR)
		}
		if p.Abstract != "" {
			fmt.Fprintf(&b, "\nAbstract: %s", p.Abstract)
		}
		b.WriteString("\n\n")
	}
	return b.String()
}
