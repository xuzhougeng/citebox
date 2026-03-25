package service

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func (s *LibraryService) ListFigures(filter model.FigureFilter) (*model.FigureListResponse, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 8
	}

	figures, total, err := s.repo.ListFigures(filter)
	if err != nil {
		return nil, err
	}

	for i := range figures {
		figures[i].ImageURL = "/files/figures/" + url.PathEscape(figures[i].Filename)
		if figures[i].Tags == nil {
			figures[i].Tags = []model.Tag{}
		}
		if figures[i].PaletteColors == nil {
			figures[i].PaletteColors = []string{}
		}
		figures[i].DisplayLabel = formatFigureDisplayLabel(figures[i].FigureIndex, figures[i].SubfigureLabel)
		if figures[i].ParentFigureID != nil {
			figures[i].ParentDisplayLabel = formatFigureDisplayLabel(figures[i].FigureIndex, "")
		}
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + filter.PageSize - 1) / filter.PageSize
	}

	return &model.FigureListResponse{
		Figures:    figures,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *LibraryService) ListPalettes(filter model.PaletteFilter) (*model.PaletteListResponse, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 12
	}

	palettes, total, err := s.repo.ListPalettes(filter)
	if err != nil {
		return nil, err
	}

	for i := range palettes {
		s.decoratePalette(&palettes[i])
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + filter.PageSize - 1) / filter.PageSize
	}

	return &model.PaletteListResponse{
		Palettes:   palettes,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *LibraryService) DeleteFigure(id int64) (*model.Paper, error) {
	figure, err := s.repo.GetFigure(id)
	if err != nil {
		return nil, err
	}
	if figure == nil {
		return nil, apperr.New(apperr.CodeNotFound, "figure not found")
	}

	paper, err := s.repo.GetPaperDetail(figure.PaperID)
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	deleteFigureIDs, deletePaths := collectFigureDeletionTargets(paper.Figures, id, s.config.FiguresDir())
	if len(deleteFigureIDs) == 0 {
		return nil, apperr.New(apperr.CodeNotFound, "figure not found")
	}

	if err := s.repo.ApplyManualFigureChanges(figure.PaperID, nil, deleteFigureIDs); err != nil {
		return nil, err
	}

	removeFiles(deletePaths)

	paper, err = s.repo.GetPaperDetail(figure.PaperID)
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	s.decoratePaper(paper)
	return paper, nil
}

