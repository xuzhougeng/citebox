package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

type LibraryService struct {
	repo                *repository.LibraryRepository
	config              *config.Config
	httpClient          *http.Client
	logger              *slog.Logger
	startBackground     bool
	weixinClientFactory func(token string) weixinBindingClient
}

const (
	extractorSettingsKey = "extractor_settings"
	runtimePasswordKey   = "runtime_admin_password"
	manualPendingStatus  = "manual_pending"
	extractionModeAuto   = "auto"
	extractionModeManual = "manual"
)

type LibraryServiceOption func(*LibraryService)

type DuplicatePaperError struct {
	Paper *model.Paper
	Err   error
}

func (e *DuplicatePaperError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "PDF 已存在"
}

func (e *DuplicatePaperError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type UploadPaperParams struct {
	Title          string
	GroupID        *int64
	Tags           []string
	ExtractionMode string
}

type UpdatePaperParams struct {
	Title          string
	PDFText        *string
	AbstractText   string
	NotesText      string
	PaperNotesText string
	GroupID        *int64
	Tags           []string
}

type UpdateFigureParams struct {
	Tags      []string
	Caption   *string
	NotesText *string
}

type CreatePaletteParams struct {
	Name   string
	Colors []string
}

type CreateSubfiguresParams struct {
	Regions []model.SubfigureExtractionRegion
}

type ManualExtractParams struct {
	Regions []model.ManualExtractionRegion
}

type extractionResult struct {
	PDFText string
	Boxes   json.RawMessage
	Figures []extractedFigure
}

type extractedFigure struct {
	Filename    string
	ContentType string
	PageNumber  int
	FigureIndex int
	Caption     string
	BBox        json.RawMessage
	Data        string
}

type extractorResponse struct {
	Success  *bool             `json:"success"`
	Status   string            `json:"status"`
	Message  string            `json:"message"`
	PDFText  string            `json:"pdf_text"`
	Text     string            `json:"text"`
	FullText string            `json:"full_text"`
	Boxes    json.RawMessage   `json:"boxes"`
	Figures  []extractorFigure `json:"figures"`
	Images   []extractorFigure `json:"images"`
}

type extractorFigure struct {
	Filename        string          `json:"filename"`
	Name            string          `json:"name"`
	ContentType     string          `json:"content_type"`
	MIMEType        string          `json:"mime_type"`
	PageNumber      int             `json:"page_number"`
	Page            int             `json:"page"`
	FigureIndex     int             `json:"figure_index"`
	Index           int             `json:"index"`
	Caption         string          `json:"caption"`
	BBox            json.RawMessage `json:"bbox"`
	Box             json.RawMessage `json:"box"`
	Data            string          `json:"data"`
	Base64          string          `json:"base64"`
	ImageBase64     string          `json:"image_base64"`
	ThumbnailBase64 string          `json:"thumbnail_base64"`
	ImageURL        string          `json:"image_url"`
	ThumbnailURL    string          `json:"thumbnail_url"`
}

type extractorJobStatusResponse struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	StatusURL string `json:"status_url"`
	ResultURL string `json:"result_url"`
	Error     string `json:"error"`
}

func WithLogger(logger *slog.Logger) LibraryServiceOption {
	return func(service *LibraryService) {
		if logger != nil {
			service.logger = logger
		}
	}
}

func WithoutBackgroundJobs() LibraryServiceOption {
	return func(service *LibraryService) {
		service.startBackground = false
	}
}

func NewLibraryService(repo *repository.LibraryRepository, cfg *config.Config, opts ...LibraryServiceOption) (*LibraryService, error) {
	for _, dir := range []string{cfg.StorageDir, cfg.PapersDir(), cfg.FiguresDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, fmt.Sprintf("创建存储目录失败: %s", dir), err)
		}
	}

	service := &LibraryService{
		repo:                repo,
		config:              cfg,
		logger:              slog.Default().With("component", "library_service"),
		startBackground:     true,
		httpClient:          &http.Client{},
		weixinClientFactory: defaultWeixinBindingClientFactory,
	}

	for _, opt := range opts {
		opt(service)
	}

	if err := service.backfillPaperChecksums(); err != nil {
		return nil, err
	}
	if err := service.migrateLegacyManualPendingPapers(); err != nil {
		return nil, err
	}

	if service.startBackground {
		go service.resumePendingExtractions()
	}

	return service, nil
}

func (s *LibraryService) ListPapers(filter model.PaperFilter) (*model.PaperListResponse, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 12
	}

	papers, total, err := s.repo.ListPapers(filter)
	if err != nil {
		return nil, err
	}

	for i := range papers {
		s.decoratePaper(&papers[i])
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + filter.PageSize - 1) / filter.PageSize
	}

	return &model.PaperListResponse{
		Papers:     papers,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *LibraryService) GetPaper(id int64) (*model.Paper, error) {
	paper, err := s.repo.GetPaperDetail(id)
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "paper not found")
	}
	s.decoratePaper(paper)
	return paper, nil
}

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

