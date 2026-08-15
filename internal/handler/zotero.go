package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

type ZoteroHandler struct {
	service *service.LibraryService
}

func NewZoteroHandler(svc *service.LibraryService) *ZoteroHandler {
	return &ZoteroHandler{service: svc}
}

func (h *ZoteroHandler) Settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := h.service.GetZoteroSettings()
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var req model.ZoteroSettings
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
			return
		}
		settings, err := h.service.UpdateZoteroSettings(req)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, settings)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *ZoteroHandler) Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := h.service.GetZoteroStatus(r.Context())
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, status)
}

func (h *ZoteroHandler) Collections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	collections, err := h.service.ListZoteroCollections(r.Context())
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]any{"collections": collections})
}

func (h *ZoteroHandler) Preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeZoteroSelection(r)
	if err != nil {
		sendError(w, err)
		return
	}
	run, err := h.service.PreviewZoteroImport(r.Context(), req.CollectionKeys, req.IncludeChildren)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, run)
}

func (h *ZoteroHandler) Import(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeZoteroSelection(r)
	if err != nil {
		sendError(w, err)
		return
	}
	run, err := h.service.StartZoteroImport(r.Context(), req.CollectionKeys, req.IncludeChildren)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusAccepted, run)
}

func (h *ZoteroHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
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
	paper, status, err := h.service.IngestZoteroPaperFromReader(file, header, model.ZoteroIngestInput{
		ItemKey:        r.FormValue("item_key"),
		LibraryID:      r.FormValue("library_id"),
		Title:          r.FormValue("title"),
		DOI:            r.FormValue("doi"),
		AuthorsText:    r.FormValue("authors_text"),
		Journal:        r.FormValue("journal"),
		PublishedAt:    r.FormValue("published_at"),
		AbstractText:   r.FormValue("abstract_text"),
		NotesText:      r.FormValue("notes"),
		Tags:           splitCSV(r.FormValue("tags")),
		CollectionPath: r.FormValue("collection_path"),
		ExtractionMode: r.FormValue("extraction_mode"),
	})
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusAccepted, map[string]any{
		"success": true,
		"status":  status,
		"paper":   paper,
	})
}

func (h *ZoteroHandler) Runs(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/integrations/zotero/runs/"), "/")
	if trimmed == "" {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "缺少导入任务 ID"))
		return
	}
	parts := strings.Split(trimmed, "/")
	runID := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		run, err := h.service.GetZoteroImportRun(runID)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, run)
		return
	}
	if len(parts) == 4 && parts[1] == "items" && parts[3] == "attach-pdf" {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
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
		run, err := h.service.AttachZoteroImportPDF(runID, parts[2], file, header)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusAccepted, run)
		return
	}
	if len(parts) == 4 && parts[1] == "items" && parts[3] == "import-by-doi" {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		run, err := h.service.ImportZoteroItemByDOI(r.Context(), runID, parts[2])
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusAccepted, run)
		return
	}
	sendError(w, apperr.New(apperr.CodeNotFound, "未知的 Zotero 导入接口"))
}

type zoteroSelectionRequest struct {
	CollectionKeys  []string `json:"collection_keys"`
	IncludeChildren *bool    `json:"include_children"`
}

type zoteroSelection struct {
	CollectionKeys  []string
	IncludeChildren bool
}

func decodeZoteroSelection(r *http.Request) (zoteroSelection, error) {
	var raw zoteroSelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return zoteroSelection{}, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误")
	}
	if len(raw.CollectionKeys) == 0 {
		return zoteroSelection{}, apperr.New(apperr.CodeInvalidArgument, "请选择至少一个 Zotero collection")
	}
	includeChildren := true
	if raw.IncludeChildren != nil {
		includeChildren = *raw.IncludeChildren
	}
	return zoteroSelection{CollectionKeys: raw.CollectionKeys, IncludeChildren: includeChildren}, nil
}
