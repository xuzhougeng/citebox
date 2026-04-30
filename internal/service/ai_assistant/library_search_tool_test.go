package ai_assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

type stubPaperStore struct {
	papers map[int64]*model.Paper
	ids    []int64
	err    error
}

func (s stubPaperStore) GetPaperDetail(id int64) (*model.Paper, error) {
	return s.papers[id], nil
}

func (s stubPaperStore) ListEvidenceCandidatePaperIDs(terms []string, limit int) ([]int64, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]int64(nil), s.ids...), nil
}

func TestLibrarySearchToolReturnsPaperHitCards(t *testing.T) {
	tool := NewLibrarySearchTool(stubPaperStore{
		ids: []int64{1, 2},
		papers: map[int64]*model.Paper{
			1: {
				ID:           1,
				Title:        "ATAC Atlas",
				DOI:          "10.1/atac",
				AbstractText: "The atlas studies chromatin accessibility.",
				PDFText:      "ATAC-seq identifies chromatin accessibility changes.",
			},
			2: {ID: 2, Title: "Unrelated", PDFText: "Protein localization only."},
		},
	})
	res, err := tool.Run(context.Background(), ToolInput{Query: "帮我查找包括 ATAC 数据的文章"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 1 || res.Cards[0].Type != "paper_hit" {
		t.Fatalf("cards = %+v", res.Cards)
	}
	if len(res.Citations) != 3 {
		t.Fatalf("citations = %+v", res.Citations)
	}
	for _, citation := range res.Citations {
		if citation.PaperID != 1 {
			t.Fatalf("citation = %+v, want PaperID 1", citation)
		}
	}
	card, ok := res.Cards[0].Payload.(PaperHitCard)
	if !ok {
		t.Fatalf("card payload = %T, want PaperHitCard", res.Cards[0].Payload)
	}
	if len(card.Snippets) != 3 {
		t.Fatalf("snippets = %+v", card.Snippets)
	}
	if !strings.Contains(res.AnswerContext, "ATAC Atlas") || !strings.Contains(res.AnswerContext, "chromatin accessibility") {
		t.Fatalf("answer context = %s", res.AnswerContext)
	}
	if len(res.Process.Stages) == 0 || res.Process.Stages[0].Label != "全库检索" {
		t.Fatalf("process = %+v", res.Process)
	}
}

func TestLibrarySearchToolReportsCandidateListerFailure(t *testing.T) {
	tool := NewLibrarySearchTool(stubPaperStore{err: errors.New("candidate query failed")})

	res, err := tool.Run(context.Background(), ToolInput{Query: "ATAC"})
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if res.Process.Intent != IntentLibrarySearch {
		t.Fatalf("process intent = %q", res.Process.Intent)
	}
	if len(res.Process.Stages) == 0 || res.Process.Stages[0].Label != "全库检索" || res.Process.Stages[0].Status != "failed" {
		t.Fatalf("process = %+v", res.Process)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].ToolName != "library_search" || res.ToolCalls[0].Status != "failed" {
		t.Fatalf("tool calls = %+v", res.ToolCalls)
	}
	if !strings.Contains(res.ToolCalls[0].Error, "candidate query failed") {
		t.Fatalf("tool call error = %q", res.ToolCalls[0].Error)
	}
	if len(res.Cards) != 0 || len(res.Citations) != 0 {
		t.Fatalf("cards=%+v citations=%+v, want empty", res.Cards, res.Citations)
	}
}

func TestLibrarySearchToolClampsLargeLimit(t *testing.T) {
	store := stubPaperStore{
		papers: make(map[int64]*model.Paper),
	}
	for i := int64(1); i <= 60; i++ {
		store.ids = append(store.ids, i)
		store.papers[i] = &model.Paper{
			ID:      i,
			Title:   fmt.Sprintf("Paper %d", i),
			PDFText: "This paper discusses chromatin accessibility.",
		}
	}
	tool := NewLibrarySearchTool(store)

	res, err := tool.Run(context.Background(), ToolInput{Query: "ATAC", Limit: 1000})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 50 {
		t.Fatalf("card count = %d, want 50", len(res.Cards))
	}
	if len(res.Citations) != 50 {
		t.Fatalf("citation count = %d, want 50", len(res.Citations))
	}
	if len(res.Process.Stages) < 2 || res.Process.Stages[1].Count != 50 {
		t.Fatalf("process = %+v", res.Process)
	}
}
