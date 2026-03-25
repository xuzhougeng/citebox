package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestExtractorSettingsDefaultsAndPersistence(t *testing.T) {
	svc, _, cfg := newTestService(t)

	defaults, err := svc.GetExtractorSettings()
	if err != nil {
		t.Fatalf("GetExtractorSettings() default error = %v", err)
	}
	if defaults.ExtractorFileField != "file" || defaults.TimeoutSeconds != cfg.ExtractorTimeoutSeconds {
		t.Fatalf("GetExtractorSettings() defaults = %+v, want config defaults", defaults)
	}
	if defaults.ExtractorProfile != extractorProfilePDFFigXV1 {
		t.Fatalf("GetExtractorSettings() extractor_profile = %q, want %q", defaults.ExtractorProfile, extractorProfilePDFFigXV1)
	}
	if defaults.PDFTextSource != pdfTextSourceExtractor {
		t.Fatalf("GetExtractorSettings() pdf_text_source = %q, want %q", defaults.PDFTextSource, pdfTextSourceExtractor)
	}

	updated, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile:    extractorProfileOpenSourceVision,
		PDFTextSource:       pdfTextSourcePDFJS,
		ExtractorURL:        "http://127.0.0.1:9000/api/v1/extract",
		ExtractorToken:      "secret",
		ExtractorFileField:  "upload",
		TimeoutSeconds:      120,
		PollIntervalSeconds: 5,
	})
	if err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}
	if updated.EffectiveExtractorURL == "" || updated.EffectiveJobsURL == "" || updated.ExtractorFileField != "upload" {
		t.Fatalf("UpdateExtractorSettings() = %+v, want normalized effective values", updated)
	}
	if updated.ExtractorProfile != extractorProfileOpenSourceVision || updated.PDFTextSource != pdfTextSourcePDFJS {
		t.Fatalf("UpdateExtractorSettings() profile/text_source = (%q,%q), want (%q,%q)", updated.ExtractorProfile, updated.PDFTextSource, extractorProfileOpenSourceVision, pdfTextSourcePDFJS)
	}
	if updated.ExtractorJobsURL != "" {
		t.Fatalf("UpdateExtractorSettings() extractor_jobs_url = %q, want empty", updated.ExtractorJobsURL)
	}
}

func TestBuiltInLLMExtractorForcesPDFJSTextSource(t *testing.T) {
	svc, _, _ := newTestService(t)

	updated, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile: extractorProfileOpenSourceVision,
		PDFTextSource:    pdfTextSourceExtractor,
	})
	if err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}
	if updated.PDFTextSource != pdfTextSourcePDFJS {
		t.Fatalf("UpdateExtractorSettings() pdf_text_source = %q, want %q", updated.PDFTextSource, pdfTextSourcePDFJS)
	}
}

func TestManualExtractorForcesPDFJSTextSource(t *testing.T) {
	svc, _, _ := newTestService(t)

	updated, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile: extractorProfileManual,
		PDFTextSource:    pdfTextSourceExtractor,
	})
	if err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}
	if updated.PDFTextSource != pdfTextSourcePDFJS {
		t.Fatalf("UpdateExtractorSettings() pdf_text_source = %q, want %q", updated.PDFTextSource, pdfTextSourcePDFJS)
	}
}

func TestPDFFigXExtractorForcesExtractorTextSource(t *testing.T) {
	svc, _, _ := newTestService(t)

	updated, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile: extractorProfilePDFFigXV1,
		PDFTextSource:    pdfTextSourcePDFJS,
	})
	if err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}
	if updated.PDFTextSource != pdfTextSourceExtractor {
		t.Fatalf("UpdateExtractorSettings() pdf_text_source = %q, want %q", updated.PDFTextSource, pdfTextSourceExtractor)
	}
}

