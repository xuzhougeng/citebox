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

type ExternalSearchTool struct {
	searcher ExternalSearcher
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
	CitationIndex int    `json:"citation_index,omitempty"`
}

func NewExternalSearchTool(searcher ExternalSearcher) *ExternalSearchTool {
	return &ExternalSearchTool{searcher: searcher}
}

func (t *ExternalSearchTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	limit := clampExternalSearchLimit(in.Limit)
	inputJSON, _ := json.Marshal(struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}{Query: in.Query, Limit: limit})

	if t == nil || t.searcher == nil {
		return externalSearchFailedResult(inputJSON, errors.New("external searcher is not configured")), nil
	}

	res, err := t.searcher.Search(ctx, in.Query, research.SearchOpts{Limit: limit})
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
				Text:        firstNonEmpty(p.TLDR, p.Abstract, p.Title),
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
				{Label: "命中", Count: len(cards), Unit: "条", Status: "completed"},
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
			fmt.Fprintf(&b, "\n%s", p.TLDR)
		}
		b.WriteString("\n\n")
	}
	return b.String()
}
