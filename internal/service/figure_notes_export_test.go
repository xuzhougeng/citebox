package service

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func readNotesArchive(t *testing.T, file *os.File) map[string]string {
	t.Helper()
	defer file.Close()
	defer os.Remove(file.Name())
	stat, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(file, stat.Size())
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{}
	for _, entry := range r.File {
		rc, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[entry.Name] = string(b)
	}
	return entries
}

func TestExportFigureNotesIncludesEveryPageAndPreservesNotes(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	input := repository.PaperUpsertInput{Title: "Export [study]", OriginalFilename: "export.pdf", StoredPDFName: "export.pdf", DOI: "10.1234/example", AuthorsText: "A. Researcher", PublishedAt: "2025", ExtractionStatus: "completed"}
	for i := 1; i <= 201; i++ {
		input.Figures = append(input.Figures, repository.FigureUpsertInput{Filename: fmt.Sprintf("export-%d.png", i), FigureIndex: i, PageNumber: 2, Caption: "Caption <comparison>", ContentType: "image/png"})
	}
	paper, err := repo.CreatePaper(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range paper.Figures {
		if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), f.Filename), []byte("image fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	note := "## Observation\n\n- **Preserve Markdown**\n"
	if _, err := repo.Figure.UpdateFigure(paper.Figures[0].ID, repository.FigureUpdateInput{Caption: "Caption <comparison>", NotesText: note}); err != nil {
		t.Fatal(err)
	}
	other := createTestPaper(t, repo) // missing image must not affect a different paper's export.
	_ = other
	file, err := svc.ExportFigureNotes(context.Background(), model.FigureFilter{PaperID: &paper.ID, Page: 9, PageSize: 1}, "en")
	if err != nil {
		t.Fatal(err)
	}
	entries := readNotesArchive(t, file)
	if len(entries) != 403 {
		t.Fatalf("entries=%d, want 403", len(entries))
	}
	md := entries[fmt.Sprintf("figures/figure-%d.md", paper.Figures[0].ID)]
	for _, want := range []string{note, "10.1234/example", "A. Researcher", "Caption &lt;comparison&gt;", "../images/figure-"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in %s", want, md)
		}
	}
	file, err = svc.ExportFigureNotes(context.Background(), model.FigureFilter{PaperID: &paper.ID, HasNotes: true}, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(readNotesArchive(t, file)); got != 3 {
		t.Fatalf("notes-only entries=%d", got)
	}
	file, err = svc.ExportFigureNotes(context.Background(), model.FigureFilter{Keyword: "Preserve Markdown"}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(readNotesArchive(t, file)); got != 3 {
		t.Fatalf("keyword entries=%d", got)
	}
}

func TestExportFigureNotesRejectsMissingOutsideAndOversizedImages(t *testing.T) {
	exportTemp := t.TempDir()
	t.Setenv("TMPDIR", exportTemp)
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)
	imagePath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	check := func() {
		t.Helper()
		defer func() {
			files, err := os.ReadDir(exportTemp)
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 0 {
				t.Fatalf("failed export leaked temporary files: %v", files)
			}
		}()
		file, err := svc.ExportFigureNotes(context.Background(), model.FigureFilter{}, "en")
		if err == nil {
			file.Close()
			os.Remove(file.Name())
			t.Fatal("expected error")
		}
	}
	check()
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, imagePath); err != nil {
		t.Fatal(err)
	}
	check()
	os.Remove(imagePath)
	f, err := os.Create(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(maxFigureNotesExportBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	check()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.ExportFigureNotes(ctx, model.FigureFilter{}, "en"); err != context.Canceled {
		t.Fatalf("canceled=%v", err)
	}
}
