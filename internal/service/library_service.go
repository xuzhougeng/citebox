package service

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"paper_image_db/internal/config"
	"paper_image_db/internal/model"
	"paper_image_db/internal/repository"
)

type LibraryService struct {
	repo       *repository.LibraryRepository
	config     *config.Config
	httpClient *http.Client
}

type UploadPaperParams struct {
	Title   string
	GroupID *int64
	Tags    []string
}

type UpdatePaperParams struct {
	Title   string
	GroupID *int64
	Tags    []string
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

func NewLibraryService(repo *repository.LibraryRepository, cfg *config.Config) *LibraryService {
	os.MkdirAll(cfg.StorageDir, 0o755)
	os.MkdirAll(cfg.PapersDir(), 0o755)
	os.MkdirAll(cfg.FiguresDir(), 0o755)

	service := &LibraryService{
		repo:   repo,
		config: cfg,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.ExtractorTimeoutSeconds) * time.Second,
		},
	}
	go service.resumePendingExtractions()
	return service
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
		return nil, nil
	}
	s.decoratePaper(paper)
	return paper, nil
}

func (s *LibraryService) ListFigures(filter model.FigureFilter) (*model.FigureListResponse, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 24
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

func (s *LibraryService) UploadPaper(file multipart.File, header *multipart.FileHeader, params UploadPaperParams) (*model.Paper, error) {
	if header.Size > s.config.MaxUploadSize {
		return nil, fmt.Errorf("PDF 大小超过限制 %s", humanFileSize(s.config.MaxUploadSize))
	}
	if !isPDF(header.Filename, header.Header.Get("Content-Type")) {
		return nil, errors.New("只支持上传 PDF 文献")
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
		return nil, fmt.Errorf("创建 PDF 文件失败: %w", err)
	}
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		os.Remove(pdfPath)
		return nil, fmt.Errorf("保存 PDF 失败: %w", err)
	}
	if err := dst.Close(); err != nil {
		os.Remove(pdfPath)
		return nil, fmt.Errorf("关闭 PDF 文件失败: %w", err)
	}

	tagInputs := s.normalizeTagInputs(params.Tags)
	paper, err := s.repo.CreatePaper(repository.PaperUpsertInput{
		Title:            title,
		OriginalFilename: header.Filename,
		StoredPDFName:    storedPDFName,
		FileSize:         header.Size,
		ContentType:      contentTypeOrDefault(header.Header.Get("Content-Type"), "application/pdf"),
		PDFText:          "",
		BoxesJSON:        "",
		ExtractionStatus: "queued",
		ExtractorMessage: "文献已入库，等待后台解析",
		ExtractorJobID:   "",
		GroupID:          params.GroupID,
		Tags:             tagInputs,
		Figures:          nil,
	})
	if err != nil {
		os.Remove(pdfPath)
		return nil, err
	}

	go s.runPaperExtraction(paper.ID, pdfPath, header.Filename)

	s.decoratePaper(paper)
	return paper, nil
}

func (s *LibraryService) UpdatePaper(id int64, params UpdatePaperParams) (*model.Paper, error) {
	title := strings.TrimSpace(params.Title)
	if title == "" {
		return nil, errors.New("标题不能为空")
	}
	if err := s.validateGroup(params.GroupID); err != nil {
		return nil, err
	}

	paper, err := s.repo.UpdatePaper(id, title, params.GroupID, s.normalizeTagInputs(params.Tags))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	s.decoratePaper(paper)
	return paper, nil
}

func (s *LibraryService) DeletePaper(id int64) error {
	paper, err := s.repo.GetPaperDetail(id)
	if err != nil {
		return err
	}
	if paper == nil {
		return errors.New("paper not found")
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

func (s *LibraryService) ReextractPaper(id int64) (*model.Paper, error) {
	paper, err := s.repo.GetPaperDetail(id)
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, nil
	}

	switch paper.ExtractionStatus {
	case "queued", "running":
		return nil, errors.New("文献正在解析中，无需重复提交")
	case "failed", "cancelled":
	default:
		return nil, errors.New("当前只有解析失败的文献支持重新解析")
	}

	pdfPath := filepath.Join(s.config.PapersDir(), paper.StoredPDFName)
	if _, err := os.Stat(pdfPath); err != nil {
		return nil, fmt.Errorf("原始 PDF 不存在，无法重新解析: %w", err)
	}

	if err := s.repo.UpdatePaperExtractionState(id, "queued", "已重新提交解析任务", ""); err != nil {
		return nil, err
	}

	go s.runPaperExtraction(id, pdfPath, paper.OriginalFilename)

	updatedPaper, err := s.repo.GetPaperDetail(id)
	if err != nil {
		return nil, err
	}
	s.decoratePaper(updatedPaper)
	return updatedPaper, nil
}

func (s *LibraryService) ListGroups() ([]model.Group, error) {
	return s.repo.ListGroups()
}

func (s *LibraryService) CreateGroup(name, description string) (*model.Group, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return nil, errors.New("分组名称不能为空")
	}
	return s.repo.CreateGroup(name, description)
}

