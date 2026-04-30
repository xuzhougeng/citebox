package ai_conversation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

type stubSettingsProvider struct {
	settings model.AISettings
}

func (s *stubSettingsProvider) GetSettings() (*model.AISettings, error) {
	return &s.settings, nil
}

type stubStreamCaller struct {
	calls       int32
	systemSeen  string
	userSeen    string
	staticReply string
}

func (s *stubStreamCaller) CallProviderStreamGeneric(ctx context.Context, settings model.AISettings,
	systemPrompt, userPrompt string, images []model.AIImageInput, onDelta func(string) error) (string, string, error) {
	atomic.AddInt32(&s.calls, 1)
	s.systemSeen = systemPrompt
	s.userSeen = userPrompt
	if err := onDelta(s.staticReply); err != nil {
		return "", "", err
	}
	return s.staticReply, "test", nil
}

func newServiceForTest(t *testing.T) (*Service, *repository.LibraryRepository, *stubStreamCaller) {
	t.Helper()
	libRepo, err := repository.NewLibraryRepository(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("NewLibraryRepository: %v", err)
	}
	t.Cleanup(func() { _ = libRepo.Close() })

	settings := model.DefaultAISettings()
	settings.APIKey = "fake"
	settings.PinPapersLimit = 5
	settings.ContextBudgetTokens = 32000

	caller := &stubStreamCaller{staticReply: "AI 回答正文"}
	svc := New(libRepo.AIConversation, libRepo.Paper,
		&stubSettingsProvider{settings: settings},
		caller, nil, nil, nil)
	return svc, libRepo, caller
}

func TestSendMessageCreatesConversationAndPersists(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)

	convID, err := svc.CreateDraft()
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	deltas := []string{}
	res, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "你好",
	}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if caller.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", caller.calls)
	}
	if res.UserMessage.Content != "你好" || res.AssistantMessage.Content != "AI 回答正文" {
		t.Fatalf("messages = %+v", res)
	}
	if !strings.Contains(strings.Join(deltas, ""), "AI 回答正文") {
		t.Fatalf("deltas = %v", deltas)
	}
	// History persisted
	msgs, _ := libRepo.AIConversation.ListMessages(convID, 0, 100)
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("persisted msgs = %+v", msgs)
	}
}

func TestSendMessageAutoPinsPaper(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)
	paperID := mustInsertPaperForTest(t, libRepo, "Auto Pin Paper", "10.1/auto")
	convID, _ := svc.CreateDraft()

	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "关于这篇论文",
		PaperID:        paperID,
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	pinned, _ := libRepo.AIConversation.ListPinnedPapers(convID)
	if len(pinned) != 1 || pinned[0].PaperID != paperID {
		t.Fatalf("expected paper auto-pinned, got %+v", pinned)
	}
}

func TestStrictEvidenceUsesRetrievedSnippetsNotPinnedFullText(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	paperID := mustInsertPaperForTest(t, libRepo, "Strict Paper", "")
	_, err := libRepo.DB().Exec(
		`UPDATE papers SET pdf_text = ? WHERE id = ?`,
		"FULL_CONTEXT_SHOULD_NOT_LEAK "+strings.Repeat("background filler ", 160)+"The study uses scRNA-seq evidence for trajectory analysis.",
		paperID,
	)
	if err != nil {
		t.Fatalf("update pdf_text: %v", err)
	}
	convID, _ := svc.CreateDraft()
	if err := svc.PinPaper(convID, paperID); err != nil {
		t.Fatalf("PinPaper: %v", err)
	}
	if err := svc.UpdateStrictEvidence(convID, true); err != nil {
		t.Fatalf("UpdateStrictEvidence: %v", err)
	}

	_, err = svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "查找单细胞测序证据",
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if strings.Contains(caller.userSeen, "FULL_CONTEXT_SHOULD_NOT_LEAK") {
		t.Fatalf("strict prompt leaked broad pinned context: %s", caller.userSeen)
	}
	if !strings.Contains(caller.userSeen, "scRNA-seq") || !strings.Contains(caller.userSeen, "内部搜索模式") {
		t.Fatalf("strict prompt missing retrieved evidence: %s", caller.userSeen)
	}
}

