package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

// ResearchService is the interface ResearchHandler depends on. Mirrors
// research.Service plus a minimum surface for testability.
type ResearchService interface {
	Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error)
	GetPaper(ctx context.Context, id string) (research.Paper, error)
	References(ctx context.Context, paperID string, offset, limit int) (research.PaperList, error)
	Citations(ctx context.Context, paperID string, offset, limit int, opts research.CitationOpts) (research.PaperList, error)
	Recommendations(ctx context.Context, paperID string) ([]research.Paper, error)
	RecommendationsForList(ctx context.Context, positive, negative []string) ([]research.Paper, error)
	Autocomplete(ctx context.Context, query string) ([]research.AutocompleteItem, error)
	SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error)
}

// BasketStore is the basket-side interface; defined here so we can stub in tests.
// Task 11 wires this to the actual basket service.
type BasketStore interface {
	List(ctx context.Context) ([]research.Paper, error)
	Add(ctx context.Context, s2PaperID, notes string) error
	Remove(ctx context.Context, s2PaperID string) error
	ImportToLibrary(ctx context.Context, ids []string) (imported int, err error)
	ExportMarkdown(ctx context.Context) (string, error)
}

// ResearchHandler aggregates routes under /api/research/*.
type ResearchHandler struct {
	service ResearchService
	basket  BasketStore
}

// NewResearchHandler builds a handler. The third parameter is reserved for an
// optional library-existence checker (see ExistsByDOI) and is currently nil.
func NewResearchHandler(service ResearchService, basket BasketStore, _ interface{}) *ResearchHandler {
	return &ResearchHandler{service: service, basket: basket}
}

// Search → GET /api/research/search?q=...
func (h *ResearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "缺少查询参数 q"))
		return
	}
	opts := research.SearchOpts{
		Year:          r.URL.Query().Get("year"),
		FieldsOfStudy: r.URL.Query().Get("fields_of_study"),
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		opts.Limit, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		opts.Offset, _ = strconv.Atoi(v)
	}

	res, err := h.service.Search(r.Context(), q, opts)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, res)
}

// GetPaper → GET /api/research/paper/:id
func (h *ResearchHandler) GetPaper(w http.ResponseWriter, r *http.Request) {
	id, err := parseResearchID(r.URL.Path, "/api/research/paper/")
	if err != nil {
		sendError(w, err)
		return
	}
	p, err := h.service.GetPaper(r.Context(), id)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, p)
}

// References → GET /api/research/paper/:id/references
func (h *ResearchHandler) References(w http.ResponseWriter, r *http.Request) {
	id, err := parseResearchIDWithSuffix(r.URL.Path, "/api/research/paper/", "/references")
	if err != nil {
		sendError(w, err)
		return
	}
	off, lim := readPageParams(r)
	res, err := h.service.References(r.Context(), id, off, lim)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, res)
}

// Citations → GET /api/research/paper/:id/citations
func (h *ResearchHandler) Citations(w http.ResponseWriter, r *http.Request) {
	id, err := parseResearchIDWithSuffix(r.URL.Path, "/api/research/paper/", "/citations")
	if err != nil {
		sendError(w, err)
		return
	}
	off, lim := readPageParams(r)
	opts := research.CitationOpts{
		InfluentialOnly: r.URL.Query().Get("influential_only") == "true",
	}
	res, err := h.service.Citations(r.Context(), id, off, lim, opts)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, res)
}

// Recommendations → GET /api/research/paper/:id/recommendations
func (h *ResearchHandler) Recommendations(w http.ResponseWriter, r *http.Request) {
	id, err := parseResearchIDWithSuffix(r.URL.Path, "/api/research/paper/", "/recommendations")
	if err != nil {
		sendError(w, err)
		return
	}
	res, err := h.service.Recommendations(r.Context(), id)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"items": res})
}

// Autocomplete → GET /api/research/autocomplete?q=...
func (h *ResearchHandler) Autocomplete(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		sendJSON(w, http.StatusOK, map[string]interface{}{"items": []research.AutocompleteItem{}})
		return
	}
	items, err := h.service.Autocomplete(r.Context(), q)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

