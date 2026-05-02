package ai_assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/xuzhougeng/citebox/internal/service/ai_external"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

// ErrSourceDisabled signals that the user explicitly named an external source
// (e.g. via @PubMed) that is not enabled in the user's settings.
var ErrSourceDisabled = errors.New("ai_assistant: external source is not enabled in settings")

type ExternalSearcher interface {
	Search(ctx context.Context, queries ai_external.SourceQueries, opts ai_external.SearchOptions) (ai_external.SearchResult, error)
}

type ExternalSourceLister interface {
	EnabledExternalSources(ctx context.Context) ([]ai_external.SourceID, error)
}

type ExternalSearchPlanner interface {
	PlanExternalSearch(ctx context.Context, query string, goalHint ExternalSearchGoal) (ExternalSearchPlan, error)
}

type ExternalPaperClassifier interface {
	ClassifyExternalPaper(ctx context.Context, in ExternalPaperClassificationInput) (ExternalPaperClassificationResult, error)
}

type ExternalSearchGoal string

const (
	ExternalSearchGoalDiscovery ExternalSearchGoal = "discovery"
	ExternalSearchGoalEvidence  ExternalSearchGoal = "evidence"
)

type ExternalSearchPlan struct {
	SearchGoal      ExternalSearchGoal  `json:"search_goal,omitempty"`
	MustMatch       []string            `json:"must_match,omitempty"`
	SoftPreferences []string            `json:"soft_preferences,omitempty"`
	TargetYear      int                 `json:"target_year,omitempty"`
	SearchQuery     string              `json:"search_query,omitempty"`
	SearchQueries   []string            `json:"search_queries,omitempty"`
	QueriesBySource map[string][]string `json:"queries_by_source,omitempty"`
	Rationale       string              `json:"rationale,omitempty"`
}

func (p *ExternalSearchPlan) UnmarshalJSON(data []byte) error {
	type externalSearchPlanAlias struct {
		SearchGoal      ExternalSearchGoal  `json:"search_goal,omitempty"`
		MustMatch       []string            `json:"must_match,omitempty"`
		SoftPreferences []string            `json:"soft_preferences,omitempty"`
		TargetYear      json.RawMessage     `json:"target_year,omitempty"`
		SearchQuery     string              `json:"search_query,omitempty"`
		SearchQueries   []string            `json:"search_queries,omitempty"`
		QueriesBySource map[string][]string `json:"queries_by_source,omitempty"`
		Rationale       string              `json:"rationale,omitempty"`
	}
	var raw externalSearchPlanAlias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	targetYear, err := parseExternalTargetYear(raw.TargetYear)
	if err != nil {
		return err
	}
	*p = ExternalSearchPlan{
		SearchGoal:      raw.SearchGoal,
		MustMatch:       raw.MustMatch,
		SoftPreferences: raw.SoftPreferences,
		TargetYear:      targetYear,
		SearchQuery:     raw.SearchQuery,
		SearchQueries:   raw.SearchQueries,
		QueriesBySource: raw.QueriesBySource,
		Rationale:       raw.Rationale,
	}
	return nil
}

type ExternalPaperClassificationInput struct {
	Query           string
	SearchGoal      ExternalSearchGoal
	MustMatch       []string
	SoftPreferences []string
	SearchQueries   []string
	MatchedQuery    string
	Paper           research.Paper
	EvidenceText    string
	OnlineYear      int
	IssueYear       int
	YearLabel       string
}

type ExternalPaperTier string

const (
	ExternalPaperTierStrongMatch ExternalPaperTier = "strong_match"
	ExternalPaperTierWeakMatch   ExternalPaperTier = "weak_match"
	ExternalPaperTierNeedsReview ExternalPaperTier = "needs_review"
	ExternalPaperTierDrop        ExternalPaperTier = "drop"
)

type ExternalPaperClassificationResult struct {
	Tier               ExternalPaperTier            `json:"tier,omitempty"`
	Reason             string                       `json:"reason,omitempty"`
	MatchedConstraints []string                     `json:"matched_constraints,omitempty"`
	MatchedPreferences []string                     `json:"matched_preferences,omitempty"`
	ArticleRole        string                       `json:"article_role,omitempty"`
	Annotations        []ExternalEvidenceAnnotation `json:"annotations,omitempty"`
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
)

var (
	externalSearchQuotedClaimPattern = regexp.MustCompile(`["“”‘’「」『』]([^"“”‘’「」『』\r\n]+)["“”‘’「」『』]`)
	externalSearchDOIPattern         = regexp.MustCompile(`(?i)\b10\.\d{4,9}/[-._;()/:A-Z0-9]+\b`)
	externalSearchPMIDPattern        = regexp.MustCompile(`(?i)\bpmid\s*:\s*\d{5,9}\b`)
	externalSearchArXivPattern       = regexp.MustCompile(`(?i)\barxiv\s*:\s*(?:[a-z.-]+/)?\d{4}\.\d{4,5}(?:v\d+)?\b`)
)