func TestBuildExtractorUploadBodyUsesRuntimeFileField(t *testing.T) {
	svc, _, cfg := newTestService(t)

	pdfPath := filepath.Join(cfg.PapersDir(), "sample.pdf")
	if err := os.MkdirAll(filepath.Dir(pdfPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	body, _, err := svc.buildExtractorUploadBody(model.ExtractorSettings{
		ExtractorFileField: "upload",
	}, pdfPath, "sample.pdf")
	if err != nil {
		t.Fatalf("buildExtractorUploadBody() error = %v", err)
	}

	if !bytes.Contains(body.Bytes(), []byte(`name="upload"`)) {
		t.Fatalf("buildExtractorUploadBody() body missing configured file field: %s", body.String())
	}
	if !bytes.Contains(body.Bytes(), []byte("name=\"include_pdf_text\"")) || !bytes.Contains(body.Bytes(), []byte("\r\n\r\ntrue")) {
		t.Fatalf("buildExtractorUploadBody() body missing include_pdf_text=true: %s", body.String())
	}
}

func TestBuildExtractorUploadBodyDisablesPDFTextWhenUsingPDFJS(t *testing.T) {
	svc, _, cfg := newTestService(t)

	pdfPath := filepath.Join(cfg.PapersDir(), "sample-pdfjs.pdf")
	if err := os.MkdirAll(filepath.Dir(pdfPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	body, _, err := svc.buildExtractorUploadBody(model.ExtractorSettings{
		ExtractorProfile:   extractorProfileOpenSourceVision,
		PDFTextSource:      pdfTextSourcePDFJS,
		ExtractorFileField: "upload",
	}, pdfPath, "sample-pdfjs.pdf")
	if err != nil {
		t.Fatalf("buildExtractorUploadBody() error = %v", err)
	}

	if !bytes.Contains(body.Bytes(), []byte("name=\"include_pdf_text\"")) || !bytes.Contains(body.Bytes(), []byte("\r\n\r\nfalse")) {
		t.Fatalf("buildExtractorUploadBody() body missing include_pdf_text=false: %s", body.String())
	}
}

func TestUploadPaperWithoutExtractorConfiguredUsesCompleted(t *testing.T) {
	svc, _, _ := newTestService(t)

	content := []byte("%PDF-1.4 test")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "manual-only.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	paper, err := svc.UploadPaper(file, header, UploadPaperParams{Title: "Manual Only"})
	if err != nil {
		t.Fatalf("UploadPaper() error = %v", err)
	}

	if paper.ExtractionStatus != "completed" {
		t.Fatalf("UploadPaper() status = %q, want %q", paper.ExtractionStatus, "completed")
	}
	if !strings.Contains(paper.ExtractorMessage, "文献已入库") {
		t.Fatalf("UploadPaper() extractor_message = %q, want library-ready hint", paper.ExtractorMessage)
	}
}

func TestUploadPaperWithAutoModeRequiresConfiguredExtractor(t *testing.T) {
	svc, _, _ := newTestService(t)

	content := []byte("%PDF-1.4 test")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "auto-mode.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	_, err := svc.UploadPaper(file, header, UploadPaperParams{
		Title:          "Auto Mode",
		ExtractionMode: "auto",
	})
	if !apperr.IsCode(err, apperr.CodeFailedPrecondition) {
		t.Fatalf("UploadPaper() code = %q, want %q", apperr.CodeOf(err), apperr.CodeFailedPrecondition)
	}
}

func TestUploadPaperWithManualModeSkipsConfiguredExtractor(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorURL:        "http://127.0.0.1:9000/api/v1/extract",
		ExtractorToken:      "secret",
		ExtractorFileField:  "upload",
		TimeoutSeconds:      120,
		PollIntervalSeconds: 5,
	}); err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}

	content := []byte("%PDF-1.4 test")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "manual-mode.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	paper, err := svc.UploadPaper(file, header, UploadPaperParams{
		Title:          "Manual Mode",
		ExtractionMode: "manual",
	})
	if err != nil {
		t.Fatalf("UploadPaper() error = %v", err)
	}

	if paper.ExtractionStatus != "completed" {
		t.Fatalf("UploadPaper() status = %q, want %q", paper.ExtractionStatus, "completed")
	}
	if !strings.Contains(paper.ExtractorMessage, "手工标注") {
		t.Fatalf("UploadPaper() extractor_message = %q, want manual hint", paper.ExtractorMessage)
	}
}

