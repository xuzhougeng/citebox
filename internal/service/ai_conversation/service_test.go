package ai_conversation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
	ai_image_gen_pkg "github.com/xuzhougeng/citebox/internal/service/ai_image_gen"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

type stubSettingsProvider struct {
	settings model.AISettings
}

func (s *stubSettingsProvider) GetSettings() (*model.AISettings, error) {
	return &s.settings, nil
}

type stubStreamCaller struct {
	calls        int32
	systemSeen   string
	userSeen     string
	settingsSeen model.AISettings
	imagesSeen   []model.AIImageInput
	staticReply  string
}

func (s *stubStreamCaller) CallProviderStreamGeneric(ctx context.Context, settings model.AISettings,
	systemPrompt, userPrompt string, images []model.AIImageInput, onDelta func(string) error) (string, string, error) {
	atomic.AddInt32(&s.calls, 1)
	s.systemSeen = systemPrompt
	s.userSeen = userPrompt
	s.settingsSeen = settings
	s.imagesSeen = images
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

func TestSendMessageUsesAssistantMasterModel(t *testing.T) {
	svc, _, caller := newServiceForTest(t)
	settings := model.DefaultAISettings()
	settings.Models = []model.AIModelConfig{
		{ID: "master", Name: "Master", Provider: model.AIProviderOpenAI, APIKey: "master-key", BaseURL: "https://api.openai.com", Model: "gpt-5.5", MaxOutputTokens: 4096, ThinkingEnabled: true, ReasoningEffort: "xhigh"},
		{ID: "subagent", Name: "Sub-Agent", Provider: model.AIProviderOpenAI, APIKey: "sub-key", BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-flash", MaxOutputTokens: 1200, OpenAILegacyMode: true},
	}
	settings.SceneModels = model.AISceneModelSelection{
		DefaultModelID:           "subagent",
		AssistantMasterModelID:   "master",
		AssistantSubagentModelID: "subagent",
	}
	settings.ContextBudgetTokens = 32000
	svc.settings = &stubSettingsProvider{settings: settings}

	convID, _ := svc.CreateDraft()
	res, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "帮我查找 ATAC 文献",
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if caller.settingsSeen.Model != "gpt-5.5" || caller.settingsSeen.ReasoningEffort != "xhigh" || !caller.settingsSeen.ThinkingEnabled {
		t.Fatalf("stream settings = %+v, want assistant master model", caller.settingsSeen)
	}
	if res.AssistantMessage.Model != "gpt-5.5" || res.AssistantMessage.Provider != string(model.AIProviderOpenAI) {
		t.Fatalf("assistant metadata = %+v, want master model metadata", res.AssistantMessage)
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
	if !strings.Contains(caller.userSeen, "外部学术搜索") || !strings.Contains(caller.userSeen, "Semantic Scholar") || !strings.Contains(caller.userSeen, "external independent evidence snippet") {
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

func TestSendMessagePropagatesSearchGoalHintToOrchestrator(t *testing.T) {
	svc, _, _ := newServiceForTest(t)
	orch := &stubOrchestrator{
		out: ai_assistant.RunOutput{
			Intent:        ai_assistant.IntentExternalSearch,
			AnswerContext: "外部证据上下文",
		},
	}
	svc.orchestrator = orch
	convID, _ := svc.CreateDraft()

	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "请找证据支持这个说法",
		IntentHint:     ai_assistant.IntentExternalSearch,
		SearchGoalHint: ai_assistant.ExternalSearchGoalEvidence,
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if orch.calls != 1 {
		t.Fatalf("orchestrator calls = %d, want 1", orch.calls)
	}
	if got, want := orch.in.SearchGoalHint, ai_assistant.ExternalSearchGoalEvidence; got != want {
		t.Fatalf("SearchGoalHint = %q, want %q", got, want)
	}
}

func TestSendMessageSearchGoalHintAloneForcesOrchestratorWhenLegacyEvidenceIsActive(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	orch := &stubOrchestrator{
		out: ai_assistant.RunOutput{
			Intent:        ai_assistant.IntentExternalSearch,
			AnswerContext: "ORCH_CONTEXT_SELECTED\n\n用户问题：\n请找证据支持这个说法",
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
		Content:        "请找证据支持这个说法",
		SearchGoalHint: ai_assistant.ExternalSearchGoalEvidence,
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if orch.calls != 1 {
		t.Fatalf("orchestrator calls = %d, want 1", orch.calls)
	}
	if orch.in.IntentHint != "" || len(orch.in.Sources) != 0 || !requestContextEmpty(orch.in.Context) {
		t.Fatalf("orchestrator input should rely on search goal hint only: %+v", orch.in)
	}
	if got, want := orch.in.SearchGoalHint, ai_assistant.ExternalSearchGoalEvidence; got != want {
		t.Fatalf("SearchGoalHint = %q, want %q", got, want)
	}
	if !strings.Contains(caller.userSeen, "ORCH_CONTEXT_SELECTED") {
		t.Fatalf("provider prompt missing orchestrator context: %s", caller.userSeen)
	}
	if strings.Contains(caller.userSeen, "内部搜索模式") {
		t.Fatalf("provider prompt used legacy strict evidence path: %s", caller.userSeen)
	}
}

func TestSendMessageInvalidSearchGoalHintDoesNotForceOrchestrator(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	orch := &stubOrchestrator{
		out: ai_assistant.RunOutput{
			Intent:        ai_assistant.IntentExternalSearch,
			AnswerContext: "ORCH_CONTEXT_SHOULD_NOT_APPEAR\n\n用户问题：\n请找证据支持这个说法",
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
		Content:        "请找证据支持这个说法",
		SearchGoalHint: ai_assistant.ExternalSearchGoal("unsupported"),
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
	if !strings.Contains(caller.userSeen, "内部搜索模式") || !strings.Contains(caller.userSeen, "证据不足") {
		t.Fatalf("prompt missing legacy strict evidence: %s", caller.userSeen)
	}
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
	if !strings.Contains(caller.userSeen, "外部学术搜索") || !strings.Contains(caller.userSeen, "Semantic Scholar") || !strings.Contains(caller.userSeen, "external independent evidence snippet") {
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
	if len(events) != 4 || events[0].Type != "process" || events[1].Type != "cards" || events[2].Type != "citations" || events[3].Type != "process" {
		t.Fatalf("events = %+v", events)
	}
	firstProcess, ok := events[0].Data.(ai_assistant.ProcessSummary)
	if !ok || stageByLabel(firstProcess.Stages, "生成回答").Status != "running" {
		t.Fatalf("first process event = %+v, want running answer generation stage", events[0].Data)
	}
	finalProcess, ok := events[3].Data.(ai_assistant.ProcessSummary)
	if !ok || stageByLabel(finalProcess.Stages, "生成回答").Status != "completed" {
		t.Fatalf("final process event = %+v, want completed answer generation stage", events[3].Data)
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
	if !strings.Contains(runs[0].ProcessSummaryJSON, "生成回答") || !strings.Contains(runs[0].ProcessSummaryJSON, "completed") {
		t.Fatalf("process summary json = %s, want completed answer generation stage", runs[0].ProcessSummaryJSON)
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

type failingStreamCaller struct {
	calls int32
	err   error
}

func (s *failingStreamCaller) CallProviderStreamGeneric(ctx context.Context, settings model.AISettings,
	systemPrompt, userPrompt string, images []model.AIImageInput, onDelta func(string) error) (string, string, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.err != nil {
		return "", "", s.err
	}
	return "", "", errors.New("provider stream failed")
}

func TestSendMessageFallsBackToToolAnswerWhenProviderFails(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)
	streamErr := errors.New("provider unavailable")
	svc.caller = &failingStreamCaller{err: streamErr}
	svc.titleCaller = nil
	svc.summaryCaller = nil
	svc.orchestrator = &stubOrchestrator{
		out: ai_assistant.RunOutput{
			Intent:     ai_assistant.IntentPaperRead,
			IntentHint: ai_assistant.IntentPaperRead,
			Process:    ai_assistant.ProcessSummary{Intent: ai_assistant.IntentPaperRead},
			Cards: []ai_assistant.ResultCard{{
				Type:    "paper_compare",
				Payload: map[string]any{"query": "compare"},
			}},
			Citations: []ai_assistant.Citation{{
				I:       1,
				PaperID: 7,
				Title:   "Paper A",
				Source:  "local",
				Snippet: research.Snippet{Text: "tool evidence", SnippetKind: "body"},
			}},
			AnswerContext: "你正在基于工具检索结果回答。\n\n工具结果：\n### Paper A\n- [1] 本地全文: tool evidence\n\n用户问题：\n对比文献",
			ToolCalls: []ai_assistant.ToolCallSummary{{
				ToolName:          "paper_read",
				InputJSON:         `{"query":"对比文献"}`,
				OutputSummaryJSON: `{"loaded":1}`,
				Status:            "completed",
			}},
		},
	}
	convID, _ := svc.CreateDraft()

	var deltas []string
	var events []StreamEvent
	res, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "对比文献",
		IntentHint:     ai_assistant.IntentPaperRead,
		OnEvent: func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if res.AssistantMessage.Mode != "tool_fallback" || !strings.Contains(res.AssistantMessage.Content, "tool evidence") {
		t.Fatalf("assistant fallback = %+v", res.AssistantMessage)
	}
	if !strings.Contains(strings.Join(deltas, ""), "provider unavailable") {
		t.Fatalf("deltas = %v", deltas)
	}
	if len(events) < 4 {
		t.Fatalf("events = %+v", events)
	}
	finalProcess, ok := events[len(events)-1].Data.(ai_assistant.ProcessSummary)
	if !ok || stageByLabel(finalProcess.Stages, "生成回答").Status != "failed" {
		t.Fatalf("final process event = %+v, want failed answer generation stage", events[len(events)-1].Data)
	}
	msgs, err := libRepo.AIConversation.ListMessages(convID, 0, 10)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 || msgs[1].Mode != "tool_fallback" || !strings.Contains(msgs[1].Content, "tool evidence") {
		t.Fatalf("messages = %+v", msgs)
	}
	runs, err := libRepo.AIConversation.ListTurnRuns(convID)
	if err != nil {
		t.Fatalf("ListTurnRuns: %v", err)
	}
	if len(runs) != 1 || !strings.Contains(runs[0].ProcessSummaryJSON, `"status":"failed"`) {
		t.Fatalf("runs = %+v", runs)
	}
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

func TestTitleGenerationUsesAssistantSubagentModel(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	caller.staticReply = "回答内容"
	settings := model.DefaultAISettings()
	settings.Models = []model.AIModelConfig{
		{ID: "master", Name: "Master", Provider: model.AIProviderOpenAI, APIKey: "master-key", BaseURL: "https://api.openai.com", Model: "gpt-5.5", MaxOutputTokens: 4096},
		{ID: "subagent", Name: "Sub-Agent", Provider: model.AIProviderOpenAI, APIKey: "sub-key", BaseURL: "https://api.deepseek.com", Model: "deepseek-v4-flash", MaxOutputTokens: 1200, OpenAILegacyMode: true},
	}
	settings.SceneModels = model.AISceneModelSelection{
		DefaultModelID:           "subagent",
		AssistantMasterModelID:   "master",
		AssistantSubagentModelID: "subagent",
	}
	settings.ContextBudgetTokens = 32000
	svc.settings = &stubSettingsProvider{settings: settings}

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

	for i := 0; i < 50; i++ {
		c, _ := libRepo.AIConversation.GetConversation(convID)
		if c.Title != "" {
			if titleCaller.settingsSeen.Model != "deepseek-v4-flash" || !titleCaller.settingsSeen.OpenAILegacyMode {
				t.Fatalf("title settings = %+v, want assistant subagent model", titleCaller.settingsSeen)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("title was not generated within 1s")
}

type stubNonStreamCaller struct {
	staticReply  string
	settingsSeen model.AISettings
}

func (s *stubNonStreamCaller) CallProviderGeneric(ctx context.Context, settings model.AISettings,
	systemPrompt, userPrompt string) (string, string, error) {
	s.settingsSeen = settings
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

func stageByLabel(stages []ai_assistant.ProcessStage, label string) ai_assistant.ProcessStage {
	for _, stage := range stages {
		if stage.Label == label {
			return stage
		}
	}
	return ai_assistant.ProcessStage{}
}

// ---------------------------------------------------------------------------
// image_generation intent routing
// ---------------------------------------------------------------------------

type fakeImageGen struct {
	fn func(context.Context, ai_image_gen_pkg.GenerateInput) error
}

func (f *fakeImageGen) Generate(ctx context.Context, in ai_image_gen_pkg.GenerateInput) error {
	return f.fn(ctx, in)
}

func TestSendMessage_RoutesImageGenerationIntent(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)

	// Seed a paper so PinPaper (inside SendMessage auto-pin) can find it.
	paperID := mustInsertPaperForTest(t, libRepo, "Image Gen Paper", "10.1/imggen")

	called := struct {
		paperIDs  []int64
		figureIDs []int64
		userText  string
		turnRunID int64
	}{}
	fake := &fakeImageGen{
		fn: func(ctx context.Context, in ai_image_gen_pkg.GenerateInput) error {
			called.paperIDs = in.PaperIDs
			called.figureIDs = in.FigureIDs
			called.userText = in.UserText
			called.turnRunID = in.TurnRunID
			_ = in.OnEvent(ai_image_gen_pkg.StreamEvent{Type: "image_generated"})
			return nil
		},
	}
	svc = svc.WithImageGen(fake)

	convID, err := svc.CreateDraft()
	if err != nil {
		t.Fatal(err)
	}

	var events []StreamEvent
	res, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "@image-gen draw it",
		IntentHint:     "image_generation",
		PaperIDs:       []int64{paperID},
		OnEvent: func(e StreamEvent) error {
			events = append(events, e)
			return nil
		},
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Verify the fake received the correct paperIDs and userText.
	if len(called.paperIDs) != 1 || called.paperIDs[0] != paperID {
		t.Fatalf("paperIDs: %v, want [%d]", called.paperIDs, paperID)
	}
	if len(called.figureIDs) != 0 {
		t.Fatalf("figureIDs: %v, want []", called.figureIDs)
	}
	if called.userText != "@image-gen draw it" {
		t.Fatalf("userText: %q", called.userText)
	}

	// Result should reflect a completed image-gen turn.
	if res.AssistantMessage.Mode != "completed" {
		t.Errorf("mode: %q, want completed", res.AssistantMessage.Mode)
	}
	if res.AssistantMessage.Content != "已生成图像。" {
		t.Errorf("content: %q", res.AssistantMessage.Content)
	}

	// The stream event forwarded from the fake should be visible.
	found := false
	for _, e := range events {
		if e.Type == "image_generated" {
			found = true
		}
	}
	if !found {
		t.Errorf("image_generated event not emitted; events = %v", events)
	}

	// DB should have user + assistant messages persisted.
	msgs, _ := libRepo.AIConversation.ListMessages(convID, 0, 10)
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("persisted msgs = %+v", msgs)
	}

	// A turn run should exist with intent=image_generation and status=completed.
	runs, _ := libRepo.AIConversation.ListTurnRuns(convID)
	if len(runs) != 1 || runs[0].Intent != "image_generation" || runs[0].Status != "completed" {
		t.Fatalf("runs = %+v", runs)
	}
	if !runs[0].AssistantMessageID.Valid || runs[0].AssistantMessageID.Int64 != msgs[1].ID {
		t.Fatalf("turn run assistant_message_id not linked: %+v", runs[0])
	}
}

func TestSendMessage_ImageGenerationFailure(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)
	paperID := mustInsertPaperForTest(t, libRepo, "Image Gen Paper 2", "10.1/imggen2")

	genErr := errors.New("image API unavailable")
	fake := &fakeImageGen{
		fn: func(ctx context.Context, in ai_image_gen_pkg.GenerateInput) error {
			return genErr
		},
	}
	svc = svc.WithImageGen(fake)

	convID, _ := svc.CreateDraft()
	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "@image-gen draw it",
		IntentHint:     "image_generation",
		PaperIDs:       []int64{paperID},
	}, func(string) error { return nil })
	if !errors.Is(err, genErr) {
		t.Fatalf("err = %v, want %v", err, genErr)
	}

	// Even on failure, the assistant message should be persisted with the error text.
	msgs, _ := libRepo.AIConversation.ListMessages(convID, 0, 10)
	if len(msgs) != 2 || msgs[1].Role != "assistant" || !strings.Contains(msgs[1].Content, "image API unavailable") {
		t.Fatalf("persisted msgs = %+v", msgs)
	}
	runs, _ := libRepo.AIConversation.ListTurnRuns(convID)
	if len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestSendMessage_ImageGenerationNoRefsRejected(t *testing.T) {
	svc, _, _ := newServiceForTest(t)
	fake := &fakeImageGen{
		fn: func(ctx context.Context, in ai_image_gen_pkg.GenerateInput) error {
			return nil
		},
	}
	svc = svc.WithImageGen(fake)

	convID, _ := svc.CreateDraft()
	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "@image-gen draw it",
		IntentHint:     "image_generation",
		// No PaperIDs and no FigureIDs — should be rejected.
	}, func(string) error { return nil })
	if err == nil {
		t.Fatal("expected error for missing refs, got nil")
	}
	if !strings.Contains(err.Error(), "请引用") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendMessage_ImageGenerationFigureOnlyDoesNotUsePinnedPapers(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)
	paperID := mustInsertPaperForTest(t, libRepo, "Pinned Figure Parent", "10.1/figurepinned")

	called := struct {
		paperIDs  []int64
		figureIDs []int64
	}{}
	fake := &fakeImageGen{
		fn: func(ctx context.Context, in ai_image_gen_pkg.GenerateInput) error {
			called.paperIDs = append([]int64(nil), in.PaperIDs...)
			called.figureIDs = append([]int64(nil), in.FigureIDs...)
			return nil
		},
	}
	svc = svc.WithImageGen(fake)

	convID, err := svc.CreateDraft()
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.PinPaper(convID, paperID); err != nil {
		t.Fatalf("PinPaper: %v", err)
	}

	_, err = svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "@image-gen draw from figure only",
		IntentHint:     "image_generation",
		Context: ai_assistant.RequestContext{
			FigureID: 99,
		},
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if len(called.paperIDs) != 0 {
		t.Fatalf("paperIDs: %v, want [] for figure-only request", called.paperIDs)
	}
	if len(called.figureIDs) != 1 || called.figureIDs[0] != 99 {
		t.Fatalf("figureIDs: %v, want [99]", called.figureIDs)
	}
}

func TestFallbackImageGenPaperIDs_LogsRepoErrors(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	ids := fallbackImageGenPaperIDs(42, func(int64) ([]repository.AIPinnedPaper, error) {
		return nil, errors.New("db unavailable")
	}, logger)

	if len(ids) != 0 {
		t.Fatalf("fallbackImageGenPaperIDs() = %v, want []", ids)
	}
	if got := logs.String(); !strings.Contains(got, "list pinned papers for image-gen fallback failed") || !strings.Contains(got, "conversation_id=42") {
		t.Fatalf("expected warning log for fallback failure, got %q", got)
	}
}

func TestSendMessage_ImageGenerationBypassedWhenNilGenerator(t *testing.T) {
	// When imageGen is nil, image_generation intent falls through to normal LLM.
	svc, _, caller := newServiceForTest(t)
	// svc.imageGen is nil by default.

	convID, _ := svc.CreateDraft()
	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "@image-gen draw it",
		IntentHint:     "image_generation",
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	// LLM should have been called (falls back to normal path).
	if caller.calls == 0 {
		t.Fatalf("expected LLM call when imageGen is nil")
	}
}

// TestSendMessage_ImageGenerationFallsBackToPinnedPapers verifies that when
// no paper IDs are present in the request (neither top-level nor Context), the
// dispatcher falls back to papers already pinned on the conversation.
func TestSendMessage_ImageGenerationFallsBackToPinnedPapers(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)

	// Insert paper and pin it before the @image-gen turn.
	paperID := mustInsertPaperForTest(t, libRepo, "Pinned Paper 42", "10.1/pinned42")

	var gotPaperIDs []int64
	fake := &fakeImageGen{
		fn: func(ctx context.Context, in ai_image_gen_pkg.GenerateInput) error {
			gotPaperIDs = in.PaperIDs
			return nil
		},
	}
	svc = svc.WithImageGen(fake)

	convID, err := svc.CreateDraft()
	if err != nil {
		t.Fatal(err)
	}
	// Pre-pin the paper (simulates a previous turn that pinned it).
	if err := svc.PinPaper(convID, paperID); err != nil {
		t.Fatalf("PinPaper: %v", err)
	}

	// Send @image-gen with NO top-level paper IDs and an empty Context.
	_, err = svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "@image-gen draw from pinned",
		IntentHint:     "image_generation",
		// PaperID, PaperIDs, Context are all zero/empty.
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if len(gotPaperIDs) != 1 || gotPaperIDs[0] != paperID {
		t.Fatalf("image-gen got paperIDs %v, want [%d]", gotPaperIDs, paperID)
	}
}

// TestSendMessage_ImageGenerationUsesContextPaperIDs verifies that paper IDs
// carried under in.Context.PaperIDs (the active-conversation frontend path) are
// passed through to the image-gen generator when no top-level IDs are set.
func TestSendMessage_ImageGenerationUsesContextPaperIDs(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)

	paper1 := mustInsertPaperForTest(t, libRepo, "Context Paper 1", "10.1/ctx1")
	paper2 := mustInsertPaperForTest(t, libRepo, "Context Paper 2", "10.1/ctx2")

	var gotPaperIDs []int64
	fake := &fakeImageGen{
		fn: func(ctx context.Context, in ai_image_gen_pkg.GenerateInput) error {
			gotPaperIDs = in.PaperIDs
			return nil
		},
	}
	svc = svc.WithImageGen(fake)

	convID, err := svc.CreateDraft()
	if err != nil {
		t.Fatal(err)
	}
	if pinned, err := libRepo.AIConversation.ListPinnedPapers(convID); err != nil {
		t.Fatalf("ListPinnedPapers: %v", err)
	} else if len(pinned) != 0 {
		t.Fatalf("expected no pinned papers before context-only image-gen request, got %+v", pinned)
	}

	// Send @image-gen with paper IDs only in Context (no top-level IDs).
	_, err = svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "@image-gen draw from context",
		IntentHint:     "image_generation",
		// Top-level PaperID / PaperIDs intentionally absent.
		Context: ai_assistant.RequestContext{
			PaperIDs: []int64{paper1, paper2},
		},
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if len(gotPaperIDs) != 2 {
		t.Fatalf("image-gen got paperIDs %v, want [%d %d]", gotPaperIDs, paper1, paper2)
	}
	found1, found2 := false, false
	for _, id := range gotPaperIDs {
		if id == paper1 {
			found1 = true
		}
		if id == paper2 {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Fatalf("image-gen got paperIDs %v, want both %d and %d", gotPaperIDs, paper1, paper2)
	}
}

// TestSendMessage_ImageGenerationUsesContextPaperID verifies that the singular
// Context.PaperID field (active-conversation path, single paper) is used when
// no top-level IDs or Context.PaperIDs are present.
func TestSendMessage_ImageGenerationUsesContextPaperID(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)

	paperID := mustInsertPaperForTest(t, libRepo, "Context Singular Paper", "10.1/ctxsingle")

	var gotPaperIDs []int64
	fake := &fakeImageGen{
		fn: func(ctx context.Context, in ai_image_gen_pkg.GenerateInput) error {
			gotPaperIDs = in.PaperIDs
			return nil
		},
	}
	svc = svc.WithImageGen(fake)

	convID, err := svc.CreateDraft()
	if err != nil {
		t.Fatal(err)
	}
	if pinned, err := libRepo.AIConversation.ListPinnedPapers(convID); err != nil {
		t.Fatalf("ListPinnedPapers: %v", err)
	} else if len(pinned) != 0 {
		t.Fatalf("expected no pinned papers before singular context-only image-gen request, got %+v", pinned)
	}

	_, err = svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "@image-gen draw from context single",
		IntentHint:     "image_generation",
		Context: ai_assistant.RequestContext{
			PaperID: paperID,
		},
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if len(gotPaperIDs) != 1 || gotPaperIDs[0] != paperID {
		t.Fatalf("image-gen got paperIDs %v, want [%d]", gotPaperIDs, paperID)
	}
}

// ---------------------------------------------------------------------------
// DeleteConversation — image storage cleanup
// ---------------------------------------------------------------------------

type fakeImageStorageCleaner struct {
	calls []int64
	err   error
}

func (f *fakeImageStorageCleaner) DeleteConversationFiles(id int64) error {
	f.calls = append(f.calls, id)
	return f.err
}

func TestDeleteConversation_CallsImageStorageCleaner(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)
	cleaner := &fakeImageStorageCleaner{}
	svc = svc.WithImageStorage(cleaner)

	convID, err := svc.CreateDraft()
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	if err := svc.DeleteConversation(convID); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}

	// The cleaner must have been called exactly once with the conversation ID.
	if len(cleaner.calls) != 1 || cleaner.calls[0] != convID {
		t.Fatalf("cleaner.calls = %v, want [%d]", cleaner.calls, convID)
	}

	// The conversation must be gone from the DB.
	_, dbErr := libRepo.AIConversation.GetConversation(convID)
	if dbErr == nil {
		t.Fatalf("expected GetConversation to return error after delete")
	}
}

func TestDeleteConversation_CleanerErrorDoesNotAbortDelete(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)
	cleaner := &fakeImageStorageCleaner{err: errors.New("disk full")}
	svc = svc.WithImageStorage(cleaner)

	convID, err := svc.CreateDraft()
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	// Even with cleaner error, DeleteConversation should succeed.
	if err := svc.DeleteConversation(convID); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}

	// DB row must still be gone.
	_, dbErr := libRepo.AIConversation.GetConversation(convID)
	if dbErr == nil {
		t.Fatalf("expected GetConversation to return error after delete")
	}
}

func TestGetConversation_PinnedPapersIncludeFigures(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)

	// Create a paper that has two figures.
	seq := atomic.AddInt64(&testPaperSeq, 1)
	fname := fmt.Sprintf("paper_fig_%d.pdf", seq)
	res, err := libRepo.DB().Exec(
		`INSERT INTO papers (title, doi, original_filename, stored_pdf_name) VALUES (?, ?, ?, ?)`,
		"Figure Test Paper", "10.1/figtest", fname, fname)
	if err != nil {
		t.Fatalf("insert paper: %v", err)
	}
	paperID, _ := res.LastInsertId()

	// Insert two top-level figures and one subfigure.
	for i, caption := range []string{"Figure A caption", "Figure B caption"} {
		_, err := libRepo.DB().Exec(
			`INSERT INTO paper_figures (paper_id, filename, page_number, figure_index, caption) VALUES (?, ?, ?, ?, ?)`,
			paperID, fmt.Sprintf("fig_%d.png", i+1), i+1, i+1, caption)
		if err != nil {
			t.Fatalf("insert figure %d: %v", i, err)
		}
	}
	// Subfigure (parent_figure_id set) — should be excluded.
	var parentFigID int64
	_ = libRepo.DB().QueryRow(`SELECT id FROM paper_figures WHERE paper_id = ? LIMIT 1`, paperID).Scan(&parentFigID)
	if parentFigID > 0 {
		_, _ = libRepo.DB().Exec(
			`INSERT INTO paper_figures (paper_id, filename, page_number, figure_index, caption, parent_figure_id) VALUES (?, ?, ?, ?, ?, ?)`,
			paperID, "subfig.png", 1, 1, "subfig caption", parentFigID)
	}

	// Create a conversation and pin the paper.
	convID, err := svc.CreateDraft()
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if err := libRepo.AIConversation.PinPaper(convID, paperID); err != nil {
		t.Fatalf("PinPaper: %v", err)
	}

	conv, err := svc.GetConversation(convID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if len(conv.PinnedPapers) != 1 {
		t.Fatalf("expected 1 pinned paper, got %d", len(conv.PinnedPapers))
	}
	pp := conv.PinnedPapers[0]
	// Only 2 top-level figures (subfigure excluded).
	if len(pp.Figures) != 2 {
		t.Fatalf("expected 2 figures, got %d: %+v", len(pp.Figures), pp.Figures)
	}
	if pp.Figures[0].Caption != "Figure A caption" {
		t.Errorf("unexpected first figure caption: %q", pp.Figures[0].Caption)
	}
	if pp.Figures[1].Caption != "Figure B caption" {
		t.Errorf("unexpected second figure caption: %q", pp.Figures[1].Caption)
	}
}

// --- User-attached context (PDF excerpts + checked figures) -----------------

type stubFigureContextLoader struct {
	calls     int32
	idsSeen   []int64
	images    []model.AIImageInput
	summaries []string
	err       error
}

func (s *stubFigureContextLoader) LoadFigureContext(_ context.Context, ids []int64) ([]model.AIImageInput, []string, error) {
	atomic.AddInt32(&s.calls, 1)
	s.idsSeen = append([]int64(nil), ids...)
	return s.images, s.summaries, s.err
}

func TestSendMessageAttachesCheckedFiguresAsVisionInput(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	loader := &stubFigureContextLoader{
		images: []model.AIImageInput{
			{MIMEType: "image/jpeg", Data: "aW1nMQ=="},
			{MIMEType: "image/jpeg", Data: "aW1nMg=="},
		},
		summaries: []string{
			"- figure_id=11；标签=第 3 页图 1；caption= volcano plot",
			"- figure_id=12；标签=第 4 页图 2；caption= heatmap",
		},
	}
	svc.WithFigureContextLoader(loader)

	convID, _ := svc.CreateDraft()
	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "这两张图分别说明什么",
		Context:        ai_assistant.RequestContext{FigureIDs: []int64{11, 12}},
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if loader.calls != 1 || len(loader.idsSeen) != 2 || loader.idsSeen[0] != 11 || loader.idsSeen[1] != 12 {
		t.Fatalf("loader call = %d ids=%v", loader.calls, loader.idsSeen)
	}
	if len(caller.imagesSeen) != 2 || caller.imagesSeen[0].Data != "aW1nMQ==" {
		t.Fatalf("images seen = %+v", caller.imagesSeen)
	}
	if !strings.Contains(caller.userSeen, "本轮随附图片（共 2 张") || !strings.Contains(caller.userSeen, "figure_id=11") {
		t.Fatalf("prompt missing figure block: %s", caller.userSeen)
	}
	if strings.Index(caller.userSeen, "本轮随附图片") > strings.Index(caller.userSeen, "用户问题：") {
		t.Fatalf("figure block must precede the user question: %s", caller.userSeen)
	}

	msgs, _ := libRepo.AIConversation.ListMessages(convID, 0, 100)
	if len(msgs) != 2 || msgs[1].IncludedFigures != 2 {
		t.Fatalf("assistant IncludedFigures = %+v", msgs)
	}
}

func TestSendMessageCheckedFiguresTextOnlyWhenModelLacksImages(t *testing.T) {
	svc, _, caller := newServiceForTest(t)
	unsupported := false
	settings := model.DefaultAISettings()
	settings.Models = []model.AIModelConfig{
		{ID: "master", Name: "Master", Provider: model.AIProviderOpenAI, APIKey: "k", Model: "text-only", SupportsImages: &unsupported},
	}
	settings.SceneModels = model.AISceneModelSelection{AssistantMasterModelID: "master"}
	svc.settings = &stubSettingsProvider{settings: settings}
	loader := &stubFigureContextLoader{
		images:    []model.AIImageInput{{MIMEType: "image/jpeg", Data: "aW1nMQ=="}},
		summaries: []string{"- figure_id=11；标签=第 3 页图 1；caption= volcano plot"},
	}
	svc.WithFigureContextLoader(loader)

	convID, _ := svc.CreateDraft()
	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "解读这张图",
		Context:        ai_assistant.RequestContext{FigureIDs: []int64{11}},
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if len(caller.imagesSeen) != 0 {
		t.Fatalf("images should not reach a text-only model: %+v", caller.imagesSeen)
	}
	if !strings.Contains(caller.userSeen, "当前模型不支持图片输入") || !strings.Contains(caller.userSeen, "figure_id=11") {
		t.Fatalf("prompt missing text-only fallback: %s", caller.userSeen)
	}
}

func TestSendMessageFigureLoaderFailureDegradesGracefully(t *testing.T) {
	svc, _, caller := newServiceForTest(t)
	svc.WithFigureContextLoader(&stubFigureContextLoader{err: errors.New("disk gone")})

	convID, _ := svc.CreateDraft()
	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "看图说话",
		Context:        ai_assistant.RequestContext{FigureIDs: []int64{11}},
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage should not fail on loader error: %v", err)
	}
	if len(caller.imagesSeen) != 0 || strings.Contains(caller.userSeen, "随附图片") {
		t.Fatalf("loader failure must not inject anything: %+v / %s", caller.imagesSeen, caller.userSeen)
	}
}

func TestSendMessageExcerptsInjectedBeforeQuestion(t *testing.T) {
	svc, _, caller := newServiceForTest(t)

	excerpts := []ai_assistant.ContextExcerpt{
		{PaperID: 1, Page: 3, Text: "  糖酵解通路在缺氧条件下被显著激活  "},
		{Text: "no-page excerpt"},
		{Text: "   "}, // blank entries are skipped
	}
	for i := 0; i < maxExcerptsPerTurn; i++ {
		excerpts = append(excerpts, ai_assistant.ContextExcerpt{Text: fmt.Sprintf("filler %d", i)})
	}

	convID, _ := svc.CreateDraft()
	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "结合引用回答",
		Context:        ai_assistant.RequestContext{Excerpts: excerpts},
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if !strings.Contains(caller.userSeen, "用户引用的原文片段") {
		t.Fatalf("prompt missing excerpt block: %s", caller.userSeen)
	}
	if !strings.Contains(caller.userSeen, "[1] (第 3 页) \"糖酵解通路在缺氧条件下被显著激活\"") {
		t.Fatalf("excerpt[1] formatting wrong: %s", caller.userSeen)
	}
	if !strings.Contains(caller.userSeen, "[2] \"no-page excerpt\"") {
		t.Fatalf("excerpt[2] formatting wrong: %s", caller.userSeen)
	}
	// 2 real + 8 fillers = 10 candidates, but the cap allows only 8 lines.
	if strings.Contains(caller.userSeen, "[9]") {
		t.Fatalf("excerpt cap %d exceeded: %s", maxExcerptsPerTurn, caller.userSeen)
	}
	if strings.Index(caller.userSeen, "用户引用的原文片段") > strings.Index(caller.userSeen, "用户问题：") {
		t.Fatalf("excerpt block must precede the user question: %s", caller.userSeen)
	}
}

func TestRequestContextEmptyWithExcerpts(t *testing.T) {
	if !requestContextEmpty(ai_assistant.RequestContext{}) {
		t.Fatal("zero context should be empty")
	}
	ctx := ai_assistant.RequestContext{Excerpts: []ai_assistant.ContextExcerpt{{Text: "x"}}}
	if requestContextEmpty(ctx) {
		t.Fatal("context with only excerpts must not be empty")
	}
}