func (s *LibraryService) UploadPaper(file multipart.File, header *multipart.FileHeader, params UploadPaperParams) (*model.Paper, error) {
	if header.Size > s.config.MaxUploadSize {
		return nil, apperr.New(apperr.CodeResourceExhausted, fmt.Sprintf("PDF 大小超过限制 %s", humanFileSize(s.config.MaxUploadSize)))
	}
	if !isPDF(header.Filename, header.Header.Get("Content-Type")) {
		return nil, apperr.New(apperr.CodeUnsupportedMedia, "只支持上传 PDF 文献")
	}

	if err := s.validateGroup(params.GroupID); err != nil {
		return nil, err
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		title = deriveTitle(header.Filename)
	}

	storedPDFName := fmt.Sprintf("paper_%d.pdf", time.Now().UnixNano())
	pdfPath := filepath.Join(s.config.PapersDir(), storedPDFName)

	dst, err := os.Create(pdfPath)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "创建 PDF 文件失败", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dst, hasher), file); err != nil {
		dst.Close()
		os.Remove(pdfPath)
		return nil, apperr.Wrap(apperr.CodeInternal, "保存 PDF 失败", err)
	}
	if err := dst.Close(); err != nil {
		os.Remove(pdfPath)
		return nil, apperr.Wrap(apperr.CodeInternal, "关闭 PDF 文件失败", err)
	}
	pdfSHA256 := hex.EncodeToString(hasher.Sum(nil))

	if duplicate, err := s.repo.FindPaperByPDFSHA256(pdfSHA256); err != nil {
		os.Remove(pdfPath)
		return nil, err
	} else if duplicate != nil {
		os.Remove(pdfPath)
		s.decoratePaper(duplicate)
		return nil, &DuplicatePaperError{
			Paper: duplicate,
			Err:   apperr.New(apperr.CodeConflict, "PDF 已存在，正在跳转到已有文献"),
		}
	}

	tagInputs := s.normalizeTagInputs(params.Tags, model.TagScopePaper)
	autoExtractionConfigured, err := s.autoExtractionConfigured()
	if err != nil {
		os.Remove(pdfPath)
		return nil, err
	}

	extractionMode := normalizeExtractionMode(params.ExtractionMode)
	if extractionMode == "" {
		if autoExtractionConfigured {
			extractionMode = extractionModeAuto
		} else {
			extractionMode = extractionModeManual
		}
	}

	extractionStatus := "completed"
	extractorMessage := manualWorkflowMessage(!autoExtractionConfigured)

	switch extractionMode {
	case extractionModeAuto:
		if !autoExtractionConfigured {
			os.Remove(pdfPath)
			return nil, apperr.New(apperr.CodeFailedPrecondition, "未配置自动解析服务，请改用手工标注")
		}
		extractionStatus = "queued"
		extractorMessage = "文献已入库，等待后台解析"
	case extractionModeManual:
	default:
		os.Remove(pdfPath)
		return nil, apperr.New(apperr.CodeInvalidArgument, "上传模式无效")
	}

	paper, err := s.repo.CreatePaper(repository.PaperUpsertInput{
		Title:            title,
		OriginalFilename: header.Filename,
		StoredPDFName:    storedPDFName,
		PDFSHA256:        pdfSHA256,
		FileSize:         header.Size,
		ContentType:      contentTypeOrDefault(header.Header.Get("Content-Type"), "application/pdf"),
		PDFText:          "",
		AbstractText:     "",
		NotesText:        "",
		PaperNotesText:   "",
		BoxesJSON:        "",
		ExtractionStatus: extractionStatus,
		ExtractorMessage: extractorMessage,
		ExtractorJobID:   "",
		GroupID:          params.GroupID,
		Tags:             tagInputs,
		Figures:          nil,
	})
	if err != nil {
		os.Remove(pdfPath)
		if apperr.IsCode(err, apperr.CodeConflict) {
			duplicate, findErr := s.repo.FindPaperByPDFSHA256(pdfSHA256)
			if findErr != nil {
				return nil, findErr
			}
			if duplicate != nil {
				s.decoratePaper(duplicate)
				return nil, &DuplicatePaperError{
					Paper: duplicate,
					Err:   apperr.New(apperr.CodeConflict, "PDF 已存在，正在跳转到已有文献"),
				}
			}
		}
		return nil, err
	}

	if extractionMode == extractionModeAuto {
		go s.runPaperExtraction(paper.ID, pdfPath, header.Filename)
	}

	s.decoratePaper(paper)
	return paper, nil
}

func (s *LibraryService) backfillPaperChecksums() error {
	items, err := s.repo.ListPapersMissingPDFSHA256()
	if err != nil {
		return err
	}

	for _, item := range items {
		pdfPath := filepath.Join(s.config.PapersDir(), item.StoredPDFName)
		checksum, err := fileSHA256(pdfPath)
		if err != nil {
			if os.IsNotExist(err) {
				s.logger.Warn("skip pdf checksum backfill because file is missing", "paper_id", item.ID, "stored_pdf_name", item.StoredPDFName)
				continue
			}
			return apperr.Wrap(apperr.CodeInternal, "计算历史 PDF 指纹失败", err)
		}

		if err := s.repo.UpdatePaperPDFSHA256(item.ID, checksum); err != nil {
			if apperr.IsCode(err, apperr.CodeConflict) {
				s.logger.Warn("skip duplicate historical pdf checksum", "paper_id", item.ID, "stored_pdf_name", item.StoredPDFName, "pdf_sha256", checksum)
				continue
			}
			return err
		}
	}

	return nil
}

func (s *LibraryService) migrateLegacyManualPendingPapers() error {
	papers, err := s.repo.ListPapersByExtractionStatuses([]string{manualPendingStatus})
	if err != nil {
		return err
	}

	for _, paper := range papers {
		message := strings.TrimSpace(paper.ExtractorMessage)
		if message == "" || !strings.Contains(message, "人工录入") {
			message = manualWorkflowMessage(true)
		}
		if err := s.repo.UpdatePaperExtractionState(paper.ID, "completed", message, ""); err != nil {
			return err
		}
	}

	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *LibraryService) UpdatePaper(id int64, params UpdatePaperParams) (*model.Paper, error) {
	title := strings.TrimSpace(params.Title)
	if title == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "标题不能为空")
	}
	if err := s.validateGroup(params.GroupID); err != nil {
		return nil, err
	}

	paper, err := s.repo.UpdatePaper(id, repository.PaperUpdateInput{
		Title:          title,
		PDFText:        params.PDFText,
		AbstractText:   strings.TrimSpace(params.AbstractText),
		NotesText:      strings.TrimSpace(params.NotesText),
		PaperNotesText: strings.TrimSpace(params.PaperNotesText),
		GroupID:        params.GroupID,
		Tags:           s.normalizeTagInputs(params.Tags, model.TagScopePaper),
	})
	if err != nil {
		return nil, err
	}

	s.decoratePaper(paper)
	return paper, nil
}

