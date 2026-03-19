package ai

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"net/http"
	"strings"

	_ "image/gif"
	_ "image/png"
)

const (
	aiFigureImageMaxBytes       = 3 * 1024 * 1024
	aiFigureImageTotalBudget    = 12 * 1024 * 1024
	aiFigureImageMaxDimension   = 2200
	aiFigureImageMinDimension   = 960
	aiFigureImageJPEGQuality    = 82
	aiFigureImageMinJPEGQuality = 58
	aiFigureImageCompressionRuns = 6
)

// ImageInput AI 图片输入
type ImageInput struct {
	MIMEType string
	Data     string
}

// CompressImage 压缩图片以满足 AI 服务的要求
func CompressImage(data []byte, mimeType string) ([]byte, string, error) {
	mimeType = normalizeImageMIMEType(mimeType, data)

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
		candidate := resizeImage(img, maxDimension)
		encoded, err := encodeJPEG(candidate, quality)
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

// normalizeImageMIMEType 标准化图片 MIME 类型
func normalizeImageMIMEType(mimeType string, data []byte) string {
	mimeType = strings.TrimSpace(strings.SplitN(strings.TrimSpace(mimeType), ";", 2)[0])
	if mimeType == "" && len(data) > 0 {
		mimeType = strings.TrimSpace(strings.SplitN(http.DetectContentType(data), ";", 2)[0])
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return "image/png"
	}
	return mimeType
}

// resizeImage 调整图片大小
func resizeImage(src image.Image, maxDimension int) image.Image {
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

// encodeJPEG 编码为 JPEG
func encodeJPEG(src image.Image, quality int) ([]byte, error) {
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

// maxInt 返回最大整数
func maxInt(values ...int) int {
	result := 0
	for _, value := range values {
		if value > result {
			result = value
		}
	}
	return result
}
