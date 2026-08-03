package handler

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

func TestFigureHandlerExportTransferPackage(t *testing.T) {
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
	t.Cleanup(func() { _ = repo.Close() })
	libraryService, err := service.NewLibraryService(repo, cfg, service.WithoutBackgroundJobs())
	if err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}
	handler := NewFigureHandler(libraryService)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Handler Transfer",
		OriginalFilename: "handler.pdf",
		StoredPDFName:    "handler.pdf",
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{{
			Filename:     "handler-figure.png",
			OriginalName: "handler-original.png",
			ContentType:  "image/png",
			PageNumber:   1,
			FigureIndex:  1,
			Source:       "manual",
		}},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), imageData.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	figureID := strconv.FormatInt(paper.Figures[0].ID, 10)
	req := httptest.NewRequest(http.MethodGet, "/api/figures/"+figureID+"/transfer-package", nil)
	rec := httptest.NewRecorder()
	handler.ExportTransferPackage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	if disposition := rec.Header().Get("Content-Disposition"); !strings.Contains(disposition, "citebox-figure-"+figureID+"-transfer-package.zip") {
		t.Fatalf("Content-Disposition = %q", disposition)
	}
	manifest, err := service.ValidateFigureTransferPackage(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("ValidateFigureTransferPackage() error = %v", err)
	}
	if manifest.Source.FigureID != paper.Figures[0].ID || manifest.Source.ExtractionMethod != "manual" {
		t.Fatalf("manifest = %+v", manifest)
	}
}
