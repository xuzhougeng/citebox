package ai_assistant

import (
	"context"
	"github.com/xuzhougeng/citebox/internal/model"
	"strings"
	"testing"
)

func longPaperFixture() string {
	return strings.Repeat("Author affiliations and front matter. ", 1200) +
		"\nINTRODUCTION\n" + strings.Repeat("Introduction. ", 600) +
		"\nRESULTS\n" + strings.Repeat("Result observations. ", 1200) +
		"\nDISCUSSION\n" + strings.Repeat("Discussion. ", 600) +
		"\nSTAR METHODS\nUnique experimental design and randomization protocol.\n" +
		strings.Repeat("Method details. ", 800) +
		"\nData and code availability\nENDPOINT_REPOSITORY_ACCESS"
}
func TestPaperTextIncludesWholeBodyBeyondOldPrefixCap(t *testing.T) {
	text := longPaperFixture()
	got := SelectPaperText(text, "研究设计", len([]rune(text)))
	if got.Text != text || got.Included != got.Total {
		t.Fatal("full text that fits was shortened")
	}
}
func TestPaperTextCoversMethodsAndTailWithoutKeywordMatch(t *testing.T) {
	text := longPaperFixture()
	got := SelectPaperText(text, "研究设计架构解构", 9000)
	for _, part := range []string{"STAR METHODS", "Unique experimental design", "ENDPOINT_REPOSITORY_ACCESS"} {
		if !strings.Contains(got.Text, part) {
			t.Fatalf("missing %s", part)
		}
	}
	if got.Included > 9000 || got.Included >= got.Total {
		t.Fatalf("incorrect sampling count: %+v", got)
	}
	count := 0
	for i, span := range got.Ranges {
		if span.Start < 0 || span.End > got.Total || span.End <= span.Start {
			t.Fatalf("invalid range %+v", span)
		}
		if i > 0 && span.Start <= got.Ranges[i-1].End {
			t.Fatal("overlapping ranges")
		}
		count += span.End - span.Start
	}
	if count != got.Included {
		t.Fatal("coverage double counted")
	}
}
func TestPaperTextUnicodeAndUnstructuredTail(t *testing.T) {
	text := strings.Repeat("正文前部。", 10000) + "TAIL_SENTINEL"
	for _, budget := range []int{1, 20, 300, 2000} {
		got := SelectPaperText(text, "unmatched", budget)
		if got.Included > budget {
			t.Fatal("budget exceeded")
		}
		if budget >= 40 && !strings.Contains(got.Text, "TAIL_SENTINEL") {
			t.Fatal("unstructured tail not covered")
		}
	}
}
func TestPaperReadToolReturnsTailForBroadQuestion(t *testing.T) {
	store := stubPaperStore{papers: map[int64]*model.Paper{1: {ID: 1, Title: "Long study", PDFText: longPaperFixture()}}}
	result, err := NewPaperReadTool(store).Run(context.Background(), ToolInput{Query: "研究设计架构解构", Context: RequestContext{PaperID: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Citations) <= 3 || !strings.Contains(result.AnswerContext, "STAR METHODS") || !strings.Contains(result.AnswerContext, "ENDPOINT_REPOSITORY_ACCESS") {
		t.Fatal("reading tool did not reach methods and tail")
	}
}