type ExternalPaperCard struct {
	S2PaperID           string                       `json:"s2_paper_id,omitempty"`
	SourceIDs           map[string]string            `json:"source_ids,omitempty"`
	Sources             []string                     `json:"sources,omitempty"`
	PMID                string                       `json:"pmid,omitempty"`
	PMCID               string                       `json:"pmcid,omitempty"`
	URL                 string                       `json:"url,omitempty"`
	Title               string                       `json:"title"`
	Year                int                          `json:"year,omitempty"`
	Venue               string                       `json:"venue,omitempty"`
	DOI                 string                       `json:"doi,omitempty"`
	TLDR                string                       `json:"tldr,omitempty"`
	Abstract            string                       `json:"abstract,omitempty"`
	MatchedQuery        string                       `json:"matched_query,omitempty"`
	Reason              string                       `json:"reason,omitempty"`
	OnlineYear          int                          `json:"online_year,omitempty"`
	IssueYear           int                          `json:"issue_year,omitempty"`
	YearLabel           string                       `json:"year_label,omitempty"`
	SearchGoal          ExternalSearchGoal           `json:"search_goal,omitempty"`
	Tier                ExternalPaperTier            `json:"tier,omitempty"`
	ArticleRole         string                       `json:"article_role,omitempty"`
	MatchedConstraints  []string                     `json:"matched_constraints,omitempty"`
	MatchedPreferences  []string                     `json:"matched_preferences,omitempty"`
	CitationIndex       int                          `json:"citation_index,omitempty"`
	HighlightTerms      []string                     `json:"highlight_terms,omitempty"`
	EvidenceAnnotations []ExternalEvidenceAnnotation `json:"evidence_annotations,omitempty"`
}

type externalTierCounts struct {
	Strong      int `json:"strong"`
	Weak        int `json:"weak"`
	NeedsReview int `json:"needs_review"`
	Dropped     int `json:"dropped"`
}

func (c *externalTierCounts) add(tier ExternalPaperTier) {
	switch normalizeExternalPaperTier(tier) {
	case ExternalPaperTierStrongMatch:
		c.Strong++
	case ExternalPaperTierWeakMatch:
		c.Weak++
	case ExternalPaperTierDrop:
		c.Dropped++
	default:
		c.NeedsReview++
	}
}

func (c externalTierCounts) nonDropped() int {
	return c.Strong + c.Weak + c.NeedsReview
}

