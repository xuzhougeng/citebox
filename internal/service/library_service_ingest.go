package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

const sniffBytesLimit = 512

type paperUploadSource struct {
	Filename     string
	ContentType  string
	DeclaredSize int64
	DOI          string
	TitleHint    string
}

func (s *LibraryService) uploadPaperFromReader(reader io.Reader, source paperUploadSource, params UploadPaperParams) (*model.Paper, error) {
	if reader == nil {
		return nil, apperr.New(apperr.CodeInvalidArgument, "缺少 PDF 文件")
	}
	if source.DeclaredSize > s.config.MaxUploadSize {
		return nil, apperr.New(apperr.CodeResourceExhausted, fmt.Sprintf("PDF 大小超过限制 %s", humanFileSize(s.config.MaxUploadSize)))
	}
	if err := s.validateGroup(params.GroupID); err != nil {
		return nil, err
	}

	doi, err := normalizeDOIInput(firstNonEmpty(strings.TrimSpace(params.DOI), strings.TrimSpace(source.DOI)))
	if err != nil {
		return nil, err
	}
	if duplicate, err := s.findDuplicateByDOI(doi); err != nil {
		return nil, err
	} else if duplicate != nil {
		return nil, duplicate
	}

	originalFilename := normalizePaperOriginalFilename(source.Filename, doi)
	title := strings.TrimSpace(params.Title)
	if title == "" {
		title = firstNonEmpty(
			strings.TrimSpace(source.TitleHint),
			deriveTitle(originalFilename),
			doi,
		)
	}

	storedPDFName := fmt.Sprintf("paper_%d.pdf", time.Now().UnixNano())
	pdfPath := filepath.Join(s.config.PapersDir(), storedPDFName)

	actualSize, sniff, pdfSHA256, err := writePaperToStorage(reader, pdfPath, s.config.MaxUploadSize)
	if err != nil {
		removeFiles([]string{pdfPath})
		return nil, err
	}

	detectedContentType := strings.ToLower(http.DetectContentType(sniff))
	if !strings.Contains(detectedContentType, "pdf") {
		removeFiles([]string{pdfPath})
		return nil, apperr.New(apperr.CodeUnsupportedMedia, "只支持上传 PDF 文献")
	}

	if duplicate, err := s.repo.FindPaperByPDFSHA256(pdfSHA256); err != nil {
		removeFiles([]string{pdfPath})
		return nil, err
	} else if duplicate != nil {
		removeFiles([]string{pdfPath})
		s.decoratePaper(duplicate)
		return nil, duplicatePaperError(duplicate, "PDF 已存在，正在跳转到已有文献")
	}

	tagInputs := s.normalizeTagInputs(params.Tags, model.TagScopePaper)
	extractorSettings, err := s.GetExtractorSettings()
	if err != nil {
		removeFiles([]string{pdfPath})
		return nil, err
	}

	autoExtractionConfigured, err := s.autoExtractionConfigured()
	if err != nil {
		removeFiles([]string{pdfPath})
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
			removeFiles([]string{pdfPath})
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
		removeFiles([]string{pdfPath})
		return nil, apperr.New(apperr.CodeInvalidArgument, "上传模式无效")
	}

	contentType := normalizeUploadedPaperContentType(source.ContentType, detectedContentType)
	paper, err := s.repo.CreatePaper(repository.PaperUpsertInput{
		Title:            title,
		DOI:              doi,
		OriginalFilename: originalFilename,
		StoredPDFName:    storedPDFName,
		PDFSHA256:        pdfSHA256,
		FileSize:         actualSize,
		ContentType:      contentType,
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
		removeFiles([]string{pdfPath})
		if apperr.IsCode(err, apperr.CodeConflict) {
			if duplicate, findErr := s.repo.FindPaperByPDFSHA256(pdfSHA256); findErr != nil {
				return nil, findErr
			} else if duplicate != nil {
				s.decoratePaper(duplicate)
				return nil, duplicatePaperError(duplicate, "PDF 已存在，正在跳转到已有文献")
			}
			if duplicate, findErr := s.findDuplicateByDOI(doi); findErr != nil {
				return nil, findErr
			} else if duplicate != nil {
				return nil, duplicate
			}
		}
		return nil, err
	}

	if extractionMode == extractionModeAuto && s.startBackground {
		go s.runPaperExtraction(paper.ID, pdfPath, originalFilename)
	}
	if s.shouldQueuePaperPDFTextBackfill(extractionMode, *extractorSettings) {
		s.queuePaperPDFTextBackfill(paper.ID, pdfPath)
	}

	s.decoratePaper(paper)
	return paper, nil
}

func writePaperToStorage(reader io.Reader, path string, maxSize int64) (int64, []byte, string, error) {
	dst, err := os.Create(path)
	if err != nil {
		return 0, nil, "", apperr.Wrap(apperr.CodeInternal, "创建 PDF 文件失败", err)
	}
	defer dst.Close()

	hasher := sha256.New()
	written, sniff, err := copyReaderWithLimit(io.MultiWriter(dst, hasher), reader, maxSize)
	if err != nil {
		return 0, sniff, "", err
	}

	if err := dst.Close(); err != nil {
		return 0, sniff, "", apperr.Wrap(apperr.CodeInternal, "关闭 PDF 文件失败", err)
	}
	return written, sniff, hex.EncodeToString(hasher.Sum(nil)), nil
}

func copyReaderWithLimit(dst io.Writer, src io.Reader, maxSize int64) (int64, []byte, error) {
	buffer := make([]byte, 32*1024)
	sniff := make([]byte, 0, sniffBytesLimit)
	var written int64

	for {
		n, err := src.Read(buffer)
		if n > 0 {
			if maxSize > 0 && written+int64(n) > maxSize {
				return written, sniff, apperr.New(apperr.CodeResourceExhausted, fmt.Sprintf("PDF 大小超过限制 %s", humanFileSize(maxSize)))
			}
			if len(sniff) < sniffBytesLimit {
				remaining := sniffBytesLimit - len(sniff)
				if remaining > n {
					remaining = n
				}
				sniff = append(sniff, buffer[:remaining]...)
			}
			wn, writeErr := dst.Write(buffer[:n])
			if writeErr != nil {
				return written, sniff, apperr.Wrap(apperr.CodeInternal, "保存 PDF 失败", writeErr)
			}
			if wn != n {
				return written, sniff, apperr.Wrap(apperr.CodeInternal, "保存 PDF 失败", io.ErrShortWrite)
			}
			written += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return written, sniff, apperr.Wrap(apperr.CodeInternal, "保存 PDF 失败", err)
		}
	}

	if written == 0 {
		return 0, sniff, apperr.New(apperr.CodeInvalidArgument, "PDF 内容为空")
	}
	return written, sniff, nil
}

func normalizeUploadedPaperContentType(declared, detected string) string {
	declared = strings.TrimSpace(strings.ToLower(declared))
	if strings.Contains(declared, "pdf") {
		return declared
	}
	detected = strings.TrimSpace(strings.ToLower(detected))
	if strings.Contains(detected, "pdf") {
		return "application/pdf"
	}
	return contentTypeOrDefault(declared, "application/pdf")
}

func normalizePaperOriginalFilename(filename, doi string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = sanitizeDOIForFilename(doi)
	}
	filename = path.Base(strings.ReplaceAll(filename, "\\", "/"))
	filename = strings.TrimSpace(filename)
	if filename == "" || filename == "." || filename == "/" {
		filename = "paper.pdf"
	}
	if !strings.EqualFold(filepath.Ext(filename), ".pdf") {
		filename += ".pdf"
	}
	return filename
}

