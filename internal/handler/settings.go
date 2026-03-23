package handler

import (
	"encoding/json"
	"net/http"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

type SettingsHandler struct {
	libraryService *service.LibraryService
	versionService *service.VersionService
}

func NewSettingsHandler(libraryService *service.LibraryService, versionService *service.VersionService) *SettingsHandler {
	return &SettingsHandler{
		libraryService: libraryService,
		versionService: versionService,
	}
}

func (h *SettingsHandler) GetExtractorSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.libraryService.GetExtractorSettings()
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, settings)
}

func (h *SettingsHandler) UpdateExtractorSettings(w http.ResponseWriter, r *http.Request) {
	var req model.ExtractorSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	settings, err := h.libraryService.UpdateExtractorSettings(req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"settings": settings,
	})
}

func (h *SettingsHandler) GetWeixinBridgeSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.libraryService.GetWeixinBridgeSettings()
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, settings)
}

func (h *SettingsHandler) UpdateWeixinBridgeSettings(w http.ResponseWriter, r *http.Request) {
	var req model.WeixinBridgeSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	settings, err := h.libraryService.UpdateWeixinBridgeSettings(req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"settings": settings,
	})
}

func (h *SettingsHandler) GetVersionStatus(w http.ResponseWriter, r *http.Request) {
	refresh := false
	switch r.URL.Query().Get("refresh") {
	case "1", "true", "TRUE", "True", "yes", "YES", "Yes":
		refresh = true
	}

	status := h.versionService.GetStatus(r.Context(), refresh)
	sendJSON(w, http.StatusOK, status)
}
