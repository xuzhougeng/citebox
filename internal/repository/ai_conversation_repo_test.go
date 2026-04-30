package repository

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func newAIConversationRepoForTest(t *testing.T) *AIConversationRepository {
	t.Helper()
	libRepo, err := NewLibraryRepository(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("NewLibraryRepository: %v", err)
	}
	t.Cleanup(func() { _ = libRepo.Close() })
	return libRepo.AIConversation
}

func TestAIConversationCreateAndGet(t *testing.T) {
	repo := newAIConversationRepoForTest(t)

	id, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if id <= 0 {
		t.Fatalf("id = %d", id)
	}

	conv, err := repo.GetConversation(id)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if conv.ID != id || conv.Title != "" || conv.StrictEvidence {
		t.Fatalf("conv = %+v", conv)
	}
}

func TestAIConversationListOrderByUpdatedAt(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	id1, _ := repo.CreateConversation()
	id2, _ := repo.CreateConversation()
	// touch id1 to bump updated_at
	if err := repo.TouchConversation(id1); err != nil {
		t.Fatalf("TouchConversation: %v", err)
	}

	list, err := repo.ListConversations("", 50, 0)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(list) != 2 || list[0].ID != id1 || list[1].ID != id2 {
		t.Fatalf("expected [%d,%d] got %+v", id1, id2, list)
	}
}

func TestAIConversationUpdate(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	id, _ := repo.CreateConversation()

	if err := repo.UpdateTitle(id, "新标题", true); err != nil {
		t.Fatalf("UpdateTitle: %v", err)
	}
	if err := repo.UpdateStrictEvidence(id, true); err != nil {
		t.Fatalf("UpdateStrictEvidence: %v", err)
	}

	conv, _ := repo.GetConversation(id)
	if conv.Title != "新标题" || !conv.TitleLocked || !conv.StrictEvidence {
		t.Fatalf("conv = %+v", conv)
	}
}

func TestAIConversationDeleteCascadesMessages(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	id, _ := repo.CreateConversation()
	if _, err := repo.AddMessage(id, "user", "hi", AIMessageMeta{}); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	if err := repo.DeleteConversation(id); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}

	msgs, err := repo.ListMessages(id, 0, 100)
	if err != nil {
		t.Fatalf("ListMessages after delete: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("messages should be cascaded; got %d", len(msgs))
	}
}

func TestAIConversationPinPaper(t *testing.T) {
	libRepo, err := NewLibraryRepository(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("NewLibraryRepository: %v", err)
	}
	t.Cleanup(func() { _ = libRepo.Close() })
	repo := libRepo.AIConversation

	// Need a real paper row to satisfy the FK.
	paperID := mustInsertTestPaper(t, libRepo, "Pinned Test", "10.1/abc")

	convID, _ := repo.CreateConversation()
	if err := repo.PinPaper(convID, paperID); err != nil {
		t.Fatalf("PinPaper: %v", err)
	}
	// idempotent: pinning twice does not error
	if err := repo.PinPaper(convID, paperID); err != nil {
		t.Fatalf("PinPaper second time: %v", err)
	}
	pinned, err := repo.ListPinnedPapers(convID)
	if err != nil {
		t.Fatalf("ListPinnedPapers: %v", err)
	}
	if len(pinned) != 1 || pinned[0].PaperID != paperID || pinned[0].Title != "Pinned Test" {
		t.Fatalf("pinned = %+v", pinned)
	}
	if err := repo.UnpinPaper(convID, paperID); err != nil {
		t.Fatalf("UnpinPaper: %v", err)
	}
	pinned, _ = repo.ListPinnedPapers(convID)
	if len(pinned) != 0 {
		t.Fatalf("expected unpin to clear; got %+v", pinned)
	}
}