func TestStrictEvidenceSearchesLocalLibraryWithoutPinnedPapers(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	atacID := mustInsertPaperForTest(t, libRepo, "ATAC Candidate", "")
	unrelatedID := mustInsertPaperForTest(t, libRepo, "Unrelated Candidate", "")
	_, err := libRepo.DB().Exec(
		`UPDATE papers SET pdf_text = CASE id WHEN ? THEN ? WHEN ? THEN ? ELSE pdf_text END WHERE id IN (?, ?)`,
		atacID,
		"We performed single-cell chromatin accessibility profiling to identify regulatory elements.",
		unrelatedID,
		"This manuscript studies protein localization without sequencing data.",
		atacID,
		unrelatedID,
	)
	if err != nil {
		t.Fatalf("update pdf_text: %v", err)
	}
	convID, _ := svc.CreateDraft()
	if err := svc.UpdateStrictEvidence(convID, true); err != nil {
		t.Fatalf("UpdateStrictEvidence: %v", err)
	}

	_, err = svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "帮我查找包括 ATAC 数据的文章",
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !strings.Contains(caller.userSeen, "ATAC Candidate") || !strings.Contains(caller.userSeen, "chromatin accessibility") {
		t.Fatalf("strict prompt missing local library ATAC evidence: %s", caller.userSeen)
	}
	if strings.Contains(caller.userSeen, "Unrelated Candidate") {
		t.Fatalf("strict prompt included unrelated library paper: %s", caller.userSeen)
	}
}

func TestExternalEvidenceCanRunWithoutStrictEvidence(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	paperID := mustInsertPaperForTest(t, libRepo, "External Candidate", "10.1/external")
	_, err := libRepo.DB().Exec(
		`UPDATE papers SET pdf_text = ? WHERE id = ?`,
		"LOCAL_CONTEXT_SHOULD_NOT_APPEAR chromatin accessibility text.",
		paperID,
	)
	if err != nil {
		t.Fatalf("update pdf_text: %v", err)
	}
	convID, _ := svc.CreateDraft()
	if err := svc.PinPaper(convID, paperID); err != nil {
		t.Fatalf("PinPaper: %v", err)
	}
	svc.searcher = &stubSnippetSearcher{
		res: research.SnippetList{
			Items: []research.SnippetMatch{
				{
					PaperID: "s2-external",
					Paper:   research.Paper{PaperID: "s2-external", Title: "External Candidate", ExternalIDs: research.IDs{DOI: "10.1/external"}},
					Snippet: research.Snippet{Text: "external independent evidence snippet", SnippetKind: "body", Section: "Results"},
					Score:   0.88,
				},
			},
		},
	}

	_, err = svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID:          convID,
		Content:                 "帮我查找包括 ATAC 数据的文章",
		IncludeExternalEvidence: true,
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !strings.Contains(caller.userSeen, "外部 Semantic Scholar") || !strings.Contains(caller.userSeen, "external independent evidence snippet") {
		t.Fatalf("prompt missing independent external evidence: %s", caller.userSeen)
	}
}

type stubOrchestrator struct {
	calls int32
	in    ai_assistant.RunInput
	out   ai_assistant.RunOutput
	err   error
}

func (s *stubOrchestrator) Run(ctx context.Context, in ai_assistant.RunInput) (ai_assistant.RunOutput, error) {
	atomic.AddInt32(&s.calls, 1)
	s.in = in
	return s.out, s.err
}