func (s *LibraryService) CreateOrUpdateFigurePalette(figureID int64, params CreatePaletteParams) (*model.Palette, *model.Paper, error) {
	figure, err := s.repo.GetFigure(figureID)
	if err != nil {
		return nil, nil, err
	}
	if figure == nil {
		return nil, nil, apperr.New(apperr.CodeNotFound, "figure not found")
	}
	if figure.ParentFigureID == nil {
		return nil, nil, apperr.New(apperr.CodeFailedPrecondition, "当前只支持对子图提取配色")
	}

	colors, err := normalizePaletteColors(params.Colors)
	if err != nil {
		return nil, nil, err
	}

	name := strings.TrimSpace(params.Name)
	if name == "" {
		existing, err := s.repo.GetPaletteByFigureID(figureID)
		if err != nil {
			return nil, nil, err
		}
		if existing != nil && strings.TrimSpace(existing.Name) != "" {
			name = strings.TrimSpace(existing.Name)
		} else {
			name = defaultPaletteName(*figure)
		}
	}

	colorsJSON, err := json.Marshal(colors)
	if err != nil {
		return nil, nil, apperr.Wrap(apperr.CodeInternal, "序列化配色失败", err)
	}

	palette, err := s.repo.UpsertPalette(repository.PaletteUpsertInput{
		PaperID:    figure.PaperID,
		FigureID:   figure.ID,
		Name:       name,
		ColorsJSON: string(colorsJSON),
	})
	if err != nil {
		return nil, nil, err
	}
	if palette == nil {
		return nil, nil, apperr.New(apperr.CodeInternal, "保存配色失败")
	}

	paper, err := s.repo.GetPaperDetail(figure.PaperID)
	if err != nil {
		return nil, nil, err
	}
	if paper == nil {
		return nil, nil, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	s.decoratePaper(paper)
	s.decoratePalette(palette)
	return palette, paper, nil
}

func (s *LibraryService) DeletePalette(id int64) error {
	return s.repo.DeletePalette(id)
}

func (s *LibraryService) CreateSubfigures(parentFigureID int64, params CreateSubfiguresParams) (*model.Paper, int, error) {
	if len(params.Regions) == 0 {
		return nil, 0, apperr.New(apperr.CodeInvalidArgument, "至少需要框选一个子图区域")
	}

	parentFigure, err := s.repo.GetFigure(parentFigureID)
	if err != nil {
		return nil, 0, err
	}
	if parentFigure == nil {
		return nil, 0, apperr.New(apperr.CodeNotFound, "figure not found")
	}
	if parentFigure.ParentFigureID != nil {
		return nil, 0, apperr.New(apperr.CodeFailedPrecondition, "当前只支持从一级大图提取子图")
	}

	paper, err := s.repo.GetPaperDetail(parentFigure.PaperID)
	if err != nil {
		return nil, 0, err
	}
	if paper == nil {
		return nil, 0, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	parentFigureDetail := findFigureByID(paper.Figures, parentFigureID)
	if parentFigureDetail == nil {
		return nil, 0, apperr.New(apperr.CodeNotFound, "parent figure not found")
	}
	if parentFigureDetail.ParentFigureID != nil {
		return nil, 0, apperr.New(apperr.CodeFailedPrecondition, "当前只支持从一级大图提取子图")
	}

	existingLabels := map[string]struct{}{}
	for _, figure := range paper.Figures {
		if figure.ParentFigureID == nil || *figure.ParentFigureID != parentFigureID {
			continue
		}
		label := subfigureLabelKey(figure.SubfigureLabel)
		if label != "" {
			existingLabels[label] = struct{}{}
		}
	}

	items := make([]repository.FigureUpsertInput, 0, len(params.Regions))
	for _, rawRegion := range params.Regions {
		region, err := normalizeSubfigureRegion(rawRegion)
		if err != nil {
			return nil, 0, err
		}

		label, err := resolveNextSubfigureLabel(region.Label, existingLabels)
		if err != nil {
			return nil, 0, err
		}
		existingLabels[subfigureLabelKey(label)] = struct{}{}

		bboxJSON, err := json.Marshal(map[string]interface{}{
			"x":                region.X,
			"y":                region.Y,
			"width":            region.Width,
			"height":           region.Height,
			"unit":             "normalized",
			"source":           "subfigure_manual",
			"coordinate_space": "figure",
			"parent_figure_id": parentFigureID,
		})
		if err != nil {
			return nil, 0, apperr.Wrap(apperr.CodeInternal, "序列化子图坐标失败", err)
		}

		baseName := strings.TrimSpace(parentFigureDetail.OriginalName)
		if baseName == "" {
			baseName = parentFigureDetail.Filename
		}
		baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))
		ext := extensionForFigure("image/png", "subfigure.png")

		items = append(items, repository.FigureUpsertInput{
			Filename:       fmt.Sprintf("%s%d_%s%s", virtualSubfigureFilenamePrefix, parentFigureID, label, ext),
			OriginalName:   fmt.Sprintf("%s_%s%s", baseName, label, ext),
			ContentType:    "image/png",
			PageNumber:     parentFigureDetail.PageNumber,
			FigureIndex:    parentFigureDetail.FigureIndex,
			ParentFigureID: &parentFigureID,
			SubfigureLabel: label,
			Source:         "manual",
			Caption:        strings.TrimSpace(region.Caption),
			BBoxJSON:       string(bboxJSON),
		})
	}

	if err := s.repo.AddPaperFigures(parentFigure.PaperID, items); err != nil {
		return nil, 0, err
	}

	updatedPaper, err := s.repo.GetPaperDetail(parentFigure.PaperID)
	if err != nil {
		return nil, 0, err
	}
	if updatedPaper == nil {
		return nil, 0, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	s.decoratePaper(updatedPaper)
	return updatedPaper, len(items), nil
}

func (s *LibraryService) UpdateFigureTags(id int64, tags []string) (*model.Paper, error) {
	return s.UpdateFigure(id, UpdateFigureParams{Tags: tags})
}

func (s *LibraryService) UpdateFigure(id int64, params UpdateFigureParams) (*model.Paper, error) {
	figure, err := s.repo.GetFigure(id)
	if err != nil {
		return nil, err
	}
	if figure == nil {
		return nil, apperr.New(apperr.CodeNotFound, "figure not found")
	}

	tagNames := params.Tags
	if tagNames == nil {
		tagNames = make([]string, 0, len(figure.Tags))
		for _, tag := range figure.Tags {
			tagNames = append(tagNames, tag.Name)
		}
	}

	notesText := figure.NotesText
	if params.NotesText != nil {
		notesText = *params.NotesText
	}

	caption := figure.Caption
	if params.Caption != nil {
		caption = strings.TrimSpace(*params.Caption)
	}

	paper, err := s.repo.UpdateFigure(id, repository.FigureUpdateInput{
		Caption:   caption,
		NotesText: notesText,
		Tags:      s.normalizeTagInputs(tagNames, model.TagScopeFigure),
	})
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	s.decoratePaper(paper)
	return paper, nil
}

func (s *LibraryService) GetManualExtractionWorkspace(id int64) (*model.ManualExtractionWorkspace, error) {
	paper, err := s.repo.GetPaperDetail(id)
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	pdfPath := filepath.Join(s.config.PapersDir(), paper.StoredPDFName)
	if _, err := os.Stat(pdfPath); err != nil {
		return nil, apperr.Wrap(apperr.CodeFailedPrecondition, "原始 PDF 不存在，无法进入人工处理", err)
	}

	s.decoratePaper(paper)
	return &model.ManualExtractionWorkspace{
		Paper:     paper,
		PageCount: 0,
	}, nil
}

func (s *LibraryService) GetManualPreview(id int64, page int) ([]byte, error) {
	return nil, apperr.New(apperr.CodeFailedPrecondition, "当前版本已改为浏览器端 PDF 预览，请刷新页面后重试")
}

func (s *LibraryService) ManualExtractFigures(id int64, params ManualExtractParams) (*model.Paper, int, error) {
	if len(params.Regions) == 0 {
		return nil, 0, apperr.New(apperr.CodeInvalidArgument, "至少需要框选一个区域")
	}

	paper, err := s.repo.GetPaperDetail(id)
	if err != nil {
		return nil, 0, err
	}
	if paper == nil {
		return nil, 0, apperr.New(apperr.CodeNotFound, "paper not found")
	}
	items := make([]repository.FigureUpsertInput, 0, len(params.Regions))
	newPaths := make([]string, 0, len(params.Regions))
	replacedPaths := make([]string, 0, len(params.Regions))
	nextFigureIndex := maxFigureIndex(paper.Figures)
	existingFiguresByID := make(map[int64]model.Figure, len(paper.Figures))
	usedReplaceIDs := make(map[int64]struct{}, len(params.Regions))
	deleteFigureIDs := make([]int64, 0, len(params.Regions))
	for _, figure := range paper.Figures {
		existingFiguresByID[figure.ID] = figure
	}

	for idx, rawRegion := range params.Regions {
		region, err := normalizeManualRegion(rawRegion)
		if err != nil {
			removeFiles(newPaths)
			return nil, 0, err
		}

		var replaceTarget *model.Figure
		if region.ReplaceFigureID != nil {
			figure, ok := existingFiguresByID[*region.ReplaceFigureID]
			if !ok {
				removeFiles(newPaths)
				return nil, 0, apperr.New(apperr.CodeNotFound, "待替换的图片不存在")
			}
			if _, exists := usedReplaceIDs[figure.ID]; exists {
				removeFiles(newPaths)
				return nil, 0, apperr.New(apperr.CodeInvalidArgument, "同一张原图不能被重复替换")
			}
			usedReplaceIDs[figure.ID] = struct{}{}
			replaceTarget = &figure
			deleteFigureIDs = append(deleteFigureIDs, figure.ID)
			replacedPaths = append(replacedPaths, filepath.Join(s.config.FiguresDir(), figure.Filename))
		}

		binary, err := decodeBase64(region.ImageData)
		if err != nil {
			removeFiles(newPaths)
			return nil, 0, apperr.Wrap(apperr.CodeInvalidArgument, "解码人工提取图片失败", err)
		}

		contentType := http.DetectContentType(binary)
		if !strings.HasPrefix(contentType, "image/") {
			removeFiles(newPaths)
			return nil, 0, apperr.New(apperr.CodeInvalidArgument, "人工提取结果不是有效图片")
		}

		source := region.Source
		ext := extensionForFigure(contentType, "manual.png")
		storedName := fmt.Sprintf("figure_%d_%s_%d%s", time.Now().UnixNano(), source, idx+1, ext)
		targetPath := filepath.Join(s.config.FiguresDir(), storedName)
		if err := os.WriteFile(targetPath, binary, 0o644); err != nil {
			removeFiles(newPaths)
			return nil, 0, apperr.Wrap(apperr.CodeInternal, "保存人工提取图片失败", err)
		}
		newPaths = append(newPaths, targetPath)

		bboxJSON, err := json.Marshal(map[string]interface{}{
			"x":           region.X,
			"y":           region.Y,
			"width":       region.Width,
			"height":      region.Height,
			"unit":        "normalized",
			"page_number": region.PageNumber,
			"source":      source,
		})
		if err != nil {
			removeFiles(newPaths)
			return nil, 0, apperr.Wrap(apperr.CodeInternal, "序列化人工框选坐标失败", err)
		}

		caption := strings.TrimSpace(region.Caption)
		figureIndex := nextFigureIndex + 1
		if replaceTarget != nil {
			figureIndex = replaceTarget.FigureIndex
			if caption == "" {
				caption = strings.TrimSpace(replaceTarget.Caption)
			}
		} else {
			nextFigureIndex++
		}

		items = append(items, repository.FigureUpsertInput{
			Filename:       storedName,
			OriginalName:   fmt.Sprintf("%s_p%d_%s_%d%s", strings.TrimSuffix(paper.OriginalFilename, filepath.Ext(paper.OriginalFilename)), region.PageNumber, source, idx+1, ext),
			ContentType:    contentType,
			PageNumber:     region.PageNumber,
			FigureIndex:    figureIndex,
			ParentFigureID: nil,
			SubfigureLabel: "",
			Source:         source,
			Caption:        caption,
			BBoxJSON:       string(bboxJSON),
		})
	}

	if err := s.repo.ApplyManualFigureChanges(id, items, deleteFigureIDs); err != nil {
		removeFiles(newPaths)
		return nil, 0, err
	}
	removeFiles(replacedPaths)

	if paper.ExtractionStatus == "failed" || paper.ExtractionStatus == "cancelled" || paper.ExtractionStatus == manualPendingStatus || (paper.ExtractionStatus == "completed" && strings.TrimSpace(paper.PDFText) == "") {
		message := fmt.Sprintf("已人工录入 %d 张图片，可继续补充或替换其他图片", len(items))
		if paper.ExtractionStatus == "failed" || paper.ExtractionStatus == "cancelled" {
			message = fmt.Sprintf("自动解析未完成，已人工录入 %d 张图片", len(items))
		}
		if err := s.repo.UpdatePaperExtractionState(id, paper.ExtractionStatus, message, paper.ExtractorJobID); err != nil && !apperr.IsCode(err, apperr.CodeNotFound) {
			s.logger.Warn("update paper message after manual extraction failed",
				"paper_id", id,
				"code", apperr.CodeOf(err),
				"error", err,
			)
		}
	}

	updatedPaper, err := s.repo.GetPaperDetail(id)
	if err != nil {
		return nil, 0, err
	}
	if updatedPaper == nil {
		return nil, 0, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	s.decoratePaper(updatedPaper)
	return updatedPaper, len(items), nil
}

func (s *LibraryService) materializeFigures(figures []extractedFigure) ([]repository.FigureUpsertInput, []string, error) {
	items := make([]repository.FigureUpsertInput, 0, len(figures))
	paths := make([]string, 0, len(figures))

	for idx, figure := range figures {
		if strings.TrimSpace(figure.Data) == "" {
			continue
		}

		binary, err := decodeBase64(figure.Data)
		if err != nil {
			return nil, paths, apperr.Wrap(apperr.CodeInternal, "无法解码提取图片", err)
		}

		contentType := contentTypeOrDefault(figure.ContentType, http.DetectContentType(binary))
		ext := extensionForFigure(contentType, figure.Filename)
		storedName := fmt.Sprintf("figure_%d_%d%s", time.Now().UnixNano(), idx+1, ext)
		path := filepath.Join(s.config.FiguresDir(), storedName)

		if err := os.WriteFile(path, binary, 0o644); err != nil {
			return nil, paths, apperr.Wrap(apperr.CodeInternal, "保存提取图片失败", err)
		}
		paths = append(paths, path)

		items = append(items, repository.FigureUpsertInput{
			Filename:       storedName,
			OriginalName:   firstNonEmpty(figure.Filename, storedName),
			ContentType:    contentType,
			PageNumber:     figure.PageNumber,
			FigureIndex:    figure.FigureIndex,
			ParentFigureID: nil,
			SubfigureLabel: "",
			Source:         figureSourceAuto,
			Caption:        figure.Caption,
			BBoxJSON:       strings.TrimSpace(string(figure.BBox)),
		})
	}

	return items, paths, nil
}

func (s *LibraryService) decoratePaper(paper *model.Paper) {
	if paper == nil {
		return
	}

	if paper.Tags == nil {
		paper.Tags = []model.Tag{}
	}
	if paper.Figures == nil {
		paper.Figures = []model.Figure{}
	}
	if paper.StoredPDFName != "" {
		paper.PDFURL = "/files/papers/" + url.PathEscape(paper.StoredPDFName)
	}
	figuresByID := make(map[int64]*model.Figure, len(paper.Figures))
	for i := range paper.Figures {
		if paper.Figures[i].Tags == nil {
			paper.Figures[i].Tags = []model.Tag{}
		}
		if paper.Figures[i].PaletteColors == nil {
			paper.Figures[i].PaletteColors = []string{}
		}
		paper.Figures[i].ImageURL = figureImageURL(paper.Figures[i])
		paper.Figures[i].DisplayLabel = formatFigureDisplayLabel(paper.Figures[i].FigureIndex, paper.Figures[i].SubfigureLabel)
		paper.Figures[i].ParentDisplayLabel = ""
		paper.Figures[i].Subfigures = nil
		figuresByID[paper.Figures[i].ID] = &paper.Figures[i]
	}
	for i := range paper.Figures {
		if paper.Figures[i].ParentFigureID == nil {
			continue
		}
		parent := figuresByID[*paper.Figures[i].ParentFigureID]
		if parent == nil {
			paper.Figures[i].ParentDisplayLabel = formatFigureDisplayLabel(paper.Figures[i].FigureIndex, "")
			continue
		}
		paper.Figures[i].ParentDisplayLabel = parent.DisplayLabel
	}
	for i := range paper.Figures {
		if paper.Figures[i].ParentFigureID == nil {
			continue
		}
		parent := figuresByID[*paper.Figures[i].ParentFigureID]
		if parent == nil {
			continue
		}
		parent.Subfigures = append(parent.Subfigures, paper.Figures[i])
	}
}

func (s *LibraryService) decoratePalette(palette *model.Palette) {
	if palette == nil {
		return
	}
	palette.ImageURL = figureImageURL(model.Figure{
		ID:             palette.FigureID,
		Filename:       palette.Filename,
		ParentFigureID: palette.ParentFigureID,
	})
	palette.FigureDisplayLabel = formatFigureDisplayLabel(palette.FigureIndex, palette.SubfigureLabel)
	if palette.ParentFigureID != nil {
		palette.ParentDisplayLabel = formatFigureDisplayLabel(palette.FigureIndex, "")
	}
	if palette.Colors == nil {
		palette.Colors = []string{}
	}
}

func maxFigureIndex(figures []model.Figure) int {
	result := 0
	for _, figure := range figures {
		if figure.FigureIndex > result {
			result = figure.FigureIndex
		}
	}
	return result
}

func normalizeManualRegion(region model.ManualExtractionRegion) (model.ManualExtractionRegion, error) {
	region.Source = normalizeManualFigureSource(region.Source)
	region.Caption = strings.TrimSpace(region.Caption)
	region.ImageData = strings.TrimSpace(region.ImageData)
	if region.PageNumber < 1 {
		return model.ManualExtractionRegion{}, apperr.New(apperr.CodeInvalidArgument, "页码必须从 1 开始")
	}
	if region.Width <= 0 || region.Height <= 0 {
		return model.ManualExtractionRegion{}, apperr.New(apperr.CodeInvalidArgument, "框选区域的宽高必须大于 0")
	}
	if region.X < 0 || region.Y < 0 || region.X >= 1 || region.Y >= 1 {
		return model.ManualExtractionRegion{}, apperr.New(apperr.CodeInvalidArgument, "框选区域坐标必须落在页面范围内")
	}
	if region.X+region.Width > 1 || region.Y+region.Height > 1 {
		return model.ManualExtractionRegion{}, apperr.New(apperr.CodeInvalidArgument, "框选区域不能超出页面边界")
	}
	if region.ImageData == "" {
		return model.ManualExtractionRegion{}, apperr.New(apperr.CodeInvalidArgument, "缺少人工提取图片数据")
	}
	return region, nil
}

func normalizeManualFigureSource(value string) string {
	switch strings.TrimSpace(value) {
	case manualFigureSourceLLM:
		return figureSourceAuto
	default:
		return manualFigureSourceManual
	}
}

func normalizeSubfigureRegion(region model.SubfigureExtractionRegion) (model.SubfigureExtractionRegion, error) {
	region.Caption = strings.TrimSpace(region.Caption)
	region.Label = strings.TrimSpace(region.Label)
	if region.Width <= 0 || region.Height <= 0 {
		return model.SubfigureExtractionRegion{}, apperr.New(apperr.CodeInvalidArgument, "子图区域的宽高必须大于 0")
	}
	if region.X < 0 || region.Y < 0 || region.X >= 1 || region.Y >= 1 {
		return model.SubfigureExtractionRegion{}, apperr.New(apperr.CodeInvalidArgument, "子图区域坐标必须落在图片范围内")
	}
	if region.X+region.Width > 1 || region.Y+region.Height > 1 {
		return model.SubfigureExtractionRegion{}, apperr.New(apperr.CodeInvalidArgument, "子图区域不能超出图片边界")
	}
	return region, nil
}

func normalizePaletteColors(colors []string) ([]string, error) {
	if len(colors) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "至少需要一个配色值")
	}

	result := make([]string, 0, len(colors))
	seen := map[string]struct{}{}
	for _, raw := range colors {
		color, err := normalizePaletteHexColor(raw)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[color]; exists {
			continue
		}
		seen[color] = struct{}{}
		result = append(result, color)
	}
	if len(result) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "没有有效的配色值")
	}
	return result, nil
}

