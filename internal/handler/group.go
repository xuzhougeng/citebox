package handler

import (
	"encoding/json"
	"net/http"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

type GroupHandler struct {
	service *service.LibraryService
}

func NewGroupHandler(svc *service.LibraryService) *GroupHandler {
	return &GroupHandler{service: svc}
}

func (h *GroupHandler) List(w http.ResponseWriter, r *http.Request) {
	groups, err := h.service.ListGroups()
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{
		"groups": groups,
	})
}

func (h *GroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	group, err := h.service.CreateGroup(req.Name, req.Description)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"group":   group,
	})
}

func (h *GroupHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/groups/")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "group id 无效"))
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	group, err := h.service.UpdateGroup(id, req.Name, req.Description)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"group":   group,
	})
}

func (h *GroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/groups/")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "group id 无效"))
		return
	}

	if err := h.service.DeleteGroup(id); err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, model.SuccessResponse{
		Success: true,
		Message: "分组已删除",
	})
}
