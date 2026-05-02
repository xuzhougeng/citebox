package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

// --- shared fakes for /ask + /interpret -----------------------------------

type fakeAI struct {
	lastReq model.AIReadRequest
	answer  string
	err     error
	calls   int
}

func (f *fakeAI) ReadPaper(_ context.Context, in model.AIReadRequest) (*model.AIReadResponse, error) {
	f.calls++
	f.lastReq = in
	if f.err != nil {
		return nil, f.err
	}
	return &model.AIReadResponse{Answer: f.answer}, nil
}

type fakeFigureLookup struct {
	ret    *model.FigureListItem
	err    error
	calls  int
	lastID int64
}

func (f *fakeFigureLookup) GetFigure(id int64) (*model.FigureListItem, error) {
	f.calls++
	f.lastID = id
	return f.ret, f.err
}

// --- /ask ------------------------------------------------------------------

func TestAskRequiresPaperSelection(t *testing.T) {
	ai := &fakeAI{answer: "ok"}
	fs := &fakeState{} // no current paper
	cmd := &AskCommand{AI: ai}
	if _, err := cmd.Execute(context.Background(), "what?", RuntimeCtx{Surface: "wechat", State: fs}); err == nil {
		t.Fatal("expected error when no current paper")
	}
	if ai.calls != 0 {
		t.Fatalf("AI must not be called when no paper selected, got %d calls", ai.calls)
	}
}

func TestAskEmptyArgErrors(t *testing.T) {
	ai := &fakeAI{answer: "ok"}
	fs := &fakeState{paperID: 5, title: "P"}
	cmd := &AskCommand{AI: ai}
	for _, arg := range []string{"", "   ", "\t\n"} {
		if _, err := cmd.Execute(context.Background(), arg, RuntimeCtx{Surface: "wechat", State: fs}); err == nil {
			t.Fatalf("expected error for arg %q", arg)
		}
	}
	if ai.calls != 0 {
		t.Fatalf("AI must not be called for empty arg, got %d calls", ai.calls)
	}
}

func TestAskCallsAIWithCorrectAction(t *testing.T) {
	ai := &fakeAI{answer: "因为蛋白质 X 与 Y 相互作用。"}
	fs := &fakeState{paperID: 42, title: "Paper"}
	cmd := &AskCommand{AI: ai}
	res, err := cmd.Execute(context.Background(), "  why?  ", RuntimeCtx{Surface: "wechat", State: fs})
	if err != nil {
		t.Fatal(err)
	}
	if ai.lastReq.Action != model.AIActionPaperQA {
		t.Errorf("want Action=%q, got %q", model.AIActionPaperQA, ai.lastReq.Action)
	}
	if ai.lastReq.PaperID != 42 {
		t.Errorf("want PaperID=42, got %d", ai.lastReq.PaperID)
	}
	if ai.lastReq.FigureID != 0 {
		t.Errorf("want FigureID=0 for /ask, got %d", ai.lastReq.FigureID)
	}
	if ai.lastReq.Question != "why?" {
		t.Errorf("want Question=%q (trimmed), got %q", "why?", ai.lastReq.Question)
	}
	if len(res.ChunksText) != 1 || res.ChunksText[0] != "因为蛋白质 X 与 Y 相互作用。" {
		t.Fatalf("want answer chunk, got %q", res.ChunksText)
	}
	if res.UsedShortcut {
		t.Error("/ask should not flag UsedShortcut (it produces a real assistant turn)")
	}
}

func TestAskEmptyAnswerFallsBack(t *testing.T) {
	ai := &fakeAI{answer: "   "}
	fs := &fakeState{paperID: 1, title: "P"}
	cmd := &AskCommand{AI: ai}
	res, err := cmd.Execute(context.Background(), "q?", RuntimeCtx{Surface: "wechat", State: fs})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ChunksText) != 1 || !contains(res.ChunksText[0], "未返回") {
		t.Fatalf("want fallback message, got %q", res.ChunksText)
	}
}

func TestAskPropagatesAIError(t *testing.T) {
	ai := &fakeAI{err: errors.New("boom")}
	fs := &fakeState{paperID: 1, title: "P"}
	cmd := &AskCommand{AI: ai}
	if _, err := cmd.Execute(context.Background(), "q?", RuntimeCtx{Surface: "wechat", State: fs}); err == nil {
		t.Fatal("expected AI error to propagate")
	}
}

