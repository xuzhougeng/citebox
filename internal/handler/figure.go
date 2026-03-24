package handler

import (
	"encoding/json"
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
	hasNotesValue := strings.TrimSpace(r.URL.Query().Get("has_notes"))

	result, err := h.service.ListFigures(model.FigureFilter{
		Keyword:  strings.TrimSpace(r.URL.Query().Get("keyword")),
		GroupID:  groupID,
		TagID:    tagID,
		HasNotes: hasNotesValue == "1" || strings.EqualFold(hasNotesValue, "true"),
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

func (h *FigureHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/figures/")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "figure id 无效"))
		return
	}

	paper, err := h.service.DeleteFigure(id)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"paper":   paper,
	})
}

func (h *FigureHandler) CreateSubfigures(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/figures/", "/subfigures")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "figure id 无效"))
		return
	}

	var req struct {
		Regions []model.SubfigureExtractionRegion `json:"regions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	paper, addedCount, err := h.service.CreateSubfigures(id, service.CreateSubfiguresParams{
		Regions: req.Regions,
	})
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"paper":       paper,
		"added_count": addedCount,
	})
}

func (h *FigureHandler) ServeImage(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/figures/", "/image")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "figure id 无效"))
		return
	}

	data, contentType, _, err := h.service.GetFigureImage(id)
	if err != nil {
		sendError(w, err)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = w.Write(data)
}

func (h *FigureHandler) CreatePalette(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/figures/", "/palette")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "figure id 无效"))
		return
	}

	var req struct {
		Name   string   `json:"name"`
		Colors []string `json:"colors"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	palette, paper, err := h.service.CreateOrUpdateFigurePalette(id, service.CreatePaletteParams{
		Name:   req.Name,
		Colors: req.Colors,
	})
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"palette": palette,
		"paper":   paper,
	})
}

func (h *FigureHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/figures/")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "figure id 无效"))
		return
	}

	var req struct {
		Tags      []string `json:"tags"`
		Caption   *string  `json:"caption"`
		NotesText *string  `json:"notes_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	paper, err := h.service.UpdateFigure(id, service.UpdateFigureParams{
		Tags:      req.Tags,
		Caption:   req.Caption,
		NotesText: req.NotesText,
	})
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"paper":   paper,
	})
}
