package ai_conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
)

const maxAutoAttachedFigures = 4

// ContextUsage is emitted immediately before the provider call and saved
// with the assistant message through its turn run. Image counts describe inputs sent, not provider comprehension.
type ContextUsage struct {
	HistoryMessages        int                       `json:"history_messages"`
	OmittedHistoryMessages int                       `json:"omitted_history_messages"`
	Papers                 []PinnedPaperContextUsage `json:"papers"`
	RequestedImages        int                       `json:"requested_images"`
	AttachedImages         int                       `json:"attached_images"`
	ImageReason            string                    `json:"image_reason"`
	EvidenceSnippets       int                       `json:"evidence_snippets"`
	EstimatedTextTokens    int                       `json:"estimated_text_tokens"`
}

func (s *Service) turnFigureIDs(input ai_assistant.RequestContext, pinned []repository.AIPinnedPaper) []int64 {
	return s.turnFigureIDsForQuestion(input, pinned, "")
}

func (s *Service) turnFigureIDsForQuestion(input ai_assistant.RequestContext, pinned []repository.AIPinnedPaper, question string) []int64 {
	ids := append([]int64(nil), input.FigureIDs...)
	if input.FigureID > 0 {
		ids = append(ids, input.FigureID)
	}
	ids = dedupeInt64(ids)
	if len(ids) > 0 || !input.AutoAttachFigures || s.papers == nil {
		return ids
	}
	var papers [][]int64
	seen := make(map[int64]bool)
	for _, pp := range pinned {
		paper, err := s.papers.GetPaperDetail(pp.PaperID)
		if err != nil || paper == nil {
			continue
		}
		var figures []int64
		for _, figure := range ai_assistant.RelevantFigures(paper.Figures, question) {
			if figure.ID <= 0 || seen[figure.ID] {
				continue
			}
			seen[figure.ID] = true
			figures = append(figures, figure.ID)
		}
		if len(figures) > 0 {
			papers = append(papers, figures)
		}
	}
	// Give each pinned paper a turn before taking another figure from any paper.
	for round := 0; ; round++ {
		added := false
		for _, figures := range papers {
			if round >= len(figures) {
				continue
			}
			ids = append(ids, figures[round])
			added = true
			if len(ids) == maxAutoAttachedFigures {
				return ids
			}
		}
		if !added {
			break
		}
	}
	return ids
}

func (s *Service) loadTurnFigures(ctx context.Context, input ai_assistant.RequestContext,
	pinned []repository.AIPinnedPaper, settings model.AISettings) ([]model.AIImageInput, string, ContextUsage) {
	return s.loadTurnFiguresForQuestion(ctx, input, pinned, settings, "")
}

func (s *Service) loadTurnFiguresForQuestion(ctx context.Context, input ai_assistant.RequestContext, pinned []repository.AIPinnedPaper, settings model.AISettings, question string) ([]model.AIImageInput, string, ContextUsage) {
	ids := s.turnFigureIDsForQuestion(input, pinned, question)
	usage := ContextUsage{RequestedImages: len(ids), ImageReason: "auto_disabled"}
	var images []model.AIImageInput
	var summaries []string
	if len(ids) == 0 {
		if input.AutoAttachFigures {
			usage.ImageReason = "no_figures"
		}
	} else if s.figureCtx == nil {
		usage.ImageReason = "loader_unavailable"
	} else {
		var err error
		images, summaries, err = s.figureCtx.LoadFigureContext(ctx, ids)
		switch {
		case err != nil:
			s.logger.Warn("ai_conversation: figure context load failed", "error", err)
			images, summaries = nil, nil
			usage.ImageReason = "load_failed"
		case assistantMasterImageReason(settings) != "":
			images = nil
			usage.ImageReason = assistantMasterImageReason(settings)
		case len(images) == 0:
			usage.ImageReason = "no_image_data"
		case len(images) < len(ids):
			usage.ImageReason = "partial"
		default:
			usage.ImageReason = "attached"
		}
	}
	usage.AttachedImages = len(images)
	reasons := map[string]string{
		"auto_disabled":      "未选择图片，自动附图未开启",
		"no_figures":         "钉住文献没有可用的已提取图片",
		"loader_unavailable": "图片加载器不可用",
		"load_failed":        "图片加载失败",
		"capability_unknown": "当前模型的图片能力未确认，请在模型设置中明确启用",
		"model_unsupported":  "当前模型不支持图片输入",
		"no_image_data":      "图片文件不可用或无法解码，仅保留可用文字说明",
		"partial":            "部分图片未加载或达到附图限制",
		"attached":           "已作为图片输入发送；是否理解图片取决于上游模型",
	}
	block := fmt.Sprintf("本轮实际图片输入：%d 张（%s）。\n", len(images), reasons[usage.ImageReason])
	if len(images) > 0 {
		block += fmt.Sprintf("本轮随附图片（共 %d 张）；下面的图片文件序号对应图片输入顺序，仅文字项没有图片输入。\n", len(images))
	} else {
		block += "未提供图片输入，不要声称看到了图片；以下如有图注，仅作为文字证据。\n"
	}
	block += strings.Join(summaries, "\n") + "\n\n"
	return images, block, usage
}
