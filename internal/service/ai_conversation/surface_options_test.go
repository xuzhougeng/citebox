package ai_conversation

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/repository"
)

// TestSendForSurfaceWeChatCapsHistoryToFiveTurns verifies that when surface ==
// "wechat" the assembled prompt contains the most-recent 10 rows (5
// user/assistant pairs) and excludes anything older. This is the load-bearing
// invariant for the WeChat conversation model.
func TestSendForSurfaceWeChatCapsHistoryToFiveTurns(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)

	// Create the WeChat main conversation directly via the kind helper so the
	// path mirrors what agent_session.Service.resolveConversation does.
	cid, err := libRepo.AIConversation.FindOrCreateByKind("main_wechat", "wechat")
	if err != nil {
		t.Fatalf("FindOrCreateByKind: %v", err)
	}

	// Insert 12 prior turns (24 rows). Tag each row with a unique marker so
	// we can grep the assembled prompt afterwards.
	for i := 0; i < 12; i++ {
		if _, err := libRepo.AIConversation.AddMessage(cid, "user", fmt.Sprintf("Q%02d", i), repository.AIMessageMeta{}); err != nil {
			t.Fatalf("AddMessage user: %v", err)
		}
		if _, err := libRepo.AIConversation.AddMessage(cid, "assistant", fmt.Sprintf("A%02d", i), repository.AIMessageMeta{}); err != nil {
			t.Fatalf("AddMessage assistant: %v", err)
		}
	}

	if _, err := svc.SendForSurface(context.Background(), SurfaceMessageInput{
		ConversationID: cid,
		Text:           "now",
		Surface:        "wechat",
	}, nil); err != nil {
		t.Fatalf("SendForSurface: %v", err)
	}

	if caller.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", caller.calls)
	}

	prompt := caller.userSeen
	// The new user message ("now") was inserted before the prompt was
	// assembled but is dropped from history (assembled into the trailing
	// "用户问题：" block instead). The 5-turn window should therefore include
	// the 5 most-recent prior pairs: Q07/A07 ... Q11/A11.
	for i := 7; i <= 11; i++ {
		if !strings.Contains(prompt, fmt.Sprintf("Q%02d", i)) {
			t.Errorf("prompt missing recent Q%02d: %q", i, prompt)
		}
		if !strings.Contains(prompt, fmt.Sprintf("A%02d", i)) {
			t.Errorf("prompt missing recent A%02d: %q", i, prompt)
		}
	}
	// Anything older than turn index 7 (which is now beyond the 5-pair / 10-row
	// window) must NOT be in the prompt.
	for i := 0; i <= 6; i++ {
		if strings.Contains(prompt, fmt.Sprintf("Q%02d", i)) {
			t.Errorf("prompt unexpectedly contains old Q%02d: %q", i, prompt)
		}
	}
	if !strings.Contains(prompt, "用户问题：\nnow") {
		t.Errorf("prompt missing trailing user question: %q", prompt)
	}
}

// TestSendForSurfaceWeChatRespectsClearBarrier verifies that after /clear
// (modeled by SetClearBarrier) the assembled prompt sees zero history.
func TestSendForSurfaceWeChatRespectsClearBarrier(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	cid, _ := libRepo.AIConversation.FindOrCreateByKind("main_wechat", "wechat")

	for i := 0; i < 4; i++ {
		_, _ = libRepo.AIConversation.AddMessage(cid, "user", fmt.Sprintf("OLD%d", i), repository.AIMessageMeta{})
	}
	// Set the barrier just past the most recent prior message.
	rows, _ := libRepo.AIConversation.ListMessagesAfterBarrier(cid, 100)
	maxID := rows[len(rows)-1].ID
	if err := libRepo.AIConversation.SetClearBarrier(cid, maxID); err != nil {
		t.Fatalf("SetClearBarrier: %v", err)
	}

	if _, err := svc.SendForSurface(context.Background(), SurfaceMessageInput{
		ConversationID: cid,
		Text:           "fresh",
		Surface:        "wechat",
	}, nil); err != nil {
		t.Fatalf("SendForSurface: %v", err)
	}

	prompt := caller.userSeen
	if strings.Contains(prompt, "近期对话") {
		t.Errorf("prompt should have no history block after /clear, got: %q", prompt)
	}
	if strings.Contains(prompt, "OLD") {
		t.Errorf("prompt unexpectedly contains pre-barrier content: %q", prompt)
	}
	if !strings.Contains(prompt, "用户问题：\nfresh") {
		t.Errorf("prompt missing fresh user question: %q", prompt)
	}
}

// TestSendForSurfaceFiresPlaceholder confirms onPlaceholder is invoked once
// before the LLM call.
func TestSendForSurfaceFiresPlaceholder(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)
	cid, _ := libRepo.AIConversation.FindOrCreateByKind("main_wechat", "wechat")

	var ph string
	res, err := svc.SendForSurface(context.Background(), SurfaceMessageInput{
		ConversationID: cid,
		Text:           "hi",
		Surface:        "wechat",
	}, func(p string) error { ph = p; return nil })
	if err != nil {
		t.Fatalf("SendForSurface: %v", err)
	}
	if ph == "" {
		t.Error("onPlaceholder was never invoked")
	}
	if res.PlaceholderText != ph {
		t.Errorf("PlaceholderText=%q want %q", res.PlaceholderText, ph)
	}
	if res.UserMessageID == 0 || res.AssistantMessageID == 0 {
		t.Errorf("missing message ids: %+v", res)
	}
}

// TestSendForSurfaceNonWechatUsesLegacyHistoryPath: when surface != wechat
// the assembled prompt should pull from ListMessages (above
// summary_through_message_id) rather than from ListMessagesAfterBarrier.
// Setting a clear barrier should NOT hide the messages.
func TestSendForSurfaceNonWechatIgnoresClearBarrier(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	cid, _ := libRepo.AIConversation.FindOrCreateByKind("default_web", "web")

	_, _ = libRepo.AIConversation.AddMessage(cid, "user", "WEB-OLD", repository.AIMessageMeta{})
	rows, _ := libRepo.AIConversation.ListMessages(cid, 0, 100)
	if err := libRepo.AIConversation.SetClearBarrier(cid, rows[len(rows)-1].ID); err != nil {
		t.Fatalf("SetClearBarrier: %v", err)
	}

	if _, err := svc.SendForSurface(context.Background(), SurfaceMessageInput{
		ConversationID: cid,
		Text:           "again",
		Surface:        "web",
	}, nil); err != nil {
		t.Fatalf("SendForSurface: %v", err)
	}

	if !strings.Contains(caller.userSeen, "WEB-OLD") {
		t.Errorf("non-wechat surface should ignore clear barrier; prompt=%q", caller.userSeen)
	}
}
