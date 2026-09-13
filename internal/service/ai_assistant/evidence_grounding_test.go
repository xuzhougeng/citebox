package ai_assistant

import (
	"context"
	"errors"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/research"
	"strings"
	"testing"
)

type stubEvidenceJudge struct {
	assessments []EvidenceAssessment
	err         error
}

func (j stubEvidenceJudge) JudgeEvidence(context.Context, string, []Citation) ([]EvidenceAssessment, error) {
	return j.assessments, j.err
}

func TestOriginalEvidenceRanksBeforeContradictoryAINote(t *testing.T) {
	p := model.Paper{ID: 1, PDFText: "ALPHAMARKER was not supported.", PaperNotesText: "## AI Assistant\nSource: [1](/ai?conversation=1&message=2)\nALPHAMARKER was supported."}
	matches := FindLocalEvidenceMatches(p, []string{"ALPHAMARKER"}, 3)
	if len(matches) != 2 || matches[0].Snippet.Origin != "original" || matches[1].Snippet.Origin != "ai_note" {
		t.Fatalf("origins: %+v", matches)
	}
}

func TestEvidenceJudgeCannotPromoteNotesOrInventCitationIndices(t *testing.T) {
	orch := NewOrchestrator(ToolSet{}).WithEvidenceJudge(stubEvidenceJudge{assessments: []EvidenceAssessment{{I: 1, Verdict: "contradicts", Rationale: "negative result"}, {I: 2, Verdict: "supports"}, {I: 99, Verdict: "supports"}}})
	res := orch.groundEvidence(context.Background(), "Does it support the hypothesis?", ToolResult{Citations: []Citation{
		{I: 1, Source: "local", Snippet: research.Snippet{Text: "negative result", SnippetKind: "body"}},
		{I: 2, Source: "local", Snippet: research.Snippet{Text: "positive AI note", SnippetKind: "notes", Origin: "ai_note"}},
	}})
	if len(res.Citations) != 2 || res.Citations[0].Verdict != "contradicts" || res.Citations[1].Verdict != "note_only" {
		t.Fatalf("unsafe judgments: %+v", res.Citations)
	}
	if !strings.Contains(res.AnswerContext, "Never use notes alone") {
		t.Fatal("missing provenance boundary")
	}
}

func TestEvidenceJudgeFailureKeepsEvidenceUnassessed(t *testing.T) {
	orch := NewOrchestrator(ToolSet{}).WithEvidenceJudge(stubEvidenceJudge{err: errors.New("offline")})
	res := orch.groundEvidence(context.Background(), "query", ToolResult{Citations: []Citation{{I: 1, Source: "local", Snippet: research.Snippet{Text: "result", SnippetKind: "body"}}}})
	if res.Citations[0].Verdict != "unassessed" || res.ToolCalls[0].Status != "failed" {
		t.Fatalf("failure: %+v", res)
	}
}

func TestMixedNotesStayDerived(t *testing.T) {
	if NoteEvidenceOrigin("My note", "AI [source](/ai?conversation=1)") != "mixed_note" {
		t.Fatal("mixed notes became primary evidence")
	}
	if NoteEvidenceOrigin("My note", "Reading notes") != "user_note" {
		t.Fatal("manual note mislabeled")
	}
}