func (s *LibraryService) PurgeLibrary() error {
	if err := s.repo.PurgeLibrary(); err != nil {
		return err
	}
	if err := clearDirectoryContents(s.config.PapersDir()); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "清理 PDF 文件失败", err)
	}
	if err := clearDirectoryContents(s.config.FiguresDir()); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "清理图片文件失败", err)
	}
	return nil
}

func (s *LibraryService) DeletePaper(id int64) error {
	paper, err := s.repo.GetPaperDetail(id)
	if err != nil {
		return err
	}
	if paper == nil {
		return apperr.New(apperr.CodeNotFound, "paper not found")
	}

	if err := s.repo.DeletePaper(id); err != nil {
		return err
	}

	paths := []string{filepath.Join(s.config.PapersDir(), paper.StoredPDFName)}
	for _, figure := range paper.Figures {
		paths = append(paths, filepath.Join(s.config.FiguresDir(), figure.Filename))
	}
	removeFiles(paths)
	return nil
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
		label := strings.ToLower(strings.TrimSpace(figure.SubfigureLabel))
		if label != "" {
			existingLabels[label] = struct{}{}
		}
	}

	items := make([]repository.FigureUpsertInput, 0, len(params.Regions))
	newPaths := make([]string, 0, len(params.Regions))
	for idx, rawRegion := range params.Regions {
		region, err := normalizeSubfigureRegion(rawRegion)
		if err != nil {
			removeFiles(newPaths)
			return nil, 0, err
		}

		label, err := resolveNextSubfigureLabel(region.Label, existingLabels)
		if err != nil {
			removeFiles(newPaths)
			return nil, 0, err
		}
		existingLabels[label] = struct{}{}

		binary, err := decodeBase64(region.ImageData)
		if err != nil {
			removeFiles(newPaths)
			return nil, 0, apperr.Wrap(apperr.CodeInvalidArgument, "解码子图图片失败", err)
		}

		contentType := http.DetectContentType(binary)
		if !strings.HasPrefix(contentType, "image/") {
			removeFiles(newPaths)
			return nil, 0, apperr.New(apperr.CodeInvalidArgument, "子图提取结果不是有效图片")
		}

		ext := extensionForFigure(contentType, "subfigure.png")
		storedName := fmt.Sprintf("figure_%d_subfigure_%d%s", time.Now().UnixNano(), idx+1, ext)
		targetPath := filepath.Join(s.config.FiguresDir(), storedName)
		if err := os.WriteFile(targetPath, binary, 0o644); err != nil {
			removeFiles(newPaths)
			return nil, 0, apperr.Wrap(apperr.CodeInternal, "保存子图图片失败", err)
		}
		newPaths = append(newPaths, targetPath)

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
			removeFiles(newPaths)
			return nil, 0, apperr.Wrap(apperr.CodeInternal, "序列化子图坐标失败", err)
		}

		baseName := strings.TrimSpace(parentFigureDetail.OriginalName)
		if baseName == "" {
			baseName = parentFigureDetail.Filename
		}
		baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))

		items = append(items, repository.FigureUpsertInput{
			Filename:       storedName,
			OriginalName:   fmt.Sprintf("%s_%s%s", baseName, label, ext),
			ContentType:    contentType,
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
		removeFiles(newPaths)
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

func (s *LibraryService) ReextractPaper(id int64) (*model.Paper, error) {
	paper, err := s.repo.GetPaperDetail(id)
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	autoExtractionConfigured, err := s.autoExtractionConfigured()
	if err != nil {
		return nil, err
	}
	if !autoExtractionConfigured {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "未配置自动解析服务，请直接使用人工处理")
	}

	switch paper.ExtractionStatus {
	case "queued", "running":
		return nil, apperr.New(apperr.CodeConflict, "文献正在解析中，无需重复提交")
	case "failed", "cancelled", manualPendingStatus:
	default:
		return nil, apperr.New(apperr.CodeFailedPrecondition, "当前只有解析失败的文献支持重新解析")
	}

	pdfPath := filepath.Join(s.config.PapersDir(), paper.StoredPDFName)
	if _, err := os.Stat(pdfPath); err != nil {
		return nil, apperr.Wrap(apperr.CodeFailedPrecondition, "原始 PDF 不存在，无法重新解析", err)
	}

	if err := s.repo.UpdatePaperExtractionState(id, "queued", "已重新提交解析任务", ""); err != nil {
		return nil, err
	}

	go s.runPaperExtraction(id, pdfPath, paper.OriginalFilename)

	updatedPaper, err := s.repo.GetPaperDetail(id)
	if err != nil {
		return nil, err
	}
	if updatedPaper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "paper not found")
	}
	s.decoratePaper(updatedPaper)
	return updatedPaper, nil
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

		ext := extensionForFigure(contentType, "manual.png")
		storedName := fmt.Sprintf("figure_%d_manual_%d%s", time.Now().UnixNano(), idx+1, ext)
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
			"source":      "manual",
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
			OriginalName:   fmt.Sprintf("%s_p%d_manual_%d%s", strings.TrimSuffix(paper.OriginalFilename, filepath.Ext(paper.OriginalFilename)), region.PageNumber, idx+1, ext),
			ContentType:    contentType,
			PageNumber:     region.PageNumber,
			FigureIndex:    figureIndex,
			ParentFigureID: nil,
			SubfigureLabel: "",
			Source:         "manual",
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

func (s *LibraryService) ListGroups() ([]model.Group, error) {
	return s.repo.ListGroups()
}

