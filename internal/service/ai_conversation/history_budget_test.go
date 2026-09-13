package ai_conversation

import (
	"context"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestLongPaperPreservesRecentQuestionAndAnswer(t *testing.T) {
	svc, repo, _ := newServiceForTest(t)
	id := mustInsertPaperForTest(t, repo, "Long study", "10.1/history-budget")
	if _, err := repo.DB().Exec(`UPDATE papers SET pdf_text=? WHERE id=?`, strings.Repeat("Methods and results. ", 12000), id); err != nil {
		t.Fatal(err)
	}
	history := []repository.AIMessage{{Role: "user", Content: "ALPHA_CRITERION: compare sample sizes"}, {Role: "assistant", Content: "PREVIOUS_ANSWER " + strings.Repeat("Comparison details. ", 300)}}
	asm, err := svc.assembleForTurn(repository.AIConversation{}, []repository.AIPinnedPaper{{PaperID: id}}, history, "Expand the second point", "", model.AISettings{ContextBudgetTokens: 4000})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asm.userPrompt, history[0].Content) || !strings.Contains(asm.userPrompt, history[1].Content) {
		t.Fatal("latest turn was lost to pinned body")
	}
	if estimateTokens(asm.userPrompt) > 4000 || asm.historyMessages != 2 || asm.papers[0].IncludedBodyRunes == 0 {
		t.Fatalf("unbalanced budget: %+v", asm)
	}
}

func TestHistoryBudgetNeverLeavesAnAnswerWithoutItsQuestion(t *testing.T) {
	history := []repository.AIMessage{{Role: "user", Content: "OLDER_QUESTION"}, {Role: "assistant", Content: strings.Repeat("old answer ", 100)}, {Role: "user", Content: "LATEST_QUESTION"}, {Role: "assistant", Content: "LATEST_ANSWER"}}
	block, kept := budgetedHistoryBlock(history, 100)
	if kept != 2 || strings.Contains(block, "OLDER_QUESTION") || !strings.Contains(block, "LATEST_QUESTION") || !strings.Contains(block, "LATEST_ANSWER") {
		t.Fatalf("history: %s", block)
	}
}

func TestOversizedLatestAnswerRetainsDisclosedFollowupContext(t *testing.T) {
	block, kept := budgetedHistoryBlock([]repository.AIMessage{{Role: "user", Content: "QUESTION_MARKER"}, {Role: "assistant", Content: "ANSWER_MARKER " + strings.Repeat("detail ", 3000)}}, 400)
	if kept != 2 || estimateTokens(block) > 400 || !strings.Contains(block, "QUESTION_MARKER") || !strings.Contains(block, "ANSWER_MARKER") || !strings.Contains(block, "truncated") {
		t.Fatalf("oversized history: %s", block)
	}
}

func TestEmptySummaryDoesNotAdvanceHistory(t *testing.T) {
	_, through, err := summarize(context.Background(), &stubNonStreamCaller{staticReply: " "}, model.DefaultAISettings(), "", []repository.AIMessage{{ID: 1, Role: "user"}, {ID: 2, Role: "assistant"}, {ID: 3, Role: "user"}, {ID: 4, Role: "assistant"}})
	if err == nil || through != 0 {
		t.Fatalf("empty summary discarded history: %d %v", through, err)
	}
}

func TestPinnedPaperTriggersSummaryBeforeHistoryFillsWholeBudget(t *testing.T) {
	svc, repo, _ := newServiceForTest(t)
	svc.summaryCaller = &stubNonStreamCaller{staticReply: "Earlier sample-size comparison"}
	settings := model.DefaultAISettings()
	settings.ContextBudgetTokens = 4000
	svc.settings = &stubSettingsProvider{settings: settings}
	id := mustInsertPaperForTest(t, repo, "Long study", "10.1/history-summary")
	if _, err := repo.DB().Exec(`UPDATE papers SET pdf_text=? WHERE id=?`, strings.Repeat("Methods. ", 10000), id); err != nil {
		t.Fatal(err)
	}
	convID, _ := svc.CreateDraft()
	if err := svc.PinPaper(convID, id); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"user", "assistant", "user", "assistant"} {
		if _, err := repo.AIConversation.AddMessage(convID, role, strings.Repeat("detail ", 380), repository.AIMessageMeta{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.SendMessage(context.Background(), SendMessageInput{ConversationID: convID, Content: "Continue comparison"}, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	conv, err := repo.AIConversation.GetConversation(convID)
	if err != nil {
		t.Fatal(err)
	}
	if conv.SummaryText == "" || !conv.SummaryThroughMessageID.Valid {
		t.Fatal("pinned context did not trigger early summary")
	}
}