func (s *LibraryService) UpdateGroup(id int64, name, description string) (*model.Group, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return nil, errors.New("分组名称不能为空")
	}
	group, err := s.repo.UpdateGroup(id, name, description)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return group, err
}

func (s *LibraryService) DeleteGroup(id int64) error {
	return s.repo.DeleteGroup(id)
}

func (s *LibraryService) ListTags() ([]model.Tag, error) {
	return s.repo.ListTags()
}

func (s *LibraryService) CreateTag(name, color string) (*model.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("标签名称不能为空")
	}
	color = normalizeColor(color)
	return s.repo.CreateTag(name, color)
}

func (s *LibraryService) UpdateTag(id int64, name, color string) (*model.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("标签名称不能为空")
	}
	tag, err := s.repo.UpdateTag(id, name, normalizeColor(color))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return tag, err
}

func (s *LibraryService) DeleteTag(id int64) error {
	return s.repo.DeleteTag(id)
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
		return errors.New("选择的分组不存在")
	}
	return nil
}

func (s *LibraryService) runPaperExtraction(paperID int64, pdfPath, originalFilename string) {
	var err error

	if jobsURL := strings.TrimSpace(s.config.EffectiveExtractorJobsURL()); jobsURL != "" {
		err = s.processPaperExtractionJob(paperID, jobsURL, pdfPath, originalFilename)
	} else {
		err = s.processPaperExtractionSync(paperID, pdfPath, originalFilename)
	}

	if err == nil || errors.Is(err, sql.ErrNoRows) {
		return
	}

	log.Printf("paper %d extraction failed: %v", paperID, err)
	s.markPaperExtractionFailed(paperID, "", err)
}

func (s *LibraryService) resumePendingExtractions() {
	papers, err := s.repo.ListPapersByExtractionStatuses([]string{"queued", "running"})
	if err != nil {
		log.Printf("resume pending extractions failed: %v", err)
		return
	}

	jobsURL := strings.TrimSpace(s.config.EffectiveExtractorJobsURL())
	for _, paper := range papers {
		paperID := paper.ID
		jobID := strings.TrimSpace(paper.ExtractorJobID)
		if jobsURL != "" && jobID != "" {
			go func() {
				if err := s.resumeRemoteExtraction(paperID, jobID); err != nil && !errors.Is(err, sql.ErrNoRows) {
					log.Printf("resume paper %d extraction failed: %v", paperID, err)
					s.markPaperExtractionFailed(paperID, jobID, err)
				}
			}()
			continue
		}

		if err := s.repo.UpdatePaperExtractionState(paperID, "failed", "后台解析在服务重启后中断", jobID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("mark stale paper %d failed: %v", paperID, err)
		}
	}
}

func (s *LibraryService) processPaperExtractionSync(paperID int64, pdfPath, originalFilename string) error {
	if err := s.repo.UpdatePaperExtractionState(paperID, "running", "解析服务正在处理 PDF", ""); err != nil {
		return err
	}

	result, err := s.extractPDFSync(pdfPath, originalFilename)
	if err != nil {
		return err
	}

	return s.persistExtractionResult(paperID, "", result)
}

func (s *LibraryService) processPaperExtractionJob(paperID int64, jobsURL, pdfPath, originalFilename string) error {
	jobStatus, err := s.createExtractJob(jobsURL, pdfPath, originalFilename)
	if err != nil {
		return err
	}

	finalStatus, err := s.pollExtractJob(paperID, jobStatus)
	if err != nil {
		return err
	}

	switch finalStatus.Status {
	case "completed":
		if err := s.repo.UpdatePaperExtractionState(paperID, "running", "解析结果已返回，正在写入文献库", finalStatus.JobID); err != nil {
			return err
		}
		result, err := s.getExtractJobResult(finalStatus.JobID)
		if err != nil {
			return err
		}
		return s.persistExtractionResult(paperID, finalStatus.JobID, result)
	case "failed":
		return nil
	case "cancelled":
		return nil
	default:
		return fmt.Errorf("解析任务状态异常: %s", finalStatus.Status)
	}
}