func TestUploadPaperWithManualModeBackfillsPDFTextInBackground(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.startBackground = true
	svc.pdfTextExtractor = func(path string) (string, error) {
		return "manual upload full text", nil
	}

	content := []byte("%PDF-1.4 manual fallback")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "manual-fallback.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	paper, err := svc.UploadPaper(file, header, UploadPaperParams{
		Title:          "Manual Fallback",
		ExtractionMode: "manual",
	})
	if err != nil {
		t.Fatalf("UploadPaper() error = %v", err)
	}

	if got := waitForPaperPDFText(t, svc, paper.ID); got != "manual upload full text" {
		t.Fatalf("waitForPaperPDFText() = %q, want %q", got, "manual upload full text")
	}
}

func TestUploadPaperWithManualExtractorProfileIgnoresConfiguredPDFFigX(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.startBackground = true
	svc.pdfTextExtractor = func(path string) (string, error) {
		return "manual profile full text", nil
	}

	if _, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile:    extractorProfileManual,
		ExtractorURL:        "http://127.0.0.1:9000/api/v1/extract",
		ExtractorToken:      "secret",
		ExtractorFileField:  "upload",
		TimeoutSeconds:      120,
		PollIntervalSeconds: 5,
	}); err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}

	content := []byte("%PDF-1.4 manual profile")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "manual-profile.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	paper, err := svc.UploadPaper(file, header, UploadPaperParams{Title: "Manual Profile"})
	if err != nil {
		t.Fatalf("UploadPaper() error = %v", err)
	}
	if paper.ExtractionStatus != "completed" {
		t.Fatalf("UploadPaper() status = %q, want %q", paper.ExtractionStatus, "completed")
	}
	if !strings.Contains(paper.ExtractorMessage, "当前 PDF 提取方案为手工") {
		t.Fatalf("UploadPaper() extractor_message = %q, want manual-profile hint", paper.ExtractorMessage)
	}
	if got := waitForPaperPDFText(t, svc, paper.ID); got != "manual profile full text" {
		t.Fatalf("waitForPaperPDFText() = %q, want %q", got, "manual profile full text")
	}
}

func TestUploadPaperRejectsAutoModeWhenManualExtractorProfileSelected(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile: extractorProfileManual,
	}); err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}

	content := []byte("%PDF-1.4 manual profile auto")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "manual-profile-auto.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	_, err := svc.UploadPaper(file, header, UploadPaperParams{
		Title:          "Manual Profile Auto",
		ExtractionMode: "auto",
	})
	if !apperr.IsCode(err, apperr.CodeFailedPrecondition) {
		t.Fatalf("UploadPaper() code = %q, want %q", apperr.CodeOf(err), apperr.CodeFailedPrecondition)
	}
	if !strings.Contains(err.Error(), "当前 PDF 提取方案为手工") {
		t.Fatalf("UploadPaper() error = %v, want manual-profile message", err)
	}
}

func TestUploadPaperWithBuiltInLLMAutoModeQueuesBackgroundTask(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	if _, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile: extractorProfileOpenSourceVision,
	}); err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}
	if _, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "figure",
				Name:            "Figure",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "test-key",
				BaseURL:         "https://api.openai.com",
				Model:           "gpt-test",
				MaxOutputTokens: 1200,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID: "figure",
			FigureModelID:  "figure",
		},
		SystemPrompt: "system",
		FigurePrompt: "figure",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	content := []byte("%PDF-1.4 built-in llm test")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "llm-auto.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	paper, err := svc.UploadPaper(file, header, UploadPaperParams{
		Title:          "Built-in LLM Auto",
		ExtractionMode: "auto",
	})
	if err != nil {
		t.Fatalf("UploadPaper() error = %v", err)
	}
	if paper.ExtractionStatus != "queued" {
		t.Fatalf("UploadPaper() status = %q, want %q", paper.ExtractionStatus, "queued")
	}
	if !strings.Contains(paper.ExtractorMessage, "等待内置 AI") {
		t.Fatalf("UploadPaper() extractor_message = %q, want built-in queue hint", paper.ExtractorMessage)
	}
}

