package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

type PaperHandler struct {
	service *service.LibraryService
}

func NewPaperHandler(svc *service.LibraryService) *PaperHandler {
	return &PaperHandler{service: svc}
}

func (h *PaperHandler) List(w http.ResponseWriter, r *http.Request) {
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
	hasPaperNotes := parseBoolQuery(r.URL.Query().Get("has_paper_notes"))

	result, err := h.service.ListPapers(model.PaperFilter{
		Keyword:       strings.TrimSpace(r.URL.Query().Get("keyword")),
		Author:        strings.TrimSpace(r.URL.Query().Get("author")),
		KeywordScope:  strings.TrimSpace(r.URL.Query().Get("keyword_scope")),
		GroupID:       groupID,
		TagID:         tagID,
		Status:        strings.TrimSpace(r.URL.Query().Get("status")),
		HasPaperNotes: hasPaperNotes,
		SortBy:        strings.TrimSpace(r.URL.Query().Get("sort_by")),
		Page:          page,
		PageSize:      pageSize,
	})
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
}

func (h *PaperHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		sendError(w, apperr.Wrap(apperr.CodeInvalidArgument, "解析上传表单失败", err))
		return
	}

	file, header, err := r.FormFile("pdf")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "缺少 PDF 文件"))
		return
	}
	defer file.Close()

	groupID, err := optionalInt64(r.FormValue("group_id"))
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "group_id 无效"))
		return
	}

	paper, err := h.service.UploadPaper(file, header, service.UploadPaperParams{
		Title:          r.FormValue("title"),
		GroupID:        groupID,
		Tags:           splitCSV(r.FormValue("tags")),
		ExtractionMode: r.FormValue("extraction_mode"),
	})
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusAccepted, map[string]interface{}{
		"success": true,
		"paper":   paper,
	})
}

func (h *PaperHandler) ImportByDOI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DOI            string   `json:"doi"`
		Title          string   `json:"title"`
		GroupID        *int64   `json:"group_id"`
		Tags           []string `json:"tags"`
		ExtractionMode string   `json:"extraction_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	paper, err := h.service.ImportPaperByDOI(r.Context(), service.ImportPaperByDOIParams{
		DOI:            req.DOI,
		Title:          req.Title,
		GroupID:        req.GroupID,
		Tags:           req.Tags,
		ExtractionMode: req.ExtractionMode,
	})
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusAccepted, map[string]interface{}{
		"success": true,
		"paper":   paper,
	})
}

func (h *PaperHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/papers/")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
		return
	}

	paper, err := h.service.GetPaper(id)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, paper)
}

func (h *PaperHandler) Reextract(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/papers/", "/reextract")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
		return
	}

	paper, err := h.service.ReextractPaper(id)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusAccepted, map[string]interface{}{
		"success": true,
		"paper":   paper,
	})
}

func (h *PaperHandler) GetManualExtractionWorkspace(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/papers/", "/manual-extraction")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
		return
	}

	workspace, err := h.service.GetManualExtractionWorkspace(id)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"workspace": workspace,
	})
}

func (h *PaperHandler) ManualPreview(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/papers/", "/manual-preview")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
		return
	}

	page, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	if err != nil || page < 1 {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "page 参数无效"))
		return
	}

	imageData, err := h.service.GetManualPreview(id, page)
	if err != nil {
		sendError(w, err)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(imageData)
}

func (h *PaperHandler) ManualExtract(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/papers/", "/manual-extraction")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
		return
	}

	var req struct {
		Regions []model.ManualExtractionRegion `json:"regions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	paper, addedCount, err := h.service.ManualExtractFigures(id, service.ManualExtractParams{
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

func (h *PaperHandler) UpdatePDFText(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/papers/", "/pdf-text")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
		return
	}

	var req struct {
		PDFText string `json:"pdf_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	paper, err := h.service.UpdatePaperPDFText(id, req.PDFText)
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"paper":   paper,
	})
}

func (h *PaperHandler) RefreshDOIMetadata(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/papers/", "/refresh-doi-metadata")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
		return
	}

	var req struct {
		DOI string `json:"doi"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	paper, err := h.service.RefreshPaperDOIMetadata(r.Context(), id, service.RefreshPaperDOIMetadataParams{
		DOI: req.DOI,
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

func (h *PaperHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/papers/")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
		return
	}

	var req struct {
		Title          string   `json:"title"`
		DOI            *string  `json:"doi"`
		PDFText        *string  `json:"pdf_text"`
		AuthorsText    string   `json:"authors_text"`
		Journal        string   `json:"journal"`
		PublishedAt    string   `json:"published_at"`
		AbstractText   string   `json:"abstract_text"`
		NotesText      string   `json:"notes_text"`
		PaperNotesText string   `json:"paper_notes_text"`
		GroupID        *int64   `json:"group_id"`
		Tags           []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	paper, err := h.service.UpdatePaper(id, service.UpdatePaperParams{
		Title:          req.Title,
		DOI:            req.DOI,
		PDFText:        req.PDFText,
		AuthorsText:    req.AuthorsText,
		Journal:        req.Journal,
		PublishedAt:    req.PublishedAt,
		AbstractText:   req.AbstractText,
		NotesText:      req.NotesText,
		PaperNotesText: req.PaperNotesText,
		GroupID:        req.GroupID,
		Tags:           req.Tags,
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

func (h *PaperHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/papers/")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
		return
	}

	if err := h.service.DeletePaper(id); err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, model.SuccessResponse{
		Success: true,
		Message: "文献已删除",
	})
}

func (h *PaperHandler) Purge(w http.ResponseWriter, r *http.Request) {
	if err := h.service.PurgeLibrary(); err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, model.SuccessResponse{
		Success: true,
		Message: "数据库已清空",
	})
}

func optionalInt64(value string) (*int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