func TestStrictEvidenceUsesLegacyPathWhenOrchestratorConfiguredWithoutExplicitIntent(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	orch := &stubOrchestrator{
		out: ai_assistant.RunOutput{
			Intent:        ai_assistant.IntentChat,
			AnswerContext: "ORCH_CONTEXT_SHOULD_NOT_APPEAR\n\n用户问题：\n查找单细胞测序证据",
		},
	}
	svc.orchestrator = orch
	paperID := mustInsertPaperForTest(t, libRepo, "Strict Paper", "")
	_, err := libRepo.DB().Exec(
		`UPDATE papers SET pdf_text = ? WHERE id = ?`,
		"The study uses scRNA-seq evidence for trajectory analysis.",
		paperID,
	)
	if err != nil {
		t.Fatalf("update pdf_text: %v", err)
	}
	convID, _ := svc.CreateDraft()
	if err := svc.PinPaper(convID, paperID); err != nil {
		t.Fatalf("PinPaper: %v", err)
	}
	if err := svc.UpdateStrictEvidence(convID, true); err != nil {
		t.Fatalf("UpdateStrictEvidence: %v", err)
	}

	_, err = svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "查找单细胞测序证据",
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if orch.calls != 0 {
		t.Fatalf("orchestrator calls = %d, want 0", orch.calls)
	}
	if strings.Contains(caller.userSeen, "ORCH_CONTEXT_SHOULD_NOT_APPEAR") {
		t.Fatalf("prompt used orchestrator context: %s", caller.userSeen)
	}
	if !strings.Contains(caller.userSeen, "内部搜索模式") || !strings.Contains(caller.userSeen, "scRNA-seq") {
		t.Fatalf("prompt missing legacy strict evidence: %s", caller.userSeen)
	}
}

func TestExternalEvidenceUsesLegacyPathWhenOrchestratorConfiguredWithoutExplicitIntent(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	orch := &stubOrchestrator{
		out: ai_assistant.RunOutput{
			Intent:        ai_assistant.IntentChat,
			AnswerContext: "ORCH_CONTEXT_SHOULD_NOT_APPEAR\n\n用户问题：\n帮我查找包括 ATAC 数据的文章",
		},
	}
	svc.orchestrator = orch
	paperID := mustInsertPaperForTest(t, libRepo, "External Candidate", "10.1/external")
	convID, _ := svc.CreateDraft()
	if err := svc.PinPaper(convID, paperID); err != nil {
		t.Fatalf("PinPaper: %v", err)
	}
	svc.searcher = &stubSnippetSearcher{
		res: research.SnippetList{
			Items: []research.SnippetMatch{{
				PaperID: "s2-external",
				Paper:   research.Paper{PaperID: "s2-external", Title: "External Candidate", ExternalIDs: research.IDs{DOI: "10.1/external"}},
				Snippet: research.Snippet{Text: "external independent evidence snippet", SnippetKind: "body", Section: "Results"},
				Score:   0.88,
			}},
		},
	}

	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID:          convID,
		Content:                 "帮我查找包括 ATAC 数据的文章",
		IncludeExternalEvidence: true,
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if orch.calls != 0 {
		t.Fatalf("orchestrator calls = %d, want 0", orch.calls)
	}
	if strings.Contains(caller.userSeen, "ORCH_CONTEXT_SHOULD_NOT_APPEAR") {
		t.Fatalf("prompt used orchestrator context: %s", caller.userSeen)
	}
	if !strings.Contains(caller.userSeen, "外部 Semantic Scholar") || !strings.Contains(caller.userSeen, "external independent evidence snippet") {
		t.Fatalf("prompt missing legacy external evidence: %s", caller.userSeen)
	}
}