func sanitizeDOIForFilename(doi string) string {
	normalized := strings.TrimSpace(doi)
	if normalized == "" {
		return "paper.pdf"
	}
	replacer := strings.NewReplacer("/", "_", ":", "_", "\\", "_", "?", "_", "&", "_", "#", "_", "%", "_", "\"", "_", "'", "_", " ", "_")
	filename := replacer.Replace(normalized)
	filename = strings.Trim(filename, "._-")
	if filename == "" {
		return "paper.pdf"
	}
	return filename + ".pdf"
}

func (s *LibraryService) findDuplicateByDOI(doi string) (*DuplicatePaperError, error) {
	if strings.TrimSpace(doi) == "" {
		return nil, nil
	}
	duplicate, err := s.repo.FindPaperByDOI(doi)
	if err != nil {
		return nil, err
	}
	if duplicate == nil {
		return nil, nil
	}
	s.decoratePaper(duplicate)
	return duplicatePaperError(duplicate, "DOI 已存在，正在跳转到已有文献"), nil
}

func duplicatePaperError(paper *model.Paper, message string) *DuplicatePaperError {
	if paper != nil {
		paperCopy := *paper
		paper = &paperCopy
	}
	return &DuplicatePaperError{
		Paper: paper,
		Err:   apperr.New(apperr.CodeConflict, message),
	}
}
