package ai_assistant

import (
	"context"
	"errors"
	"github.com/xuzhougeng/citebox/internal/service/research"
	"strings"
	"testing"
)

type sequencePlanner struct {
	plans  []RetrievalPlan
	inputs []RetrievalPlanningInput
	err    error
}

func (p *sequencePlanner) PlanRetrieval(_ context.Context, in RetrievalPlanningInput) (RetrievalPlan, error) {
	p.inputs = append(p.inputs, in)
	if p.err != nil {
		return RetrievalPlan{}, p.err
	}
	i := min(len(p.inputs)-1, len(p.plans)-1)
	return p.plans[i], nil
}

func TestRetrievalPlannerResolvesFollowupAndRestoresPins(t *testing.T) {
	planner := &sequencePlanner{plans: []RetrievalPlan{{Intent: IntentPaperRead, Scope: "pinned", ResolvedQuery: "Did Study A correct batch effects?", SearchTerms: []string{"batch correction"}}, {Sufficient: true}}}
	tool := &capturingTool{res: ToolResult{AnswerContext: "Methods: batch correction"}}
	orch := NewOrchestrator(ToolSet{PaperRead: tool}).WithPlanner(planner)
	out, err := orch.Run(context.Background(), RunInput{Content: "它有没有控制批次效应？", History: []PlanningMessage{{Role: "user", Content: "Read Study A"}}, PinnedPapers: []PlanningPaper{{ID: 7, Title: "Study A"}}})
	if err != nil {
		t.Fatal(err)
	}
	if tool.in.Query != "Did Study A correct batch effects?" || len(tool.in.SearchTerms) != 1 || tool.in.Context.PaperIDs[0] != 7 {
		t.Fatalf("tool input: %+v", tool.in)
	}
	if len(planner.inputs) != 2 || planner.inputs[0].History[0].Content != "Read Study A" || planner.inputs[1].Evidence == "" {
		t.Fatalf("planning inputs: %+v", planner.inputs)
	}
	if out.ToolCalls[0].ToolName != "retrieval_plan" {
		t.Fatal("plan not persisted")
	}
}

func TestRetrievalPlannerPreservesExplicitIntentAndScope(t *testing.T) {
	planner := &sequencePlanner{plans: []RetrievalPlan{{Intent: IntentExternalSearch, Scope: "external", ResolvedQuery: "batch correction"}}}
	tool := &capturingTool{}
	orch := NewOrchestrator(ToolSet{LibrarySearch: tool}).WithPlanner(planner)
	out, err := orch.Run(context.Background(), RunInput{Content: "search", IntentHint: IntentLibrarySearch, ExplicitPaperScope: true, Context: RequestContext{PaperIDs: []int64{7}}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Intent != IntentLibrarySearch || len(tool.in.Context.PaperIDs) != 1 {
		t.Fatalf("scope changed: %+v %+v", out, tool.in)
	}
}

func TestRetrievalPlannerFailureFallsBackToExistingRoute(t *testing.T) {
	planner := &sequencePlanner{err: errors.New("planner offline")}
	tool := &capturingTool{}
	out, err := NewOrchestrator(ToolSet{PaperRead: tool}).WithPlanner(planner).Run(context.Background(), RunInput{Content: "Explain methods", Context: RequestContext{PaperID: 1}})
	if err != nil || out.Intent != IntentPaperRead || tool.in.Query != "Explain methods" {
		t.Fatalf("fallback: %+v %v", out, err)
	}
}

func TestReadingFollowupRenumbersEvidenceAndStopsAfterOneRetry(t *testing.T) {
	planner := &sequencePlanner{plans: []RetrievalPlan{{Intent: IntentPaperRead, ResolvedQuery: "methods", SearchTerms: []string{"initial"}}, {SearchTerms: []string{"followup"}}}}
	tool := stubTool{res: ToolResult{AnswerContext: "[1] evidence", Citations: []Citation{{I: 1, Snippet: research.Snippet{Text: "evidence"}}}, Cards: []ResultCard{{Type: "paper_read", Payload: PaperCompareCard{Papers: []PaperCompareItem{{Evidence: []PaperHitSnippet{{CitationIndex: 1, Text: "evidence"}}}}}}}}}
	out, err := NewOrchestrator(ToolSet{PaperRead: tool}).WithPlanner(planner).Run(context.Background(), RunInput{Content: "methods", Context: RequestContext{PaperID: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(planner.inputs) != 2 || len(out.Citations) != 2 || out.Citations[0].I != 1 || out.Citations[1].I != 2 || !strings.Contains(out.AnswerContext, "[2] evidence") {
		t.Fatalf("followup: %+v", out)
	}
	card := out.Cards[1].Payload.(PaperCompareCard)
	if card.Papers[0].Evidence[0].CitationIndex != 2 {
		t.Fatal("card reference not renumbered")
	}
}

func TestPlanningInputIsBoundedWithoutMutatingHistory(t *testing.T) {
	input := RetrievalPlanningInput{History: []PlanningMessage{{Content: strings.Repeat("x", 2000)}}}
	out := boundedPlanningInput(input)
	if len(out.History[0].Content) > 1510 || len(input.History[0].Content) != 2000 {
		t.Fatal("history not safely bounded")
	}
}
