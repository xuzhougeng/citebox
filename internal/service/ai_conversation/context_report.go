package ai_conversation

import (
	"context"
	"fmt"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
)

type ContextReport struct {
	Papers               []PaperContextCoverage `json:"papers"`
	RequestedFigures     int                    `json:"requested_figures"`
	IncludedFigures      int                    `json:"included_figures"`
	FigureStatus         string                 `json:"figure_status"`
	ToolContextTruncated bool                   `json:"tool_context_truncated"`
}

// Explicit selections take priority; automatic selection adds main figures in
// round-robin paper order, up to the same eight-image cap as the image loader.
func (s *Service) turnFigureIDs(in ai_assistant.RequestContext, pinned []repository.AIPinnedPaper) []int64 {
	ids := []int64{}
	seen := map[int64]bool{}
	add := func(id int64) {
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	add(in.FigureID)
	for _, id := range in.FigureIDs {
		add(id)
	}
	if in.AutoFigures && len(ids) < 8 {
		groups := [][]int64{}
		for _, pp := range pinned {
			paper, err := s.papers.GetPaperDetail(pp.PaperID)
			if err != nil || paper == nil {
				continue
			}
			group := []int64{}
			for _, f := range paper.Figures {
				if f.ParentFigureID == nil {
					group = append(group, f.ID)
				}
			}
			groups = append(groups, group)
		}
		for i := 0; len(ids) < 8; i++ {
			found := false
			for _, group := range groups {
				if i < len(group) {
					found = true
					add(group[i])
				}
				if len(ids) >= 8 {
					break
				}
			}
			if !found {
				break
			}
		}
	}
	return ids
}

func (s *Service) loadTurnFigures(ctx context.Context, ids []int64, settings model.AISettings) ([]model.AIImageInput, string, ContextReport) {
	report := ContextReport{RequestedFigures: len(ids), FigureStatus: "none"}
	if len(ids) == 0 {
		return nil, "本轮未附带图片；钉住文献文本不等于已看到图片。\n", report
	}
	report.FigureStatus = "unavailable"
	if s.figureCtx == nil {
		return nil, "本轮图片读取器不可用，未附带图片。\n", report
	}
	images, summaries, err := s.figureCtx.LoadFigureContext(ctx, ids)
	if err != nil {
		s.logger.Warn("ai_conversation: figure context load failed", "error", err)
		return nil, "本轮图片加载失败，未附带图片。\n", report
	}
	if !assistantMasterSupportsImages(settings) {
		report.FigureStatus = "text_only"
		return nil, buildFigureContextBlock(summaries, false), report
	}
	report.IncludedFigures = len(images)
	if len(images) == len(ids) {
		report.FigureStatus = "attached"
	} else if len(images) > 0 {
		report.FigureStatus = "partial"
	}
	block := fmt.Sprintf("本轮请求 %d 张图，实际向模型提交 %d 张图片输入；文字图注共 %d 条。图片缺失或超出数量/大小限制时只有文字，不能声称看到了未提交的图片。\n", len(ids), len(images), len(summaries))
	for _, summary := range summaries {
		block += summary + "\n"
	}
	return images, block + "\n", report
}
