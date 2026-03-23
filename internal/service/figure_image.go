package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

type normalizedFigureCrop struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

const virtualSubfigureFilenamePrefix = "_subfigure_meta_"

func (s *LibraryService) GetFigureImage(id int64) ([]byte, string, string, error) {
	figureRef, err := s.repo.GetFigure(id)
	if err != nil {
		return nil, "", "", err
	}
	if figureRef == nil {
		return nil, "", "", apperr.New(apperr.CodeNotFound, "figure not found")
	}

	paper, err := s.repo.GetPaperDetail(figureRef.PaperID)
	if err != nil {
		return nil, "", "", err
	}
	if paper == nil {
		return nil, "", "", apperr.New(apperr.CodeNotFound, "paper not found")
	}

	figure := findFigureByID(paper.Figures, id)
	if figure == nil {
		return nil, "", "", apperr.New(apperr.CodeNotFound, "figure not found")
	}

	data, mimeType, err := loadFigureImageData(s.config.FiguresDir(), paper.Figures, *figure)
	if err != nil {
		return nil, "", "", err
	}

	filename := firstNonEmpty(
		strings.TrimSpace(figure.OriginalName),
		strings.TrimSpace(figure.Filename),
		fmt.Sprintf("figure-%d.png", figure.ID),
	)
	if filepath.Ext(filename) == "" {
		filename += extensionForFigure(mimeType, filename)
	}

	return data, mimeType, filename, nil
}

func figureImageURL(figure model.Figure) string {
	if figure.ParentFigureID != nil || strings.TrimSpace(figure.Filename) == "" {
		return fmt.Sprintf("/api/figures/%d/image", figure.ID)
	}
	return "/files/figures/" + url.PathEscape(strings.TrimSpace(figure.Filename))
}

func topLevelFigures(figures []model.Figure) []model.Figure {
	if len(figures) == 0 {
		return nil
	}

	result := make([]model.Figure, 0, len(figures))
	for _, figure := range figures {
		if figure.ParentFigureID != nil {
			continue
		}
		result = append(result, figure)
	}
	return result
}

func figurePhysicalFilePath(figuresDir string, figure model.Figure) string {
	filename := strings.TrimSpace(figure.Filename)
	if filename == "" || isVirtualFigureFilename(filename) {
		return ""
	}
	return filepath.Join(figuresDir, filepath.Base(filename))
}

func isVirtualFigureFilename(filename string) bool {
	return strings.HasPrefix(strings.TrimSpace(filename), virtualSubfigureFilenamePrefix)
}

