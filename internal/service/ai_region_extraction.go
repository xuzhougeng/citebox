package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

const aiFigureRegionDetectionSystemPrompt = `You detect scientific figure regions on academic paper pages.

Your task is to return bounding boxes for full figure bodies only.

Important policy:
- Treat a composite multi-panel figure as one figure when panels belong together.
- Panel labels such as A/B/C/D, a/b/c/d, i/ii/iii are subpanels, not separate main figures.
- Do not split one larger figure into several boxes just because it contains several subpanels.
- Shared legends, shared axes, shared whitespace, connectors, arrows, and sparse diagram parts still belong to the same overall figure.
- Only return multiple boxes when the page clearly contains separate main figures rather than one composite figure.

Include:
- plots, charts, heatmaps, microscopy images, gels, blots, diagrams, schematics, photos
- axes, tick labels, legends, and labels that are visually inside the figure body

Exclude:
- figure captions starting with Figure/Fig.
- caption paragraphs, body text, headings, page headers/footers, page numbers
- tables

Output only valid JSON with this shape:
{"figures":[{"bbox":[x1,y1,x2,y2],"confidence":0.95}]}

Coordinates must use a normalized 0-1000 scale where (0,0) is top-left and (1000,1000) is bottom-right.
If there is no figure on the page, return {"figures":[]}.
Do not wrap JSON in markdown.`

type aiFigureRegionCandidate struct {
	BBox       []float64 `json:"bbox"`
	Confidence float64   `json:"confidence"`
}

type aiFigureRegionPayload struct {
	Figures []aiFigureRegionCandidate `json:"figures"`
}

func (s *AIService) DetectFigureRegions(ctx context.Context, input model.AIFigureRegionDetectRequest) (*model.AIFigureRegionDetectResponse, error) {
	if input.PaperID <= 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "paper_id 无效")
	}
	if input.PageNumber <= 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "page_number 无效")
	}
	if strings.TrimSpace(input.ImageData) == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "缺少页面图片数据")
	}

	paper, err := s.repo.GetPaperDetail(input.PaperID)
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "文献不存在")
	}

	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}
	modelConfig, err := resolveModelForAction(*settings, model.AIActionFigureInterpretation)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelConfig.APIKey) == "" {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先在 AI 页面为图片场景配置可用模型和 API Key")
	}

	binary, mimeType, err := decodeAIRequestImage(input.ImageData)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInvalidArgument, "解码页面图片失败", err)
	}
	compressedData, compressedMIMEType, err := compressAIImage(binary, mimeType)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInvalidArgument, "压缩页面图片失败", err)
	}

	runtimeSettings := *settings
	runtimeSettings.Provider = modelConfig.Provider
	runtimeSettings.APIKey = modelConfig.APIKey
	runtimeSettings.BaseURL = modelConfig.BaseURL
	runtimeSettings.Model = modelConfig.Model
	runtimeSettings.MaxOutputTokens = minInt(maxInt(modelConfig.MaxOutputTokens, 512), 1200)
	runtimeSettings.OpenAILegacyMode = modelConfig.OpenAILegacyMode
	runtimeSettings.Temperature = 0

	rawText, err := s.callTextProvider(ctx, runtimeSettings, aiFigureRegionDetectionSystemPrompt, buildAIFigureRegionDetectionUserPrompt(input, paper), []aiImageInput{
		{
			MIMEType: compressedMIMEType,
			Data:     base64.StdEncoding.EncodeToString(compressedData),
		},
	})
	if err != nil {
		return nil, err
	}

	regions, err := parseAIFigureRegions(rawText)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "解析模型返回的坐标失败", err)
	}

	return &model.AIFigureRegionDetectResponse{
		Success:    true,
		Provider:   runtimeSettings.Provider,
		Model:      runtimeSettings.Model,
		PageNumber: input.PageNumber,
		Regions:    regions,
		RawText:    strings.TrimSpace(rawText),
	}, nil
}

func buildAIFigureRegionDetectionUserPrompt(input model.AIFigureRegionDetectRequest, paper *model.Paper) string {
	title := ""
	if paper != nil {
		title = strings.TrimSpace(paper.Title)
	}
	if title == "" {
		title = "unknown"
	}

	return fmt.Sprintf(`Task: detect all scientific figure bodies on this PDF page.

Paper title: %s
Page number: %d
Rendered page size: %.0f x %.0f

Rules:
1. Composite figures with subpanels A/B/C/D should usually be returned as one larger figure box.
2. Do not return one box per subpanel unless they are clearly separate main figures.
3. Include the full outer visual extent of the figure body.
4. Exclude caption paragraphs and nearby body text outside the figure body.
5. Slight extra whitespace is acceptable; missing part of the figure is not.
6. If no figure exists on this page, return {"figures":[]}.

Return JSON only.`, title, input.PageNumber, input.PageWidth, input.PageHeight)
}

