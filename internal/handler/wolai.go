package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/service"
)

type WolaiHandler struct {
	libraryService *service.LibraryService
}

func NewWolaiHandler(libraryService *service.LibraryService) *WolaiHandler {
	return &WolaiHandler{libraryService: libraryService}
}

func (h *WolaiHandler) SavePaperNote(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/wolai/papers/", "/notes")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
		return
	}

	var req struct {
		NotesText string `json:"notes_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	result, err := h.libraryService.SavePaperNoteToWolai(id, req.NotesText)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
}

func (h *WolaiHandler) SaveFigureNote(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/wolai/figures/", "/notes")
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

	result, err := h.libraryService.SaveFigureNoteToWolai(id, req.NotesText)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
}
