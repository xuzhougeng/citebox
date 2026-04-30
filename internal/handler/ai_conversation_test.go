package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/service/ai_conversation"
)

type stubAIConversationService struct {
	listResult []ai_conversation.Conversation
	getResult  ai_conversation.Conversation
	getErr     error
	deleted    int64
}

func (s *stubAIConversationService) ListConversations(q string, limit, offset int) ([]ai_conversation.Conversation, error) {
	return s.listResult, nil
}
func (s *stubAIConversationService) GetConversation(id int64) (ai_conversation.Conversation, error) {
	return s.getResult, s.getErr
}
func (s *stubAIConversationService) ListMessages(id, after int64, limit int) ([]ai_conversation.Message, error) {
	return nil, nil
}
func (s *stubAIConversationService) CreateDraft() (int64, error) { return 99, nil }
func (s *stubAIConversationService) UpdateTitle(id int64, title string, lock bool) error {
	return nil
}
func (s *stubAIConversationService) UpdateStrictEvidence(id int64, on bool) error { return nil }
func (s *stubAIConversationService) DeleteConversation(id int64) error            { s.deleted = id; return nil }
func (s *stubAIConversationService) PinPaper(c, p int64) error                    { return nil }
func (s *stubAIConversationService) UnpinPaper(c, p int64) error                  { return nil }
func (s *stubAIConversationService) SendMessage(ctx context.Context, in ai_conversation.SendMessageInput, onDelta func(string) error) (ai_conversation.SendMessageResult, error) {
	_ = onDelta("hi")
	return ai_conversation.SendMessageResult{
		ConversationID:   in.ConversationID,
		UserMessage:      ai_conversation.Message{ID: 1, Role: "user", Content: in.Content},
		AssistantMessage: ai_conversation.Message{ID: 2, Role: "assistant", Content: "hi"},
	}, nil
}
func (s *stubAIConversationService) ExportMarkdown(id int64) (string, string, error) {
	return "# md\n", "x.md", nil
}

func TestAIConversationListEndpoint(t *testing.T) {
	stub := &stubAIConversationService{
		listResult: []ai_conversation.Conversation{{ID: 1, Title: "Test"}},
	}
	h := NewAIConversationHandler(stub)
	req := httptest.NewRequest(http.MethodGet, "/api/ai/conversations", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Items []ai_conversation.Conversation `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Title != "Test" {
		t.Fatalf("body = %+v", body)
	}
}

func TestAIConversationDeleteEndpoint(t *testing.T) {
	stub := &stubAIConversationService{}
	h := NewAIConversationHandler(stub)
	req := httptest.NewRequest(http.MethodDelete, "/api/ai/conversations/42", nil)
	rec := httptest.NewRecorder()
	h.Detail(rec, req)
	if rec.Code != 204 {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if stub.deleted != 42 {
		t.Fatalf("deleted id = %d", stub.deleted)
	}
}

func TestAIConversationSendMessageStreams(t *testing.T) {
	stub := &stubAIConversationService{}
	h := NewAIConversationHandler(stub)
	body := strings.NewReader(`{"content":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/conversations/7/messages", body)
	rec := httptest.NewRecorder()
	h.PostMessage(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"delta\":\"hi\"") {
		t.Fatalf("expected NDJSON delta, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"type\":\"final\"") {
		t.Fatalf("expected final event, got %s", rec.Body.String())
	}
}
