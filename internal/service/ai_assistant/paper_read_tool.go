package ai_assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

type PaperReadTool struct {
	papers PaperGetter
}

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
	if len(ids) == 0 {
		return ToolResult{
			Process: ProcessSummary{Intent: IntentPaperRead, Note: "没有指定文献"},
			ToolCalls: []ToolCallSummary{{
				ToolName:          "paper_read",
				Status:            "skipped",
				OutputSummaryJSON: `{"reason":"no_papers"}`,
			}},
		}, nil
	}

	terms := EvidenceSearchTerms(in.Query)
	items := make([]PaperCompareItem, 0, len(ids))
	citations := make([]Citation, 0, len(ids)*3)
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return ToolResult{}, err
		}
		p, err := t.papers.GetPaperDetail(id)
		if err != nil || p == nil {
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

	cardType := "paper_compare"
	if len(items) == 1 {
		cardType = "paper_read"
	}
	inputJSON, _ := json.Marshal(struct {
		PaperIDs []int64 `json:"paper_ids"`
		Query    string  `json:"query"`
	}{PaperIDs: ids, Query: in.Query})
	outputJSON, _ := json.Marshal(struct {
		Papers    int `json:"papers"`
		Citations int `json:"citations"`
	}{Papers: len(items), Citations: len(citations)})

	return ToolResult{
		Process: ProcessSummary{
			Intent: IntentPaperRead,
			Stages: []ProcessStage{
				{Label: "全文扫描", Count: len(ids), Unit: "篇", Status: "completed"},
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
			OutputSummaryJSON: string(outputJSON),
			Status:            "completed",
		}},
	}, nil
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
