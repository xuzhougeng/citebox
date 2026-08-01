package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/service"
)

type NotionHandler struct {
	library *service.LibraryService
	notion  *service.NotionAPIService
}

func NewNotionHandler(library *service.LibraryService, notion *service.NotionAPIService) *NotionHandler {
	return &NotionHandler{library: library, notion: notion}
}

func (h *NotionHandler) Settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := h.notion.GetSettings()
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
			return
		}
		status, err := h.notion.SaveToken(r.Context(), req.Token)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, status)
	case http.MethodDelete:
		if err := h.notion.DeleteToken(); err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"success": true})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *NotionHandler) TestToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	status, err := h.notion.TestToken(r.Context(), req.Token)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, status)
}

func (h *NotionHandler) SaveFigureNote(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/notion/figures/", "/notes")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "figure id 无效"))
		return
	}
	var req struct {
		NotesText string `json:"notes_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	figure, err := h.library.GetFigure(id)
	if err != nil {
		sendError(w, err)
		return
	}
	if figure == nil {
		sendError(w, apperr.New(apperr.CodeNotFound, "figure not found"))
		return
	}
	notesText := strings.TrimSpace(req.NotesText)
	if notesText == "" {
		notesText = strings.TrimSpace(figure.NotesText)
	}
	imageData, mimeType, _, err := h.library.GetFigureImage(id)
	if err != nil {
		sendError(w, err)
		return
	}
	result, err := h.notion.SaveFigureNoteToNotion(r.Context(), figure, imageData, mimeType, notesText)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, result)
}
