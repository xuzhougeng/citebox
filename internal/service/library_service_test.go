package service

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"paper_image_db/internal/apperr"
	"paper_image_db/internal/config"
	"paper_image_db/internal/model"
	"paper_image_db/internal/repository"
)

func newTestService(t *testing.T) (*LibraryService, *repository.LibraryRepository, *config.Config) {
	t.Helper()

	root := t.TempDir()
	cfg := &config.Config{
		StorageDir:              filepath.Join(root, "storage"),
		DatabasePath:            filepath.Join(root, "library.db"),
		MaxUploadSize:           10 << 20,
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
		Title:        "Atlas Study Revised",
		AbstractText: "Updated abstract",
		NotesText:    "Updated notes",
		Tags:         []string{"Atlas", "Revised"},
	})
	if err != nil {
		t.Fatalf("UpdatePaper() error = %v", err)
	}

	if updated.AbstractText != "Updated abstract" || updated.NotesText != "Updated notes" {
		t.Fatalf("UpdatePaper() metadata = (%q, %q), want updated values", updated.AbstractText, updated.NotesText)
	}
	if len(updated.Tags) != 2 {
		t.Fatalf("UpdatePaper() tags = %d, want 2", len(updated.Tags))
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
