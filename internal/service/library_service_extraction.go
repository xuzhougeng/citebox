package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
)

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
		ExtractorProfile:    normalizeExtractorProfile(s.config.ExtractorProfile),
		PDFTextSource:       normalizePDFTextSource(s.config.ExtractorPDFTextSource, normalizeExtractorProfile(s.config.ExtractorProfile)),
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

	settings.ExtractorProfile = normalizeExtractorProfile(firstNonEmpty(strings.TrimSpace(settings.ExtractorProfile), defaults.ExtractorProfile))
	settings.PDFTextSource = normalizePDFTextSource(firstNonEmpty(strings.TrimSpace(settings.PDFTextSource), defaults.PDFTextSource), settings.ExtractorProfile)

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

func (s *LibraryService) runPaperExtraction(paperID int64, pdfPath, originalFilename string) {
	settings, err := s.GetExtractorSettings()
	if err == nil {
		if usesManualExtractionProfile(*settings) {
			err = apperr.New(apperr.CodeFailedPrecondition, "当前 PDF 提取方案为手工，不执行自动解析")
		} else if usesBuiltInLLMCoordinateExtraction(*settings) {
			err = s.processBuiltInLLMExtraction(*settings, paperID, pdfPath, originalFilename)
		} else if jobsURL := strings.TrimSpace(settings.EffectiveJobsURL); jobsURL != "" {
			err = s.processPaperExtractionJob(*settings, paperID, jobsURL, pdfPath, originalFilename)
		} else {
			err = s.processPaperExtractionSync(*settings, paperID, pdfPath, originalFilename)
		}
	}

	if err == nil || apperr.IsCode(err, apperr.CodeNotFound) {
		return
	}
	if settings != nil && normalizePDFTextSource(settings.PDFTextSource, settings.ExtractorProfile) == pdfTextSourcePDFJS {
		if backfillErr := s.backfillPaperPDFTextIfMissing(paperID, pdfPath); backfillErr != nil && !apperr.IsCode(backfillErr, apperr.CodeNotFound) {
			s.logger.Warn("paper extraction text backfill failed",
				"paper_id", paperID,
				"path", pdfPath,
				"error", backfillErr,
			)
		}
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
	s.repoMu.RLock()
	err := s.repo.UpdatePaperExtractionState(paperID, "running", "解析服务正在处理 PDF", "")
	s.repoMu.RUnlock()
	if err != nil {
		return err
	}

	result, err := s.extractPDFSync(settings, pdfPath, originalFilename)
	if err != nil {
		return err
	}

	return s.persistExtractionResult(paperID, "", settings, result)
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
		s.repoMu.RLock()
		err = s.repo.UpdatePaperExtractionState(paperID, "running", "解析结果已返回，正在写入文献库", finalStatus.JobID)
		s.repoMu.RUnlock()
		if err != nil {
			return err
		}
		result, err := s.getExtractJobResult(settings, finalStatus.JobID)
		if err != nil {
			return err
		}
		return s.persistExtractionResult(paperID, finalStatus.JobID, settings, result)
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

	s.repoMu.RLock()
	err = s.repo.UpdatePaperExtractionState(paperID, "running", "解析结果已返回，正在写入文献库", finalStatus.JobID)
	s.repoMu.RUnlock()
	if err != nil {
		return err
	}

	result, err := s.getExtractJobResult(settings, finalStatus.JobID)
	if err != nil {
		return err
	}
	return s.persistExtractionResult(paperID, finalStatus.JobID, settings, result)
}

func (s *LibraryService) pollExtractJob(settings model.ExtractorSettings, paperID int64, initial *extractorJobStatusResponse) (*extractorJobStatusResponse, error) {
	current := initial
	if current == nil {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "缺少解析任务信息")
	}

	maxDuration := time.Duration(maxInt(settings.TimeoutSeconds, 1800)) * time.Second
	deadline := time.Now().Add(maxDuration)

	for {
		if time.Now().After(deadline) {
			return nil, apperr.New(apperr.CodeDeadlineExceeded, fmt.Sprintf("轮询解析任务超时（超过 %v）", maxDuration))
		}

		if current.JobID == "" {
			return nil, apperr.New(apperr.CodeUnavailable, "解析任务未返回 job_id")
		}

		status, err := s.getExtractJobStatus(settings, current.JobID)
		if err != nil {
			return nil, err
		}
		current = status

		s.repoMu.RLock()
		switch normalizeExtractionStatus(status.Status) {
		case "queued":
			err = s.repo.UpdatePaperExtractionState(paperID, "queued", "文献已提交到解析队列", status.JobID)
		case "running":
			err = s.repo.UpdatePaperExtractionState(paperID, "running", "解析服务正在处理 PDF", status.JobID)
		case "completed":
			s.repoMu.RUnlock()
			return status, nil
		case "cancelled":
			err = s.repo.UpdatePaperExtractionState(paperID, "cancelled", "解析任务已取消", status.JobID)
			s.repoMu.RUnlock()
			if err != nil {
				return nil, err
			}
			return status, nil
		case "failed":
			message := firstNonEmpty(status.Error, "解析后端返回失败状态")
			err = s.repo.UpdatePaperExtractionState(paperID, "failed", message, status.JobID)
			s.repoMu.RUnlock()
			if err != nil {
				return nil, err
			}
			return status, nil
		default:
			s.repoMu.RUnlock()
			return nil, apperr.New(apperr.CodeUnavailable, fmt.Sprintf("未知的解析任务状态: %s", status.Status))
		}
		s.repoMu.RUnlock()

		if err != nil {
			return nil, err
		}

		time.Sleep(time.Duration(maxInt(settings.PollIntervalSeconds, 1)) * time.Second)
	}
}

