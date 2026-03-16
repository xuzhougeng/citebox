package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"paper_image_db/internal/apperr"
	"paper_image_db/internal/model"
	"paper_image_db/internal/service"
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

	result, err := h.service.ListPapers(model.PaperFilter{
		Keyword:  strings.TrimSpace(r.URL.Query().Get("keyword")),
		GroupID:  groupID,
		TagID:    tagID,
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		Page:     page,
		PageSize: pageSize,
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
		Title:   r.FormValue("title"),
		GroupID: groupID,
		Tags:    splitCSV(r.FormValue("tags")),
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

func (h *PaperHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/papers/")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
		return
	}

	var req struct {
		Title   string   `json:"title"`
		GroupID *int64   `json:"group_id"`
		Tags    []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	paper, err := h.service.UpdatePaper(id, service.UpdatePaperParams{
		Title:   req.Title,
		GroupID: req.GroupID,
		Tags:    req.Tags,
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
