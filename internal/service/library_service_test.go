package service

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
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

func newTestService(t *testing.T) (*LibraryService, *repository.LibraryRepository, *config.Config) {
	t.Helper()

	root := t.TempDir()
	cfg := &config.Config{
		StorageDir:              filepath.Join(root, "storage"),
		DatabasePath:            filepath.Join(root, "library.db"),
		MaxUploadSize:           10 << 20,
		AdminUsername:           "wanglab",
		AdminPassword:           "wanglab789",
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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := NewLibraryService(repo, cfg, WithLogger(logger), WithoutBackgroundJobs())
	if err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}

	return svc, repo, cfg
}

func createTestPaper(t *testing.T, repo *repository.LibraryRepository) *model.Paper {
	t.Helper()

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Atlas Study",
		OriginalFilename: "atlas-study.pdf",
		StoredPDFName:    "paper_test.pdf",
		FileSize:         512,
		ContentType:      "application/pdf",
		AbstractText:     "Atlas abstract",
		NotesText:        "Atlas notes",
		PaperNotesText:   "Atlas paper notes",
		ExtractionStatus: "completed",
		Tags: []repository.TagUpsertInput{
			{Name: "Atlas", Color: "#123456"},
		},
		Figures: []repository.FigureUpsertInput{
			{
				Filename:     "figure_test.png",
				OriginalName: "figure-original.png",
				ContentType:  "image/png",
				PageNumber:   1,
				FigureIndex:  1,
				Caption:      "Figure",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	return paper
}

type testMultipartFile struct {
	*bytes.Reader
}

func (f *testMultipartFile) Close() error {
	return nil
}

func testPNGDataURL(t *testing.T, width, height int) string {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(10 + x), G: uint8(20 + y), B: 180, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestListPapersAppliesDefaultsAndDecoratesURLs(t *testing.T) {
	svc, repo, _ := newTestService(t)
	createTestPaper(t, repo)

	result, err := svc.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}

	if result.Page != 1 || result.PageSize != 12 || result.Total != 1 || result.TotalPages != 1 {
		t.Fatalf("ListPapers() pagination = %+v", result)
	}
	if got := result.Papers[0].PDFURL; got != "/files/papers/paper_test.pdf" {
		t.Fatalf("ListPapers() pdf_url = %q, want %q", got, "/files/papers/paper_test.pdf")
	}
}

func TestGetPaperDecoratesFigureURLs(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	got, err := svc.GetPaper(paper.ID)
	if err != nil {
		t.Fatalf("GetPaper() error = %v", err)
	}

	if got.PDFURL != "/files/papers/paper_test.pdf" {
		t.Fatalf("GetPaper() pdf_url = %q, want %q", got.PDFURL, "/files/papers/paper_test.pdf")
	}
	if len(got.Figures) != 1 || got.Figures[0].ImageURL != "/files/figures/figure_test.png" {
		t.Fatalf("GetPaper() figures = %+v", got.Figures)
	}
	if got.Figures[0].Source != "auto" {
		t.Fatalf("GetPaper() figure source = %q, want %q", got.Figures[0].Source, "auto")
	}
}

func TestDeletePaperRemovesFilesAndReturnsNotFound(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	if err := os.WriteFile(filepath.Join(cfg.PapersDir(), paper.StoredPDFName), []byte("pdf"), 0o644); err != nil {
		t.Fatalf("WriteFile(pdf) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), []byte("img"), 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	if err := svc.DeletePaper(paper.ID); err != nil {
		t.Fatalf("DeletePaper() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.PapersDir(), paper.StoredPDFName)); !os.IsNotExist(err) {
		t.Fatalf("paper file still exists, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)); !os.IsNotExist(err) {
		t.Fatalf("figure file still exists, stat err = %v", err)
	}

	if err := svc.DeletePaper(paper.ID); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("DeletePaper() missing code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
}

func TestUpdatePaperValidationErrors(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.UpdatePaper(1, UpdatePaperParams{Title: "   "}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("UpdatePaper() empty title code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}

	groupID := int64(999)
	if _, err := svc.UpdatePaper(1, UpdatePaperParams{Title: "Valid", GroupID: &groupID}); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("UpdatePaper() missing group code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
}

func TestUpdatePaperPersistsMetadata(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	updated, err := svc.UpdatePaper(paper.ID, UpdatePaperParams{
		Title:          "Atlas Study Revised",
		AbstractText:   "Updated abstract",
		NotesText:      "Updated notes",
		PaperNotesText: "Updated paper notes",
		Tags:           []string{"Atlas", "Revised"},
	})
	if err != nil {
		t.Fatalf("UpdatePaper() error = %v", err)
	}

	if updated.AbstractText != "Updated abstract" || updated.NotesText != "Updated notes" || updated.PaperNotesText != "Updated paper notes" {
		t.Fatalf("UpdatePaper() metadata = (%q, %q, %q), want updated values", updated.AbstractText, updated.NotesText, updated.PaperNotesText)
	}
	if len(updated.Tags) != 2 {
		t.Fatalf("UpdatePaper() tags = %d, want 2", len(updated.Tags))
	}
}

func TestUpdateFigureTagsOnlyTouchesSelectedFigure(t *testing.T) {
	svc, repo, _ := newTestService(t)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Figure Tags",
		OriginalFilename: "figure-tags.pdf",
		StoredPDFName:    "figure-tags.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "figure_a.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "A"},
			{Filename: "figure_b.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 2, Caption: "B"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	updated, err := svc.UpdateFigureTags(paper.Figures[0].ID, []string{"signal"})
	if err != nil {
		t.Fatalf("UpdateFigureTags() error = %v", err)
	}

	if got := len(updated.Figures[0].Tags); got != 1 {
		t.Fatalf("updated first figure tags = %d, want 1", got)
	}
	if got := len(updated.Figures[1].Tags); got != 0 {
		t.Fatalf("updated second figure tags = %d, want 0", got)
	}
	if got := updated.Figures[0].ImageURL; got != "/files/figures/figure_a.png" {
		t.Fatalf("updated first figure image_url = %q, want %q", got, "/files/figures/figure_a.png")
	}

	tagID := updated.Figures[0].Tags[0].ID
	result, err := svc.ListFigures(model.FigureFilter{TagID: &tagID})
	if err != nil {
		t.Fatalf("ListFigures() error = %v", err)
	}
	if result.Total != 1 || len(result.Figures) != 1 {
		t.Fatalf("ListFigures() total=%d len=%d, want 1/1", result.Total, len(result.Figures))
	}
	if result.Figures[0].ID != paper.Figures[0].ID {
		t.Fatalf("ListFigures() figure id = %d, want %d", result.Figures[0].ID, paper.Figures[0].ID)
	}
}

func TestUpdateFigureNotesAreSearchable(t *testing.T) {
	svc, repo, _ := newTestService(t)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Figure Notes",
		OriginalFilename: "figure-notes.pdf",
		StoredPDFName:    "figure-notes.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "figure_a.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "A"},
			{Filename: "figure_b.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 2, Caption: "B"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	notes := "AI 总结：该图片强调了微环境重塑。"
	updated, err := svc.UpdateFigure(paper.Figures[0].ID, UpdateFigureParams{NotesText: &notes})
	if err != nil {
		t.Fatalf("UpdateFigure() error = %v", err)
	}

	if updated.Figures[0].NotesText != notes {
		t.Fatalf("updated first figure notes_text = %q, want %q", updated.Figures[0].NotesText, notes)
	}
	if updated.Figures[0].Caption != "A" {
		t.Fatalf("updated first figure caption = %q, want %q", updated.Figures[0].Caption, "A")
	}
	if updated.Figures[1].NotesText != "" {
		t.Fatalf("updated second figure notes_text = %q, want empty", updated.Figures[1].NotesText)
	}
	if updated.Figures[1].Caption != "B" {
		t.Fatalf("updated second figure caption = %q, want %q", updated.Figures[1].Caption, "B")
	}

	result, err := svc.ListFigures(model.FigureFilter{Keyword: "微环境重塑"})
	if err != nil {
		t.Fatalf("ListFigures() error = %v", err)
	}
	if result.Total != 1 || len(result.Figures) != 1 {
		t.Fatalf("ListFigures() total=%d len=%d, want 1/1", result.Total, len(result.Figures))
	}
	if result.Figures[0].ID != paper.Figures[0].ID {
		t.Fatalf("ListFigures() figure id = %d, want %d", result.Figures[0].ID, paper.Figures[0].ID)
	}
	if result.Figures[0].NotesText != notes {
		t.Fatalf("ListFigures() notes_text = %q, want %q", result.Figures[0].NotesText, notes)
	}
}

func TestPurgeLibraryRemovesStoredAssets(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	if err := os.WriteFile(filepath.Join(cfg.PapersDir(), paper.StoredPDFName), []byte("pdf"), 0o644); err != nil {
		t.Fatalf("WriteFile(pdf) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), []byte("img"), 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	if err := svc.PurgeLibrary(); err != nil {
		t.Fatalf("PurgeLibrary() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.PapersDir(), paper.StoredPDFName)); !os.IsNotExist(err) {
		t.Fatalf("paper file still exists, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)); !os.IsNotExist(err) {
		t.Fatalf("figure file still exists, stat err = %v", err)
	}

	result, err := svc.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if result.Total != 0 || len(result.Papers) != 0 {
		t.Fatalf("ListPapers() after purge = total:%d len:%d", result.Total, len(result.Papers))
	}
}

func TestExtractorSettingsDefaultsAndPersistence(t *testing.T) {
	svc, _, cfg := newTestService(t)

	defaults, err := svc.GetExtractorSettings()
	if err != nil {
		t.Fatalf("GetExtractorSettings() default error = %v", err)
	}
	if defaults.ExtractorFileField != "file" || defaults.TimeoutSeconds != cfg.ExtractorTimeoutSeconds {
		t.Fatalf("GetExtractorSettings() defaults = %+v, want config defaults", defaults)
	}

	updated, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
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
	if updated.ExtractorJobsURL != "" {
		t.Fatalf("UpdateExtractorSettings() extractor_jobs_url = %q, want empty", updated.ExtractorJobsURL)
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
}

func TestUploadPaperWithoutExtractorConfiguredUsesManualPending(t *testing.T) {
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

	if paper.ExtractionStatus != manualPendingStatus {
		t.Fatalf("UploadPaper() status = %q, want %q", paper.ExtractionStatus, manualPendingStatus)
	}
	if paper.ExtractorMessage == "" {
		t.Fatalf("UploadPaper() extractor_message should not be empty")
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

func TestDeleteFigureRemovesFileAndReturnsUpdatedPaper(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	figurePath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	if err := os.WriteFile(figurePath, []byte("img"), 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	updated, err := svc.DeleteFigure(paper.Figures[0].ID)
	if err != nil {
		t.Fatalf("DeleteFigure() error = %v", err)
	}
	if len(updated.Figures) != 0 {
		t.Fatalf("DeleteFigure() figures = %d, want 0", len(updated.Figures))
	}
	if _, err := os.Stat(figurePath); !os.IsNotExist(err) {
		t.Fatalf("figure file still exists, stat err = %v", err)
	}
}

func TestNormalizeManualRegionRejectsMissingImageData(t *testing.T) {
	if _, err := normalizeManualRegion(model.ManualExtractionRegion{
		PageNumber: 1,
		X:          0.1,
		Y:          0.1,
		Width:      0.4,
		Height:     0.4,
	}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("normalizeManualRegion() code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestManualExtractFiguresStoresClientRenderedImage(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	if err := os.WriteFile(filepath.Join(cfg.PapersDir(), paper.StoredPDFName), []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatalf("WriteFile(pdf) error = %v", err)
	}

	updated, addedCount, err := svc.ManualExtractFigures(paper.ID, ManualExtractParams{
		Regions: []model.ManualExtractionRegion{
			{
				PageNumber: 1,
				X:          0.1,
				Y:          0.2,
				Width:      0.3,
				Height:     0.4,
				ImageData:  testPNGDataURL(t, 24, 18),
				Caption:    "Manual figure",
			},
		},
	})
	if err != nil {
		t.Fatalf("ManualExtractFigures() error = %v", err)
	}

	if addedCount != 1 {
		t.Fatalf("ManualExtractFigures() addedCount = %d, want 1", addedCount)
	}
	if len(updated.Figures) != 2 {
		t.Fatalf("ManualExtractFigures() figures = %d, want 2", len(updated.Figures))
	}

	var manualFigure *model.Figure
	for i := range updated.Figures {
		if updated.Figures[i].Source == "manual" {
			manualFigure = &updated.Figures[i]
			break
		}
	}
	if manualFigure == nil {
		t.Fatalf("ManualExtractFigures() missing manual figure: %+v", updated.Figures)
	}
	if manualFigure.Caption != "Manual figure" {
		t.Fatalf("ManualExtractFigures() caption = %q, want %q", manualFigure.Caption, "Manual figure")
	}
	if !strings.HasSuffix(manualFigure.Filename, ".png") {
		t.Fatalf("ManualExtractFigures() filename = %q, want .png suffix", manualFigure.Filename)
	}
	if _, err := os.Stat(filepath.Join(cfg.FiguresDir(), manualFigure.Filename)); err != nil {
		t.Fatalf("stored manual figure missing, stat err = %v", err)
	}
}