func (s *LibraryService) resumeRemoteExtraction(paperID int64, jobID string) error {
	return s.processPaperExtractionJobWithExistingJob(paperID, &extractorJobStatusResponse{
		JobID:  jobID,
		Status: "queued",
	})
}

func (s *LibraryService) processPaperExtractionJobWithExistingJob(paperID int64, initial *extractorJobStatusResponse) error {
	finalStatus, err := s.pollExtractJob(paperID, initial)
	if err != nil {
		return err
	}

	if finalStatus.Status != "completed" {
		return nil
	}

	if err := s.repo.UpdatePaperExtractionState(paperID, "running", "解析结果已返回，正在写入文献库", finalStatus.JobID); err != nil {
		return err
	}

	result, err := s.getExtractJobResult(finalStatus.JobID)
	if err != nil {
		return err
	}
	return s.persistExtractionResult(paperID, finalStatus.JobID, result)
}

func (s *LibraryService) pollExtractJob(paperID int64, initial *extractorJobStatusResponse) (*extractorJobStatusResponse, error) {
	current := initial
	if current == nil {
		return nil, errors.New("缺少解析任务信息")
	}

	for {
		if current.JobID == "" {
			return nil, errors.New("解析任务未返回 job_id")
		}

		status, err := s.getExtractJobStatus(current.JobID)
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
			return nil, fmt.Errorf("未知的解析任务状态: %s", status.Status)
		}

		time.Sleep(time.Duration(maxInt(s.config.ExtractorPollInterval, 1)) * time.Second)
	}
}

func (s *LibraryService) persistExtractionResult(paperID int64, jobID string, result *extractionResult) error {
	if result == nil {
		return errors.New("解析结果为空")
	}

	figures, figurePaths, err := s.materializeFigures(result.Figures)
	if err != nil {
		return fmt.Errorf("解析图片失败: %w", err)
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
	if updateErr := s.repo.UpdatePaperExtractionState(paperID, "failed", message, jobID); updateErr != nil && !errors.Is(updateErr, sql.ErrNoRows) {
		log.Printf("mark paper %d extraction failed: %v", paperID, updateErr)
	}
}

func (s *LibraryService) extractPDFSync(pdfPath, originalFilename string) (*extractionResult, error) {
	extractURL := strings.TrimSpace(s.config.EffectiveExtractorURL())
	if extractURL == "" {
		return nil, errors.New("PDF_EXTRACTOR_URL 未配置，无法调用解析后端")
	}

	req, err := s.newExtractorUploadRequest(http.MethodPost, extractURL, pdfPath, originalFilename)
	if err != nil {
		return nil, err
	}

	respBody, err := s.doExtractorRequest(req)
	if err != nil {
		return nil, err
	}

	return parseExtractionResult(respBody)
}

func (s *LibraryService) createExtractJob(jobsURL, pdfPath, originalFilename string) (*extractorJobStatusResponse, error) {
	req, err := s.newExtractorUploadRequest(http.MethodPost, jobsURL, pdfPath, originalFilename)
	if err != nil {
		return nil, err
	}

	respBody, err := s.doExtractorRequest(req)
	if err != nil {
		return nil, err
	}

	var payload extractorJobStatusResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("解析任务响应不是有效 JSON: %w", err)
	}
	payload.Status = normalizeExtractionStatus(payload.Status)
	return &payload, nil
}

func (s *LibraryService) getExtractJobStatus(jobID string) (*extractorJobStatusResponse, error) {
	jobsURL := strings.TrimSpace(s.config.EffectiveExtractorJobsURL())
	if jobsURL == "" {
		return nil, errors.New("PDF_EXTRACTOR_JOBS_URL 未配置，无法轮询解析任务")
	}

	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(jobsURL, "/")+"/"+url.PathEscape(jobID), nil)
	if err != nil {
		return nil, fmt.Errorf("创建解析任务轮询请求失败: %w", err)
	}
	s.authorizeExtractorRequest(req)

	respBody, err := s.doExtractorRequest(req)
	if err != nil {
		return nil, err
	}

	var payload extractorJobStatusResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("解析任务状态响应不是有效 JSON: %w", err)
	}
	payload.Status = normalizeExtractionStatus(payload.Status)
	return &payload, nil
}