func TestExplicitIntentUsesOrchestratorWhenLegacyEvidenceFlagIsSet(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	orch := &stubOrchestrator{
		out: ai_assistant.RunOutput{
			Intent:        ai_assistant.IntentFigureLookup,
			IntentHint:    ai_assistant.IntentFigureLookup,
			Process:       ai_assistant.ProcessSummary{Intent: ai_assistant.IntentFigureLookup},
			AnswerContext: "工具结果：\nORCH_CONTEXT_SELECTED\n\n用户问题：\n看图 1",
		},
	}
	svc.orchestrator = orch
	paperID := mustInsertPaperForTest(t, libRepo, "Strict Paper", "")
	_, err := libRepo.DB().Exec(
		`UPDATE papers SET pdf_text = ? WHERE id = ?`,
		"The study uses scRNA-seq evidence for trajectory analysis.",
		paperID,
	)
	if err != nil {
		t.Fatalf("update pdf_text: %v", err)
	}
	convID, _ := svc.CreateDraft()
	if err := svc.PinPaper(convID, paperID); err != nil {
		t.Fatalf("PinPaper: %v", err)
	}
	if err := svc.UpdateStrictEvidence(convID, true); err != nil {
		t.Fatalf("UpdateStrictEvidence: %v", err)
	}

	_, err = svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "看图 1",
		IntentHint:     ai_assistant.IntentFigureLookup,
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if orch.calls != 1 {
		t.Fatalf("orchestrator calls = %d, want 1", orch.calls)
	}
	if !strings.Contains(caller.userSeen, "ORCH_CONTEXT_SELECTED") {
		t.Fatalf("provider prompt missing orchestrator context: %s", caller.userSeen)
	}
	if strings.Contains(caller.userSeen, "内部搜索模式") || strings.Contains(caller.userSeen, "外部搜索模式") {
		t.Fatalf("provider prompt included legacy evidence mode: %s", caller.userSeen)
	}
}

func TestSendMessageUsesOrchestratorEventsAndPersistsArtifacts(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	orch := &stubOrchestrator{
		out: ai_assistant.RunOutput{
			Intent:     ai_assistant.IntentFigureLookup,
			IntentHint: ai_assistant.IntentFigureLookup,
			Process: ai_assistant.ProcessSummary{
				Intent: ai_assistant.IntentFigureLookup,
				Stages: []ai_assistant.ProcessStage{{
					Label:  "图表检索",
					Count:  1,
					Status: "completed",
				}},
			},
			Cards: []ai_assistant.ResultCard{{
				Type:    "figure_result",
				Payload: map[string]any{"figure_id": int64(3), "title": "Figure 1"},
			}},
			Citations: []ai_assistant.Citation{{
				I:       1,
				PaperID: 7,
				Title:   "Paper A",
				Source:  "local",
				Snippet: research.Snippet{Text: "caption evidence", SnippetKind: "figure"},
			}},
			AnswerContext: "工具结果：\nORCH_CONTEXT\n\n用户问题：\n看图 1",
			ToolCalls: []ai_assistant.ToolCallSummary{{
				ToolName:          "figure_lookup",
				InputJSON:         `{"query":"看图 1"}`,
				OutputSummaryJSON: `{"hits":1}`,
				Status:            "completed",
				DurationMS:        12,
			}},
		},
	}
	svc.orchestrator = orch

	convID, err := svc.CreateDraft()
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	var events []StreamEvent
	_, err = svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "看图 1",
		IntentHint:     ai_assistant.IntentFigureLookup,
		Context: ai_assistant.RequestContext{
			Source:   "paper",
			PaperID:  7,
			FigureID: 3,
		},
		OnEvent: func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if orch.in.IntentHint != ai_assistant.IntentFigureLookup || orch.in.Context.PaperID != 7 || orch.in.Context.FigureID != 3 {
		t.Fatalf("orchestrator input = %+v", orch.in)
	}
	if !strings.Contains(caller.userSeen, "ORCH_CONTEXT") {
		t.Fatalf("provider prompt missing orchestrator context: %s", caller.userSeen)
	}
	if len(events) != 3 || events[0].Type != "process" || events[1].Type != "cards" || events[2].Type != "citations" {
		t.Fatalf("events = %+v", events)
	}

	msgs, err := libRepo.AIConversation.ListMessages(convID, 0, 10)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 || !strings.Contains(msgs[1].CitationsJSON, "Paper A") {
		t.Fatalf("messages = %+v", msgs)
	}
	runs, err := libRepo.AIConversation.ListTurnRuns(convID)
	if err != nil {
		t.Fatalf("ListTurnRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Intent != ai_assistant.IntentFigureLookup || runs[0].Status != "completed" {
		t.Fatalf("runs = %+v", runs)
	}
	calls, err := libRepo.AIConversation.ListToolCalls(runs[0].ID)
	if err != nil {
		t.Fatalf("ListToolCalls: %v", err)
	}
	if len(calls) != 1 || calls[0].ToolName != "figure_lookup" || calls[0].DurationMS != 12 {
		t.Fatalf("calls = %+v", calls)
	}
	cards, err := libRepo.AIConversation.ListResultCards(runs[0].ID)
	if err != nil {
		t.Fatalf("ListResultCards: %v", err)
	}
	if len(cards) != 1 || cards[0].CardType != "figure_result" || !strings.Contains(cards[0].PayloadJSON, "Figure 1") {
		t.Fatalf("cards = %+v", cards)
	}
	conv, err := svc.GetConversation(convID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if len(conv.TurnRuns) != 1 || len(conv.TurnRuns[0].Cards) != 1 {
		t.Fatalf("conversation turn runs = %+v", conv.TurnRuns)
	}
}

func TestSendMessageReturnsOnEventErrorBeforeProviderCall(t *testing.T) {
	svc, _, caller := newServiceForTest(t)
	svc.orchestrator = &stubOrchestrator{
		out: ai_assistant.RunOutput{
			Intent:  ai_assistant.IntentFigureLookup,
			Process: ai_assistant.ProcessSummary{Intent: ai_assistant.IntentFigureLookup},
		},
	}
	convID, _ := svc.CreateDraft()
	eventErr := errors.New("client stream closed")

	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "看图",
		IntentHint:     ai_assistant.IntentFigureLookup,
		OnEvent: func(StreamEvent) error {
			return eventErr
		},
	}, func(string) error { return nil })
	if !errors.Is(err, eventErr) {
		t.Fatalf("SendMessage error = %v, want %v", err, eventErr)
	}
	if caller.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", caller.calls)
	}
}

