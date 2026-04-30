package ai_assistant

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type stubTool struct {
	res ToolResult
}

func (s stubTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	return s.res, nil
}

type errorTool struct {
	err error
}

func (e errorTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	return ToolResult{}, e.err
}

type capturingTool struct {
	res ToolResult
	in  ToolInput
}

func (c *capturingTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	c.in = in
	return c.res, nil
}

func TestOrchestratorRunsSelectedTool(t *testing.T) {
	orch := NewOrchestrator(ToolSet{
		LibrarySearch: stubTool{res: ToolResult{
			Process:       ProcessSummary{Intent: IntentLibrarySearch, Stages: []ProcessStage{{Label: "全库检索", Count: 2}}},
			Cards:         []ResultCard{{Type: "paper_hit", Payload: PaperHitCard{PaperID: 1, Title: "ATAC Paper"}}},
			AnswerContext: "ATAC evidence",
		}},
	})
	out, err := orch.Run(context.Background(), RunInput{Content: "帮我查找 ATAC 文章"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Intent != IntentLibrarySearch || len(out.Cards) != 1 {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(out.AnswerContext, "ATAC evidence") {
		t.Fatalf("answer context = %q", out.AnswerContext)
	}
}

func TestOrchestratorPassesSelectedToolInput(t *testing.T) {
	tool := &capturingTool{res: ToolResult{
		Process: ProcessSummary{Intent: IntentFigureLookup},
	}}
	orch := NewOrchestrator(ToolSet{
		FigureLookup: tool,
	})
	ctx := RequestContext{PaperID: 42, PaperIDs: []int64{42, 43}, FigureID: 7}

	_, err := orch.Run(context.Background(), RunInput{
		Content:    "  解释这张图  ",
		IntentHint: IntentFigureLookup,
		Context:    ctx,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := ToolInput{Query: "  解释这张图  ", Context: ctx, IntentHint: IntentFigureLookup}
	if !reflect.DeepEqual(tool.in, want) {
		t.Fatalf("tool input = %+v, want %+v", tool.in, want)
	}
}

func TestOrchestratorHintSelectsFigureLookup(t *testing.T) {
	orch := NewOrchestrator(ToolSet{
		FigureLookup: stubTool{res: ToolResult{
			Process:       ProcessSummary{Intent: IntentFigureLookup, Stages: []ProcessStage{{Label: "图表检索", Count: 1}}},
			Cards:         []ResultCard{{Type: "figure_hit", Payload: map[string]any{"figure_id": int64(7)}}},
			AnswerContext: "figure evidence",
		}},
	})

	out, err := orch.Run(context.Background(), RunInput{Content: "解释一下", IntentHint: IntentFigureLookup})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Intent != IntentFigureLookup || len(out.Cards) != 1 {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(out.AnswerContext, "figure evidence") {
		t.Fatalf("answer context = %q", out.AnswerContext)
	}
}

func TestOrchestratorPreservesRecoverableFailedToolResult(t *testing.T) {
	orch := NewOrchestrator(ToolSet{
		ExternalSearch: stubTool{res: ToolResult{
			Process:       ProcessSummary{Intent: IntentExternalSearch, Note: "external provider unavailable"},
			AnswerContext: "外部检索暂时不可用",
			ToolCalls: []ToolCallSummary{{
				ToolName: "external_search",
				Status:   "failed",
				Error:    "timeout",
			}},
		}},
	})

	out, err := orch.Run(context.Background(), RunInput{
		Content:    "查一下外部有没有 ATAC 综述",
		IntentHint: IntentExternalSearch,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Process.Intent != IntentExternalSearch || out.Process.Note != "external provider unavailable" {
		t.Fatalf("process = %+v", out.Process)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].Status != "failed" || out.ToolCalls[0].Error != "timeout" {
		t.Fatalf("tool calls = %+v", out.ToolCalls)
	}
	if out.AnswerContext == "" || !strings.Contains(out.AnswerContext, "查一下外部有没有 ATAC 综述") {
		t.Fatalf("answer context = %q", out.AnswerContext)
	}
}

func TestOrchestratorFallsBackWithoutTool(t *testing.T) {
	orch := NewOrchestrator(ToolSet{})

	out, err := orch.Run(context.Background(), RunInput{Content: "  随便聊聊  "})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Intent != IntentChat || out.Process.Intent != IntentChat {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(out.AnswerContext, "用户问题：\n随便聊聊") {
		t.Fatalf("answer context = %q", out.AnswerContext)
	}
}

func TestOrchestratorPropagatesToolError(t *testing.T) {
	wantErr := errors.New("tool failed")
	orch := NewOrchestrator(ToolSet{
		LibrarySearch: errorTool{err: wantErr},
	})

	_, err := orch.Run(context.Background(), RunInput{Content: "帮我查找 ATAC 文章"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