func (s *LibraryService) persistExtractionResult(paperID int64, jobID string, settings model.ExtractorSettings, result *extractionResult) error {
	if result == nil {
		return apperr.New(apperr.CodeUnavailable, "解析结果为空")
	}

	pdfText, err := s.resolvePDFTextForPersistence(paperID, settings, result.PDFText)
	if err != nil {
		return err
	}

	figures, figurePaths, err := s.materializeFigures(result.Figures)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "解析图片失败", err)
	}

	s.repoMu.RLock()
	err = s.repo.ApplyPaperExtractionResult(
		paperID,
		pdfText,
		strings.TrimSpace(string(result.Boxes)),
		"completed",
		"",
		jobID,
		figures,
	)
	s.repoMu.RUnlock()
	if err != nil {
		removeFiles(figurePaths)
		return err
	}

	return nil
}

func (s *LibraryService) resolvePDFTextForPersistence(paperID int64, settings model.ExtractorSettings, incoming string) (string, error) {
	if normalizePDFTextSource(settings.PDFTextSource, settings.ExtractorProfile) != pdfTextSourcePDFJS {
		return incoming, nil
	}
	if strings.TrimSpace(incoming) != "" {
		return incoming, nil
	}

	s.repoMu.RLock()
	paper, err := s.repo.GetPaperDetail(paperID)
	s.repoMu.RUnlock()
	if err != nil {
		return "", err
	}
	if paper == nil {
		return "", nil
	}
	if strings.TrimSpace(paper.PDFText) != "" {
		return paper.PDFText, nil
	}

	pdfPath := filepath.Join(s.config.PapersDir(), paper.StoredPDFName)
	fallbackText, fallbackErr := s.extractServerPDFTextFallback(pdfPath)
	if fallbackErr != nil {
		s.logger.Warn("server-side pdf text fallback failed",
			"paper_id", paperID,
			"path", pdfPath,
			"error", fallbackErr,
		)
		return paper.PDFText, nil
	}
	return fallbackText, nil
}

func (s *LibraryService) shouldQueuePaperPDFTextBackfill(extractionMode string, settings model.ExtractorSettings) bool {
	if normalizeExtractionMode(extractionMode) == extractionModeManual {
		return true
	}
	return normalizePDFTextSource(settings.PDFTextSource, settings.ExtractorProfile) == pdfTextSourcePDFJS
}

func (s *LibraryService) queuePaperPDFTextBackfill(paperID int64, pdfPath string) {
	if !s.startBackground {
		return
	}

	go func() {
		if err := s.backfillPaperPDFTextIfMissing(paperID, pdfPath); err != nil && !apperr.IsCode(err, apperr.CodeNotFound) {
			s.logger.Warn("paper pdf text backfill failed",
				"paper_id", paperID,
				"path", pdfPath,
				"error", err,
			)
		}
	}()
}