func (s *LibraryService) CreateGroup(name, description string) (*model.Group, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "分组名称不能为空")
	}
	return s.repo.CreateGroup(name, description)
}

func (s *LibraryService) UpdateGroup(id int64, name, description string) (*model.Group, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "分组名称不能为空")
	}
	return s.repo.UpdateGroup(id, name, description)
}

func (s *LibraryService) DeleteGroup(id int64) error {
	return s.repo.DeleteGroup(id)
}

func (s *LibraryService) ListTags(scope model.TagScope) ([]model.Tag, error) {
	return s.repo.ListTags(scope)
}

func (s *LibraryService) CreateTag(scope model.TagScope, name, color string) (*model.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "标签名称不能为空")
	}
	color = normalizeColor(color)
	return s.repo.CreateTag(model.NormalizeTagScope(string(scope)), name, color)
}

func (s *LibraryService) UpdateTag(id int64, name, color string) (*model.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "标签名称不能为空")
	}
	return s.repo.UpdateTag(id, name, normalizeColor(color))
}

func (s *LibraryService) DeleteTag(id int64) error {
	return s.repo.DeleteTag(id)
}

func (s *LibraryService) GetExtractorSettings() (*model.ExtractorSettings, error) {
	raw, err := s.repo.GetAppSetting(extractorSettingsKey)
	if err != nil {
		return nil, err
	}

	settings := s.defaultExtractorSettings()
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "解析 PDF 提取配置失败", err)
		}
	}

	normalized, err := s.normalizeExtractorSettings(settings)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func (s *LibraryService) UpdateExtractorSettings(input model.ExtractorSettings) (*model.ExtractorSettings, error) {
	settings, err := s.normalizeExtractorSettings(input)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(settings)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化 PDF 提取配置失败", err)
	}

	if err := s.repo.UpsertAppSetting(extractorSettingsKey, string(payload)); err != nil {
		return nil, err
	}

	return &settings, nil
}

func (s *LibraryService) defaultExtractorSettings() model.ExtractorSettings {
	settings := model.ExtractorSettings{
		ExtractorURL:        strings.TrimSpace(s.config.ExtractorURL),
		ExtractorJobsURL:    "",
		ExtractorToken:      strings.TrimSpace(s.config.ExtractorToken),
		ExtractorFileField:  strings.TrimSpace(s.config.ExtractorFileField),
		TimeoutSeconds:      s.config.ExtractorTimeoutSeconds,
		PollIntervalSeconds: s.config.ExtractorPollInterval,
	}

	cfg := &config.Config{
		ExtractorURL:     settings.ExtractorURL,
		ExtractorJobsURL: settings.ExtractorJobsURL,
	}
	settings.EffectiveExtractorURL = cfg.EffectiveExtractorURL()
	settings.EffectiveJobsURL = cfg.EffectiveExtractorJobsURL()
	return settings
}

func (s *LibraryService) normalizeExtractorSettings(input model.ExtractorSettings) (model.ExtractorSettings, error) {
	defaults := s.defaultExtractorSettings()
	settings := input

	if strings.TrimSpace(settings.ExtractorFileField) == "" {
		settings.ExtractorFileField = firstNonEmpty(defaults.ExtractorFileField, "file")
	}
	if settings.TimeoutSeconds <= 0 {
		if defaults.TimeoutSeconds > 0 {
			settings.TimeoutSeconds = defaults.TimeoutSeconds
		} else {
			settings.TimeoutSeconds = 300
		}
	}
	if settings.PollIntervalSeconds <= 0 {
		if defaults.PollIntervalSeconds > 0 {
			settings.PollIntervalSeconds = defaults.PollIntervalSeconds
		} else {
			settings.PollIntervalSeconds = 2
		}
	}
	if settings.TimeoutSeconds > 3600 {
		return model.ExtractorSettings{}, apperr.New(apperr.CodeInvalidArgument, "PDF 提取超时时间不能超过 3600 秒")
	}
	if settings.PollIntervalSeconds > 300 {
		return model.ExtractorSettings{}, apperr.New(apperr.CodeInvalidArgument, "轮询间隔不能超过 300 秒")
	}

	settings.ExtractorURL = strings.TrimSpace(settings.ExtractorURL)
	settings.ExtractorJobsURL = ""
	settings.ExtractorToken = strings.TrimSpace(settings.ExtractorToken)
	settings.ExtractorFileField = strings.TrimSpace(settings.ExtractorFileField)

	cfg := &config.Config{
		ExtractorURL:     settings.ExtractorURL,
		ExtractorJobsURL: settings.ExtractorJobsURL,
	}
	settings.EffectiveExtractorURL = cfg.EffectiveExtractorURL()
	settings.EffectiveJobsURL = cfg.EffectiveExtractorJobsURL()

	return settings, nil
}

func (s *LibraryService) validateGroup(groupID *int64) error {
	if groupID == nil {
		return nil
	}

	exists, err := s.repo.GroupExists(*groupID)
	if err != nil {
		return err
	}
	if !exists {
		return apperr.New(apperr.CodeNotFound, "选择的分组不存在")
	}
	return nil
}

func (s *LibraryService) runPaperExtraction(paperID int64, pdfPath, originalFilename string) {
	settings, err := s.GetExtractorSettings()
	if err == nil {
		if jobsURL := strings.TrimSpace(settings.EffectiveJobsURL); jobsURL != "" {
			err = s.processPaperExtractionJob(*settings, paperID, jobsURL, pdfPath, originalFilename)
		} else {
			err = s.processPaperExtractionSync(*settings, paperID, pdfPath, originalFilename)
		}
	}

	if err == nil || apperr.IsCode(err, apperr.CodeNotFound) {
		return
	}

	s.logger.Error("paper extraction failed",
		"paper_id", paperID,
		"code", apperr.CodeOf(err),
		"error", err,
	)
	s.markPaperExtractionFailed(paperID, "", err)
}

