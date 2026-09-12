package ai_assistant

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

func longStructuredPaper() string {
	return "TITLE AND AUTHORS\n" + strings.Repeat("front matter ", 3000) +
		"\n# RESULTS\n" + strings.Repeat("result observation ", 2000) +
		"\nDISCUSSION\n" + strings.Repeat("interpretation ", 2000) +
		"\nSTAR METHODS\nMETHODS_SENTINEL randomized design\n" + strings.Repeat("protocol details ", 2000) +
		"\nQUANTIFICATION AND STATISTICAL ANALYSIS\nSTATISTICS_SENTINEL\n" + strings.Repeat("analysis details ", 1000) +
		"\nData and code availability\nCODE_SENTINEL\n" + strings.Repeat("deposited data ", 500) + "FINAL_SENTINEL"
}

func TestSamplePaperTextCoversLateSections(t *testing.T) {
	text := longStructuredPaper()
	excerpts := SamplePaperText(text, 6400)
	var joined strings.Builder
	for _, excerpt := range excerpts {
		joined.WriteString(excerpt.Text)
	}
	for _, marker := range []string{"TITLE AND AUTHORS", "METHODS_SENTINEL", "STATISTICS_SENTINEL", "CODE_SENTINEL", "FINAL_SENTINEL"} {
		if !strings.Contains(joined.String(), marker) {
			t.Fatalf("missing %s", marker)
		}
	}
}

func TestSamplePaperTextBoundsOffsetsAndDeterminism(t *testing.T) {
	for _, text := range []string{longStructuredPaper(), strings.Repeat("正文方法结果。", 2000), "", "short body"} {
		for _, budget := range []int{0, 1, 100, 321, 800, 6399, 6400, 24000, len([]rune(text))} {
			excerpts := SamplePaperText(text, budget)
			if !reflect.DeepEqual(excerpts, SamplePaperText(text, budget)) {
				t.Fatal("nondeterministic samples")
			}
			used, end := 0, 0
			for _, ex := range excerpts {
				if ex.StartRune < end || ex.EndRune > len([]rune(text)) || ex.EndRune <= ex.StartRune {
					t.Fatalf("invalid span %+v", ex)
				}
				if ex.Text != string([]rune(text)[ex.StartRune:ex.EndRune]) {
					t.Fatal("excerpt is not verbatim")
				}
				used += len([]rune(ex.Text))
				end = ex.EndRune
			}
			if used > budget || len(excerpts) > maxPaperTextExcerpts {
				t.Fatalf("budget %d, used %d, count %d", budget, used, len(excerpts))
			}
			if text != "" && budget >= len([]rune(text)) && (len(excerpts) != 1 || excerpts[0].Text != text) {
				t.Fatal("short text not preserved")
			}
			if len([]rune(text)) > budget && budget >= 320 && end != len([]rune(text)) {
				t.Fatal("tail not covered")
			}
		}
	}
}

func TestPaperReadBroadQuestionReachesMethodsWithoutKeywordHit(t *testing.T) {
	paper := model.Paper{ID: 1, Title: "Study", PDFText: longStructuredPaper()}
	tool := NewPaperReadTool(stubPaperStore{papers: map[int64]*model.Paper{1: &paper}})
	res, err := tool.Run(context.Background(), ToolInput{Query: "研究设计架构解构", Context: RequestContext{PaperIDs: []int64{1}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"METHODS_SENTINEL", "STATISTICS_SENTINEL", "CODE_SENTINEL", "FINAL_SENTINEL"} {
		if !strings.Contains(res.AnswerContext, marker) {
			t.Fatalf("missing %s in tool evidence", marker)
		}
	}
	if len(res.Citations) <= 3 || len(res.Citations) > maxPaperReadEvidence {
		t.Fatalf("citations %d", len(res.Citations))
	}
	used := 0
	for _, c := range res.Citations {
		used += len([]rune(c.Snippet.Text))
		if !strings.Contains(c.Snippet.Section, "非完整全文") {
			t.Fatal("missing coverage boundary")
		}
	}
	if used > maxPaperReadEvidence*paperReadExcerptRunes {
		t.Fatal("unbounded tool evidence")
	}
}

func TestPaperReadKeepsTargetedHitAlongsideCoverage(t *testing.T) {
	paper := model.Paper{ID: 1, Title: "Study", PDFText: longStructuredPaper() + "\nUniqueTargetEvidence"}
	res, err := NewPaperReadTool(stubPaperStore{papers: map[int64]*model.Paper{1: &paper}}).Run(context.Background(),
		ToolInput{Query: "UniqueTargetEvidence", Context: RequestContext{PaperID: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.AnswerContext, "UniqueTargetEvidence") || !strings.Contains(res.AnswerContext, "METHODS_SENTINEL") {
		t.Fatal("lost targeted hit or methods coverage")
	}
	if len(res.Citations) > maxPaperReadEvidence {
		t.Fatal("evidence count exceeded")
	}
}
