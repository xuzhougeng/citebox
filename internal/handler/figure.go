package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

type FigureHandler struct {
	service *service.LibraryService
}

func NewFigureHandler(svc *service.LibraryService) *FigureHandler {
	return &FigureHandler{service: svc}
}

func (h *FigureHandler) List(w http.ResponseWriter, r *http.Request) {
	groupID, err := optionalInt64(r.URL.Query().Get("group_id"))
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "group_id 无效"))
		return
	}

	tagID, err := optionalInt64(r.URL.Query().Get("tag_id"))
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "tag_id 无效"))
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	result, err := h.service.ListFigures(model.FigureFilter{
		Keyword:  strings.TrimSpace(r.URL.Query().Get("keyword")),
		GroupID:  groupID,
		TagID:    tagID,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
}