func (s *LibraryService) resumePendingExtractions() {
	papers, err := s.repo.ListPapersByExtractionStatuses([]string{"queued", "running"})
	if err != nil {
		s.logger.Error("resume pending extractions failed", "error", err, "code", apperr.CodeOf(err))
		return
	}

	settings, err := s.GetExtractorSettings()
	if err != nil {
		s.logger.Error("load extractor settings failed", "error", err, "code", apperr.CodeOf(err))
		return
	}

	jobsURL := strings.TrimSpace(settings.EffectiveJobsURL)
	for _, paper := range papers {
		paperID := paper.ID
		jobID := strings.TrimSpace(paper.ExtractorJobID)
		if jobsURL != "" && jobID != "" {
			go func() {
				if err := s.resumeRemoteExtraction(*settings, paperID, jobID); err != nil && !apperr.IsCode(err, apperr.CodeNotFound) {
					s.logger.Error("resume paper extraction failed",
						"paper_id", paperID,
						"job_id", jobID,
						"code", apperr.CodeOf(err),
						"error", err,
					)
					s.markPaperExtractionFailed(paperID, jobID, err)
				}
			}()
			continue
		}

		if err := s.repo.UpdatePaperExtractionState(paperID, "failed", "后台解析在服务重启后中断", jobID); err != nil && !apperr.IsCode(err, apperr.CodeNotFound) {
			s.logger.Warn("mark stale paper failed",
				"paper_id", paperID,
				"job_id", jobID,
				"code", apperr.CodeOf(err),
				"error", err,
			)
		}
	}
}

func (s *LibraryService) processPaperExtractionSync(settings model.ExtractorSettings, paperID int64, pdfPath, originalFilename string) error {
	if err := s.repo.UpdatePaperExtractionState(paperID, "running", "解析服务正在处理 PDF", ""); err != nil {
		return err
	}

	result, err := s.extractPDFSync(settings, pdfPath, originalFilename)
	if err != nil {
		return err
	}

	return s.persistExtractionResult(paperID, "", result)
}

func (s *LibraryService) processPaperExtractionJob(settings model.ExtractorSettings, paperID int64, jobsURL, pdfPath, originalFilename string) error {
	jobStatus, err := s.createExtractJob(settings, jobsURL, pdfPath, originalFilename)
	if err != nil {
		return err
	}

	finalStatus, err := s.pollExtractJob(settings, paperID, jobStatus)
	if err != nil {
		return err
	}

	switch finalStatus.Status {
	case "completed":
		if err := s.repo.UpdatePaperExtractionState(paperID, "running", "解析结果已返回，正在写入文献库", finalStatus.JobID); err != nil {
			return err
		}
		result, err := s.getExtractJobResult(settings, finalStatus.JobID)
		if err != nil {
			return err
		}
		return s.persistExtractionResult(paperID, finalStatus.JobID, result)
	case "failed":
		return nil
	case "cancelled":
		return nil
	default:
		return apperr.New(apperr.CodeUnavailable, fmt.Sprintf("解析任务状态异常: %s", finalStatus.Status))
	}
}

func (s *LibraryService) resumeRemoteExtraction(settings model.ExtractorSettings, paperID int64, jobID string) error {
	return s.processPaperExtractionJobWithExistingJob(settings, paperID, &extractorJobStatusResponse{
		JobID:  jobID,
		Status: "queued",
	})
}

func (s *LibraryService) processPaperExtractionJobWithExistingJob(settings model.ExtractorSettings, paperID int64, initial *extractorJobStatusResponse) error {
	finalStatus, err := s.pollExtractJob(settings, paperID, initial)
	if err != nil {
		return err
	}

	if finalStatus.Status != "completed" {
		return nil
	}

	if err := s.repo.UpdatePaperExtractionState(paperID, "running", "解析结果已返回，正在写入文献库", finalStatus.JobID); err != nil {
		return err
	}

	result, err := s.getExtractJobResult(settings, finalStatus.JobID)
	if err != nil {
		return err
	}
	return s.persistExtractionResult(paperID, finalStatus.JobID, result)
}

func (s *LibraryService) pollExtractJob(settings model.ExtractorSettings, paperID int64, initial *extractorJobStatusResponse) (*extractorJobStatusResponse, error) {
	current := initial
	if current == nil {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "缺少解析任务信息")
	}

	for {
		if current.JobID == "" {
			return nil, apperr.New(apperr.CodeUnavailable, "解析任务未返回 job_id")
		}

		status, err := s.getExtractJobStatus(settings, current.JobID)
		if err != nil {
			return nil, err
		}
		current = status

		switch normalizeExtractionStatus(status.Status) {
		case "queued":
			if err := s.repo.UpdatePaperExtractionState(paperID, "queued", "文献已提交到解析队列", status.JobID); err != nil {
				return nil, err
			}
		case "running":
			if err := s.repo.UpdatePaperExtractionState(paperID, "running", "解析服务正在处理 PDF", status.JobID); err != nil {
				return nil, err
			}
		case "completed":
			return status, nil
		case "cancelled":
			if err := s.repo.UpdatePaperExtractionState(paperID, "cancelled", "解析任务已取消", status.JobID); err != nil {
				return nil, err
			}
			return status, nil
		case "failed":
			message := firstNonEmpty(status.Error, "解析后端返回失败状态")
			if err := s.repo.UpdatePaperExtractionState(paperID, "failed", message, status.JobID); err != nil {
				return nil, err
			}
			return status, nil
		default:
			return nil, apperr.New(apperr.CodeUnavailable, fmt.Sprintf("未知的解析任务状态: %s", status.Status))
		}

		time.Sleep(time.Duration(maxInt(settings.PollIntervalSeconds, 1)) * time.Second)
	}
}

