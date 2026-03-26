package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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

func (h *SettingsHandler) GetWolaiSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.libraryService.GetWolaiSettings()
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, settings)
}

func (h *SettingsHandler) UpdateWolaiSettings(w http.ResponseWriter, r *http.Request) {
	var req model.WolaiSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	settings, err := h.libraryService.UpdateWolaiSettings(req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"settings": settings,
	})
}

func (h *SettingsHandler) GetDesktopCloseSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.libraryService.GetDesktopCloseSettings()
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, settings)
}

func (h *SettingsHandler) UpdateDesktopCloseSettings(w http.ResponseWriter, r *http.Request) {
	var req model.DesktopCloseSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	settings, err := h.libraryService.UpdateDesktopCloseSettings(req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"settings": settings,
	})
}

func (h *SettingsHandler) TestWolaiSettings(w http.ResponseWriter, r *http.Request) {
	var req model.WolaiSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	result, err := h.libraryService.TestWolaiSettings(req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
}

func (h *SettingsHandler) InsertWolaiTestPage(w http.ResponseWriter, r *http.Request) {
	var req model.WolaiSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	result, err := h.libraryService.InsertWolaiTestPage(req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
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

func (h *SettingsHandler) GetTTSSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.libraryService.GetTTSSettings()
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, settings)
}

func (h *SettingsHandler) UpdateTTSSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AppID                    string `json:"app_id"`
		AccessKey                string `json:"access_key"`
		ResourceID               string `json:"resource_id"`
		Speaker                  string `json:"speaker"`
		WeixinVoiceOutputEnabled *bool  `json:"weixin_voice_output_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	input := model.TTSSettings{
		AppID:                    req.AppID,
		AccessKey:                req.AccessKey,
		ResourceID:               req.ResourceID,
		Speaker:                  req.Speaker,
		WeixinVoiceOutputEnabled: model.DefaultWeixinVoiceOutputEnabled,
	}
	if req.WeixinVoiceOutputEnabled != nil {
		input.WeixinVoiceOutputEnabled = *req.WeixinVoiceOutputEnabled
		input.WeixinVoiceOutputEnabledSet = true
	}

	settings, err := h.libraryService.UpdateTTSSettings(input)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"settings": settings,
	})
}

func (h *SettingsHandler) TestTTS(w http.ResponseWriter, r *http.Request) {
	var req model.TTSSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	audio, filename, contentType, err := h.libraryService.TestTTS(r.Context(), req)
	if err != nil {
		sendError(w, err)
		return
	}

	if strings.TrimSpace(contentType) == "" {
		contentType = "audio/mpeg"
	}
	if strings.TrimSpace(filename) == "" {
		filename = "tts-test.mp3"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(audio)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
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
