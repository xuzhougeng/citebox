package service

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/ai_image_gen"
)

// LoadImageGenInputs implements ai_image_gen.LoadInputsFn.
// It loads paper/figure context and base64-encoded vision images for the
// requested paper and figure IDs.
func (s *AIService) LoadImageGenInputs(ctx context.Context, paperIDs, figureIDs []int64) (
	ai_image_gen.VisionInputContext, []model.AIImageInput, error) {

	out := ai_image_gen.VisionInputContext{}
	imageInputs := make([]aiImageInput, 0)

	includedFigureIDs := make(map[int64]struct{})

	// Load full papers first: each paper's figures are already embedded via GetPaperDetail.
	for _, pid := range paperIDs {
		paper, err := s.repo.GetPaperDetail(pid)
		if err != nil {
			return ai_image_gen.VisionInputContext{}, nil, apperr.Wrap(apperr.CodeInvalidArgument, "无法读取文献", err)
		}
		if paper == nil {
			return ai_image_gen.VisionInputContext{}, nil, apperr.New(apperr.CodeNotFound, "文献不存在")
		}

		figures := topLevelFigures(paper.Figures)
		images, _, err := s.loadFigureInputs(paper, figures, model.AIActionFigureInterpretation)
		if err != nil {
			return ai_image_gen.VisionInputContext{}, nil, err
		}
		imageInputs = append(imageInputs, images...)

		pCtx := ai_image_gen.PaperContext{
			ID:              paper.ID,
			Title:           paper.Title,
			Abstract:        paper.AbstractText,
			FullTextSnippet: truncateRunesForImageGen(paper.PDFText, 4000),
		}
		for _, fig := range figures {
			pCtx.Figures = append(pCtx.Figures, ai_image_gen.FigureContext{
				ID:          fig.ID,
				PaperTitle:  paper.Title,
				PageNumber:  fig.PageNumber,
				FigureIndex: fig.FigureIndex,
				Caption:     fig.Caption,
			})
			includedFigureIDs[fig.ID] = struct{}{}
		}
		out.Papers = append(out.Papers, pCtx)
	}

	// Load individually requested figures (dedup against already-loaded paper figures).
	for _, fid := range figureIDs {
		if _, dup := includedFigureIDs[fid]; dup {
			continue
		}

		figItem, err := s.repo.GetFigure(fid)
		if err != nil {
			return ai_image_gen.VisionInputContext{}, nil, err
		}
		if figItem == nil {
			return ai_image_gen.VisionInputContext{}, nil, apperr.New(apperr.CodeNotFound, "图片不存在")
		}

		paper, err := s.repo.GetPaperDetail(figItem.PaperID)
		if err != nil {
			return ai_image_gen.VisionInputContext{}, nil, err
		}
		if paper == nil {
			return ai_image_gen.VisionInputContext{}, nil, apperr.New(apperr.CodeNotFound, "图片所属文献不存在")
		}

		// Find the matching model.Figure within the paper (loadFigureInputs needs it for image loading).
		figPtr := findFigureByID(paper.Figures, fid)
		if figPtr == nil {
			// Figure exists in the repo but the paper's embedded list doesn't contain it —
			// treat as a lightweight context-only entry with no image.
			out.IsolatedFigures = append(out.IsolatedFigures, ai_image_gen.FigureContext{
				ID:          figItem.ID,
				PaperTitle:  figItem.PaperTitle,
				PageNumber:  figItem.PageNumber,
				FigureIndex: figItem.FigureIndex,
				Caption:     figItem.Caption,
			})
			includedFigureIDs[fid] = struct{}{}
			continue
		}
		fig := *figPtr

		images, _, err := s.loadFigureInputs(paper, []model.Figure{fig}, model.AIActionFigureInterpretation)
		if err != nil {
			return ai_image_gen.VisionInputContext{}, nil, err
		}
		imageInputs = append(imageInputs, images...)

		out.IsolatedFigures = append(out.IsolatedFigures, ai_image_gen.FigureContext{
			ID:          fig.ID,
			PaperTitle:  paper.Title,
			PageNumber:  fig.PageNumber,
			FigureIndex: fig.FigureIndex,
			Caption:     fig.Caption,
		})
		includedFigureIDs[fid] = struct{}{}
	}

	return out, toModelImageInputs(imageInputs), nil
}

// CallVisionForImageGen satisfies ai_image_gen.VisionCaller.
// It converts the public model.AIImageInput slice to the package-private
// aiImageInput type and delegates to callTextProvider (non-streaming).
func (s *AIService) CallVisionForImageGen(ctx context.Context, settings model.AISettings,
	systemPrompt, userPrompt string, images []model.AIImageInput) (string, error) {
	internal := make([]aiImageInput, 0, len(images))
	for _, img := range images {
		internal = append(internal, aiImageInput{MIMEType: img.MIMEType, Data: img.Data})
	}
	return s.callTextProvider(ctx, settings, systemPrompt, userPrompt, internal)
}

// toModelImageInputs converts the package-private aiImageInput slice to the
// public model.AIImageInput slice expected by the ai_image_gen package.
func toModelImageInputs(in []aiImageInput) []model.AIImageInput {
	out := make([]model.AIImageInput, 0, len(in))
	for _, img := range in {
		out = append(out, model.AIImageInput{MIMEType: img.MIMEType, Data: img.Data})
	}
	return out
}

// truncateRunesForImageGen truncates s to at most n runes, appending "..." if
// truncated. Local copy so this file has no dependency on ai_conversation.
func truncateRunesForImageGen(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
