package ai_assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
)

type FigureRecord struct {
	FigureID     int64  `json:"figure_id"`
	PaperID      int64  `json:"paper_id"`
	PaperTitle   string `json:"paper_title"`
	DisplayLabel string `json:"display_label"`
	ImageURL     string `json:"image_url,omitempty"`
	Caption      string `json:"caption,omitempty"`
	NotesText    string `json:"notes_text,omitempty"`
}

type FigureSearcher interface {
	SearchFigures(query string, paperID int64, limit int) ([]FigureRecord, int, error)
}

type FigureLookupTool struct {
	figures FigureSearcher
}

const (
	defaultFigureLookupLimit = 12
	maxFigureLookupLimit     = 50
)

type FigureResultCard FigureRecord

type FigureListProvider interface {
	ListFigures(filter model.FigureFilter) ([]model.FigureListItem, int, error)
}

type RepositoryFigureSearcher struct {
	repo FigureListProvider
}

func NewFigureLookupTool(figures FigureSearcher) *FigureLookupTool {
	return &FigureLookupTool{figures: figures}
}

func (t *FigureLookupTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolResult{}, err
	}
	limit := clampFigureLookupLimit(in.Limit)
	inputJSON, _ := json.Marshal(struct {
		Query   string `json:"query"`
		PaperID int64  `json:"paper_id,omitempty"`
		Limit   int    `json:"limit"`
	}{Query: in.Query, PaperID: in.Context.PaperID, Limit: limit})

	if t == nil || t.figures == nil {
		return figureLookupFailedResult(inputJSON, errors.New("figure searcher is not configured")), nil
	}

	items, total, err := t.searchFigures(ctx, in.Query, in.Context.PaperID, limit)
	if err != nil {
		return figureLookupFailedResult(inputJSON, err), nil
	}

	cards := make([]ResultCard, 0, len(items))
	for _, item := range items {
		cards = append(cards, ResultCard{Type: "figure_result", Payload: FigureResultCard(item)})
	}
	outputJSON, _ := json.Marshal(struct {
		Total int `json:"total"`
		Hits  int `json:"hits"`
	}{Total: total, Hits: len(cards)})

	return ToolResult{
		Process: ProcessSummary{
			Intent: IntentFigureLookup,
			Stages: []ProcessStage{
				{Label: "图文检索", Count: total, Unit: "张图", Status: "completed"},
				{Label: "命中", Count: len(cards), Unit: "张", Status: "completed"},
			},
		},
		Cards:         cards,
		AnswerContext: figureAnswerContext(items),
		ToolCalls: []ToolCallSummary{{
			ToolName:          "figure_lookup",
			InputJSON:         string(inputJSON),
			OutputSummaryJSON: string(outputJSON),
			Status:            "completed",
		}},
	}, nil
}

func (t *FigureLookupTool) searchFigures(ctx context.Context, query string, paperID int64, limit int) ([]FigureRecord, int, error) {
	items, total, err := t.figures.SearchFigures(query, paperID, limit)
	if err != nil || len(items) > 0 || total > 0 {
		return items, total, err
	}
	for _, term := range FigureSearchTerms(query) {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		if strings.EqualFold(strings.TrimSpace(term), strings.TrimSpace(query)) {
			continue
		}
		items, total, err = t.figures.SearchFigures(term, paperID, limit)
		if err != nil || len(items) > 0 || total > 0 {
			return items, total, err
		}
	}
	return items, total, err
}

func FigureSearchTerms(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	lower := strings.ToLower(q)
	terms := make([]string, 0, 12)
	if containsAnyEvidenceText(lower, "chip-seq", "chip seq", "chipseq", "chip", "染色质免疫沉淀") {
		terms = append(terms,
			"ChIP-seq",
			"ChIP seq",
			"chromatin immunoprecipitation",
			"H3K27ac ChIP-seq",
			"H3K4me3 ChIP-seq",
		)
	}
	terms = append(terms, EvidenceSearchTerms(q)...)
	return sanitizeEvidenceTerms(terms)
}

func clampFigureLookupLimit(limit int) int {
	if limit <= 0 {
		return defaultFigureLookupLimit
	}
	if limit > maxFigureLookupLimit {
		return maxFigureLookupLimit
	}
	return limit
}

func figureLookupFailedResult(inputJSON []byte, err error) ToolResult {
	return ToolResult{
		Process: ProcessSummary{
			Intent: IntentFigureLookup,
			Stages: []ProcessStage{
				{Label: "图文检索", Status: "failed"},
			},
		},
		ToolCalls: []ToolCallSummary{{
			ToolName:  "figure_lookup",
			InputJSON: string(inputJSON),
			Status:    "failed",
			Error:     err.Error(),
		}},
	}
}

func NewRepositoryFigureSearcher(repo FigureListProvider) *RepositoryFigureSearcher {
	return &RepositoryFigureSearcher{repo: repo}
}

func (s *RepositoryFigureSearcher) SearchFigures(query string, paperID int64, limit int) ([]FigureRecord, int, error) {
	if s == nil || s.repo == nil {
		return nil, 0, errors.New("figure repository is not configured")
	}
	limit = clampFigureLookupLimit(limit)
	filter := model.FigureFilter{Keyword: query, Page: 1, PageSize: limit}
	if paperID > 0 {
		filter.PaperID = &paperID
	}
	figures, total, err := s.repo.ListFigures(filter)
	if err != nil {
		return nil, 0, err
	}
	out := make([]FigureRecord, 0, len(figures))
	for _, figure := range figures {
		out = append(out, FigureRecordFromListItem(figure))
	}
	return out, total, nil
}

func FigureRecordFromListItem(item model.FigureListItem) FigureRecord {
	return FigureRecord{
		FigureID:     item.ID,
		PaperID:      item.PaperID,
		PaperTitle:   item.PaperTitle,
		DisplayLabel: firstNonEmpty(item.DisplayLabel, item.ParentDisplayLabel, item.Filename),
		ImageURL:     figureImageURL(item),
		Caption:      item.Caption,
		NotesText:    item.NotesText,
	}
}

func figureImageURL(item model.FigureListItem) string {
	if strings.TrimSpace(item.ImageURL) != "" {
		return item.ImageURL
	}
	if strings.TrimSpace(item.Filename) == "" {
		return ""
	}
	return "/files/figures/" + url.PathEscape(item.Filename)
}

func figureAnswerContext(items []FigureRecord) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "### %s - %s\n", item.PaperTitle, item.DisplayLabel)
		if item.Caption != "" {
			fmt.Fprintf(&b, "Caption: %s\n", item.Caption)
		}
		if item.NotesText != "" {
			fmt.Fprintf(&b, "Notes: %s\n", item.NotesText)
		}
		b.WriteString("\n")
	}
	return b.String()
}