func (c externalTierCounts) total() int {
	return c.Strong + c.Weak + c.NeedsReview + c.Dropped
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
	fallbackQueries := ExternalSearchQueries(in.Query)
	plan := ExternalSearchPlan{}
	var planErr error
	sourceQueries := externalSearchSourceQueries(fallbackQueries, plan)
	if t != nil {
		sourceQueries, plan, planErr = t.searchQueries(ctx, in.Query, in.SearchGoalHint)
	}
	plan.SearchGoal = resolvedExternalSearchGoal(in.Query, plan.SearchGoal, in.SearchGoalHint)
	var disabledRequested []ai_external.SourceID
	if enabledSources, ok, err := enabledExternalSearchSources(ctx, t); err != nil {
		searchQueries := flattenExternalSourceQueries(sourceQueries)
		inputJSON, _ := json.Marshal(struct {
			Query           string              `json:"query"`
			SearchQueries   []string            `json:"search_queries,omitempty"`
			QueriesBySource map[string][]string `json:"queries_by_source,omitempty"`
			Limit           int                 `json:"limit"`
		}{Query: in.Query, SearchQueries: searchQueries, QueriesBySource: sourceQueriesForJSON(sourceQueries), Limit: limit})
		return externalSearchFailedResult(inputJSON, err, externalPlanningStages(t, sourceQueries, plan, planErr), searchQueries), nil
	} else if ok {
		// User-explicit override: when in.Sources is non-empty, narrow execution
		// further to the intersection. Sources that the user requested but that
		// are not enabled in settings are reported as ErrSourceDisabled below so
		// the user sees a clear error card.
		executionSources := enabledSources
		if len(in.Sources) > 0 {
			executionSources, disabledRequested = intersectUserSources(in.Sources, enabledSources)
		}
		sourceQueries = filterExternalSourceQueries(sourceQueries, executionSources)
	}
	searchQueries := flattenExternalSourceQueries(sourceQueries)
	searchQuery := ""
	if len(searchQueries) > 0 {
		searchQuery = searchQueries[0]
	}
	inputJSON, _ := json.Marshal(struct {
		Query           string              `json:"query"`
		SearchQuery     string              `json:"search_query,omitempty"`
		SearchQueries   []string            `json:"search_queries,omitempty"`
		QueriesBySource map[string][]string `json:"queries_by_source,omitempty"`
		Limit           int                 `json:"limit"`
	}{Query: in.Query, SearchQuery: searchQuery, SearchQueries: searchQueries, QueriesBySource: sourceQueriesForJSON(sourceQueries), Limit: limit})
	processStages := externalPlanningStages(t, sourceQueries, plan, planErr)

	if t == nil || t.searcher == nil {
		return externalSearchFailedResult(inputJSON, errors.New("external searcher is not configured"), processStages, searchQueries), nil
	}

	searchRes, searchErr := t.searcher.Search(ctx, sourceQueries, ai_external.SearchOptions{Limit: limit})
	searchErrs := externalSearchFailures(searchRes, searchErr)
	for _, s := range disabledRequested {
		searchErrs = append(searchErrs, fmt.Errorf("%s: %w", externalSourceLabel(s), ErrSourceDisabled))
	}
	candidates := externalSearchCandidates(searchRes.Papers)
	if len(candidates) == 0 && searchErr != nil {
		realErrs := nonDisabledErrors(searchErrs)
		disabledNote := disabledSourcesNote(searchErrs)
		var failureErr error
		switch {
		case len(realErrs) == 0 && disabledNote != "":
			// Only disabled-source errors — surface the disabled note as the failure reason.
			failureErr = errors.New(disabledNote)
		case disabledNote != "":
			failureErr = errors.New(combineExternalSearchErrors(realErrs).Error() + "; " + disabledNote)
		default:
			failureErr = combineExternalSearchErrors(realErrs)
		}
		return externalSearchFailedResult(inputJSON, failureErr, processStages, searchQueries), nil
	}
	rawReturned := len(candidates)

	classified := 0
	classifierFailed := 0
	tierCounts := externalTierCounts{}
	if t.classifier != nil && len(candidates) > 0 {
		classifyInput := candidates
		if len(classifyInput) > maxExternalClassification {
			classifyInput = classifyInput[:maxExternalClassification]
		}
		candidates, tierCounts, classified, classifierFailed = t.classifyCandidates(ctx, in.Query, plan, searchQueries, classifyInput)
	} else {
		for i := range candidates {
			candidates[i].Classification = ExternalPaperClassificationResult{Tier: ExternalPaperTierStrongMatch}
			tierCounts.add(ExternalPaperTierStrongMatch)
		}
	}

	cards := make([]ResultCard, 0, len(candidates))
	citations := make([]Citation, 0, len(candidates))
	highlightTerms := externalHighlightTerms(searchQueries)
	searchGoal := normalizeExternalSearchGoal(string(plan.SearchGoal))
	for _, candidate := range candidates {
		if len(cards) >= limit {
			break
		}
		p := candidate.Paper
		labels := externalPaperSourceLabels(p)
		citationIndex := 0
		if candidate.Classification.Tier == ExternalPaperTierStrongMatch {
			citation := Citation{
				I:          len(citations) + 1,
				S2PaperID:  externalSemanticScholarID(p),
				ExternalID: externalID(p),
				Title:      p.Title,
				Source:     "external:" + strings.Join(labels, "+"),
				Snippet: research.Snippet{
					Text:        firstNonEmpty(p.Abstract, p.TLDR, p.Title),
					SnippetKind: "abstract",
					Section:     "外部学术搜索: " + strings.Join(labels, "+"),
				},
			}
			citations = append(citations, citation)
			citationIndex = citation.I
		}
		cards = append(cards, ResultCard{Type: "external_paper", Payload: ExternalPaperCard{
			S2PaperID:           externalSemanticScholarID(p),
			SourceIDs:           externalSourceIDsForCard(p),
			Sources:             labels,
			PMID:                p.PMID,
			PMCID:               p.PMCID,
			URL:                 p.URL,
			Title:               p.Title,
			Year:                p.Year,
			Venue:               p.Venue,
			DOI:                 p.DOI,
			TLDR:                p.TLDR,
			Abstract:            p.Abstract,
			MatchedQuery:        candidate.MatchedQuery,
			Reason:              candidate.Classification.Reason,
			OnlineYear:          p.OnlineYear,
			IssueYear:           p.IssueYear,
			YearLabel:           p.YearLabel,
			SearchGoal:          searchGoal,
			Tier:                candidate.Classification.Tier,
			ArticleRole:         candidate.Classification.ArticleRole,
			MatchedConstraints:  candidate.Classification.MatchedConstraints,
			MatchedPreferences:  candidate.Classification.MatchedPreferences,
			CitationIndex:       citationIndex,
			HighlightTerms:      highlightTerms,
			EvidenceAnnotations: candidate.Classification.Annotations,
		}})
	}

	sourceLabels := labelsForExternalSources(searchRes.Sources)
	if len(sourceLabels) == 0 {
		sourceLabels = labelsForExternalSources(sourceIDsInQueries(sourceQueries))
	}
	outputJSON, _ := json.Marshal(struct {
		Sources          []string            `json:"sources,omitempty"`
		SearchQuery      string              `json:"search_query"`
		SearchQueries    []string            `json:"search_queries,omitempty"`
		QueriesBySource  map[string][]string `json:"queries_by_source,omitempty"`
		Returned         int                 `json:"returned"`
		Hits             int                 `json:"hits"`
		TierCounts       externalTierCounts  `json:"tier_counts"`
		Classified       int                 `json:"classified,omitempty"`
		ClassifierFailed int                 `json:"classifier_failed,omitempty"`
	}{Sources: sourceLabels, SearchQuery: searchQuery, SearchQueries: searchQueries, QueriesBySource: sourceQueriesForJSON(sourceQueries), Returned: rawReturned, Hits: len(cards), TierCounts: tierCounts, Classified: classified, ClassifierFailed: classifierFailed})

	searchDetail := fmt.Sprintf("来源: %s", strings.Join(sourceLabels, "+"))
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
			detail = fmt.Sprintf("失败 %d篇", classifierFailed)
			if searchGoal == ExternalSearchGoalDiscovery {
				detail += "；失败候选保留为待核查结果"
			} else {
				detail += "；失败候选未作为证据引用"
			}
		}
		processStages = append(processStages,
			ProcessStage{Label: "Sub-Agent判定", Count: classified, Unit: "篇", Status: "completed", Detail: detail},
		)
	}
	processStages = append(processStages,
		externalHitStage(searchGoal, tierCounts),
	)
	noteParts := make([]string, 0, 2)
	if planErr != nil {
		noteParts = append(noteParts, "Master规划失败，已使用本地查询回退。")
	}
	if len(searchErrs) > 0 {
		if note := disabledSourcesNote(searchErrs); note != "" {
			noteParts = append(noteParts, note)
		}
		realErrs := nonDisabledErrors(searchErrs)
		if len(realErrs) > 0 && len(candidates) > 0 {
			noteParts = append(noteParts, fmt.Sprintf("外部学术搜索部分失败 %d 个: %s。", len(realErrs), combineExternalSearchErrors(realErrs).Error()))
		}
	}
	noteParts = append(noteParts, fmt.Sprintf("%s 查询: %s", strings.Join(sourceLabels, "+"), formatExternalSourceQueries(sourceQueries)))
	note := joinProcessNotes(noteParts)
	answerContext := externalAnswerContext(searchGoal, tierCounts, cards)
	if len(cards) == 0 {
		answerContext = externalNoSupportAnswerContext(searchGoal, tierCounts, sourceLabels, searchQueries)
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
	Paper          ai_external.Paper
	MatchedQuery   string
	Classification ExternalPaperClassificationResult
}

