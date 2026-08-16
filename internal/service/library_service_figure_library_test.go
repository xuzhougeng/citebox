package service

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestFigureLibrarySettingsRejectRelativeDir(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.UpdateFigureLibrarySettings(model.FigureLibrarySettings{DropDir: "inbox/citebox"}); err == nil || !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("relative dir error = %v", err)
	}
}

func TestSendFigureToFigureLibraryWritesPackage(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	dropDir := t.TempDir()
	if _, err := svc.UpdateFigureLibrarySettings(model.FigureLibrarySettings{DropDir: dropDir}); err != nil {
		t.Fatalf("UpdateFigureLibrarySettings() error = %v", err)
	}
	status, err := svc.GetFigureLibraryStatus()
	if err != nil || !status.Ready {
		t.Fatalf("GetFigureLibraryStatus() = %+v err=%v", status, err)
	}

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Atlas",
		OriginalFilename: "atlas.pdf",
		StoredPDFName:    "atlas.pdf",
		ExtractionStatus: "completed",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	figureDir := cfg.FiguresDir()
	if err := os.MkdirAll(figureDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	imageName := "figure_1.png"
	png := []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 1, 0, 0, 0, 1, 8, 2, 0, 0, 0, 144, 119, 83, 222, 0, 0, 0, 12, 73, 68, 65, 84, 8, 215, 99, 248, 207, 192, 0, 0, 3, 1, 1, 0, 24, 221, 141, 176, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130}
	if err := os.WriteFile(filepath.Join(figureDir, imageName), png, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := repo.AddPaperFigures(paper.ID, []repository.FigureUpsertInput{{
		Filename:     imageName,
		OriginalName: "fig1.png",
		ContentType:  "image/png",
		PageNumber:   1,
		FigureIndex:  1,
		Source:       "manual",
		Caption:      "Fig 1",
	}}); err != nil {
		t.Fatalf("AddPaperFigures() error = %v", err)
	}
	detail, err := repo.GetPaperDetail(paper.ID)
	if err != nil || detail == nil || len(detail.Figures) == 0 {
		t.Fatalf("GetPaperDetail() = %+v err=%v", detail, err)
	}
	figureID := detail.Figures[0].ID

	sent, err := svc.SendFigureToFigureLibrary(figureID)
	if err != nil {
		t.Fatalf("SendFigureToFigureLibrary() error = %v", err)
	}
	if sent.Path == "" || sent.FigureID != figureID {
		t.Fatalf("send result = %+v", sent)
	}
	data, err := os.ReadFile(sent.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.HasPrefix(data, []byte("PK")) {
		t.Fatalf("written file is not a zip: %q", data[:min(4, len(data))])
	}
}

func TestSendFigureToFigureLibraryRequiresSettings(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.SendFigureToFigureLibrary(1); err == nil || !apperr.IsCode(err, apperr.CodeFailedPrecondition) {
		t.Fatalf("unconfigured send error = %v", err)
	}
}
