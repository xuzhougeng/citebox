package handler

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func (h *FigureHandler) ExportNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := model.FigureFilter{Keyword: strings.TrimSpace(q.Get("keyword")), SortBy: q.Get("sort_by"), HasNotes: q.Get("has_notes") == "1" || q.Get("has_notes") == "true"}
	for key, target := range map[string]**int64{"paper_id": &filter.PaperID, "group_id": &filter.GroupID, "tag_id": &filter.TagID} {
		id, err := optionalInt64(q.Get(key))
		if err != nil || (id != nil && *id <= 0) {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, key+" is invalid"))
			return
		}
		*target = id
	}
	if value := strings.TrimSpace(q.Get("figure_type")); value != "" {
		filter.FigureType = model.NormalizeFigureType(value)
	}
	file, err := h.service.ExportFigureNotes(r.Context(), filter, q.Get("language"))
	if err != nil {
		sendError(w, err)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="citebox-figure-notes.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "citebox-figure-notes.zip", time.Time{}, file)
}