func decodeAIRequestImage(value string) ([]byte, string, error) {
	value = strings.TrimSpace(value)
	mimeType := ""
	if strings.HasPrefix(value, "data:") {
		if comma := strings.Index(value, ","); comma > 5 {
			header := value[5:comma]
			if semi := strings.Index(header, ";"); semi >= 0 {
				mimeType = strings.TrimSpace(header[:semi])
			} else {
				mimeType = strings.TrimSpace(header)
			}
			value = value[comma+1:]
		}
	}

	if data, err := base64.StdEncoding.DecodeString(value); err == nil {
		return data, contentTypeOrDefault(mimeType, http.DetectContentType(data)), nil
	}
	if data, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		return data, contentTypeOrDefault(mimeType, http.DetectContentType(data)), nil
	}
	return nil, "", errors.New("无法解码页面图片的 base64 数据")
}

func parseAIFigureRegions(text string) ([]model.AIFigureRegion, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return []model.AIFigureRegion{}, nil
	}

	var payload aiFigureRegionPayload
	var parsed bool
	for _, candidate := range []string{
		trimmed,
		trimCodeFence(trimmed),
		trimJSONObject(trimmed),
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if err := json.Unmarshal([]byte(candidate), &payload); err == nil {
			parsed = true
			break
		}
	}
	if !parsed {
		return nil, errors.New("模型没有返回有效 JSON")
	}

	regions := make([]model.AIFigureRegion, 0, len(payload.Figures))
	for _, figure := range payload.Figures {
		region, ok := normalizeAIFigureRegion(figure)
		if !ok {
			continue
		}
		regions = append(regions, region)
	}

	sort.SliceStable(regions, func(i, j int) bool {
		if absFloat(regions[i].Y-regions[j].Y) > 0.02 {
			return regions[i].Y < regions[j].Y
		}
		if absFloat(regions[i].X-regions[j].X) > 0.02 {
			return regions[i].X < regions[j].X
		}
		if absFloat(regions[i].Width-regions[j].Width) > 0.01 {
			return regions[i].Width > regions[j].Width
		}
		return regions[i].Height > regions[j].Height
	})

	return dedupeAIFigureRegions(regions), nil
}

func normalizeAIFigureRegion(candidate aiFigureRegionCandidate) (model.AIFigureRegion, bool) {
	if len(candidate.BBox) != 4 {
		return model.AIFigureRegion{}, false
	}

	x1, y1, x2, y2 := candidate.BBox[0], candidate.BBox[1], candidate.BBox[2], candidate.BBox[3]
	if maxFloat(absFloat(x1), absFloat(y1), absFloat(x2), absFloat(y2)) > 1.5 {
		x1 /= 1000
		y1 /= 1000
		x2 /= 1000
		y2 /= 1000
	}

	x1 = clampFloat(x1, 0, 1)
	y1 = clampFloat(y1, 0, 1)
	x2 = clampFloat(x2, 0, 1)
	y2 = clampFloat(y2, 0, 1)
	if x2 <= x1 || y2 <= y1 {
		return model.AIFigureRegion{}, false
	}

	width := x2 - x1
	height := y2 - y1
	if width < 0.02 || height < 0.02 {
		return model.AIFigureRegion{}, false
	}

	confidence := candidate.Confidence
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return model.AIFigureRegion{
		X:          x1,
		Y:          y1,
		Width:      width,
		Height:     height,
		Confidence: confidence,
	}, true
}

func dedupeAIFigureRegions(regions []model.AIFigureRegion) []model.AIFigureRegion {
	if len(regions) <= 1 {
		return regions
	}

	result := make([]model.AIFigureRegion, 0, len(regions))
	for _, region := range regions {
		duplicate := false
		for _, existing := range result {
			if absFloat(region.X-existing.X) <= 0.015 &&
				absFloat(region.Y-existing.Y) <= 0.015 &&
				absFloat(region.Width-existing.Width) <= 0.02 &&
				absFloat(region.Height-existing.Height) <= 0.02 {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, region)
		}
	}
	return result
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func maxFloat(values ...float64) float64 {
	result := 0.0
	for _, value := range values {
		if value > result {
			result = value
		}
	}
	return result
}
