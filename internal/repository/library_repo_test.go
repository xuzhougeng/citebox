package repository

import (
	"path/filepath"
	"testing"

	"paper_image_db/internal/apperr"
	"paper_image_db/internal/model"
)

func newTestRepository(t *testing.T) *LibraryRepository {
	t.Helper()

	repo, err := NewLibraryRepository(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}

	t.Cleanup(func() {
		_ = repo.Close()
	})

	return repo
}

func TestCreatePaperAndListEntities(t *testing.T) {
	repo := newTestRepository(t)

	group, err := repo.CreateGroup("Vision", "microscopy papers")
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Cell Atlas",
		OriginalFilename: "cell-atlas.pdf",
		StoredPDFName:    "paper_1.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		GroupID:          &group.ID,
		Tags: []TagUpsertInput{
			{Name: "Cell", Color: "#111111"},
			{Name: "Microscopy", Color: "#222222"},
		},
		Figures: []FigureUpsertInput{
			{
				Filename:     "figure_1.png",
				OriginalName: "figure-original.png",
				ContentType:  "image/png",
				PageNumber:   2,
				FigureIndex:  1,
				Caption:      "Overview image",
				BBoxJSON:     `{"x":1}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	if paper.GroupID == nil || *paper.GroupID != group.ID {
		t.Fatalf("CreatePaper() group id = %v, want %d", paper.GroupID, group.ID)
	}
	if got := len(paper.Tags); got != 2 {
		t.Fatalf("CreatePaper() tags = %d, want 2", got)
	}
	if got := len(paper.Figures); got != 1 {
		t.Fatalf("CreatePaper() figures = %d, want 1", got)
	}

	papers, total, err := repo.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if total != 1 || len(papers) != 1 {
		t.Fatalf("ListPapers() total=%d len=%d, want 1/1", total, len(papers))
	}
	if got := len(papers[0].Tags); got != 2 {
		t.Fatalf("ListPapers() tags = %d, want 2", got)
	}

	figures, total, err := repo.ListFigures(model.FigureFilter{})
	if err != nil {
		t.Fatalf("ListFigures() error = %v", err)
	}
	if total != 1 || len(figures) != 1 {
		t.Fatalf("ListFigures() total=%d len=%d, want 1/1", total, len(figures))
	}
	if figures[0].PaperID != paper.ID {
		t.Fatalf("ListFigures() paper id = %d, want %d", figures[0].PaperID, paper.ID)
	}
	if got := len(figures[0].Tags); got != 2 {
		t.Fatalf("ListFigures() tags = %d, want 2", got)
	}
}

func TestGroupAndTagErrorsUseErrorCodes(t *testing.T) {
	repo := newTestRepository(t)

	if _, err := repo.CreateGroup("Cancer", ""); err != nil {
		t.Fatalf("CreateGroup() setup error = %v", err)
	}
	if _, err := repo.CreateGroup("Cancer", "duplicate"); !apperr.IsCode(err, apperr.CodeConflict) {
		t.Fatalf("CreateGroup() duplicate code = %q, want %q", apperr.CodeOf(err), apperr.CodeConflict)
	}

	if _, err := repo.UpdateGroup(999, "Missing", ""); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("UpdateGroup() missing code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
	if err := repo.DeleteGroup(999); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("DeleteGroup() missing code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}

	if _, err := repo.CreateTag("Atlas", "#123456"); err != nil {
		t.Fatalf("CreateTag() setup error = %v", err)
	}
	if _, err := repo.CreateTag("Atlas", "#654321"); !apperr.IsCode(err, apperr.CodeConflict) {
		t.Fatalf("CreateTag() duplicate code = %q, want %q", apperr.CodeOf(err), apperr.CodeConflict)
	}
	if err := repo.DeleteTag(999); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("DeleteTag() missing code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
}