func (s *LibraryService) persistExtractionResult(paperID int64, jobID string, result *extractionResult) error {
	if result == nil {
		return apperr.New(apperr.CodeUnavailable, "解析结果为空")
	}

	figures, figurePaths, err := s.materializeFigures(result.Figures)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "解析图片失败", err)
	}

	err = s.repo.ApplyPaperExtractionResult(
		paperID,
		result.PDFText,
		strings.TrimSpace(string(result.Boxes)),
		"completed",
		"",
		jobID,
		figures,
	)
	if err != nil {
		removeFiles(figurePaths)
		return err
	}

	return nil
}

func (s *LibraryService) markPaperExtractionFailed(paperID int64, jobID string, err error) {
	if strings.TrimSpace(jobID) == "" {
		paper, getErr := s.repo.GetPaperDetail(paperID)
		if getErr == nil && paper != nil {
			jobID = paper.ExtractorJobID
		}
	}
	message := firstNonEmpty(errorMessage(err), "解析失败")
	if updateErr := s.repo.UpdatePaperExtractionState(paperID, "failed", message, jobID); updateErr != nil && !apperr.IsCode(updateErr, apperr.CodeNotFound) {
		s.logger.Warn("mark paper extraction failed state failed",
			"paper_id", paperID,
			"job_id", jobID,
			"code", apperr.CodeOf(updateErr),
			"error", updateErr,
		)
	}
}

func (s *LibraryService) extractPDFSync(settings model.ExtractorSettings, pdfPath, originalFilename string) (*extractionResult, error) {
	extractURL := strings.TrimSpace(settings.EffectiveExtractorURL)
	if extractURL == "" {
		return nil, apperr.New(apperr.CodeUnavailable, "PDF_EXTRACTOR_URL 未配置，无法调用解析后端")
	}

	req, err := s.newExtractorUploadRequest(settings, http.MethodPost, extractURL, pdfPath, originalFilename)
	if err != nil {
		return nil, err
	}

	respBody, err := s.doExtractorRequest(req, settings)
	if err != nil {
		return nil, err
	}

	return parseExtractionResult(respBody)
}

func (s *LibraryService) createExtractJob(settings model.ExtractorSettings, jobsURL, pdfPath, originalFilename string) (*extractorJobStatusResponse, error) {
	req, err := s.newExtractorUploadRequest(settings, http.MethodPost, jobsURL, pdfPath, originalFilename)
	if err != nil {
		return nil, err
	}

	respBody, err := s.doExtractorRequest(req, settings)
	if err != nil {
		return nil, err
	}

	var payload extractorJobStatusResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "解析任务响应不是有效 JSON", err)
	}
	payload.Status = normalizeExtractionStatus(payload.Status)
	return &payload, nil
}

func (s *LibraryService) getExtractJobStatus(settings model.ExtractorSettings, jobID string) (*extractorJobStatusResponse, error) {
	jobsURL := strings.TrimSpace(settings.EffectiveJobsURL)
	if jobsURL == "" {
		return nil, apperr.New(apperr.CodeUnavailable, "PDF_EXTRACTOR_JOBS_URL 未配置，无法轮询解析任务")
	}

	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(jobsURL, "/")+"/"+url.PathEscape(jobID), nil)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "创建解析任务轮询请求失败", err)
	}
	s.authorizeExtractorRequest(settings, req)

	respBody, err := s.doExtractorRequest(req, settings)
	if err != nil {
		return nil, err
	}

	var payload extractorJobStatusResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "解析任务状态响应不是有效 JSON", err)
	}
	payload.Status = normalizeExtractionStatus(payload.Status)
	return &payload, nil
}

func (s *LibraryService) getExtractJobResult(settings model.ExtractorSettings, jobID string) (*extractionResult, error) {
	jobsURL := strings.TrimSpace(settings.EffectiveJobsURL)
	if jobsURL == "" {
		return nil, apperr.New(apperr.CodeUnavailable, "PDF_EXTRACTOR_JOBS_URL 未配置，无法读取解析结果")
	}

	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(jobsURL, "/")+"/"+url.PathEscape(jobID)+"/result", nil)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "创建解析结果请求失败", err)
	}
	s.authorizeExtractorRequest(settings, req)

	respBody, err := s.doExtractorRequest(req, settings)
	if err != nil {
		return nil, err
	}

	return parseExtractionResult(respBody)
}

func (s *LibraryService) newExtractorUploadRequest(settings model.ExtractorSettings, method, targetURL, pdfPath, originalFilename string) (*http.Request, error) {
	body, contentType, err := s.buildExtractorUploadBody(settings, pdfPath, originalFilename)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, targetURL, body)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "创建解析请求失败", err)
	}
	req.Header.Set("Content-Type", contentType)
	s.authorizeExtractorRequest(settings, req)
	return req, nil
}