func normalizePaletteHexColor(raw string) (string, error) {
	color := strings.ToUpper(strings.TrimSpace(raw))
	if color == "" {
		return "", apperr.New(apperr.CodeInvalidArgument, "配色值不能为空")
	}
	if !strings.HasPrefix(color, "#") || len(color) != 7 {
		return "", apperr.New(apperr.CodeInvalidArgument, "配色值必须是 #RRGGBB 格式")
	}
	if _, err := hex.DecodeString(color[1:]); err != nil {
		return "", apperr.New(apperr.CodeInvalidArgument, "配色值必须是有效的十六进制颜色")
	}
	return color, nil
}

func formatFigureDisplayLabel(figureIndex int, subfigureLabel string) string {
	if figureIndex <= 0 {
		return ""
	}
	label := strings.TrimSpace(subfigureLabel)
	if label == "" {
		return fmt.Sprintf("Fig %d", figureIndex)
	}
	return fmt.Sprintf("Fig %d%s", figureIndex, strings.ToLower(label))
}

func defaultPaletteName(figure model.FigureListItem) string {
	label := formatFigureDisplayLabel(figure.FigureIndex, figure.SubfigureLabel)
	if label == "" {
		label = "Figure"
	}
	return label + " 配色"
}

func resolveNextSubfigureLabel(requested string, used map[string]struct{}) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		normalized := normalizeSubfigureLabel(requested)
		if normalized == "" {
			return "", apperr.New(apperr.CodeInvalidArgument, "子图命名只支持英文字母 a-z")
		}
		if _, exists := used[subfigureLabelKey(normalized)]; exists {
			return "", apperr.New(apperr.CodeConflict, "子图命名已存在")
		}
		return normalized, nil
	}

	for index := 0; index < 26*26; index++ {
		candidate := subfigureLabelForIndex(index)
		if _, exists := used[candidate]; exists {
			continue
		}
		return candidate, nil
	}
	return "", apperr.New(apperr.CodeResourceExhausted, "子图命名数量已超过当前支持上限")
}