// --- /interpret ------------------------------------------------------------

func TestInterpretRequiresFigureSelection(t *testing.T) {
	ai := &fakeAI{answer: "ok"}
	lookup := &fakeFigureLookup{}
	fs := &fakeState{paperID: 5} // paper but no figure
	cmd := &InterpretCommand{AI: ai, Lookup: lookup}
	if _, err := cmd.Execute(context.Background(), "", RuntimeCtx{Surface: "wechat", State: fs}); err == nil {
		t.Fatal("expected error when no figure selected")
	}
	if ai.calls != 0 {
		t.Fatalf("AI must not be called when no figure selected, got %d calls", ai.calls)
	}
}

func TestInterpretSendsFigureAndPaperIDs(t *testing.T) {
	ai := &fakeAI{answer: "图中显示了..."}
	lookup := &fakeFigureLookup{ret: &model.FigureListItem{ID: 77, PaperID: 9, Caption: "Fig 1"}}
	fs := &fakeState{figID: 77, paperID: 9, title: "P9"}
	cmd := &InterpretCommand{AI: ai, Lookup: lookup}
	res, err := cmd.Execute(context.Background(), " 这张图说明了什么? ", RuntimeCtx{Surface: "wechat", State: fs})
	if err != nil {
		t.Fatal(err)
	}
	if lookup.lastID != 77 {
		t.Fatalf("want lookup of figure 77, got %d", lookup.lastID)
	}
	if ai.lastReq.Action != model.AIActionFigureInterpretation {
		t.Errorf("want Action=%q, got %q", model.AIActionFigureInterpretation, ai.lastReq.Action)
	}
	if ai.lastReq.PaperID != 9 {
		t.Errorf("want PaperID=9 (from figure), got %d", ai.lastReq.PaperID)
	}
	if ai.lastReq.FigureID != 77 {
		t.Errorf("want FigureID=77, got %d", ai.lastReq.FigureID)
	}
	if ai.lastReq.Question != "这张图说明了什么?" {
		t.Errorf("want trimmed question, got %q", ai.lastReq.Question)
	}
	if len(res.ChunksText) != 1 || res.ChunksText[0] != "图中显示了..." {
		t.Fatalf("want answer chunk, got %q", res.ChunksText)
	}
}

func TestInterpretAcceptsEmptyArg(t *testing.T) {
	ai := &fakeAI{answer: "解读..."}
	lookup := &fakeFigureLookup{ret: &model.FigureListItem{ID: 1, PaperID: 2}}
	fs := &fakeState{figID: 1, paperID: 2}
	cmd := &InterpretCommand{AI: ai, Lookup: lookup}
	if _, err := cmd.Execute(context.Background(), "", RuntimeCtx{Surface: "wechat", State: fs}); err != nil {
		t.Fatalf("/interpret should accept empty arg, got error: %v", err)
	}
	if ai.lastReq.Question != "" {
		t.Errorf("want empty question for empty arg, got %q", ai.lastReq.Question)
	}
}

func TestInterpretMissingFigureErrors(t *testing.T) {
	ai := &fakeAI{}
	lookup := &fakeFigureLookup{ret: nil} // figure not found
	fs := &fakeState{figID: 99, paperID: 9}
	cmd := &InterpretCommand{AI: ai, Lookup: lookup}
	if _, err := cmd.Execute(context.Background(), "q", RuntimeCtx{Surface: "wechat", State: fs}); err == nil {
		t.Fatal("expected error when figure not found")
	}
	if ai.calls != 0 {
		t.Fatalf("AI must not be called when figure not found, got %d calls", ai.calls)
	}
}

func TestInterpretEmptyAnswerFallsBack(t *testing.T) {
	ai := &fakeAI{answer: ""}
	lookup := &fakeFigureLookup{ret: &model.FigureListItem{ID: 1, PaperID: 2}}
	fs := &fakeState{figID: 1, paperID: 2}
	cmd := &InterpretCommand{AI: ai, Lookup: lookup}
	res, err := cmd.Execute(context.Background(), "", RuntimeCtx{Surface: "wechat", State: fs})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ChunksText) != 1 || !contains(res.ChunksText[0], "未返回") {
		t.Fatalf("want fallback message, got %q", res.ChunksText)
	}
}