func (s *LibraryService) buildExtractorUploadBody(settings model.ExtractorSettings, pdfPath, originalFilename string) (*bytes.Buffer, string, error) {
	file, err := os.Open(pdfPath)
	if err != nil {
		return nil, "", apperr.Wrap(apperr.CodeInternal, "打开 PDF 失败", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(settings.ExtractorFileField, originalFilename)
	if err != nil {
		return nil, "", apperr.Wrap(apperr.CodeInternal, "创建上传表单失败", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, "", apperr.Wrap(apperr.CodeInternal, "写入 PDF 数据失败", err)
	}

	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "image_mode", value: "base64"},
		{name: "include_pdf_text", value: "true"},
		{name: "include_boxes", value: "true"},
		{name: "persist_artifacts", value: "false"},
	} {
		if err := writer.WriteField(field.name, field.value); err != nil {
			return nil, "", apperr.Wrap(apperr.CodeInternal, fmt.Sprintf("写入解析参数 %s 失败", field.name), err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", apperr.Wrap(apperr.CodeInternal, "关闭表单失败", err)
	}

	return body, writer.FormDataContentType(), nil
}

func (s *LibraryService) authorizeExtractorRequest(settings model.ExtractorSettings, req *http.Request) {
	if token := strings.TrimSpace(settings.ExtractorToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func (s *LibraryService) doExtractorRequest(req *http.Request, settings model.ExtractorSettings) ([]byte, error) {
	timeout := time.Duration(maxInt(settings.TimeoutSeconds, 1)) * time.Second
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()

	req = req.WithContext(ctx)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "调用解析后端失败", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "读取解析结果失败", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, apperr.New(apperr.CodeUnavailable, fmt.Sprintf("解析后端返回 %d: %s", resp.StatusCode, extractorErrorMessage(respBody)))
	}
	return respBody, nil
}

func parseExtractionResult(respBody []byte) (*extractionResult, error) {
	var payload extractorResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "解析后端响应不是有效 JSON", err)
	}
	if payload.Success != nil && !*payload.Success {
		if payload.Message != "" {
			return nil, apperr.New(apperr.CodeUnavailable, payload.Message)
		}
		return nil, apperr.New(apperr.CodeUnavailable, "解析后端返回失败状态")
	}
	if status := normalizeExtractionStatus(payload.Status); status != "" && status != "completed" {
		if payload.Message != "" {
			return nil, apperr.New(apperr.CodeUnavailable, payload.Message)
		}
		return nil, apperr.New(apperr.CodeUnavailable, fmt.Sprintf("解析后端状态异常: %s", payload.Status))
	}

	pdfText := firstNonEmpty(payload.PDFText, payload.Text, payload.FullText)
	rawFigures := payload.Figures
	if len(rawFigures) == 0 {
		rawFigures = payload.Images
	}

	figures := make([]extractedFigure, 0, len(rawFigures))
	for idx, figure := range rawFigures {
		contentType := firstNonEmpty(figure.ContentType, figure.MIMEType)
		filename := firstNonEmpty(figure.Filename, figure.Name)
		page := figure.PageNumber
		if page == 0 {
			page = figure.Page
		}
		figureIndex := figure.FigureIndex
		if figureIndex == 0 {
			figureIndex = figure.Index
		}
		if figureIndex == 0 {
			figureIndex = idx + 1
		}
		bbox := figure.BBox
		if len(bbox) == 0 {
			bbox = figure.Box
		}

		figures = append(figures, extractedFigure{
			Filename:    filename,
			ContentType: contentType,
			PageNumber:  page,
			FigureIndex: figureIndex,
			Caption:     strings.TrimSpace(figure.Caption),
			BBox:        bbox,
			Data:        firstNonEmpty(figure.ImageBase64, figure.Data, figure.Base64),
		})
	}

	return &extractionResult{
		PDFText: pdfText,
		Boxes:   payload.Boxes,
		Figures: figures,
	}, nil
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
			Source:         "auto",
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
		paper.Figures[i].ImageURL = "/files/figures/" + url.PathEscape(paper.Figures[i].Filename)
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
	palette.ImageURL = "/files/figures/" + url.PathEscape(palette.Filename)
	palette.FigureDisplayLabel = formatFigureDisplayLabel(palette.FigureIndex, palette.SubfigureLabel)
	if palette.ParentFigureID != nil {
		palette.ParentDisplayLabel = formatFigureDisplayLabel(palette.FigureIndex, "")
	}
	if palette.Colors == nil {
		palette.Colors = []string{}
	}
}

func (s *LibraryService) autoExtractionConfigured() (bool, error) {
	settings, err := s.GetExtractorSettings()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(settings.EffectiveExtractorURL) != "", nil
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

func normalizeSubfigureRegion(region model.SubfigureExtractionRegion) (model.SubfigureExtractionRegion, error) {
	region.Caption = strings.TrimSpace(region.Caption)
	region.Label = strings.TrimSpace(region.Label)
	region.ImageData = strings.TrimSpace(region.ImageData)
	if region.Width <= 0 || region.Height <= 0 {
		return model.SubfigureExtractionRegion{}, apperr.New(apperr.CodeInvalidArgument, "子图区域的宽高必须大于 0")
	}
	if region.X < 0 || region.Y < 0 || region.X >= 1 || region.Y >= 1 {
		return model.SubfigureExtractionRegion{}, apperr.New(apperr.CodeInvalidArgument, "子图区域坐标必须落在图片范围内")
	}
	if region.X+region.Width > 1 || region.Y+region.Height > 1 {
		return model.SubfigureExtractionRegion{}, apperr.New(apperr.CodeInvalidArgument, "子图区域不能超出图片边界")
	}
	if region.ImageData == "" {
		return model.SubfigureExtractionRegion{}, apperr.New(apperr.CodeInvalidArgument, "缺少子图图片数据")
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
			return "", apperr.New(apperr.CodeInvalidArgument, "子图命名只支持英文字母")
		}
		if _, exists := used[normalized]; exists {
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

	for _, figure := range figures {
		if figure.ID != figureID && (figure.ParentFigureID == nil || *figure.ParentFigureID != figureID) {
			continue
		}
		if _, exists := seen[figure.ID]; exists {
			continue
		}
		seen[figure.ID] = struct{}{}
		targets = append(targets, figure.ID)
		paths = append(paths, filepath.Join(figuresDir, figure.Filename))
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

func deriveTitle(filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.TrimSpace(base)
	if base == "" {
		return "未命名文献"
	}
	return base
}

func isPDF(filename, contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(contentType, "pdf") {
		return true
	}
	return strings.EqualFold(filepath.Ext(filename), ".pdf")
}

func decodeBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, ","); strings.HasPrefix(value, "data:") && idx >= 0 {
		value = value[idx+1:]
	}

	if data, err := base64.StdEncoding.DecodeString(value); err == nil {
		return data, nil
	}
	if data, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		return data, nil
	}
	return nil, errors.New("无法解码提取图片的 base64 数据")
}

func extensionForFigure(contentType, originalName string) string {
	if ext := filepath.Ext(originalName); ext != "" {
		return ext
	}
	if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
		return exts[0]
	}
	return ".png"
}

func normalizeExtractionStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "", "completed", "queued", "running", "failed", "cancelled":
		return status
	default:
		return status
	}
}

func normalizeExtractionMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", extractionModeAuto, extractionModeManual:
		return mode
	default:
		return mode
	}
}

func manualWorkflowMessage(autoUnavailable bool) string {
	if autoUnavailable {
		return "未配置自动解析服务，文献已入库，可随时进入手工标注补录图片"
	}
	return "已跳过自动解析，文献已入库，可随时进入手工标注补录图片"
}

func extractorErrorMessage(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return "空响应"
	}

	var payload struct {
		Detail  interface{} `json:"detail"`
		Error   string      `json:"error"`
		Message string      `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		switch detail := payload.Detail.(type) {
		case string:
			if strings.TrimSpace(detail) != "" {
				return strings.TrimSpace(detail)
			}
		case []interface{}:
			parts := make([]string, 0, len(detail))
			for _, item := range detail {
				parts = append(parts, strings.TrimSpace(fmt.Sprint(item)))
			}
			if message := strings.TrimSpace(strings.Join(parts, "; ")); message != "" {
				return message
			}
		}
		if message := firstNonEmpty(payload.Error, payload.Message); message != "" {
			return message
		}
	}

	return string(body)
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(apperr.Message(err))
}

func maxInt(values ...int) int {
	result := 0
	for _, value := range values {
		if value > result {
			result = value
		}
	}
	return result
}

func normalizeColor(color string) string {
	color = strings.TrimSpace(color)
	if color == "" {
		return "#A45C40"
	}
	if !strings.HasPrefix(color, "#") {
		color = "#" + color
	}
	if len(color) != 7 {
		return "#A45C40"
	}
	return strings.ToUpper(color)
}

func colorForName(name string) string {
	palette := []string{
		"#A45C40",
		"#7B8C5A",
		"#416788",
		"#C67B5C",
		"#6C4E80",
		"#B98B2F",
		"#3E7C6B",
	}
	sum := 0
	for _, r := range name {
		sum += int(r)
	}
	return palette[sum%len(palette)]
}

func contentTypeOrDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func removeFiles(paths []string) {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		_ = os.Remove(path)
	}
}

func clearDirectoryContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}

	return nil
}

func humanFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
}

func (s *LibraryService) DatabasePath() string {
	return s.config.DatabasePath
}

// GetRuntimePassword 获取运行时密码（从数据库或环境变量）
func (s *LibraryService) GetRuntimePassword() string {
	password, err := s.repo.GetAppSetting(runtimePasswordKey)
	if err != nil {
		s.logger.Warn("failed to get runtime password", "error", err)
	}
	if strings.TrimSpace(password) != "" {
		return password
	}
	return s.config.AdminPassword
}

// ChangePassword 修改密码
func (s *LibraryService) ChangePassword(currentPassword, newPassword string) error {
	currentPassword = strings.TrimSpace(currentPassword)
	newPassword = strings.TrimSpace(newPassword)

	if newPassword == "" {
		return apperr.New(apperr.CodeInvalidArgument, "新密码不能为空")
	}
	if len(newPassword) < 6 {
		return apperr.New(apperr.CodeInvalidArgument, "新密码长度不能少于 6 位")
	}

	// 验证当前密码
	runtimePassword := s.GetRuntimePassword()
	if currentPassword != runtimePassword {
		return apperr.New(apperr.CodeUnauthenticated, "当前密码错误")
	}

	// 保存新密码到数据库
	if err := s.repo.UpsertAppSetting(runtimePasswordKey, newPassword); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "保存新密码失败", err)
	}

	s.logger.Info("admin password changed successfully")
	return nil
}

// ValidateCredentials 验证用户凭据
func (s *LibraryService) ValidateCredentials(username, password string) bool {
	expectedUsername := s.config.AdminUsername
	expectedPassword := s.GetRuntimePassword()
	return username == expectedUsername && password == expectedPassword
}

// GetAuthSettings 获取认证设置
func (s *LibraryService) GetAuthSettings() model.AuthSettings {
	return model.AuthSettings{
		Username:       s.config.AdminUsername,
		PasswordFromDB: s.GetRuntimePassword() != s.config.AdminPassword,
		WeixinBinding:  s.getWeixinBindingSummary(),
	}
}

func (s *LibraryService) ImportDatabase(sourcePath string) error {
	if err := s.repo.Close(); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "关闭当前数据库失败", err)
	}

	dbPath := s.config.DatabasePath
	backupPath := dbPath + ".backup." + time.Now().Format("20060102150405")

	if err := copyFile(dbPath, backupPath); err != nil {
		_ = s.reopenRepo()
		return apperr.Wrap(apperr.CodeInternal, "备份当前数据库失败", err)
	}

	if err := copyFile(sourcePath, dbPath); err != nil {
		_ = copyFile(backupPath, dbPath)
		_ = s.reopenRepo()
		return apperr.Wrap(apperr.CodeInternal, "替换数据库文件失败", err)
	}

	if err := s.reopenRepo(); err != nil {
		_ = copyFile(backupPath, dbPath)
		_ = s.reopenRepo()
		return apperr.Wrap(apperr.CodeInternal, "重新打开数据库失败", err)
	}

	return nil
}

func (s *LibraryService) reopenRepo() error {
	repo, err := repository.NewLibraryRepository(s.config.DatabasePath)
	if err != nil {
		return err
	}
	s.repo = repo
	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}