func (s *LibraryService) getExtractJobResult(jobID string) (*extractionResult, error) {
	jobsURL := strings.TrimSpace(s.config.EffectiveExtractorJobsURL())
	if jobsURL == "" {
		return nil, errors.New("PDF_EXTRACTOR_JOBS_URL 未配置，无法读取解析结果")
	}

	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(jobsURL, "/")+"/"+url.PathEscape(jobID)+"/result", nil)
	if err != nil {
		return nil, fmt.Errorf("创建解析结果请求失败: %w", err)
	}
	s.authorizeExtractorRequest(req)

	respBody, err := s.doExtractorRequest(req)
	if err != nil {
		return nil, err
	}

	return parseExtractionResult(respBody)
}

func (s *LibraryService) newExtractorUploadRequest(method, targetURL, pdfPath, originalFilename string) (*http.Request, error) {
	body, contentType, err := s.buildExtractorUploadBody(pdfPath, originalFilename)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, targetURL, body)
	if err != nil {
		return nil, fmt.Errorf("创建解析请求失败: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	s.authorizeExtractorRequest(req)
	return req, nil
}

func (s *LibraryService) buildExtractorUploadBody(pdfPath, originalFilename string) (*bytes.Buffer, string, error) {
	file, err := os.Open(pdfPath)
	if err != nil {
		return nil, "", fmt.Errorf("打开 PDF 失败: %w", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(s.config.ExtractorFileField, originalFilename)
	if err != nil {
		return nil, "", fmt.Errorf("创建上传表单失败: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, "", fmt.Errorf("写入 PDF 数据失败: %w", err)
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
			return nil, "", fmt.Errorf("写入解析参数 %s 失败: %w", field.name, err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("关闭表单失败: %w", err)
	}

	return body, writer.FormDataContentType(), nil
}

func (s *LibraryService) authorizeExtractorRequest(req *http.Request) {
	if token := strings.TrimSpace(s.config.ExtractorToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func (s *LibraryService) doExtractorRequest(req *http.Request) ([]byte, error) {
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用解析后端失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取解析结果失败: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("解析后端返回 %d: %s", resp.StatusCode, extractorErrorMessage(respBody))
	}
	return respBody, nil
}

func parseExtractionResult(respBody []byte) (*extractionResult, error) {
	var payload extractorResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("解析后端响应不是有效 JSON: %w", err)
	}
	if payload.Success != nil && !*payload.Success {
		if payload.Message != "" {
			return nil, errors.New(payload.Message)
		}
		return nil, errors.New("解析后端返回失败状态")
	}
	if status := normalizeExtractionStatus(payload.Status); status != "" && status != "completed" {
		if payload.Message != "" {
			return nil, errors.New(payload.Message)
		}
		return nil, fmt.Errorf("解析后端状态异常: %s", payload.Status)
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
			return nil, paths, err
		}

		contentType := contentTypeOrDefault(figure.ContentType, http.DetectContentType(binary))
		ext := extensionForFigure(contentType, figure.Filename)
		storedName := fmt.Sprintf("figure_%d_%d%s", time.Now().UnixNano(), idx+1, ext)
		path := filepath.Join(s.config.FiguresDir(), storedName)

		if err := os.WriteFile(path, binary, 0o644); err != nil {
			return nil, paths, err
		}
		paths = append(paths, path)

		items = append(items, repository.FigureUpsertInput{
			Filename:     storedName,
			OriginalName: firstNonEmpty(figure.Filename, storedName),
			ContentType:  contentType,
			PageNumber:   figure.PageNumber,
			FigureIndex:  figure.FigureIndex,
			Caption:      figure.Caption,
			BBoxJSON:     strings.TrimSpace(string(figure.BBox)),
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
	for i := range paper.Figures {
		paper.Figures[i].ImageURL = "/files/figures/" + url.PathEscape(paper.Figures[i].Filename)
	}
}

func (s *LibraryService) normalizeTagInputs(names []string) []repository.TagUpsertInput {
	seen := map[string]bool{}
	result := []repository.TagUpsertInput{}

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
	return strings.TrimSpace(err.Error())
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