type cancelingStreamCaller struct {
	calls   int32
	rawText string
}

func (s *cancelingStreamCaller) CallProviderStreamGeneric(ctx context.Context, settings model.AISettings,
	systemPrompt, userPrompt string, images []model.AIImageInput, onDelta func(string) error) (string, string, error) {
	atomic.AddInt32(&s.calls, 1)
	return s.rawText, "", context.Canceled
}

func TestSendMessagePersistsOrchestratorArtifactsForStoppedStream(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)
	cancelingCaller := &cancelingStreamCaller{rawText: "partial assistant text"}
	svc.caller = cancelingCaller
	svc.titleCaller = nil
	svc.summaryCaller = nil
	svc.orchestrator = &stubOrchestrator{
		out: ai_assistant.RunOutput{
			Intent:     ai_assistant.IntentFigureLookup,
			IntentHint: ai_assistant.IntentFigureLookup,
			Process:    ai_assistant.ProcessSummary{Intent: ai_assistant.IntentFigureLookup},
			Cards: []ai_assistant.ResultCard{{
				Type:    "figure_result",
				Payload: map[string]any{"figure_id": int64(3)},
			}},
			ToolCalls: []ai_assistant.ToolCallSummary{{
				ToolName:          "figure_lookup",
				InputJSON:         `{"query":"看图"}`,
				OutputSummaryJSON: `{"hits":1}`,
				Status:            "completed",
			}},
		},
	}
	convID, _ := svc.CreateDraft()

	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "看图",
		IntentHint:     ai_assistant.IntentFigureLookup,
	}, func(string) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SendMessage error = %v, want context.Canceled", err)
	}
	msgs, err := libRepo.AIConversation.ListMessages(convID, 0, 10)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 || msgs[1].Role != "assistant" || msgs[1].Content != "partial assistant text" || msgs[1].Mode != "stopped" {
		t.Fatalf("messages = %+v", msgs)
	}
	runs, err := libRepo.AIConversation.ListTurnRuns(convID)
	if err != nil {
		t.Fatalf("ListTurnRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "stopped" || !runs[0].AssistantMessageID.Valid || runs[0].AssistantMessageID.Int64 != msgs[1].ID {
		t.Fatalf("runs = %+v", runs)
	}
	calls, err := libRepo.AIConversation.ListToolCalls(runs[0].ID)
	if err != nil {
		t.Fatalf("ListToolCalls: %v", err)
	}
	if len(calls) != 1 || calls[0].ToolName != "figure_lookup" || calls[0].Status != "completed" {
		t.Fatalf("calls = %+v", calls)
	}
	cards, err := libRepo.AIConversation.ListResultCards(runs[0].ID)
	if err != nil {
		t.Fatalf("ListResultCards: %v", err)
	}
	if len(cards) != 1 || cards[0].CardType != "figure_result" {
		t.Fatalf("cards = %+v", cards)
	}
}