func TestAIConversationRunArtifactsRoundTrip(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	convID, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	userID, err := repo.AddMessage(convID, "user", "帮我查找 ATAC 数据", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage user: %v", err)
	}
	assistantID, err := repo.AddMessage(convID, "assistant", "找到相关文献", AIMessageMeta{CitationsJSON: `[{"i":1}]`})
	if err != nil {
		t.Fatalf("AddMessage assistant: %v", err)
	}

	processSummaryJSON := `{"stages":[{"label":"全库检索","count":184},{"label":"命中","count":12}]}`
	runID, err := repo.CreateTurnRun(AITurnRun{
		ConversationID:     convID,
		UserMessageID:      userID,
		AssistantMessageID: sql.NullInt64{Int64: assistantID, Valid: true},
		Intent:             "library_search",
		IntentHint:         "library_search",
		ProcessSummaryJSON: processSummaryJSON,
		Status:             "completed",
	})
	if err != nil {
		t.Fatalf("CreateTurnRun: %v", err)
	}
	inputJSON := `{"query":"ATAC"}`
	outputSummaryJSON := `{"scanned":184,"hits":12}`
	toolCallID, err := repo.AddToolCall(AIToolCall{
		TurnRunID:         runID,
		ToolName:          "library_search",
		InputJSON:         inputJSON,
		OutputSummaryJSON: outputSummaryJSON,
		Status:            "completed",
		DurationMS:        17,
	})
	if err != nil {
		t.Fatalf("AddToolCall: %v", err)
	}
	payloadJSON := ` {"paper_id":42,"title":"ATAC Paper"} `
	cardID, err := repo.AddResultCard(AIResultCard{
		TurnRunID:   runID,
		CardType:    "paper_hit",
		SortOrder:   1,
		PayloadJSON: payloadJSON,
	})
	if err != nil {
		t.Fatalf("AddResultCard: %v", err)
	}
	if err := repo.UpdateTurnRunAssistant(runID, assistantID, " completed "); err != nil {
		t.Fatalf("UpdateTurnRunAssistant: %v", err)
	}

	runs, err := repo.ListTurnRuns(convID)
	if err != nil {
		t.Fatalf("ListTurnRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Intent != "library_search" || runs[0].ProcessSummaryJSON == "" {
		t.Fatalf("runs = %+v", runs)
	}
	run := runs[0]
	if run.ID != runID ||
		run.ConversationID != convID ||
		run.UserMessageID != userID ||
		!run.AssistantMessageID.Valid ||
		run.AssistantMessageID.Int64 != assistantID ||
		run.IntentHint != "library_search" ||
		run.ProcessSummaryJSON != processSummaryJSON ||
		run.Status != "completed" ||
		run.CreatedAt.IsZero() ||
		run.UpdatedAt.IsZero() {
		t.Fatalf("run = %+v", run)
	}
	calls, err := repo.ListToolCalls(runID)
	if err != nil {
		t.Fatalf("ListToolCalls: %v", err)
	}
	if len(calls) != 1 || calls[0].ToolName != "library_search" || calls[0].DurationMS != 17 {
		t.Fatalf("calls = %+v", calls)
	}
	call := calls[0]
	if call.ID != toolCallID ||
		call.TurnRunID != runID ||
		call.InputJSON != inputJSON ||
		call.OutputSummaryJSON != outputSummaryJSON ||
		call.Status != "completed" ||
		call.Error != "" ||
		call.CreatedAt.IsZero() {
		t.Fatalf("call = %+v", call)
	}
	cards, err := repo.ListResultCards(runID)
	if err != nil {
		t.Fatalf("ListResultCards: %v", err)
	}
	if len(cards) != 1 || cards[0].CardType != "paper_hit" || cards[0].SortOrder != 1 {
		t.Fatalf("cards = %+v", cards)
	}
	card := cards[0]
	if card.ID != cardID || card.TurnRunID != runID || card.PayloadJSON != payloadJSON || card.CreatedAt.IsZero() {
		t.Fatalf("card = %+v", card)
	}
}

func TestAIConversationCreateTurnRunRejectsCrossConversationUserMessage(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	convID, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation convID: %v", err)
	}
	otherConvID, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation otherConvID: %v", err)
	}
	userID, err := repo.AddMessage(otherConvID, "user", "q", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage user: %v", err)
	}

	if _, err := repo.CreateTurnRun(AITurnRun{
		ConversationID: convID,
		UserMessageID:  userID,
		Intent:         "library_search",
		Status:         "completed",
	}); err == nil {
		t.Fatal("CreateTurnRun accepted user message from another conversation")
	}
}

func TestAIConversationCreateTurnRunRejectsCrossConversationAssistantMessage(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	convID, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation convID: %v", err)
	}
	otherConvID, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation otherConvID: %v", err)
	}
	userID, err := repo.AddMessage(convID, "user", "q", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage user: %v", err)
	}
	assistantID, err := repo.AddMessage(otherConvID, "assistant", "a", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage assistant: %v", err)
	}

	if _, err := repo.CreateTurnRun(AITurnRun{
		ConversationID:     convID,
		UserMessageID:      userID,
		AssistantMessageID: sql.NullInt64{Int64: assistantID, Valid: true},
		Intent:             "library_search",
		Status:             "completed",
	}); err == nil {
		t.Fatal("CreateTurnRun accepted assistant message from another conversation")
	}
}

