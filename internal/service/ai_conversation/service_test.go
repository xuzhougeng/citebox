package ai_conversation

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
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
		caller, nil)
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