func normalizeSubfigureLabel(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" {
		return ""
	}
	for _, ch := range label {
		if ch < 'a' || ch > 'z' {
			return ""
		}
	}
	return label
}

func subfigureLabelKey(label string) string {
	return strings.ToLower(strings.TrimSpace(label))
}

func subfigureLabelForIndex(index int) string {
	if index < 26 {
		return string(rune('a' + index))
	}
	first := (index / 26) - 1
	second := index % 26
	return string([]rune{rune('a' + first), rune('a' + second)})
}

func collectFigureDeletionTargets(figures []model.Figure, figureID int64, figuresDir string) ([]int64, []string) {
	targets := []int64{}
	paths := []string{}
	seen := map[int64]struct{}{}
	seenPaths := map[string]struct{}{}

	for _, figure := range figures {
		if figure.ID != figureID && (figure.ParentFigureID == nil || *figure.ParentFigureID != figureID) {
			continue
		}
		if _, exists := seen[figure.ID]; exists {
			continue
		}
		seen[figure.ID] = struct{}{}
		targets = append(targets, figure.ID)
		if path := figurePhysicalFilePath(figuresDir, figure); path != "" {
			if _, exists := seenPaths[path]; !exists {
				seenPaths[path] = struct{}{}
				paths = append(paths, path)
			}
		}
	}

	return targets, paths
}

func (s *LibraryService) normalizeTagInputs(names []string, scope model.TagScope) []repository.TagUpsertInput {
	seen := map[string]bool{}
	result := []repository.TagUpsertInput{}
	scope = model.NormalizeTagScope(string(scope))

	for _, name := range names {
		normalized := strings.TrimSpace(name)
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, repository.TagUpsertInput{
			Scope: scope,
			Name:  normalized,
			Color: colorForName(normalized),
		})
	}

	return result
}
