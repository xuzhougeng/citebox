package ai_assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

type PaperReadTool struct {
	papers PaperGetter
}

const maxPaperReadPapers = 2

type PaperCompareCard struct {
	Query  string             `json:"query"`
	Papers []PaperCompareItem `json:"papers"`
	Note   string             `json:"note,omitempty"`
}

type PaperCompareItem struct {
	PaperID  int64             `json:"paper_id"`
	Title    string            `json:"title"`
	Evidence []PaperHitSnippet `json:"evidence"`
}

func NewPaperReadTool(papers PaperGetter) *PaperReadTool {
	return &PaperReadTool{papers: papers}
}

func (t *PaperReadTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	ids := append([]int64(nil), in.Context.PaperIDs...)
	if len(ids) == 0 && in.Context.PaperID > 0 {
		ids = []int64{in.Context.PaperID}
	}
	requested := len(ids)
	inputJSON, _ := json.Marshal(struct {
		PaperIDs []int64 `json:"paper_ids"`
		Query    string  `json:"query"`
		Limit    int     `json:"limit,omitempty"`
	}{PaperIDs: ids, Query: in.Query, Limit: in.Limit})
	if len(ids) == 0 {
		return paperReadSkippedResult(inputJSON, paperReadSummaryJSON(requested, 0, 0, 0, "no_papers"), "没有指定文献", nil), nil
	}
	if t == nil || t.papers == nil {
		return paperReadSkippedResult(inputJSON, paperReadSummaryJSON(requested, 0, requested, 0, "no_paper_getter"), "文献读取器未配置", errors.New("paper getter is not configured")), nil
	}

	readLimit := paperReadLimit(in.Limit)
	readIDs := ids
	if len(readIDs) > readLimit {
		readIDs = readIDs[:readLimit]
	}
	terms := EvidenceSearchTerms(in.Query)
	items := make([]PaperCompareItem, 0, len(readIDs))
	citations := make([]Citation, 0, len(readIDs)*3)
	skipped := requested - len(readIDs)
	for _, id := range readIDs {
		if err := ctx.Err(); err != nil {
			return ToolResult{}, err
		}
		p, err := t.papers.GetPaperDetail(id)
		if err != nil || p == nil {
			skipped++
			continue
		}
		matches := FindLocalEvidenceMatches(*p, terms, 3)
		if len(matches) == 0 {
			matches = fallbackPaperEvidenceMatches(*p, 1)
		}
		item := PaperCompareItem{PaperID: p.ID, Title: p.Title}
		for _, m := range matches {
			citation := Citation{
				I:       len(citations) + 1,
				PaperID: p.ID,
				Title:   p.Title,
				Source:  "local",
				Snippet: m.Snippet,
				Score:   m.Score,
			}
			citations = append(citations, citation)
			item.Evidence = append(item.Evidence, PaperHitSnippet{
				CitationIndex: citation.I,
				Location:      m.Location,
				Text:          m.Snippet.Text,
			})
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return paperReadSkippedResult(inputJSON, paperReadSummaryJSON(requested, 0, skipped, 0, "no_loaded_papers"), "没有读取到可用文献", nil), nil
	}

	cardType := "paper_compare"
	if len(items) == 1 {
		cardType = "paper_read"
	}
	outputJSON := paperReadSummaryJSON(requested, len(items), skipped, len(citations), "")
	note := ""
	if skipped > 0 {
		note = fmt.Sprintf("已读取 %d 篇，跳过 %d 篇。", len(items), skipped)
	}

	return ToolResult{
		Process: ProcessSummary{
			Intent: IntentPaperRead,
			Note:   note,
			Stages: []ProcessStage{
				{Label: "全文扫描", Count: len(items), Unit: "篇", Status: "completed"},
				{Label: "命中证据", Count: len(citations), Unit: "段", Status: "completed"},
			},
		},
		Cards: []ResultCard{{Type: cardType, Payload: PaperCompareCard{
			Query:  in.Query,
			Papers: items,
			Note:   compareNote(len(ids)),
		}}},
		Citations:     citations,
		AnswerContext: paperCompareAnswerContext(items),
		ToolCalls: []ToolCallSummary{{
			ToolName:          "paper_read",
			InputJSON:         string(inputJSON),
			OutputSummaryJSON: outputJSON,
			Status:            "completed",
		}},
	}, nil
}

func paperReadLimit(limit int) int {
	if limit <= 0 || limit > maxPaperReadPapers {
		return maxPaperReadPapers
	}
	return limit
}

func paperReadSummaryJSON(requested, loaded, skipped, evidence int, reason string) string {
	outputJSON, _ := json.Marshal(struct {
		Requested int    `json:"requested"`
		Loaded    int    `json:"loaded"`
		Skipped   int    `json:"skipped"`
		Evidence  int    `json:"evidence"`
		Reason    string `json:"reason,omitempty"`
	}{Requested: requested, Loaded: loaded, Skipped: skipped, Evidence: evidence, Reason: reason})
	return string(outputJSON)
}

func paperReadSkippedResult(inputJSON []byte, outputJSON, note string, err error) ToolResult {
	call := ToolCallSummary{
		ToolName:          "paper_read",
		InputJSON:         string(inputJSON),
		OutputSummaryJSON: outputJSON,
		Status:            "skipped",
	}
	if err != nil {
		call.Error = err.Error()
	}
	return ToolResult{
		Process: ProcessSummary{
			Intent: IntentPaperRead,
			Note:   note,
			Stages: []ProcessStage{
				{Label: "全文扫描", Count: 0, Unit: "篇", Status: "skipped"},
			},
		},
		ToolCalls: []ToolCallSummary{call},
	}
}

func fallbackPaperEvidenceMatches(paper model.Paper, limit int) []LocalEvidenceMatch {
	if limit <= 0 {
		return nil
	}
	text := firstNonEmpty(paper.AbstractText, paper.PDFText, paper.NotesText, paper.PaperNotesText)
	if text == "" {
		return nil
	}
	if len([]rune(text)) > 360 {
		text = string([]rune(text)[:360])
	}
	return []LocalEvidenceMatch{{
		Location: "全文",
		Snippet: research.Snippet{
			Text:          text,
			SnippetKind:   "body",
			Section:       "全文",
			SnippetOffset: research.SnippetOffset{Start: 0, End: len(text)},
		},
		Score: 1,
	}}
}

func compareNote(n int) string {
	if n <= 2 {
		return ""
	}
	return "已完成多篇全文证据扫描；请选择 1-2 篇继续深入展开。"
}

func paperCompareAnswerContext(items []PaperCompareItem) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "### %s\n", item.Title)
		for _, ev := range item.Evidence {
			fmt.Fprintf(&b, "- [%d] %s: %s\n", ev.CitationIndex, ev.Location, ev.Text)
		}
		b.WriteString("\n")
	}
	return b.String()
}