func TestPersistExtractionResultMapsBuiltInLLMFiguresToAutoSource(t *testing.T) {
	svc, repo, _ := newTestService(t)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "LLM Source Mapping",
		OriginalFilename: "llm-source.pdf",
		StoredPDFName:    "llm-source.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "queued",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	imageData := strings.TrimPrefix(testPNGDataURL(t, 24, 18), "data:image/png;base64,")
	if err := svc.persistExtractionResult(paper.ID, "", model.ExtractorSettings{
		ExtractorProfile: extractorProfileOpenSourceVision,
		PDFTextSource:    pdfTextSourcePDFJS,
	}, &extractionResult{
		PDFText: "full text",
		Boxes:   json.RawMessage(`[]`),
		Figures: []extractedFigure{
			{
				Filename:    "llm.png",
				ContentType: "image/png",
				PageNumber:  1,
				FigureIndex: 1,
				BBox:        json.RawMessage(`{"source":"llm"}`),
				Data:        imageData,
				Source:      manualFigureSourceLLM,
			},
		},
	}); err != nil {
		t.Fatalf("persistExtractionResult() error = %v", err)
	}

	updated, err := svc.GetPaper(paper.ID)
	if err != nil {
		t.Fatalf("GetPaper() error = %v", err)
	}
	if len(updated.Figures) != 1 {
		t.Fatalf("GetPaper() figures = %d, want 1", len(updated.Figures))
	}
	if updated.Figures[0].Source != figureSourceAuto {
		t.Fatalf("figure source = %q, want %q", updated.Figures[0].Source, figureSourceAuto)
	}
}

func TestUploadPaperRejectsDuplicatePDFAndReturnsExistingPaper(t *testing.T) {
	svc, repo, cfg := newTestService(t)

	content := []byte("%PDF-1.4 duplicate test")
	existing, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Existing",
		OriginalFilename: "existing.pdf",
		StoredPDFName:    "existing.pdf",
		FileSize:         int64(len(content)),
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.PapersDir(), existing.StoredPDFName), content, 0o644); err != nil {
		t.Fatalf("WriteFile(existing pdf) error = %v", err)
	}
	if err := svc.backfillPaperChecksums(); err != nil {
		t.Fatalf("backfillPaperChecksums() error = %v", err)
	}

	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "duplicate.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	_, err = svc.UploadPaper(file, header, UploadPaperParams{Title: "Duplicate"})
	var duplicateErr *DuplicatePaperError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("UploadPaper() error = %T %v, want DuplicatePaperError", err, err)
	}
	if duplicateErr.Paper == nil || duplicateErr.Paper.ID != existing.ID {
		t.Fatalf("DuplicatePaperError paper = %+v, want existing paper id %d", duplicateErr.Paper, existing.ID)
	}
	if !apperr.IsCode(err, apperr.CodeConflict) {
		t.Fatalf("UploadPaper() code = %q, want %q", apperr.CodeOf(err), apperr.CodeConflict)
	}
}

func TestMigrateLegacyManualPendingPapersMarksCompleted(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		StorageDir:              filepath.Join(root, "storage"),
		DatabasePath:            filepath.Join(root, "library.db"),
		MaxUploadSize:           10 << 20,
		AdminUsername:           "citebox",
		AdminPassword:           "citebox123",
		ExtractorTimeoutSeconds: 1,
		ExtractorPollInterval:   1,
		ExtractorFileField:      "file",
	}

	repo, err := repository.NewLibraryRepository(cfg.DatabasePath)
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	if _, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Legacy Manual Workflow",
		OriginalFilename: "legacy-manual-workflow.pdf",
		StoredPDFName:    "legacy_manual_workflow.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: manualPendingStatus,
		ExtractorMessage: "未配置自动解析服务，请直接进入人工处理",
	}); err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := NewLibraryService(repo, cfg, WithLogger(logger), WithoutBackgroundJobs())
	if err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}

	result, err := svc.ListPapers(model.PaperFilter{Status: "completed"})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if result.Total != 1 || len(result.Papers) != 1 {
		t.Fatalf("ListPapers() total=%d len=%d, want 1/1", result.Total, len(result.Papers))
	}
	if !strings.Contains(result.Papers[0].ExtractorMessage, "文献已入库") {
		t.Fatalf("ListPapers() extractor_message = %q, want migrated library-ready hint", result.Papers[0].ExtractorMessage)
	}
}
