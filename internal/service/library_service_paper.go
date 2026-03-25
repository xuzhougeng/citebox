package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

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
	extractorSettings, err := s.GetExtractorSettings()
	if err != nil {
		os.Remove(pdfPath)
		return nil, err
	}

	autoExtractionConfigured, err := s.autoExtractionConfigured()
	if err != nil {
		os.Remove(pdfPath)
		return nil, err
	}
	usesManualProfile := usesManualExtractionProfile(*extractorSettings)
	usesBuiltInLLMExtraction := usesBuiltInLLMCoordinateExtraction(*extractorSettings)

	extractionMode := normalizeExtractionMode(params.ExtractionMode)
	if extractionMode == "" {
		if autoExtractionConfigured {
			extractionMode = extractionModeAuto
		} else {
			extractionMode = extractionModeManual
		}
	}

	extractionStatus := "completed"
	extractorMessage := manualWorkflowMessage(!autoExtractionConfigured, usesManualProfile)

	switch extractionMode {
	case extractionModeAuto:
		if !autoExtractionConfigured {
			os.Remove(pdfPath)
			if usesManualProfile {
				return nil, apperr.New(apperr.CodeFailedPrecondition, "当前 PDF 提取方案为手工，请改用手工上传")
			}
			return nil, apperr.New(apperr.CodeFailedPrecondition, "未配置自动解析服务，请改用手工标注")
		}
		if usesBuiltInLLMExtraction {
			extractionStatus = "queued"
			extractorMessage = builtInLLMWorkflowMessage()
		} else {
			extractionStatus = "queued"
			extractorMessage = "文献已入库，等待后台解析"
		}
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

	if extractionMode == extractionModeAuto && s.startBackground {
		go s.runPaperExtraction(paper.ID, pdfPath, header.Filename)
	}
	if s.shouldQueuePaperPDFTextBackfill(extractionMode, *extractorSettings) {
		s.queuePaperPDFTextBackfill(paper.ID, pdfPath)
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
			message = manualWorkflowMessage(true, false)
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

func (s *LibraryService) UpdatePaperPDFText(id int64, pdfText string) (*model.Paper, error) {
	normalized := strings.TrimSpace(pdfText)
	if normalized == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "PDF 全文不能为空")
	}

	paper, err := s.repo.UpdatePaperPDFText(id, normalized)
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
	paths = append(paths, collectFigureFilePaths(paper.Figures, s.config.FiguresDir())...)
	removeFiles(paths)
	return nil
}

func (s *LibraryService) ReextractPaper(id int64) (*model.Paper, error) {
	paper, err := s.repo.GetPaperDetail(id)
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	settings, err := s.GetExtractorSettings()
	if err != nil {
		return nil, err
	}
	if usesManualExtractionProfile(*settings) {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "当前 PDF 提取方案为手工，不支持重新自动解析")
	}
	if !usesBuiltInLLMCoordinateExtraction(*settings) && strings.TrimSpace(settings.EffectiveExtractorURL) == "" {
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

	message := "已重新提交解析任务"
	if usesBuiltInLLMCoordinateExtraction(*settings) {
		message = "已重新提交内置 AI 解析任务"
	}
	if err := s.repo.UpdatePaperExtractionState(id, "queued", message, ""); err != nil {
		return nil, err
	}

	if s.startBackground {
		go s.runPaperExtraction(id, pdfPath, paper.OriginalFilename)
	}

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
