package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

type AIHandler struct {
	service *service.AIService
}

func NewAIHandler(svc *service.AIService) *AIHandler {
	return &AIHandler{service: svc}
}

func (h *AIHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.service.GetSettings()
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, settings)
}

func (h *AIHandler) GetDefaultSettings(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, http.StatusOK, model.DefaultAISettings())
}

func (h *AIHandler) GetRolePrompts(w http.ResponseWriter, r *http.Request) {
	rolePrompts, err := h.service.GetRolePrompts()
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, model.AIRolePromptCollection{
		RolePrompts: rolePrompts,
	})
}

func (h *AIHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req model.AISettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	settings, err := h.service.UpdateSettings(req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"settings": settings,
	})
}

func (h *AIHandler) UpdateModelSettings(w http.ResponseWriter, r *http.Request) {
	var req model.AIModelSettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	settings, err := h.service.UpdateModelSettings(req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"settings": settings,
	})
}

func (h *AIHandler) UpdatePromptSettings(w http.ResponseWriter, r *http.Request) {
	var req model.AIPromptSettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	settings, err := h.service.UpdatePromptSettings(req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"settings": settings,
	})
}

func (h *AIHandler) UpdateRolePrompts(w http.ResponseWriter, r *http.Request) {
	var req model.AIRolePromptCollection
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	rolePrompts, err := h.service.UpdateRolePrompts(req.RolePrompts)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"role_prompts": rolePrompts,
	})
}

func (h *AIHandler) CheckModel(w http.ResponseWriter, r *http.Request) {
	var req model.AIModelConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	result, err := h.service.CheckModel(r.Context(), req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
}

func (h *AIHandler) Read(w http.ResponseWriter, r *http.Request) {
	var req model.AIReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	result, err := h.service.ReadPaper(r.Context(), req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
}

func (h *AIHandler) Translate(w http.ResponseWriter, r *http.Request) {
	var req model.AITranslateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	result, err := h.service.Translate(r.Context(), req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
}

func (h *AIHandler) DetectFigureRegions(w http.ResponseWriter, r *http.Request) {
	var req model.AIFigureRegionDetectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	result, err := h.service.DetectFigureRegions(r.Context(), req)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
}

func (h *AIHandler) ReadStream(w http.ResponseWriter, r *http.Request) {
	var req model.AIReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	started := false
	encoder := json.NewEncoder(w)
	controller := http.NewResponseController(w)
	sendEvent := func(event model.AIReadStreamEvent) error {
		if !started {
			started = true
			w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(http.StatusOK)
		}
		if err := encoder.Encode(event); err != nil {
			return err
		}
		return controller.Flush()
	}

	err := h.service.ReadPaperStream(r.Context(), req, sendEvent)
	if err == nil {
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	if !started {
		sendError(w, err)
		return
	}
	_ = sendEvent(model.AIReadStreamEvent{
		Type:  "error",
		Error: apperr.Message(err),
		Code:  string(apperr.CodeOf(err)),
	})
}

func (h *AIHandler) ExportReadMarkdown(w http.ResponseWriter, r *http.Request) {
	var req model.AIReadExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	filename, archive, err := h.service.ExportReadMarkdown(r.Context(), req)
	if err != nil {
		sendError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(archive)))
	if _, err := w.Write(archive); err != nil {
		if isClientDisconnectError(err) {
			return
		}
		sendError(w, apperr.Wrap(apperr.CodeInternal, "下载 AI Markdown 导出包失败", err))
		return
	}
}
