package ai_assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/service/research"
)

type ExternalSearcher interface {
	Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error)
	SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error)
}

type ExternalSearchPlanner interface {
	PlanExternalSearch(ctx context.Context, query string) (ExternalSearchPlan, error)
}

type ExternalSearchPlan struct {
	SearchQuery string `json:"search_query"`
	Rationale   string `json:"rationale,omitempty"`
}

type ExternalSearchTool struct {
	searcher ExternalSearcher
	planner  ExternalSearchPlanner
}

const (
	defaultExternalSearchLimit = 8
	maxExternalSearchLimit     = 100
)

type ExternalPaperCard struct {
	S2PaperID     string `json:"s2_paper_id"`
	Title         string `json:"title"`
	Year          int    `json:"year,omitempty"`
	Venue         string `json:"venue,omitempty"`
	DOI           string `json:"doi,omitempty"`
	TLDR          string `json:"tldr,omitempty"`
	Abstract      string `json:"abstract,omitempty"`
	CitationIndex int    `json:"citation_index,omitempty"`
}

func NewExternalSearchTool(searcher ExternalSearcher) *ExternalSearchTool {
	return NewExternalSearchToolWithPlanner(searcher, nil)
}

func NewExternalSearchToolWithPlanner(searcher ExternalSearcher, planner ExternalSearchPlanner) *ExternalSearchTool {
	return &ExternalSearchTool{searcher: searcher, planner: planner}
}

func (t *ExternalSearchTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	limit := clampExternalSearchLimit(in.Limit)
	searchQuery := ExternalSearchQuery(in.Query)
	if t != nil {
		searchQuery, _ = t.searchQuery(ctx, in.Query)
	}
	inputJSON, _ := json.Marshal(struct {
		Query       string `json:"query"`
		SearchQuery string `json:"search_query,omitempty"`
		Limit       int    `json:"limit"`
	}{Query: in.Query, SearchQuery: searchQuery, Limit: limit})

	if t == nil || t.searcher == nil {
		return externalSearchFailedResult(inputJSON, errors.New("external searcher is not configured")), nil
	}

	res, err := t.searcher.Search(ctx, searchQuery, research.SearchOpts{Limit: limit})
	if err != nil {
		return externalSearchFailedResult(inputJSON, err), nil
	}

	cards := make([]ResultCard, 0, len(res.Items))
	citations := make([]Citation, 0, len(res.Items))
	for _, p := range res.Items {
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
			S2PaperID:     p.PaperID,
			Title:         p.Title,
			Year:          p.Year,
			Venue:         p.Venue,
			DOI:           p.ExternalIDs.DOI,
			TLDR:          p.TLDR,
			Abstract:      p.Abstract,
			CitationIndex: citation.I,
		}})
	}

	outputJSON, _ := json.Marshal(struct {
		Hits int `json:"hits"`
	}{Hits: len(cards)})

	return ToolResult{
		Process: ProcessSummary{
			Intent: IntentExternalSearch,
			Stages: []ProcessStage{
				{Label: "外部搜索", Count: len(res.Items), Unit: "条", Status: "completed"},
				externalHitStage(len(cards)),
			},
		},
		Cards:         cards,
		Citations:     citations,
		AnswerContext: externalAnswerContext(cards),
		ToolCalls: []ToolCallSummary{{
			ToolName:          "external_search",
			InputJSON:         string(inputJSON),
			OutputSummaryJSON: string(outputJSON),
			Status:            "completed",
		}},
	}, nil
}

func (t *ExternalSearchTool) searchQuery(ctx context.Context, query string) (string, ExternalSearchPlan) {
	fallback := ExternalSearchQuery(query)
	if t == nil || t.planner == nil {
		return fallback, ExternalSearchPlan{}
	}
	plan, err := t.planner.PlanExternalSearch(ctx, query)
	if err != nil {
		return fallback, ExternalSearchPlan{}
	}
	plan.SearchQuery = strings.Join(strings.Fields(plan.SearchQuery), " ")
	plan.Rationale = strings.TrimSpace(plan.Rationale)
	if plan.SearchQuery == "" {
		return fallback, ExternalSearchPlan{}
	}
	return plan.SearchQuery, plan
}

func ExternalSearchQuery(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return q
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
	if containsAnyEvidenceText(lower, "变慢", "速度变慢", "slowed", "declined") {
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
		return strings.Join(terms, " ")
	}
	return strings.TrimSpace(strings.NewReplacer(
		"外部", " ",
		"查一下", " ",
		"查找", " ",
		"检索", " ",
		"有没有", " ",
		"关于", " ",
		"综述", " review ",
		"：", " ",
		":", " ",
	).Replace(q))
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

func externalSearchFailedResult(inputJSON []byte, err error) ToolResult {
	return ToolResult{
		Process: ProcessSummary{
			Intent: IntentExternalSearch,
			Stages: []ProcessStage{
				{Label: "外部搜索", Status: "failed"},
			},
		},
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
