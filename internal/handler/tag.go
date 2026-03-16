package handler

import (
	"encoding/json"
	"net/http"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

type TagHandler struct {
	service *service.LibraryService
}

func NewTagHandler(svc *service.LibraryService) *TagHandler {
	return &TagHandler{service: svc}
}

func (h *TagHandler) List(w http.ResponseWriter, r *http.Request) {
	tags, err := h.service.ListTags()
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{
		"tags": tags,
	})
}

func (h *TagHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	tag, err := h.service.CreateTag(req.Name, req.Color)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"tag":     tag,
	})
}

func (h *TagHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/tags/")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "tag id 无效"))
		return
	}

	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	tag, err := h.service.UpdateTag(id, req.Name, req.Color)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"tag":     tag,
	})
}

func (h *TagHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/tags/")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "tag id 无效"))
		return
	}

	if err := h.service.DeleteTag(id); err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, model.SuccessResponse{
		Success: true,
		Message: "标签已删除",
	})
}
