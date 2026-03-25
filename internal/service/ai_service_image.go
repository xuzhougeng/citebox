package service

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"net/http"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"

	_ "image/gif"
	_ "image/png"
)

func (s *AIService) loadFigureInputs(paper *model.Paper, figures []model.Figure, action model.AIAction) ([]aiImageInput, []string, error) {
	images := make([]aiImageInput, 0, len(figures))
	summaries := make([]string, 0, len(figures))
	totalBytes := 0
	budgetReached := false
	for _, figure := range figures {
		summary := buildAIFigureSummary(figure, action)
		summaries = append(summaries, summary)

		if budgetReached {
			continue
		}
		data, mimeType, err := loadFigureImageData(s.config.FiguresDir(), paper.Figures, figure)
		if err != nil {
			if apperr.IsCode(err, apperr.CodeNotFound) {
				s.logger.Warn("ai figure image missing",
					"paper_id", paper.ID,
					"figure_id", figure.ID,
					"filename", figure.Filename,
				)
				continue
			}
			return nil, nil, err
		}

		compressedData, compressedMIMEType, err := compressAIImage(data, mimeType)
		if err != nil {
			s.logger.Warn("ai figure compression failed",
				"paper_id", paper.ID,
				"figure_id", figure.ID,
				"filename", figure.Filename,
				"error", err,
			)
			continue
		}
		if totalBytes > 0 && totalBytes+len(compressedData) > aiFigureImageTotalBudget {
			s.logger.Warn("ai figure image budget reached",
				"paper_id", paper.ID,
				"figure_id", figure.ID,
				"filename", figure.Filename,
				"included", len(images),
				"budget_bytes", aiFigureImageTotalBudget,
			)
			budgetReached = true
			continue
		}

		images = append(images, aiImageInput{
			MIMEType: compressedMIMEType,
			Data:     base64.StdEncoding.EncodeToString(compressedData),
		})
		totalBytes += len(compressedData)
	}

	return images, summaries, nil
}

func buildAIFigureSummary(figure model.Figure, action model.AIAction) string {
	label := fmt.Sprintf("第 %d 页图 %d", figure.PageNumber, figure.FigureIndex)
	caption := fallbackText(strings.TrimSpace(figure.Caption), "无")
	if action == model.AIActionPaperQA {
		return fmt.Sprintf("- figure_id=%d；标签=%s；caption=%s；如需插图请使用 ![%s](figure://%d)", figure.ID, label, caption, label, figure.ID)
	}
	return fmt.Sprintf("- %s：caption=%s", label, caption)
}

func compressAIImage(data []byte, mimeType string) ([]byte, string, error) {
	mimeType = normalizeAIImageMIMEType(mimeType, data)

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		if len(data) <= aiFigureImageMaxBytes {
			return data, mimeType, nil
		}
		return nil, "", err
	}

	bounds := img.Bounds()
	if len(data) <= aiFigureImageMaxBytes && maxInt(bounds.Dx(), bounds.Dy()) <= aiFigureImageMaxDimension {
		return data, mimeType, nil
	}

	maxDimension := aiFigureImageMaxDimension
	quality := aiFigureImageJPEGQuality
	var best []byte
	for attempt := 0; attempt < aiFigureImageCompressionRuns; attempt++ {
		candidate := resizeImageForAI(img, maxDimension)
		encoded, err := encodeAIJPEG(candidate, quality)
		if err != nil {
			return nil, "", err
		}
		if len(best) == 0 || len(encoded) < len(best) {
			best = encoded
		}
		if len(encoded) <= aiFigureImageMaxBytes {
			return encoded, "image/jpeg", nil
		}

		nextDimension := int(float64(maxDimension) * 0.82)
		if nextDimension < aiFigureImageMinDimension {
			nextDimension = aiFigureImageMinDimension
		}
		maxDimension = nextDimension

		quality -= 6
		if quality < aiFigureImageMinJPEGQuality {
			quality = aiFigureImageMinJPEGQuality
		}
	}

	if len(best) > 0 {
		return best, "image/jpeg", nil
	}
	return nil, "", errors.New("无法压缩图片")
}

func normalizeAIImageMIMEType(mimeType string, data []byte) string {
	mimeType = strings.TrimSpace(strings.SplitN(strings.TrimSpace(mimeType), ";", 2)[0])
	if mimeType == "" && len(data) > 0 {
		mimeType = strings.TrimSpace(strings.SplitN(http.DetectContentType(data), ";", 2)[0])
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return "image/png"
	}
	return mimeType
}

func resizeImageForAI(src image.Image, maxDimension int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 || maxDimension <= 0 {
		return src
	}

	largest := maxInt(width, height)
	if largest <= maxDimension {
		return src
	}

	scale := float64(maxDimension) / float64(largest)
	dstWidth := maxInt(1, int(float64(width)*scale))
	dstHeight := maxInt(1, int(float64(height)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))

	for y := 0; y < dstHeight; y++ {
		srcY := bounds.Min.Y + int(float64(y)*float64(height)/float64(dstHeight))
		if srcY >= bounds.Max.Y {
			srcY = bounds.Max.Y - 1
		}
		for x := 0; x < dstWidth; x++ {
			srcX := bounds.Min.X + int(float64(x)*float64(width)/float64(dstWidth))
			if srcX >= bounds.Max.X {
				srcX = bounds.Max.X - 1
			}
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	return dst
}

func encodeAIJPEG(src image.Image, quality int) ([]byte, error) {
	bounds := src.Bounds()
	canvas := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), src, bounds.Min, draw.Over)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
