package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// aiFigureContextMaxCount caps how many checked figures a single turn may
// attach as vision context.
const aiFigureContextMaxCount = 8

// LoadFigureContext resolves user-checked figure IDs into provider-ready
// base64 image inputs plus one text summary per resolved figure for the
// prompt. It implements the ai_conversation.FigureContextLoader interface.
// Missing/unloadable figures are skipped with a warning instead of failing
// the whole turn.
func (s *AIService) LoadFigureContext(_ context.Context, figureIDs []int64) ([]model.AIImageInput, []string, error) {
	seen := make(map[int64]struct{}, len(figureIDs))
	ids := make([]int64, 0, len(figureIDs))
	for _, id := range figureIDs {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		if len(ids) >= aiFigureContextMaxCount {
			break
		}
	}
	if len(ids) == 0 {
		return nil, nil, nil
	}

	paperCache := make(map[int64]*model.Paper)
	images := make([]model.AIImageInput, 0, len(ids))
	summaries := make([]string, 0, len(ids))
	totalBytes := 0
	budgetReached := false

	for _, id := range ids {
		item, err := s.repo.GetFigure(id)
		if err != nil {
			s.logger.Warn("ai figure context: figure lookup failed", "figure_id", id, "error", err)
			continue
		}
		if item == nil {
			s.logger.Warn("ai figure context: figure not found", "figure_id", id)
			continue
		}
		paper, ok := paperCache[item.PaperID]
		if !ok {
			paper, err = s.repo.GetPaperDetail(item.PaperID)
			if err != nil {
				s.logger.Warn("ai figure context: paper lookup failed", "figure_id", id, "paper_id", item.PaperID, "error", err)
				continue
			}
			paperCache[item.PaperID] = paper
		}
		figure := findFigureInPaper(paper.Figures, id)
		if figure == nil {
			s.logger.Warn("ai figure context: figure not in paper detail", "figure_id", id, "paper_id", item.PaperID)
			continue
		}

		summaries = append(summaries, figureContextSummary(*figure))
		if budgetReached {
			continue
		}
		data, mimeType, err := loadFigureImageData(s.config.FiguresDir(), paper.Figures, *figure)
		if err != nil {
			if !apperr.IsCode(err, apperr.CodeNotFound) {
				return nil, nil, err
			}
			s.logger.Warn("ai figure context: image missing", "figure_id", id, "filename", figure.Filename)
			continue
		}
		compressedData, compressedMIMEType, err := compressAIImage(data, mimeType)
		if err != nil {
			s.logger.Warn("ai figure context: compression failed", "figure_id", id, "error", err)
			continue
		}
		if totalBytes > 0 && totalBytes+len(compressedData) > aiFigureImageTotalBudget {
			s.logger.Warn("ai figure context: image budget reached",
				"figure_id", id, "included", len(images), "budget_bytes", aiFigureImageTotalBudget)
			budgetReached = true
			continue
		}
		images = append(images, model.AIImageInput{
			MIMEType: compressedMIMEType,
			Data:     base64.StdEncoding.EncodeToString(compressedData),
		})
		totalBytes += len(compressedData)
	}

	return images, summaries, nil
}

func findFigureInPaper(figures []model.Figure, id int64) *model.Figure {
	for i := range figures {
		if figures[i].ID == id {
			return &figures[i]
		}
	}
	return nil
}

func figureContextSummary(figure model.Figure) string {
	label := strings.TrimSpace(figure.DisplayLabel)
	if label == "" {
		label = fmt.Sprintf("第 %d 页图 %d", figure.PageNumber, figure.FigureIndex)
	}
	if sub := strings.TrimSpace(figure.SubfigureLabel); sub != "" {
		label = label + " " + sub
	}
	caption := fallbackText(strings.TrimSpace(figure.Caption), "无")
	return fmt.Sprintf("- figure_id=%d；标签=%s；caption=%s", figure.ID, label, caption)
}