func TestSendMessagePinLimit(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)
	convID, _ := svc.CreateDraft()
	// Pin 5 to fill quota.
	for i := 0; i < 5; i++ {
		pid := mustInsertPaperForTest(t, libRepo, "P", fmt.Sprintf("10.1/x%d", i))
		if err := svc.PinPaper(convID, pid); err != nil {
			t.Fatalf("pin %d: %v", i, err)
		}
	}
	// 6th via auto-pin must reject the message.
	pid6 := mustInsertPaperForTest(t, libRepo, "P6", "10.1/six")
	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "再加一篇",
		PaperID:        pid6,
	}, func(string) error { return nil })
	if err == nil {
		t.Fatalf("expected pin-limit rejection")
	}
	if !strings.Contains(err.Error(), "最多") {
		t.Fatalf("error message = %v", err)
	}
}

var testPaperSeq int64

func mustInsertPaperForTest(t *testing.T, libRepo *repository.LibraryRepository, title, doi string) int64 {
	t.Helper()
	seq := atomic.AddInt64(&testPaperSeq, 1)
	fname := fmt.Sprintf("paper_%d.pdf", seq)
	res, err := libRepo.DB().Exec(
		`INSERT INTO papers (title, doi, original_filename, stored_pdf_name) VALUES (?, ?, ?, ?)`,
		title, doi, fname, fname)
	if err != nil {
		t.Fatalf("insert paper: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestTitleGeneratedOnFirstTurn(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	caller.staticReply = "回答内容"

	titleCaller := &stubNonStreamCaller{staticReply: "  对话标题  "}
	svc.titleCaller = titleCaller

	convID, _ := svc.CreateDraft()
	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "第一条消息",
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// title generation is async; wait briefly
	for i := 0; i < 50; i++ {
		c, _ := libRepo.AIConversation.GetConversation(convID)
		if c.Title != "" {
			if c.Title != "对话标题" {
				t.Fatalf("title = %q, want trimmed", c.Title)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("title was not generated within 1s")
}

type stubNonStreamCaller struct {
	staticReply string
}

func (s *stubNonStreamCaller) CallProviderGeneric(ctx context.Context, settings model.AISettings,
	systemPrompt, userPrompt string) (string, string, error) {
	return s.staticReply, "test", nil
}

func TestSendMessageTriggersSummaryWhenOverBudget(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	titleCaller := &stubNonStreamCaller{staticReply: "压缩 / 标题"}
	svc.titleCaller = titleCaller
	svc.summaryCaller = titleCaller

	convID, _ := svc.CreateDraft()
	// Seed many messages so we exceed budget.
	bigText := strings.Repeat("一段很长的内容。", 800)
	for i := 0; i < 6; i++ {
		_, _ = libRepo.AIConversation.AddMessage(convID, "user", bigText, repository.AIMessageMeta{})
		_, _ = libRepo.AIConversation.AddMessage(convID, "assistant", bigText, repository.AIMessageMeta{})
	}

	// Force a tiny budget so summarization fires.
	smallSettings := model.DefaultAISettings()
	smallSettings.APIKey = "fake"
	smallSettings.PinPapersLimit = 5
	smallSettings.ContextBudgetTokens = 500
	svc.settings = &stubSettingsProvider{settings: smallSettings}

	caller.staticReply = "答案"
	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "新问题",
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	conv, _ := libRepo.AIConversation.GetConversation(convID)
	if conv.SummaryText == "" {
		t.Fatalf("expected summary to be persisted")
	}
}
