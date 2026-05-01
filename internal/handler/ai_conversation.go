package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
	"github.com/xuzhougeng/citebox/internal/service/ai_conversation"
)

// AIConversationService is the surface AIConversationHandler depends on.
type AIConversationService interface {
	ListConversations(q string, limit, offset int) ([]ai_conversation.Conversation, error)
	GetConversation(id int64) (ai_conversation.Conversation, error)
	ListMessages(id int64, afterID int64, limit int) ([]ai_conversation.Message, error)
	CreateDraft() (int64, error)
	UpdateTitle(id int64, title string, lock bool) error
	UpdateStrictEvidence(id int64, on bool) error
	DeleteConversation(id int64) error
	TruncateLastTurn(id int64) error
	PinPaper(conversationID, paperID int64) error
	UnpinPaper(conversationID, paperID int64) error
	SendMessage(ctx context.Context, in ai_conversation.SendMessageInput, onDelta func(string) error) (ai_conversation.SendMessageResult, error)
	ExportMarkdown(id int64) (string, string, error)
}

// AIConversationHandler implements /api/ai/conversations/* routes.
type AIConversationHandler struct {
	svc AIConversationService
}

// NewAIConversationHandler returns the handler.
func NewAIConversationHandler(svc AIConversationService) *AIConversationHandler {
	return &AIConversationHandler{svc: svc}
}

// List → GET /api/ai/conversations?q=&limit=&offset=
func (h *AIConversationHandler) List(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, err := h.svc.ListConversations(q, limit, offset)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

// Detail → GET/PATCH/DELETE /api/ai/conversations/:id
func (h *AIConversationHandler) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseConversationID(r.URL.Path)
	if err != nil {
		sendError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		conv, err := h.svc.GetConversation(id)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, conv)
	case http.MethodPatch:
		var body struct {
			Title          *string `json:"title,omitempty"`
			StrictEvidence *bool   `json:"strict_evidence,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
			return
		}
		if body.Title != nil {
			if err := h.svc.UpdateTitle(id, *body.Title, true); err != nil {
				sendError(w, err)
				return
			}
		}
		if body.StrictEvidence != nil {
			if err := h.svc.UpdateStrictEvidence(id, *body.StrictEvidence); err != nil {
				sendError(w, err)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := h.svc.DeleteConversation(id); err != nil {
			sendError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Messages → GET /api/ai/conversations/:id/messages?after=&limit=
func (h *AIConversationHandler) Messages(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseConversationID(strings.TrimSuffix(r.URL.Path, "/messages"))
	if err != nil {
		sendError(w, err)
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.svc.ListMessages(id, after, limit)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

// PostMessage → POST /api/ai/conversations/:id/messages with NDJSON streaming reply.
func (h *AIConversationHandler) PostMessage(w http.ResponseWriter, r *http.Request) {
	idPart := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/ai/conversations/"), "/messages")
	var conversationID int64
	if idPart == "new" {
		newID, err := h.svc.CreateDraft()
		if err != nil {
			sendError(w, err)
			return
		}
		conversationID = newID
	} else {
		n, err := strconv.ParseInt(idPart, 10, 64)
		if err != nil {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "conversation id 无效"))
			return
		}
		conversationID = n
	}

	var body struct {
		Content                 string                      `json:"content"`
		PaperID                 int64                       `json:"paper_id,omitempty"`
		StrictEvidence          *bool                       `json:"strict_evidence,omitempty"`
		IncludeExternalEvidence bool                        `json:"include_external_evidence,omitempty"`
		IntentHint              string                      `json:"intent_hint,omitempty"`
		Sources                 []string                    `json:"sources,omitempty"`
		ReplaceLast             bool                        `json:"replace_last,omitempty"`
		Context                 ai_assistant.RequestContext `json:"context,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	if body.StrictEvidence != nil {
		if err := h.svc.UpdateStrictEvidence(conversationID, *body.StrictEvidence); err != nil {
			sendError(w, err)
			return
		}
	}
	if body.ReplaceLast {
		if idPart == "new" {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "新会话不能重新发送上一轮"))
			return
		}
		if err := h.svc.TruncateLastTurn(conversationID); err != nil {
			sendError(w, err)
			return
		}
	}

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	controller := http.NewResponseController(w)
	send := func(payload map[string]interface{}) error {
		if err := encoder.Encode(payload); err != nil {
			return err
		}
		return controller.Flush()
	}

	// emit conversation id immediately so the client can update the URL
	if err := send(map[string]interface{}{"type": "meta", "conversation_id": conversationID}); err != nil {
		return
	}

	res, err := h.svc.SendMessage(r.Context(), ai_conversation.SendMessageInput{
		ConversationID:          conversationID,
		Content:                 body.Content,
		PaperID:                 body.PaperID,
		IncludeExternalEvidence: body.IncludeExternalEvidence,
		IntentHint:              body.IntentHint,
		Sources:                 body.Sources,
		Context:                 body.Context,
		OnEvent: func(event ai_conversation.StreamEvent) error {
			return send(map[string]interface{}{"type": event.Type, "data": event.Data})
		},
	}, func(delta string) error {
		return send(map[string]interface{}{"type": "delta", "delta": delta})
	})
	if err != nil {
		_ = send(map[string]interface{}{
			"type":  "error",
			"error": apperr.Message(err),
			"code":  string(apperr.CodeOf(err)),
		})
		return
	}
	_ = send(map[string]interface{}{
		"type":              "final",
		"user_message":      res.UserMessage,
		"assistant_message": res.AssistantMessage,
	})
}

// Pins → POST/DELETE /api/ai/conversations/:id/papers[/:pid]
func (h *AIConversationHandler) Pins(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/ai/conversations/"), "/")
	if len(parts) < 2 || parts[1] != "papers" {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "路径错误"))
		return
	}
	convID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "conversation id 无效"))
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			PaperID int64 `json:"paper_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
			return
		}
		if err := h.svc.PinPaper(convID, body.PaperID); err != nil {
			sendError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if len(parts) < 3 {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 缺失"))
			return
		}
		paperID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
			return
		}
		if err := h.svc.UnpinPaper(convID, paperID); err != nil {
			sendError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Export → GET /api/ai/conversations/:id/export
func (h *AIConversationHandler) Export(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseConversationID(strings.TrimSuffix(r.URL.Path, "/export"))
	if err != nil {
		sendError(w, err)
		return
	}
	md, filename, err := h.svc.ExportMarkdown(id)
	if err != nil {
		sendError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, _ = w.Write([]byte(md))
}

func (h *AIConversationHandler) parseConversationID(path string) (int64, error) {
	rest := strings.TrimPrefix(path, "/api/ai/conversations/")
	if idx := strings.Index(rest, "/"); idx >= 0 {
		rest = rest[:idx]
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.New(apperr.CodeInvalidArgument, "conversation id 无效")
	}
	return id, nil
}

// keep errors import alive for future use
var _ = errors.Is
