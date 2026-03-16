package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"paper_image_db/internal/apperr"
	"paper_image_db/internal/model"
	"paper_image_db/internal/service"
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
