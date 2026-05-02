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
	libraryService  *service.LibraryService
	versionService  *service.VersionService
	researchClient  researchSettingsClient
	researchRuntime ResearchRuntimeSettings
	pubmedClient    pubmedSettingsClient
	pubmedRuntime   PubMedRuntimeSettings
}

type researchSettingsClient interface {
	SetAPIKey(key string)
}

type pubmedSettingsClient interface {
	SetSettings(apiKey, email, tool string)
}

type ResearchRuntimeSettings struct {
	APIKey    string
	APIKeySet bool
}

type PubMedRuntimeSettings struct {
	APIKey    string
	APIKeySet bool
	Email     string
	EmailSet  bool
	Tool      string
	ToolSet   bool
}

type researchSettingsPayload struct {
	S2APIKey     string    `json:"s2_api_key"`
	PubMedAPIKey *string   `json:"pubmed_api_key,omitempty"`
	PubMedEmail  *string   `json:"pubmed_email,omitempty"`
	PubMedTool   *string   `json:"pubmed_tool,omitempty"`
	Sources      *[]string `json:"sources,omitempty"`
}

type aiExternalSearchSettingsPayload struct {
	Sources      *[]string `json:"sources,omitempty"`
	PubMedAPIKey *string   `json:"pubmed_api_key,omitempty"`
	PubMedEmail  *string   `json:"pubmed_email,omitempty"`
	PubMedTool   *string   `json:"pubmed_tool,omitempty"`
}

func NewSettingsHandler(libraryService *service.LibraryService, versionService *service.VersionService) *SettingsHandler {
	return &SettingsHandler{
		libraryService: libraryService,
		versionService: versionService,
	}
}

// SetResearchClient lets the caller wire in the live S2 client so that
// PutResearchSettings can hot-reload the API key without a server restart.
func (h *SettingsHandler) SetResearchClient(client researchSettingsClient, runtime ResearchRuntimeSettings) {
	h.researchClient = client
	h.researchRuntime = runtime
}

func (h *SettingsHandler) SetPubMedClient(client pubmedSettingsClient, runtime PubMedRuntimeSettings) {
	h.pubmedClient = client
	h.pubmedRuntime = runtime
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

func (h *SettingsHandler) TestWeixinDailyRecommendation(w http.ResponseWriter, r *http.Request) {
	var req model.WeixinDailyRecommendationSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	message, err := h.libraryService.TestWeixinDailyRecommendation(r.Context(), req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": message,
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

// GetResearchSettings → GET /api/settings/research
func (h *SettingsHandler) GetResearchSettings(w http.ResponseWriter, r *http.Request) {
	key, err := h.libraryService.GetAppSetting("s2_api_key")
	if err != nil {
		sendError(w, err)
		return
	}
	aiExternalSettings, err := h.libraryService.GetAIExternalSearchSettings()
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]any{
		"s2_api_key":     key,
		"pubmed_api_key": aiExternalSettings.PubMedAPIKey,
		"pubmed_email":   aiExternalSettings.PubMedEmail,
		"pubmed_tool":    aiExternalSettings.PubMedTool,
		"sources":        aiExternalSettings.Sources,
	})
}

// PutResearchSettings → PUT /api/settings/research
func (h *SettingsHandler) PutResearchSettings(w http.ResponseWriter, r *http.Request) {
	var body researchSettingsPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	if err := h.libraryService.UpsertAppSetting("s2_api_key", body.S2APIKey); err != nil {
		sendError(w, err)
		return
	}
	if h.researchClient != nil {
		h.researchClient.SetAPIKey(h.effectiveResearchAPIKey(body.S2APIKey))
	}
	if body.hasPubMedSettings() || body.Sources != nil {
		settings, err := h.libraryService.GetAIExternalSearchSettings()
		if err != nil {
			sendError(w, err)
			return
		}
		if body.PubMedAPIKey != nil {
			settings.PubMedAPIKey = *body.PubMedAPIKey
		}
		if body.PubMedEmail != nil {
			settings.PubMedEmail = *body.PubMedEmail
		}
		if body.PubMedTool != nil {
			settings.PubMedTool = *body.PubMedTool
		}
		if body.Sources != nil {
			settings.Sources = *body.Sources
		}
		updatedSettings, err := h.libraryService.UpdateAIExternalSearchSettings(*settings)
		if err != nil {
			sendError(w, err)
			return
		}
		if h.pubmedClient != nil {
			apiKey, email, tool := h.effectivePubMedSettings(updatedSettings)
			h.pubmedClient.SetSettings(apiKey, email, tool)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetAIExternalSearchSettings -> GET /api/settings/ai-external-search
func (h *SettingsHandler) GetAIExternalSearchSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.libraryService.GetAIExternalSearchSettings()
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, settings)
}

// PutAIExternalSearchSettings -> PUT /api/settings/ai-external-search
func (h *SettingsHandler) PutAIExternalSearchSettings(w http.ResponseWriter, r *http.Request) {
	var body aiExternalSearchSettingsPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	current, err := h.libraryService.GetAIExternalSearchSettings()
	if err != nil {
		sendError(w, err)
		return
	}
	input := *current
	if body.Sources != nil {
		input.Sources = *body.Sources
	}
	if body.PubMedAPIKey != nil {
		input.PubMedAPIKey = *body.PubMedAPIKey
	}
	if body.PubMedEmail != nil {
		input.PubMedEmail = *body.PubMedEmail
	}
	if body.PubMedTool != nil {
		input.PubMedTool = *body.PubMedTool
	}
	settings, err := h.libraryService.UpdateAIExternalSearchSettings(input)
	if err != nil {
		sendError(w, err)
		return
	}
	if h.pubmedClient != nil {
		apiKey, email, tool := h.effectivePubMedSettings(settings)
		h.pubmedClient.SetSettings(apiKey, email, tool)
	}
	sendJSON(w, http.StatusOK, settings)
}

func (p researchSettingsPayload) hasPubMedSettings() bool {
	return p.PubMedAPIKey != nil || p.PubMedEmail != nil || p.PubMedTool != nil
}

func (h *SettingsHandler) effectiveResearchAPIKey(saved string) string {
	if h.researchRuntime.APIKeySet {
		return strings.TrimSpace(h.researchRuntime.APIKey)
	}
	return strings.TrimSpace(saved)
}

func (h *SettingsHandler) effectivePubMedSettings(saved *model.AIExternalSearchSettings) (apiKey, email, tool string) {
	if saved != nil {
		apiKey = strings.TrimSpace(saved.PubMedAPIKey)
		email = strings.TrimSpace(saved.PubMedEmail)
		tool = strings.TrimSpace(saved.PubMedTool)
	}
	if h.pubmedRuntime.APIKeySet {
		apiKey = strings.TrimSpace(h.pubmedRuntime.APIKey)
	}
	if h.pubmedRuntime.EmailSet {
		email = strings.TrimSpace(h.pubmedRuntime.Email)
	}
	if h.pubmedRuntime.ToolSet {
		tool = strings.TrimSpace(h.pubmedRuntime.Tool)
	}
	return apiKey, email, tool
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