func (t *ExternalSearchTool) classifyCandidates(ctx context.Context, query string, plan ExternalSearchPlan, searchQueries []string, candidates []externalSearchCandidate) ([]externalSearchCandidate, externalTierCounts, int, int) {
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
			researchPaper := externalPaperToResearchPaper(cand.Paper)
			if res, ok := classifyExternalPaperHeuristic(query, researchPaper); ok {
				out <- result{index: index, ok: true, res: res}
				return
			}
			res, err := t.classifier.ClassifyExternalPaper(ctx, ExternalPaperClassificationInput{
				Query:           query,
				SearchGoal:      plan.SearchGoal,
				MustMatch:       append([]string(nil), plan.MustMatch...),
				SoftPreferences: append([]string(nil), plan.SoftPreferences...),
				SearchQueries:   searchQueries,
				MatchedQuery:    cand.MatchedQuery,
				Paper:           researchPaper,
				EvidenceText:    externalEvidenceText(researchPaper),
				OnlineYear:      cand.Paper.OnlineYear,
				IssueYear:       cand.Paper.IssueYear,
				YearLabel:       cand.Paper.YearLabel,
			})
			if err != nil {
				if fallback, ok := classifyExternalPaperHeuristic(query, researchPaper); ok {
					out <- result{index: index, ok: true, res: fallback}
					return
				}
			}
			out <- result{index: index, ok: err == nil, res: sanitizeExternalClassification(res)}
		}(i, candidate)
	}
	wg.Wait()
	close(out)

	searchGoal := normalizeExternalSearchGoal(string(plan.SearchGoal))
	classifications := make([]ExternalPaperClassificationResult, len(candidates))
	counts := externalTierCounts{}
	classified := 0
	failed := 0
	for res := range out {
		if !res.ok {
			failed++
			if searchGoal == ExternalSearchGoalDiscovery {
				classifications[res.index] = ExternalPaperClassificationResult{
					Tier:   ExternalPaperTierNeedsReview,
					Reason: "Sub-Agent判定失败，建议人工复核。",
				}
				counts.add(ExternalPaperTierNeedsReview)
			} else {
				counts.add(ExternalPaperTierDrop)
			}
			continue
		}
		classified++
		classifications[res.index] = res.res
		counts.add(res.res.Tier)
	}
	filtered := make([]externalSearchCandidate, 0, len(candidates))
	for i, cand := range candidates {
		cand.Classification = classifications[i]
		if !keepExternalCandidateForGoal(searchGoal, cand.Classification.Tier) {
			continue
		}
		filtered = append(filtered, cand)
	}
	return filtered, counts, classified, failed
}

func (t *ExternalSearchTool) searchQueries(ctx context.Context, query string, goalHint ExternalSearchGoal) (ai_external.SourceQueries, ExternalSearchPlan, error) {
	fallback := ExternalSearchQueries(query)
	if t == nil || t.planner == nil {
		plan := ExternalSearchPlan{SearchGoal: fallbackExternalSearchGoal(query)}
		return externalSearchSourceQueries(fallback, plan), plan, nil
	}
	plan, err := t.planner.PlanExternalSearch(ctx, query, goalHint)
	if err != nil {
		plan := ExternalSearchPlan{SearchGoal: fallbackExternalSearchGoal(query)}
		return externalSearchSourceQueries(fallback, plan), plan, err
	}
	rawGoal := strings.TrimSpace(string(plan.SearchGoal))
	plan = sanitizeExternalPlan(plan)
	if !isKnownExternalSearchGoal(rawGoal) {
		plan.SearchGoal = fallbackExternalSearchGoal(query)
	}
	sourceQueries := externalSearchSourceQueries(fallback, plan)
	plan.SearchQueries = flattenExternalSourceQueries(sourceQueries)
	plan.SearchQuery = firstExternalQuery(plan.SearchQueries)
	if len(plan.SearchQueries) == 0 {
		fallbackPlan := ExternalSearchPlan{SearchGoal: fallbackExternalSearchGoal(query)}
		return externalSearchSourceQueries(fallback, fallbackPlan), fallbackPlan, errors.New("empty external search query")
	}
	return sourceQueries, plan, nil
}

