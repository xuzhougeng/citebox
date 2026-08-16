package handler

import (
	"encoding/json"
	"net/http"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

type FigureLibraryHandler struct {
	service *service.LibraryService
}

func NewFigureLibraryHandler(svc *service.LibraryService) *FigureLibraryHandler {
	return &FigureLibraryHandler{service: svc}
}

func (h *FigureLibraryHandler) Settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := h.service.GetFigureLibrarySettings()
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var req model.FigureLibrarySettings
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
			return
		}
		settings, err := h.service.UpdateFigureLibrarySettings(req)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, settings)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *FigureLibraryHandler) Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := h.service.GetFigureLibraryStatus()
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, status)
}

func (h *FigureHandler) SendToFigureLibrary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := parseIDWithSuffix(r.URL.Path, "/api/figures/", "/send-to-figure-library")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "figure id 无效"))
		return
	}
	result, err := h.service.SendFigureToFigureLibrary(id)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, result)
}
