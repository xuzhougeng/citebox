package ai_conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
	"strings"
	"testing"
)

func TestUnknownImageCapabilityDoesNotSendImages(t *testing.T) {
	svc, _, caller := newServiceForTest(t)
	settings := model.DefaultAISettings()
	settings.Models[0].SupportsImages = nil
	svc.settings = &stubSettingsProvider{settings: settings}
	svc.WithFigureContextLoader(&stubFigureContextLoader{images: []model.AIImageInput{{Data: "image"}}, summaries: []string{"caption"}})
	id, _ := svc.CreateDraft()
	_, err := svc.SendMessage(context.Background(), SendMessageInput{ConversationID: id, Content: "read", Context: ai_assistant.RequestContext{FigureIDs: []int64{1}}}, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.imagesSeen) > 0 {
		t.Fatal("unknown model received images")
	}
	conv, _ := svc.GetConversation(id)
	cards := conv.TurnRuns[0].Cards
	var report ContextReport
	if err := json.Unmarshal([]byte(cards[len(cards)-1].PayloadJSON), &report); err != nil {
		t.Fatal(err)
	}
	if report.FigureStatus != "text_only" || report.IncludedFigures != 0 || report.RequestedFigures != 1 {
		t.Fatalf("report=%+v", report)
	}
}
func TestSendMessageContextBudgetIncludesToolResults(t *testing.T) {
	svc, repo, caller := newServiceForTest(t)
	id := mustInsertPaperForTest(t, repo, "Long paper", "10.1/long-context")
	body := strings.Repeat("Body text. ", 10000) + " END_OF_PAPER"
	if _, err := repo.DB().Exec(`UPDATE papers SET pdf_text=? WHERE id=?`, body, id); err != nil {
		t.Fatal(err)
	}
	settings := model.DefaultAISettings()
	settings.ContextBudgetTokens = 4000
	svc.settings = &stubSettingsProvider{settings: settings}
	orch := &stubOrchestrator{out: ai_assistant.RunOutput{Intent: ai_assistant.IntentPaperRead, AnswerContext: strings.Repeat("Tool evidence. ", 10000)}}
	svc.orchestrator = orch
	convID, _ := svc.CreateDraft()
	if err := svc.PinPaper(convID, id); err != nil {
		t.Fatal(err)
	}
	_, err := svc.SendMessage(context.Background(), SendMessageInput{ConversationID: convID, Content: "study architecture"}, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if orch.in.Context.PaperID != id {
		t.Fatal("existing pins missing from reading tool")
	}
	if tokens := estimateTokens(caller.systemSeen) + estimateTokens(caller.userSeen); tokens > 4000 {
		t.Fatalf("prompt over budget: %d", tokens)
	}
	if !strings.Contains(caller.userSeen, "END_OF_PAPER") {
		t.Fatal("tail excluded")
	}
}
func TestPinnedCoverageReportsFullLongBodyAndOmittedBody(t *testing.T) {
	svc, repo, _ := newServiceForTest(t)
	id := mustInsertPaperForTest(t, repo, "Full long paper", "10.1/full-long")
	body := strings.Repeat("Real body. ", 4000)
	if _, err := repo.DB().Exec(`UPDATE papers SET pdf_text=? WHERE id=?`, body, id); err != nil {
		t.Fatal(err)
	}
	asm, err := svc.assembleForTurn(repository.AIConversation{}, []repository.AIPinnedPaper{{PaperID: id}}, nil, "Read", "", model.AISettings{ContextBudgetTokens: 32000})
	if err != nil {
		t.Fatal(err)
	}
	if len(asm.papers) != 1 || asm.papers[0].Included != len([]rune(body)) {
		t.Fatal("body >24k not fully included despite sufficient budget")
	}
	asm, err = svc.assembleForTurn(repository.AIConversation{}, []repository.AIPinnedPaper{{PaperID: id}}, nil, strings.Repeat("mandatory ", 1000), "", model.AISettings{ContextBudgetTokens: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(asm.papers) != 1 || asm.papers[0].Included != 0 || asm.papers[0].Total == 0 {
		t.Fatal("omitted paper coverage missing")
	}
}
func TestPartialFigureReportUsesActualImageCount(t *testing.T) {
	svc, _, _ := newServiceForTest(t)
	svc.WithFigureContextLoader(&stubFigureContextLoader{images: []model.AIImageInput{{Data: "one"}}, summaries: []string{"one", "two"}})
	images, block, report := svc.loadTurnFigures(context.Background(), []int64{1, 2}, model.DefaultAISettings())
	if len(images) != 1 || report.IncludedFigures != 1 || report.FigureStatus != "partial" || !strings.Contains(block, "实际向模型提交 1 张") {
		t.Fatal("incorrect partial image status")
	}
}

func TestAutoFiguresUsesPinnedMainFiguresAndManualPriority(t *testing.T) {
	svc, repo, _ := newServiceForTest(t)
	p1 := mustInsertPaperForTest(t, repo, "Auto one", "10.1/auto-one")
	p2 := mustInsertPaperForTest(t, repo, "Auto two", "10.1/auto-two")
	for _, row := range []struct{ id, paper int64 }{{10, p1}, {11, p1}, {12, p1}, {13, p1}, {14, p1}, {15, p1}, {16, p1}, {17, p1}, {18, p1}, {20, p2}, {21, p2}} {
		if _, err := repo.DB().Exec(`INSERT INTO paper_figures(id,paper_id,filename,page_number,figure_index) VALUES(?,?,?,1,?)`, row.id, row.paper, fmt.Sprintf("image-%d.png", row.id), row.id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.DB().Exec(`UPDATE paper_figures SET parent_figure_id=10, subfigure_label='A' WHERE id=11`); err != nil {
		t.Fatal(err)
	}
	pins := []repository.AIPinnedPaper{{PaperID: p1}, {PaperID: p2}}
	ids := svc.turnFigureIDs(ai_assistant.RequestContext{AutoFigures: true, FigureIDs: []int64{21, 21}}, pins)
	if len(ids) != 8 || ids[0] != 21 {
		t.Fatalf("manual priority / cap: %v", ids)
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id == 11 || seen[id] {
			t.Fatalf("subfigure or duplicate selected: %v", ids)
		}
		seen[id] = true
	}
	if !seen[20] {
		t.Fatal("second paper starved")
	}
	if got := svc.turnFigureIDs(ai_assistant.RequestContext{}, pins); len(got) != 0 {
		t.Fatal("automatic figures should require opt-in")
	}
}

func TestExistingPinsDoNotNarrowExplicitLibrarySearch(t *testing.T) {
	svc, repo, _ := newServiceForTest(t)
	paperID := mustInsertPaperForTest(t, repo, "Pinned source", "10.1/search-scope")
	id, _ := svc.CreateDraft()
	if err := svc.PinPaper(id, paperID); err != nil {
		t.Fatal(err)
	}
	orch := &stubOrchestrator{}
	svc.orchestrator = orch
	_, err := svc.SendMessage(context.Background(), SendMessageInput{ConversationID: id, Content: "Find articles across the library", IntentHint: ai_assistant.IntentLibrarySearch}, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if orch.in.Context.PaperID != 0 || len(orch.in.Context.PaperIDs) != 0 {
		t.Fatal("pins unexpectedly narrowed library search")
	}
}