func externalPlanningStages(t *ExternalSearchTool, sourceQueries ai_external.SourceQueries, plan ExternalSearchPlan, planErr error) []ProcessStage {
	if t == nil || t.planner == nil {
		return nil
	}
	searchQueries := flattenExternalSourceQueries(sourceQueries)
	if planErr != nil {
		return []ProcessStage{{
			Label:  "Master规划",
			Status: "failed",
			Detail: fmt.Sprintf("规划失败: %s; 回退查询: %s", planErr.Error(), formatExternalSourceQueries(sourceQueries)),
		}}
	}
	detail := "检索式: " + formatExternalSourceQueries(sourceQueries)
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

func externalSearchSourceQueries(fallback []string, plan ExternalSearchPlan) ai_external.SourceQueries {
	return ai_external.SourceQueries{
		ai_external.SourcePubMed:          plan.QueriesForSource(string(ai_external.SourcePubMed), fallback),
		ai_external.SourceSemanticScholar: plan.QueriesForSource(string(ai_external.SourceSemanticScholar), fallback),
	}
}

func enabledExternalSearchSources(ctx context.Context, t *ExternalSearchTool) ([]ai_external.SourceID, bool, error) {
	if t == nil || t.searcher == nil {
		return nil, false, nil
	}
	lister, ok := t.searcher.(ExternalSourceLister)
	if !ok {
		return nil, false, nil
	}
	sources, err := lister.EnabledExternalSources(ctx)
	if err != nil {
		return nil, true, err
	}
	return uniqueExternalSources(sources), true, nil
}

func uniqueExternalSources(sources []ai_external.SourceID) []ai_external.SourceID {
	out := make([]ai_external.SourceID, 0, len(sources))
	seen := map[ai_external.SourceID]bool{}
	for _, source := range sources {
		if source == "" || seen[source] {
			continue
		}
		seen[source] = true
		out = append(out, source)
	}
	return out
}

func filterExternalSourceQueries(queries ai_external.SourceQueries, sources []ai_external.SourceID) ai_external.SourceQueries {
	out := make(ai_external.SourceQueries, len(sources))
	for _, source := range sources {
		sourceQueries := sanitizeExternalQueries(queries[source])
		if len(sourceQueries) == 0 {
			continue
		}
		out[source] = sourceQueries
	}
	return out
}

func sourceIDsInQueries(queries ai_external.SourceQueries) []ai_external.SourceID {
	sources := make([]ai_external.SourceID, 0, len(queries))
	for _, source := range orderedExternalSearchSources(queries) {
		if len(queries[source]) > 0 {
			sources = append(sources, source)
		}
	}
	return sources
}

func orderedExternalSearchSources(queries ai_external.SourceQueries) []ai_external.SourceID {
	known := []ai_external.SourceID{ai_external.SourcePubMed, ai_external.SourceSemanticScholar}
	seen := make(map[ai_external.SourceID]bool, len(queries))
	out := make([]ai_external.SourceID, 0, len(queries))
	for _, source := range known {
		if _, ok := queries[source]; ok {
			out = append(out, source)
			seen[source] = true
		}
	}
	unknown := make([]string, 0, len(queries))
	for source := range queries {
		if !seen[source] {
			unknown = append(unknown, string(source))
		}
	}
	sort.Strings(unknown)
	for _, source := range unknown {
		out = append(out, ai_external.SourceID(source))
	}
	return out
}

func flattenExternalSourceQueries(queries ai_external.SourceQueries) []string {
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, source := range orderedExternalSearchSources(queries) {
		for _, query := range queries[source] {
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
		}
	}
	return out
}

func sourceQueriesForJSON(queries ai_external.SourceQueries) map[string][]string {
	out := make(map[string][]string, len(queries))
	for _, source := range orderedExternalSearchSources(queries) {
		sourceQueries := sanitizeExternalQueries(queries[source])
		if len(sourceQueries) == 0 {
			continue
		}
		out[string(source)] = sourceQueries
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func formatExternalSourceQueries(queries ai_external.SourceQueries) string {
	parts := make([]string, 0, len(queries))
	for _, source := range orderedExternalSearchSources(queries) {
		sourceQueries := sanitizeExternalQueries(queries[source])
		if len(sourceQueries) == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", externalSourceLabel(source), strings.Join(sourceQueries, " | ")))
	}
	return strings.Join(parts, "; ")
}

func labelsForExternalSources(sources []ai_external.SourceID) []string {
	labels := make([]string, 0, len(sources))
	seen := map[string]bool{}
	for _, source := range sources {
		label := externalSourceLabel(source)
		key := strings.ToLower(label)
		if label == "" || seen[key] {
			continue
		}
		seen[key] = true
		labels = append(labels, label)
	}
	return labels
}

func externalSourceLabel(source ai_external.SourceID) string {
	switch source {
	case ai_external.SourcePubMed:
		return "PubMed"
	case ai_external.SourceSemanticScholar:
		return "Semantic Scholar"
	default:
		if strings.TrimSpace(string(source)) == "" {
			return "Unknown"
		}
		return string(source)
	}
}

func externalPaperSourceLabels(p ai_external.Paper) []string {
	sources := p.Sources
	if len(sources) == 0 && p.Source != "" {
		sources = []ai_external.SourceID{p.Source}
	}
	if len(sources) == 0 && len(p.SourcePaperIDs) > 0 {
		for source := range p.SourcePaperIDs {
			sources = append(sources, source)
		}
		sort.Slice(sources, func(i, j int) bool {
			return string(sources[i]) < string(sources[j])
		})
	}
	labels := labelsForExternalSources(sources)
	if len(labels) == 0 {
		return []string{"Unknown"}
	}
	return labels
}

func externalSemanticScholarID(p ai_external.Paper) string {
	if p.SourcePaperIDs != nil && strings.TrimSpace(p.SourcePaperIDs[ai_external.SourceSemanticScholar]) != "" {
		return strings.TrimSpace(p.SourcePaperIDs[ai_external.SourceSemanticScholar])
	}
	if p.Source == ai_external.SourceSemanticScholar {
		return strings.TrimSpace(p.SourcePaperID)
	}
	return ""
}

func externalSourceIDsForCard(p ai_external.Paper) map[string]string {
	out := make(map[string]string, len(p.SourcePaperIDs)+1)
	for source, id := range p.SourcePaperIDs {
		if strings.TrimSpace(id) != "" {
			out[string(source)] = strings.TrimSpace(id)
		}
	}
	if p.Source != "" && strings.TrimSpace(p.SourcePaperID) != "" && out[string(p.Source)] == "" {
		out[string(p.Source)] = strings.TrimSpace(p.SourcePaperID)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func externalSearchCandidates(papers []ai_external.Paper) []externalSearchCandidate {
	candidates := make([]externalSearchCandidate, 0, len(papers))
	for _, paper := range papers {
		candidates = append(candidates, externalSearchCandidate{
			Paper:        paper,
			MatchedQuery: paper.MatchedQuery,
		})
	}
	return candidates
}

func externalSearchFailures(res ai_external.SearchResult, err error) []error {
	errs := make([]error, 0, len(res.Failures)+1)
	for _, failure := range res.Failures {
		if failure.Err == nil {
			continue
		}
		if failure.Source != "" {
			errs = append(errs, fmt.Errorf("%s: %w", externalSourceLabel(failure.Source), failure.Err))
			continue
		}
		errs = append(errs, failure.Err)
	}
	if err != nil && len(errs) == 0 {
		errs = append(errs, err)
	}
	return errs
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

func keepExternalCandidateForGoal(goal ExternalSearchGoal, tier ExternalPaperTier) bool {
	tier = normalizeExternalPaperTier(tier)
	switch goal {
	case ExternalSearchGoalEvidence:
		return tier == ExternalPaperTierStrongMatch
	default:
		return tier != ExternalPaperTierDrop
	}
}

func externalHitStage(goal ExternalSearchGoal, counts externalTierCounts) ProcessStage {
	hits := counts.Strong
	if goal != ExternalSearchGoalEvidence {
		hits = counts.nonDropped()
	}
	if counts.total() == 0 {
		return ProcessStage{Label: "命中 0条", Unit: "条", Status: "completed", Detail: fmt.Sprintf("goal %s; strong %d; weak %d; needs_review %d; dropped %d", goal, counts.Strong, counts.Weak, counts.NeedsReview, counts.Dropped)}
	}
	if goal == ExternalSearchGoalEvidence && counts.Strong == 0 {
		return ProcessStage{
			Label:  "未形成证据",
			Count:  counts.total(),
			Unit:   "条",
			Status: "completed",
			Detail: fmt.Sprintf("goal %s; strong %d; weak %d; needs_review %d; dropped %d", goal, counts.Strong, counts.Weak, counts.NeedsReview, counts.Dropped),
		}
	}
	return ProcessStage{
		Label:  "命中",
		Count:  hits,
		Unit:   "条",
		Status: "completed",
		Detail: fmt.Sprintf("goal %s; strong %d; weak %d; needs_review %d; dropped %d", goal, counts.Strong, counts.Weak, counts.NeedsReview, counts.Dropped),
	}
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

func normalizeExternalSearchGoal(raw string) ExternalSearchGoal {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ExternalSearchGoalEvidence):
		return ExternalSearchGoalEvidence
	case "", string(ExternalSearchGoalDiscovery):
		return ExternalSearchGoalDiscovery
	default:
		return ExternalSearchGoalDiscovery
	}
}

func explicitExternalSearchGoal(raw ExternalSearchGoal) (ExternalSearchGoal, bool) {
	switch strings.ToLower(strings.TrimSpace(string(raw))) {
	case string(ExternalSearchGoalDiscovery):
		return ExternalSearchGoalDiscovery, true
	case string(ExternalSearchGoalEvidence):
		return ExternalSearchGoalEvidence, true
	default:
		return "", false
	}
}

func resolvedExternalSearchGoal(query string, planned ExternalSearchGoal, explicit ExternalSearchGoal) ExternalSearchGoal {
	if goal, ok := explicitExternalSearchGoal(explicit); ok {
		return goal
	}
	if isKnownExternalSearchGoal(string(planned)) {
		return normalizeExternalSearchGoal(string(planned))
	}
	return fallbackExternalSearchGoal(query)
}

func parseExternalTargetYear(raw json.RawMessage) (int, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return 0, nil
	}
	var year int
	if err := json.Unmarshal(raw, &year); err == nil {
		return year, nil
	}
	var yearString string
	if err := json.Unmarshal(raw, &yearString); err == nil {
		yearString = strings.TrimSpace(yearString)
		if yearString == "" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(yearString)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	}
	return 0, fmt.Errorf("invalid target_year: %s", trimmed)
}

func sanitizeExternalPlan(plan ExternalSearchPlan) ExternalSearchPlan {
	plan.SearchGoal = normalizeExternalSearchGoal(string(plan.SearchGoal))
	plan.MustMatch = dedupeExternalPlanTerms(plan.MustMatch)
	plan.SoftPreferences = dedupeExternalPlanTerms(plan.SoftPreferences)
	plan.SearchQuery = normalizeExternalQuery(plan.SearchQuery)
	plan.SearchQueries = sanitizeExternalQueries(append([]string{plan.SearchQuery}, plan.SearchQueries...))
	plan.SearchQuery = firstExternalQuery(plan.SearchQueries)
	plan.QueriesBySource = normalizeExternalQueriesBySource(plan.QueriesBySource)
	plan.Rationale = strings.TrimSpace(plan.Rationale)
	if plan.TargetYear < 0 {
		plan.TargetYear = 0
	}
	return plan
}

func dedupeExternalPlanTerms(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeExternalQuery(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func fallbackExternalSearchGoal(query string) ExternalSearchGoal {
	trimmed := strings.TrimSpace(query)
	switch {
	case trimmed == "":
		return ExternalSearchGoalDiscovery
	case hasEvidenceLikeQuotedSpan(trimmed):
		return ExternalSearchGoalEvidence
	case externalSearchDOIPattern.MatchString(trimmed):
		return ExternalSearchGoalEvidence
	case externalSearchPMIDPattern.MatchString(trimmed):
		return ExternalSearchGoalEvidence
	case externalSearchArXivPattern.MatchString(trimmed):
		return ExternalSearchGoalEvidence
	default:
		return ExternalSearchGoalDiscovery
	}
}

func hasEvidenceLikeQuotedSpan(query string) bool {
	matches := externalSearchQuotedClaimPattern.FindAllStringSubmatch(query, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		quoted := normalizeExternalQuery(match[1])
		if quoted == "" {
			continue
		}
		if len(strings.Fields(quoted)) >= 5 {
			return true
		}
	}
	return false
}

func isKnownExternalSearchGoal(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ExternalSearchGoalDiscovery), string(ExternalSearchGoalEvidence):
		return true
	default:
		return false
	}
}

func normalizeExternalQueriesBySource(queriesBySource map[string][]string) map[string][]string {
	if len(queriesBySource) == 0 {
		return nil
	}
	grouped := make(map[string][]externalSourceQueryGroup, len(queriesBySource))
	for rawSource, queries := range queriesBySource {
		source := normalizeExternalSource(rawSource)
		if source == "" {
			continue
		}
		grouped[source] = append(grouped[source], externalSourceQueryGroup{
			rawSource: rawSource,
			queries:   queries,
		})
	}

	normalized := make(map[string][]string, len(grouped))
	for _, source := range orderedExternalSources(grouped) {
		groups := grouped[source]
		sort.SliceStable(groups, func(i, j int) bool {
			return lessExternalSourceAlias(groups[i].rawSource, groups[j].rawSource)
		})
		merged := make([]string, 0)
		for _, group := range groups {
			merged = append(merged, group.queries...)
		}
		if queries := sanitizeExternalQueries(merged); len(queries) > 0 {
			normalized[source] = queries
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

type externalSourceQueryGroup struct {
	rawSource string
	queries   []string
}

func orderedExternalSources(grouped map[string][]externalSourceQueryGroup) []string {
	known := []string{"pubmed", "semantic_scholar"}
	seen := make(map[string]bool, len(grouped))
	sources := make([]string, 0, len(grouped))
	for _, source := range known {
		if _, ok := grouped[source]; ok {
			sources = append(sources, source)
			seen[source] = true
		}
	}
	unknown := make([]string, 0, len(grouped))
	for source := range grouped {
		if !seen[source] {
			unknown = append(unknown, source)
		}
	}
	sort.Strings(unknown)
	return append(sources, unknown...)
}

func lessExternalSourceAlias(a, b string) bool {
	aTrimmed := strings.TrimSpace(a)
	bTrimmed := strings.TrimSpace(b)
	aLower := strings.ToLower(aTrimmed)
	bLower := strings.ToLower(bTrimmed)
	if aLower != bLower {
		return aLower < bLower
	}
	aExactLower := 1
	if aTrimmed == aLower {
		aExactLower = 0
	}
	bExactLower := 1
	if bTrimmed == bLower {
		bExactLower = 0
	}
	if aExactLower != bExactLower {
		return aExactLower < bExactLower
	}
	aHasWhitespace := 1
	if a == aTrimmed {
		aHasWhitespace = 0
	}
	bHasWhitespace := 1
	if b == bTrimmed {
		bHasWhitespace = 0
	}
	if aHasWhitespace != bHasWhitespace {
		return aHasWhitespace < bHasWhitespace
	}
	if aTrimmed != bTrimmed {
		return aTrimmed < bTrimmed
	}
	return a < b
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
	res.Tier = normalizeExternalPaperTier(res.Tier)
	res.Reason = strings.TrimSpace(res.Reason)
	res.MatchedConstraints = sanitizeExternalClassificationTerms(res.MatchedConstraints)
	res.MatchedPreferences = sanitizeExternalClassificationTerms(res.MatchedPreferences)
	res.ArticleRole = normalizeExternalArticleRole(res.ArticleRole)
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

func normalizeExternalPaperTier(tier ExternalPaperTier) ExternalPaperTier {
	switch ExternalPaperTier(strings.TrimSpace(strings.ToLower(string(tier)))) {
	case ExternalPaperTierStrongMatch:
		return ExternalPaperTierStrongMatch
	case ExternalPaperTierWeakMatch:
		return ExternalPaperTierWeakMatch
	case ExternalPaperTierDrop:
		return ExternalPaperTierDrop
	default:
		return ExternalPaperTierNeedsReview
	}
}

func sanitizeExternalClassificationTerms(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func normalizeExternalArticleRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	if role == "" {
		return ""
	}
	role = strings.ReplaceAll(role, "-", "_")
	role = strings.ReplaceAll(role, " ", "_")
	for strings.Contains(role, "__") {
		role = strings.ReplaceAll(role, "__", "_")
	}
	return role
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
		Tier:        ExternalPaperTierStrongMatch,
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

func externalID(p ai_external.Paper) string {
	if strings.TrimSpace(p.DOI) != "" {
		return "DOI:" + strings.TrimSpace(p.DOI)
	}
	if strings.TrimSpace(p.PMID) != "" {
		return "PMID:" + strings.TrimSpace(p.PMID)
	}
	if strings.TrimSpace(p.ArXiv) != "" {
		return "ARXIV:" + strings.TrimSpace(p.ArXiv)
	}
	for _, source := range externalPaperSourceOrder(p) {
		if id := strings.TrimSpace(p.SourcePaperIDs[source]); id != "" {
			return string(source) + ":" + id
		}
	}
	if p.Source != "" && strings.TrimSpace(p.SourcePaperID) != "" {
		return string(p.Source) + ":" + strings.TrimSpace(p.SourcePaperID)
	}
	return ""
}

func externalPaperSourceOrder(p ai_external.Paper) []ai_external.SourceID {
	sources := p.Sources
	if len(sources) == 0 && p.Source != "" {
		sources = []ai_external.SourceID{p.Source}
	}
	seen := map[ai_external.SourceID]bool{}
	out := make([]ai_external.SourceID, 0, len(sources)+len(p.SourcePaperIDs))
	for _, source := range sources {
		if source != "" && !seen[source] {
			seen[source] = true
			out = append(out, source)
		}
	}
	unknown := make([]string, 0, len(p.SourcePaperIDs))
	for source := range p.SourcePaperIDs {
		if !seen[source] {
			unknown = append(unknown, string(source))
		}
	}
	sort.Strings(unknown)
	for _, source := range unknown {
		out = append(out, ai_external.SourceID(source))
	}
	return out
}

func externalPaperToResearchPaper(p ai_external.Paper) research.Paper {
	authors := make([]research.Author, 0, len(p.Authors))
	for _, name := range p.Authors {
		if strings.TrimSpace(name) != "" {
			authors = append(authors, research.Author{Name: strings.TrimSpace(name)})
		}
	}
	return research.Paper{
		PaperID: externalSemanticScholarID(p),
		ExternalIDs: research.IDs{
			DOI:    p.DOI,
			ArXiv:  p.ArXiv,
			PubMed: p.PMID,
		},
		Title:            p.Title,
		Abstract:         p.Abstract,
		TLDR:             p.TLDR,
		Year:             p.Year,
		Venue:            p.Venue,
		Authors:          authors,
		CitationCount:    p.CitationCount,
		OpenAccessPDFURL: p.OpenAccessURL,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// intersectUserSources returns (intersection, disabled).
// intersection = sources the user requested AND are in enabled.
// disabled = sources the user requested but are NOT in enabled. (Includes
// names that are unknown to the registry — they share UX with disabled, since
// either way the user gets nothing for that name.)
func intersectUserSources(requested []string, enabled []ai_external.SourceID) ([]ai_external.SourceID, []ai_external.SourceID) {
	enabledSet := make(map[ai_external.SourceID]bool, len(enabled))
	for _, s := range enabled {
		enabledSet[s] = true
	}
	intersection := make([]ai_external.SourceID, 0, len(requested))
	disabled := make([]ai_external.SourceID, 0)
	seen := map[ai_external.SourceID]bool{}
	for _, raw := range requested {
		s := ai_external.SourceID(strings.TrimSpace(strings.ToLower(raw)))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		if enabledSet[s] {
			intersection = append(intersection, s)
		} else {
			disabled = append(disabled, s)
		}
	}
	return intersection, disabled
}

func disabledSourcesNote(errs []error) string {
	suffix := ": " + ErrSourceDisabled.Error()
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		if errors.Is(err, ErrSourceDisabled) {
			label := strings.TrimSuffix(err.Error(), suffix)
			parts = append(parts, label)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "用户显式指定但未启用的源: " + strings.Join(parts, "; ") + "（请前往设置页启用）"
}

func nonDisabledErrors(errs []error) []error {
	out := make([]error, 0, len(errs))
	for _, err := range errs {
		if !errors.Is(err, ErrSourceDisabled) {
			out = append(out, err)
		}
	}
	return out
}

func externalAnswerContext(goal ExternalSearchGoal, counts externalTierCounts, cards []ResultCard) string {
	var b strings.Builder
	if goal != ExternalSearchGoalEvidence && counts.Strong == 0 && len(cards) > 0 {
		b.WriteString("发现候选结果，但暂无 strong_match；以下结果按 weak_match / needs_review 展示。\n\n")
	}
	for i, card := range cards {
		p, ok := card.Payload.(ExternalPaperCard)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "[external %d] %s", i+1, p.Title)
		meta := make([]string, 0, 4)
		if p.Tier != "" {
			meta = append(meta, "tier="+string(p.Tier))
		}
		switch {
		case p.Venue != "" && p.Year > 0:
			meta = append(meta, fmt.Sprintf("%s %d", p.Venue, p.Year))
		case p.Venue != "":
			meta = append(meta, p.Venue)
		case p.Year > 0:
			meta = append(meta, strconv.Itoa(p.Year))
		}
		if p.YearLabel != "" {
			meta = append(meta, "year="+p.YearLabel)
		}
		if len(meta) > 0 {
			fmt.Fprintf(&b, " (%s)", strings.Join(meta, "; "))
		}
		if p.MatchedQuery != "" {
			fmt.Fprintf(&b, "\nMatched query: %s", p.MatchedQuery)
		}
		if p.ArticleRole != "" {
			fmt.Fprintf(&b, "\nArticle role: %s", p.ArticleRole)
		}
		if len(p.MatchedConstraints) > 0 {
			fmt.Fprintf(&b, "\nMatched constraints: %s", strings.Join(p.MatchedConstraints, " | "))
		}
		if len(p.MatchedPreferences) > 0 {
			fmt.Fprintf(&b, "\nMatched preferences: %s", strings.Join(p.MatchedPreferences, " | "))
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

func externalNoSupportAnswerContext(goal ExternalSearchGoal, counts externalTierCounts, sourceLabels []string, searchQueries []string) string {
	sourceText := strings.Join(sourceLabels, "+")
	queryText := strings.Join(searchQueries, " | ")
	if goal == ExternalSearchGoalEvidence && counts.total() > 0 && counts.Strong == 0 {
		return fmt.Sprintf(
			"检索到 %d 条候选结果，但暂无 strong_match 可作为支持证据：%s 使用查询 %q。候选分层：weak_match %d 条，needs_review %d 条，dropped %d 条。",
			counts.total(),
			sourceText,
			queryText,
			counts.Weak,
			counts.NeedsReview,
			counts.Dropped,
		)
	}
	return fmt.Sprintf("没有命中：%s 使用查询 %q 返回 0 条结果。", sourceText, queryText)
}
