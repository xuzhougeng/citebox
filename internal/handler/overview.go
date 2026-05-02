package handler

import (
	"net/http"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/daily_figure"
)

// OverviewHandler powers the /overview dashboard. Three small endpoints feed
// the three panels (Research Dashboard / 图片 / Status). Everything is
// read-only — no mutations live on this surface.
type OverviewHandler struct {
	library OverviewLibrarySource
	picker  *daily_figure.Picker
	bridge  OverviewStatusSource
}

// OverviewLibrarySource is the LibraryService surface the overview uses.
type OverviewLibrarySource interface {
	ListPapers(filter model.PaperFilter) (*model.PaperListResponse, error)
}

// OverviewStatusSource exposes runtime status (bridge enabled, etc.) for the
// Status panel. *service.LibraryService satisfies this via GetWeixinBridgeSettings.
type OverviewStatusSource interface {
	GetWeixinBridgeSettings() (*model.WeixinBridgeSettings, error)
}

func NewOverviewHandler(library OverviewLibrarySource, picker *daily_figure.Picker, bridge OverviewStatusSource) *OverviewHandler {
	return &OverviewHandler{library: library, picker: picker, bridge: bridge}
}

// Summary → GET /api/overview/summary
func (h *OverviewHandler) Summary(w http.ResponseWriter, r *http.Request) {
	resp, err := h.library.ListPapers(model.PaperFilter{Page: 1, PageSize: 7, SortBy: "created_at"})
	if err != nil {
		sendError(w, err)
		return
	}
	recentPapers := make([]map[string]any, 0, 7)
	if resp != nil {
		for _, p := range resp.Papers {
			recentPapers = append(recentPapers, map[string]any{
				"id":         p.ID,
				"title":      p.Title,
				"created_at": p.CreatedAt,
			})
		}
	}
	sendJSON(w, http.StatusOK, map[string]any{
		"recent_papers": recentPapers,
	})
}

// DailyFigure → GET /api/overview/daily-figure
func (h *OverviewHandler) DailyFigure(w http.ResponseWriter, r *http.Request) {
	if h.picker == nil {
		sendError(w, apperr.New(apperr.CodeInternal, "图片选择器未初始化"))
		return
	}
	fig, err := h.picker.PickForDate(time.Now())
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]any{
		"id":      fig.ID,
		"caption": fig.Caption,
		"label":   fig.DisplayLabel,
		"paper":   fig.PaperTitle,
		"page":    fig.PageNumber,
	})
}

// Status → GET /api/overview/status
func (h *OverviewHandler) Status(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"server_time": time.Now().Format(time.RFC3339),
	}
	if h.bridge != nil {
		if s, err := h.bridge.GetWeixinBridgeSettings(); err == nil && s != nil {
			out["weixin_bridge"] = map[string]any{
				"enabled":                  s.Enabled,
				"daily_recommendation_on":  s.DailyRecommendation.Enabled,
				"daily_recommendation_at":  s.DailyRecommendation.SendTime,
			}
		}
	}
	sendJSON(w, http.StatusOK, out)
}
