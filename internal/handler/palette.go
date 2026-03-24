package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

type PaletteHandler struct {
	service *service.LibraryService
}

func NewPaletteHandler(svc *service.LibraryService) *PaletteHandler {
	return &PaletteHandler{service: svc}
}

func (h *PaletteHandler) List(w http.ResponseWriter, r *http.Request) {
	groupID, err := optionalInt64(r.URL.Query().Get("group_id"))
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "group_id 无效"))
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	result, err := h.service.ListPalettes(model.PaletteFilter{
		Keyword:  strings.TrimSpace(r.URL.Query().Get("keyword")),
		GroupID:  groupID,
		SortBy:   strings.TrimSpace(r.URL.Query().Get("sort_by")),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
}

func (h *PaletteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/palettes/")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "palette id 无效"))
		return
	}

	if err := h.service.DeletePalette(id); err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, model.SuccessResponse{
		Success: true,
	})
}