// SnippetSearch → GET /api/research/snippets?q=...&paper_ids=...
func (h *ResearchHandler) SnippetSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "缺少查询参数 q"))
		return
	}
	opts := research.SnippetSearchOpts{
		Year:          r.URL.Query().Get("year"),
		Venue:         r.URL.Query().Get("venue"),
		FieldsOfStudy: r.URL.Query().Get("fields_of_study"),
	}
	if v := r.URL.Query().Get("paper_ids"); v != "" {
		ids := strings.Split(v, ",")
		clean := make([]string, 0, len(ids))
		for _, id := range ids {
			if id = strings.TrimSpace(id); id != "" {
				clean = append(clean, id)
			}
		}
		opts.PaperIDs = clean
	}
	if v := r.URL.Query().Get("min_citation_count"); v != "" {
		opts.MinCitationCount, _ = strconv.Atoi(v)
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		opts.Limit, _ = strconv.Atoi(v)
	}
	res, err := h.service.SnippetSearch(r.Context(), q, opts)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, res)
}

// RecommendationsForList → POST /api/research/recommendations
func (h *ResearchHandler) RecommendationsForList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Positive []string `json:"positive"`
		Negative []string `json:"negative"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	if len(body.Positive) == 0 {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "positive 不能为空"))
		return
	}
	res, err := h.service.RecommendationsForList(r.Context(), body.Positive, body.Negative)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"items": res})
}

func parseResearchID(path, prefix string) (string, error) {
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || rest == path {
		return "", apperr.New(apperr.CodeInvalidArgument, "paper id 缺失")
	}
	id, err := url.PathUnescape(rest)
	if err != nil {
		return "", apperr.New(apperr.CodeInvalidArgument, "paper id 无效")
	}
	return id, nil
}

func parseResearchIDWithSuffix(path, prefix, suffix string) (string, error) {
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || rest == path {
		return "", apperr.New(apperr.CodeInvalidArgument, "paper id 缺失")
	}
	rest = strings.TrimSuffix(rest, suffix)
	id, err := url.PathUnescape(rest)
	if err != nil {
		return "", apperr.New(apperr.CodeInvalidArgument, "paper id 无效")
	}
	return id, nil
}

func readPageParams(r *http.Request) (int, int) {
	off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if lim <= 0 || lim > 100 {
		lim = 20
	}
	return off, lim
}

func writeResearchError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, research.ErrPaperNotFound):
		sendError(w, apperr.New(apperr.CodeNotFound, "未找到对应论文"))
	case errors.Is(err, research.ErrRateLimited):
		sendError(w, apperr.New(apperr.CodeUnavailable, "Semantic Scholar 限流，请稍后再试"))
	default:
		slog.Default().Warn("research upstream error", "error", err.Error())
		sendError(w, apperr.New(apperr.CodeUnavailable, "文献搜索服务暂不可用: "+err.Error()))
	}
}

// BasketList → GET /api/research/basket
func (h *ResearchHandler) BasketList(w http.ResponseWriter, r *http.Request) {
	if h.basket == nil {
		sendError(w, apperr.New(apperr.CodeUnavailable, "basket not configured"))
		return
	}
	items, err := h.basket.List(r.Context())
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

// BasketAdd → POST /api/research/basket
func (h *ResearchHandler) BasketAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		S2PaperID string `json:"s2_paper_id"`
		Notes     string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	if body.S2PaperID == "" {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "s2_paper_id 必填"))
		return
	}
	if err := h.basket.Add(r.Context(), body.S2PaperID, body.Notes); err != nil {
		writeResearchError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BasketRemove → DELETE /api/research/basket/:id
func (h *ResearchHandler) BasketRemove(w http.ResponseWriter, r *http.Request) {
	id, err := parseResearchID(r.URL.Path, "/api/research/basket/")
	if err != nil {
		sendError(w, err)
		return
	}
	if err := h.basket.Remove(r.Context(), id); err != nil {
		writeResearchError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BasketImportToLibrary → POST /api/research/basket/import-to-library
func (h *ResearchHandler) BasketImportToLibrary(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	n, err := h.basket.ImportToLibrary(r.Context(), body.IDs)
	if err != nil {
		writeResearchError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"imported": n})
}

// BasketExport → GET /api/research/basket/export
func (h *ResearchHandler) BasketExport(w http.ResponseWriter, r *http.Request) {
	md, err := h.basket.ExportMarkdown(r.Context())
	if err != nil {
		writeResearchError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="research-basket.md"`)
	_, _ = w.Write([]byte(md))
}