func TestAIConversationResultCardPayloadValidation(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	convID, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	userID, err := repo.AddMessage(convID, "user", "q", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage user: %v", err)
	}
	runID, err := repo.CreateTurnRun(AITurnRun{
		ConversationID: convID,
		UserMessageID:  userID,
		Intent:         "library_search",
		Status:         "completed",
	})
	if err != nil {
		t.Fatalf("CreateTurnRun: %v", err)
	}

	if _, err := repo.AddResultCard(AIResultCard{TurnRunID: runID, CardType: "empty_payload", PayloadJSON: ""}); err != nil {
		t.Fatalf("AddResultCard empty payload: %v", err)
	}
	payloadJSON := ` {"ok":true} `
	if _, err := repo.AddResultCard(AIResultCard{TurnRunID: runID, CardType: "spaced_payload", SortOrder: 1, PayloadJSON: payloadJSON}); err != nil {
		t.Fatalf("AddResultCard spaced payload: %v", err)
	}
	if _, err := repo.AddResultCard(AIResultCard{TurnRunID: runID, CardType: "blank_payload", PayloadJSON: "   "}); err == nil {
		t.Fatal("AddResultCard accepted whitespace-only payload")
	}
	if _, err := repo.AddResultCard(AIResultCard{TurnRunID: runID, CardType: "invalid_payload", PayloadJSON: `{"bad"`}); err == nil {
		t.Fatal("AddResultCard accepted invalid JSON payload")
	}

	cards, err := repo.ListResultCards(runID)
	if err != nil {
		t.Fatalf("ListResultCards: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("cards = %+v", cards)
	}
	if cards[0].PayloadJSON != "{}" {
		t.Fatalf("empty payload card = %+v", cards[0])
	}
	if cards[1].PayloadJSON != payloadJSON {
		t.Fatalf("spaced payload card = %+v", cards[1])
	}
}

func TestAIConversationAddToolCallRejectsNegativeDuration(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	convID, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	userID, err := repo.AddMessage(convID, "user", "q", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage user: %v", err)
	}
	runID, err := repo.CreateTurnRun(AITurnRun{
		ConversationID: convID,
		UserMessageID:  userID,
		Intent:         "library_search",
		Status:         "completed",
	})
	if err != nil {
		t.Fatalf("CreateTurnRun: %v", err)
	}

	if _, err := repo.AddToolCall(AIToolCall{TurnRunID: runID, ToolName: "library_search", DurationMS: -1}); err == nil {
		t.Fatal("AddToolCall accepted negative duration")
	}
}

func TestAIConversationRunArtifactsUpdateTurnRunAssistantRejectsCrossConversationMessage(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	convID, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation convID: %v", err)
	}
	otherConvID, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation otherConvID: %v", err)
	}
	userID, err := repo.AddMessage(convID, "user", "q", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage user: %v", err)
	}
	assistantID, err := repo.AddMessage(convID, "assistant", "a", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage assistant: %v", err)
	}
	otherAssistantID, err := repo.AddMessage(otherConvID, "assistant", "other", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage other assistant: %v", err)
	}
	runID, err := repo.CreateTurnRun(AITurnRun{
		ConversationID:     convID,
		UserMessageID:      userID,
		AssistantMessageID: sql.NullInt64{Int64: assistantID, Valid: true},
		Intent:             "library_search",
		Status:             "running",
	})
	if err != nil {
		t.Fatalf("CreateTurnRun: %v", err)
	}

	if err := repo.UpdateTurnRunAssistant(runID, otherAssistantID, "completed"); err == nil {
		t.Fatal("UpdateTurnRunAssistant accepted assistant message from another conversation")
	}
	runs, err := repo.ListTurnRuns(convID)
	if err != nil {
		t.Fatalf("ListTurnRuns: %v", err)
	}
	if len(runs) != 1 || !runs[0].AssistantMessageID.Valid || runs[0].AssistantMessageID.Int64 != assistantID || runs[0].Status != "running" {
		t.Fatalf("runs after rejected update = %+v", runs)
	}
}

func TestAIConversationRunArtifactsUpdateTurnRunAssistantRejectsMissingRun(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	convID, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	assistantID, err := repo.AddMessage(convID, "assistant", "a", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage assistant: %v", err)
	}

	if err := repo.UpdateTurnRunAssistant(999, assistantID, "completed"); err == nil {
		t.Fatal("UpdateTurnRunAssistant accepted missing run")
	}
}

func TestAIConversationRunArtifactsCascadeWithConversation(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	convID, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	userID, err := repo.AddMessage(convID, "user", "q", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage user: %v", err)
	}
	assistantID, err := repo.AddMessage(convID, "assistant", "a", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage assistant: %v", err)
	}
	runID, err := repo.CreateTurnRun(AITurnRun{
		ConversationID:     convID,
		UserMessageID:      userID,
		AssistantMessageID: sql.NullInt64{Int64: assistantID, Valid: true},
		Intent:             "library_search",
		Status:             "completed",
	})
	if err != nil {
		t.Fatalf("CreateTurnRun: %v", err)
	}
	if _, err := repo.AddToolCall(AIToolCall{TurnRunID: runID, ToolName: "library_search", Status: "completed"}); err != nil {
		t.Fatalf("AddToolCall: %v", err)
	}
	if _, err := repo.AddResultCard(AIResultCard{TurnRunID: runID, CardType: "paper_hit", SortOrder: 1, PayloadJSON: `{}`}); err != nil {
		t.Fatalf("AddResultCard: %v", err)
	}

	if err := repo.DeleteConversation(convID); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	runs, err := repo.ListTurnRuns(convID)
	if err != nil {
		t.Fatalf("ListTurnRuns after delete: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs after delete = %+v, want empty", runs)
	}
}

// mustInsertTestPaper inserts a minimal papers row (FK target for pin test).
func mustInsertTestPaper(t *testing.T, libRepo *LibraryRepository, title, doi string) int64 {
	t.Helper()
	res, err := libRepo.db.Exec(`
		INSERT INTO papers (title, doi, original_filename, stored_pdf_name)
		VALUES (?, ?, 'test.pdf', 'test.pdf')
	`, title, doi)
	if err != nil {
		t.Fatalf("insert paper: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}
