package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
	"github.com/xuzhougeng/citebox/internal/service/ai_conversation"
)

type stubAIConversationService struct {
	listResult []ai_conversation.Conversation
	getResult  ai_conversation.Conversation
	getErr     error
	deleted    int64
	truncated  int64
	strictID   int64
	strictOn   bool
	sentInput  ai_conversation.SendMessageInput
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
func (s *stubAIConversationService) UpdateStrictEvidence(id int64, on bool) error {
	s.strictID = id
	s.strictOn = on
	return nil
}
func (s *stubAIConversationService) DeleteConversation(id int64) error { s.deleted = id; return nil }
func (s *stubAIConversationService) TruncateLastTurn(id int64) error {
	s.truncated = id
	return nil
}
func (s *stubAIConversationService) PinPaper(c, p int64) error   { return nil }
func (s *stubAIConversationService) UnpinPaper(c, p int64) error { return nil }
func (s *stubAIConversationService) SendMessage(ctx context.Context, in ai_conversation.SendMessageInput, onDelta func(string) error) (ai_conversation.SendMessageResult, error) {
	s.sentInput = in
	if in.OnEvent != nil {
		_ = in.OnEvent(ai_conversation.StreamEvent{Type: "process", Data: map[string]string{"status": "ok"}})
	}
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

func TestAIConversationNewMessageAppliesStrictEvidenceBeforeSend(t *testing.T) {
	stub := &stubAIConversationService{}
	h := NewAIConversationHandler(stub)
	body := strings.NewReader(`{"content":"hi","strict_evidence":true,"include_external_evidence":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/conversations/new/messages", body)
	rec := httptest.NewRecorder()
	h.PostMessage(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if stub.strictID != 99 || !stub.strictOn {
		t.Fatalf("strict update = (%d, %v), want (99, true)", stub.strictID, stub.strictOn)
	}
	if stub.sentInput.ConversationID != 99 || !stub.sentInput.IncludeExternalEvidence {
		t.Fatalf("sent input = %+v", stub.sentInput)
	}
}

func TestAIConversationPostMessageAcceptsIntentHintAndContext(t *testing.T) {
	stub := &stubAIConversationService{}
	h := NewAIConversationHandler(stub)
	body := strings.NewReader(`{"content":"看图 1","intent_hint":"figure_lookup","context":{"source":"paper","paper_id":7,"figure_id":3}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/conversations/new/messages", body)
	rr := httptest.NewRecorder()

	h.PostMessage(rr, req)

	if stub.sentInput.IntentHint != "figure_lookup" {
		t.Fatalf("IntentHint = %q", stub.sentInput.IntentHint)
	}
	if stub.sentInput.Context.PaperID != 7 || stub.sentInput.Context.FigureID != 3 || stub.sentInput.Context.Source != "paper" {
		t.Fatalf("Context = %+v", stub.sentInput.Context)
	}
}

func TestAIConversationPostMessageCanReplaceLastTurn(t *testing.T) {
	stub := &stubAIConversationService{}
	h := NewAIConversationHandler(stub)
	body := strings.NewReader(`{"content":"updated question","replace_last":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/conversations/7/messages", body)
	rr := httptest.NewRecorder()

	h.PostMessage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	if stub.truncated != 7 {
		t.Fatalf("truncated = %d, want 7", stub.truncated)
	}
	if stub.sentInput.Content != "updated question" {
		t.Fatalf("sent content = %q", stub.sentInput.Content)
	}
}

func TestAIConversationPostMessageDecodesSources(t *testing.T) {
	stub := &stubAIConversationService{}
	h := NewAIConversationHandler(stub)
	body := strings.NewReader(`{"content":"找综述","intent_hint":"external_search","sources":["pubmed","semantic_scholar"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/conversations/new/messages", body)
	rec := httptest.NewRecorder()
	h.PostMessage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if stub.sentInput.IntentHint != "external_search" {
		t.Fatalf("IntentHint = %q, want external_search", stub.sentInput.IntentHint)
	}
	want := []string{"pubmed", "semantic_scholar"}
	if !reflect.DeepEqual(stub.sentInput.Sources, want) {
		t.Fatalf("Sources = %+v, want %+v", stub.sentInput.Sources, want)
	}
}

func TestAIConversationPostMessageDecodesSearchGoalHint(t *testing.T) {
	stub := &stubAIConversationService{}
	h := NewAIConversationHandler(stub)
	body := strings.NewReader(`{"content":"找支持这个说法的证据","intent_hint":"external_search","search_goal_hint":"evidence"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/conversations/new/messages", body)
	rec := httptest.NewRecorder()

	h.PostMessage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got, want := stub.sentInput.SearchGoalHint, ai_assistant.ExternalSearchGoalEvidence; got != want {
		t.Fatalf("SearchGoalHint = %q, want %q", got, want)
	}
}

func TestAIConversationPostMessageDecodesPaperIDs(t *testing.T) {
	stub := &stubAIConversationService{}
	h := NewAIConversationHandler(stub)
	body := strings.NewReader(`{"content":"对比这两篇","paper_id":11,"paper_ids":[11,10]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/conversations/new/messages", body)
	rec := httptest.NewRecorder()
	h.PostMessage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if stub.sentInput.PaperID != 11 {
		t.Fatalf("PaperID = %d, want 11", stub.sentInput.PaperID)
	}
	want := []int64{11, 10}
	if !reflect.DeepEqual(stub.sentInput.PaperIDs, want) {
		t.Fatalf("PaperIDs = %+v, want %+v", stub.sentInput.PaperIDs, want)
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
	if !strings.Contains(rec.Body.String(), "\"type\":\"process\"") {
		t.Fatalf("expected process event, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"type\":\"final\"") {
		t.Fatalf("expected final event, got %s", rec.Body.String())
	}
}
