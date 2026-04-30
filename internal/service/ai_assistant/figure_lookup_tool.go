package ai_assistant

import (
	"context"
	"encoding/json"
	"fmt"
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
	limit := in.Limit
	if limit <= 0 {
		limit = 12
	}
	inputJSON, _ := json.Marshal(struct {
		Query   string `json:"query"`
		PaperID int64  `json:"paper_id,omitempty"`
		Limit   int    `json:"limit"`
	}{Query: in.Query, PaperID: in.Context.PaperID, Limit: limit})

	items, total, err := t.figures.SearchFigures(in.Query, in.Context.PaperID, limit)
	if err != nil {
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
		}, nil
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

func NewRepositoryFigureSearcher(repo FigureListProvider) *RepositoryFigureSearcher {
	return &RepositoryFigureSearcher{repo: repo}
}

func (s *RepositoryFigureSearcher) SearchFigures(query string, paperID int64, limit int) ([]FigureRecord, int, error) {
	if limit <= 0 {
		limit = 12
	}
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
		ImageURL:     item.ImageURL,
		Caption:      item.Caption,
		NotesText:    item.NotesText,
	}
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