func collectFigureFilePaths(figures []model.Figure, figuresDir string) []string {
	if len(figures) == 0 {
		return nil
	}

	paths := make([]string, 0, len(figures))
	seen := make(map[string]struct{}, len(figures))
	for _, figure := range figures {
		path := figurePhysicalFilePath(figuresDir, figure)
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func loadFigureImageData(figuresDir string, figures []model.Figure, figure model.Figure) ([]byte, string, error) {
	if figure.ParentFigureID == nil {
		return loadStoredFigureImage(figuresDir, figure)
	}

	if data, mimeType, err := renderSubfigureImage(figuresDir, figures, figure); err == nil {
		return data, mimeType, nil
	}

	if strings.TrimSpace(figure.Filename) != "" {
		return loadStoredFigureImage(figuresDir, figure)
	}

	return nil, "", apperr.New(apperr.CodeNotFound, fmt.Sprintf("subfigure image unavailable (figure #%d)", figure.ID))
}

func loadStoredFigureImage(figuresDir string, figure model.Figure) ([]byte, string, error) {
	path := figurePhysicalFilePath(figuresDir, figure)
	if path == "" {
		return nil, "", apperr.New(apperr.CodeNotFound, fmt.Sprintf("figure image missing (figure #%d)", figure.ID))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", apperr.New(apperr.CodeNotFound, fmt.Sprintf("figure image not found (figure #%d)", figure.ID))
		}
		return nil, "", apperr.Wrap(apperr.CodeInternal, "读取图片失败", err)
	}

	return data, normalizeFigureImageMIME(figure.ContentType, figure.Filename, data), nil
}

func renderSubfigureImage(figuresDir string, figures []model.Figure, figure model.Figure) ([]byte, string, error) {
	if figure.ParentFigureID == nil {
		return loadStoredFigureImage(figuresDir, figure)
	}

	crop, err := parseNormalizedFigureCrop(figure.BBox)
	if err != nil {
		return nil, "", err
	}

	parent := findFigureByID(figures, *figure.ParentFigureID)
	if parent == nil {
		return nil, "", apperr.New(apperr.CodeNotFound, fmt.Sprintf("parent figure not found (figure #%d)", figure.ID))
	}

	parentData, _, err := loadStoredFigureImage(figuresDir, *parent)
	if err != nil {
		return nil, "", err
	}

	return cropFigureImage(parentData, crop)
}

func parseNormalizedFigureCrop(raw json.RawMessage) (normalizedFigureCrop, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return normalizedFigureCrop{}, apperr.New(apperr.CodeInvalidArgument, "missing subfigure crop metadata")
	}

	var crop normalizedFigureCrop
	if err := json.Unmarshal(raw, &crop); err != nil {
		return normalizedFigureCrop{}, apperr.Wrap(apperr.CodeInvalidArgument, "invalid subfigure crop metadata", err)
	}
	if crop.Width <= 0 || crop.Height <= 0 {
		return normalizedFigureCrop{}, apperr.New(apperr.CodeInvalidArgument, "subfigure crop size must be positive")
	}
	if crop.X < 0 || crop.Y < 0 || crop.X >= 1 || crop.Y >= 1 {
		return normalizedFigureCrop{}, apperr.New(apperr.CodeInvalidArgument, "subfigure crop origin is out of bounds")
	}
	if crop.X+crop.Width > 1 || crop.Y+crop.Height > 1 {
		return normalizedFigureCrop{}, apperr.New(apperr.CodeInvalidArgument, "subfigure crop exceeds parent bounds")
	}
	return crop, nil
}

func cropFigureImage(data []byte, crop normalizedFigureCrop) ([]byte, string, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", apperr.Wrap(apperr.CodeInvalidArgument, "解码主图失败", err)
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, "", apperr.New(apperr.CodeInvalidArgument, "主图尺寸无效")
	}

	left := bounds.Min.X + int(crop.X*float64(width))
	top := bounds.Min.Y + int(crop.Y*float64(height))
	right := bounds.Min.X + int((crop.X+crop.Width)*float64(width)+0.999999)
	bottom := bounds.Min.Y + int((crop.Y+crop.Height)*float64(height)+0.999999)

	if right <= left || bottom <= top {
		return nil, "", apperr.New(apperr.CodeInvalidArgument, "子图裁剪区域无效")
	}
	if right > bounds.Max.X {
		right = bounds.Max.X
	}
	if bottom > bounds.Max.Y {
		bottom = bounds.Max.Y
	}

	dst := image.NewRGBA(image.Rect(0, 0, right-left, bottom-top))
	draw.Draw(dst, dst.Bounds(), src, image.Point{X: left, Y: top}, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, "", apperr.Wrap(apperr.CodeInternal, "编码子图失败", err)
	}
	return buf.Bytes(), "image/png", nil
}

func normalizeFigureImageMIME(contentType, filename string, data []byte) string {
	contentType = strings.TrimSpace(strings.SplitN(strings.TrimSpace(contentType), ";", 2)[0])
	if strings.HasPrefix(contentType, "image/") {
		return contentType
	}
	if ext := strings.ToLower(filepath.Ext(strings.TrimSpace(filename))); ext != "" {
		if guessed := mime.TypeByExtension(ext); strings.HasPrefix(guessed, "image/") {
			return guessed
		}
	}
	if detected := strings.TrimSpace(strings.SplitN(http.DetectContentType(data), ";", 2)[0]); strings.HasPrefix(detected, "image/") {
		return detected
	}
	return "image/png"
}