func (s *LibraryService) backfillPaperPDFTextIfMissing(paperID int64, pdfPath string) error {
	s.repoMu.RLock()
	paper, err := s.repo.GetPaperDetail(paperID)
	s.repoMu.RUnlock()
	if err != nil {
		return err
	}
	if paper == nil || strings.TrimSpace(paper.PDFText) != "" {
		return nil
	}

	extract := s.pdfTextExtractor
	if extract == nil {
		extract = s.extractServerPDFTextFallback
	}
	pdfText, err := extract(pdfPath)
	if err != nil {
		return err
	}
	pdfText = strings.TrimSpace(pdfText)
	if pdfText == "" {
		return nil
	}

	s.repoMu.RLock()
	current, err := s.repo.GetPaperDetail(paperID)
	s.repoMu.RUnlock()
	if err != nil {
		return err
	}
	if current == nil || strings.TrimSpace(current.PDFText) != "" {
		return nil
	}

	s.repoMu.RLock()
	_, err = s.repo.UpdatePaperPDFText(paperID, pdfText)
	s.repoMu.RUnlock()
	return err
}

func (s *LibraryService) markPaperExtractionFailed(paperID int64, jobID string, err error) {
	if strings.TrimSpace(jobID) == "" {
		s.repoMu.RLock()
		paper, getErr := s.repo.GetPaperDetail(paperID)
		s.repoMu.RUnlock()
		if getErr == nil && paper != nil {
			jobID = paper.ExtractorJobID
		}
	}
	message := firstNonEmpty(errorMessage(err), "解析失败")
	s.repoMu.RLock()
	updateErr := s.repo.UpdatePaperExtractionState(paperID, "failed", message, jobID)
	s.repoMu.RUnlock()
	if updateErr != nil && !apperr.IsCode(updateErr, apperr.CodeNotFound) {
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
		{name: "include_pdf_text", value: boolString(shouldRequestPDFTextFromExtractor(settings))},
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

func normalizeExtractorProfile(value string) string {
	switch strings.TrimSpace(value) {
	case extractorProfileManual:
		return extractorProfileManual
	case extractorProfileOpenSourceVision:
		return extractorProfileOpenSourceVision
	case extractorProfilePDFFigXV1:
		return extractorProfilePDFFigXV1
	default:
		return extractorProfilePDFFigXV1
	}
}

func normalizePDFTextSource(value, profile string) string {
	switch normalizeExtractorProfile(profile) {
	case extractorProfileManual:
		return pdfTextSourcePDFJS
	case extractorProfileOpenSourceVision:
		return pdfTextSourcePDFJS
	case extractorProfilePDFFigXV1:
		return pdfTextSourceExtractor
	default:
		return pdfTextSourceExtractor
	}
}

func shouldRequestPDFTextFromExtractor(settings model.ExtractorSettings) bool {
	return normalizePDFTextSource(settings.PDFTextSource, settings.ExtractorProfile) != pdfTextSourcePDFJS
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (s *LibraryService) autoExtractionConfigured() (bool, error) {
	settings, err := s.GetExtractorSettings()
	if err != nil {
		return false, err
	}
	if usesManualExtractionProfile(*settings) {
		return false, nil
	}
	if usesBuiltInLLMCoordinateExtraction(*settings) {
		return s.builtInLLMCoordinateExtractionConfigured()
	}
	return strings.TrimSpace(settings.EffectiveExtractorURL) != "", nil
}

func (s *LibraryService) builtInLLMCoordinateExtractionConfigured() (bool, error) {
	raw, err := s.repo.GetAppSetting(aiSettingsKey)
	if err != nil {
		return false, err
	}

	settings := model.DefaultAISettings()
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			return false, apperr.Wrap(apperr.CodeInternal, "解析 AI 设置失败", err)
		}
	}

	normalized, err := normalizeAISettings(settings)
	if err != nil {
		return false, err
	}
	modelConfig, err := resolveModelForAction(normalized, model.AIActionFigureInterpretation)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(modelConfig.APIKey) != "" && strings.TrimSpace(modelConfig.Model) != "", nil
}

func usesBuiltInLLMCoordinateExtraction(settings model.ExtractorSettings) bool {
	return normalizeExtractorProfile(settings.ExtractorProfile) == extractorProfileOpenSourceVision
}

func usesManualExtractionProfile(settings model.ExtractorSettings) bool {
	return normalizeExtractorProfile(settings.ExtractorProfile) == extractorProfileManual
}

func manualWorkflowMessage(autoUnavailable, manualProfile bool) string {
	if manualProfile {
		return "当前 PDF 提取方案为手工，文献已入库；系统会自动提取并保存 PDF 全文"
	}
	if autoUnavailable {
		return "未配置自动解析服务，文献已入库，可随时进入手工标注补录图片"
	}
	return "已跳过自动解析，文献已入库，可随时进入手工标注补录图片"
}

func builtInLLMWorkflowMessage() string {
	return "文献已入库，等待内置 AI 在后台解析图片坐标；若浏览器未写回全文，服务端也会补提 PDF 全文"
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
