//go:build cgo && !nocgo

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"path/filepath"
	"strings"

	fitz "github.com/xuzhougeng/citebox/third_party/go-fitz"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

const builtInLLMPageRenderDPI = 180.0

func (s *LibraryService) processBuiltInLLMExtraction(settings model.ExtractorSettings, paperID int64, pdfPath, originalFilename string) error {
	if err := s.repo.UpdatePaperExtractionState(paperID, "running", "内置 AI 正在解析 PDF", ""); err != nil {
		return err
	}

	result, err := s.extractBuiltInLLMResult(context.Background(), paperID, pdfPath, originalFilename)
	if err != nil {
		return err
	}

	return s.persistExtractionResult(paperID, "", settings, result)
}

func (s *LibraryService) extractBuiltInLLMResult(ctx context.Context, paperID int64, pdfPath, originalFilename string) (*extractionResult, error) {
	doc, err := fitz.New(pdfPath)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "打开 PDF 失败，无法执行内置 AI 解析", err)
	}
	defer doc.Close()

	aiSvc := NewAIService(s.repo, s.config, s.logger.With("component", "builtin_llm_extractor"))
	pageCount := doc.NumPage()
	boxes := make([]map[string]interface{}, 0)
	figures := make([]extractedFigure, 0)
	nextFigureIndex := 0
	baseName := strings.TrimSpace(strings.TrimSuffix(filepath.Base(originalFilename), filepath.Ext(originalFilename)))
	if baseName == "" {
		baseName = fmt.Sprintf("paper_%d", paperID)
	}

	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		pageNumber := pageIndex + 1
		if err := s.repo.UpdatePaperExtractionState(paperID, "running", fmt.Sprintf("内置 AI 正在解析第 %d / %d 页", pageNumber, pageCount), ""); err != nil && !apperr.IsCode(err, apperr.CodeNotFound) {
			s.logger.Warn("update built-in extraction progress failed",
				"paper_id", paperID,
				"page_number", pageNumber,
				"error", err,
			)
		}

		pageImage, err := doc.ImageDPI(pageIndex, builtInLLMPageRenderDPI)
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeUnavailable, fmt.Sprintf("渲染第 %d 页失败", pageNumber), err)
		}

		pageWidth := pageImage.Bounds().Dx()
		pageHeight := pageImage.Bounds().Dy()
		pageDataURL, err := encodeImageDataURL(pageImage)
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, fmt.Sprintf("编码第 %d 页图片失败", pageNumber), err)
		}

		detected, err := aiSvc.DetectFigureRegions(ctx, model.AIFigureRegionDetectRequest{
			PaperID:    paperID,
			PageNumber: pageNumber,
			PageWidth:  float64(pageWidth),
			PageHeight: float64(pageHeight),
			ImageData:  pageDataURL,
		})
		if err != nil {
			return nil, err
		}

		for regionIndex, region := range detected.Regions {
			cropped, err := cropDetectedRegion(pageImage, region)
			if err != nil {
				return nil, apperr.Wrap(apperr.CodeInternal, fmt.Sprintf("裁剪第 %d 页识别区域失败", pageNumber), err)
			}

			nextFigureIndex++
			bboxPayload := map[string]interface{}{
				"x":           region.X,
				"y":           region.Y,
				"width":       region.Width,
				"height":      region.Height,
				"unit":        "normalized",
				"page_number": pageNumber,
				"source":      manualFigureSourceLLM,
			}
			if region.Confidence > 0 {
				bboxPayload["confidence"] = region.Confidence
			}

			bboxJSON, err := json.Marshal(bboxPayload)
			if err != nil {
				return nil, apperr.Wrap(apperr.CodeInternal, "序列化内置 AI 坐标失败", err)
			}

			boxes = append(boxes, bboxPayload)
			figures = append(figures, extractedFigure{
				Filename:    fmt.Sprintf("%s_p%d_llm_%d.png", baseName, pageNumber, regionIndex+1),
				ContentType: "image/png",
				PageNumber:  pageNumber,
				FigureIndex: nextFigureIndex,
				Caption:     "",
				BBox:        bboxJSON,
				Data:        base64.StdEncoding.EncodeToString(cropped),
				Source:      manualFigureSourceLLM,
			})
		}
	}

	boxesJSON, err := json.Marshal(boxes)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化内置 AI 提取结果失败", err)
	}

	return &extractionResult{
		Boxes:   boxesJSON,
		Figures: figures,
	}, nil
}

func (s *LibraryService) extractServerPDFTextFallback(pdfPath string) (string, error) {
	doc, err := fitz.New(pdfPath)
	if err != nil {
		return "", err
	}
	defer doc.Close()

	pages := make([]string, 0, doc.NumPage())
	for pageIndex := 0; pageIndex < doc.NumPage(); pageIndex++ {
		text, err := doc.Text(pageIndex)
		if err != nil {
			return "", err
		}
		text = strings.TrimSpace(text)
		if text != "" {
			pages = append(pages, text)
		}
	}

	return strings.TrimSpace(strings.Join(pages, "\n\n")), nil
}

func encodeImageDataURL(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func cropDetectedRegion(img image.Image, region model.AIFigureRegion) ([]byte, error) {
	bounds := img.Bounds()
	left := bounds.Min.X + int(clampFloat(region.X, 0, 1)*float64(bounds.Dx()))
	top := bounds.Min.Y + int(clampFloat(region.Y, 0, 1)*float64(bounds.Dy()))
	right := bounds.Min.X + int(clampFloat(region.X+region.Width, 0, 1)*float64(bounds.Dx()))
	bottom := bounds.Min.Y + int(clampFloat(region.Y+region.Height, 0, 1)*float64(bounds.Dy()))

	if right <= left || bottom <= top {
		return nil, apperr.New(apperr.CodeInvalidArgument, "识别出的图片区域无效")
	}

	dst := image.NewRGBA(image.Rect(0, 0, right-left, bottom-top))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(image.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, image.Point{X: left, Y: top}, draw.Over)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
