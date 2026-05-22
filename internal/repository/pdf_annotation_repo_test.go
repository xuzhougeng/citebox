package repository

import (
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func TestPDFAnnotationRepositoryCreateAndList(t *testing.T) {
	repo := newTestRepository(t)
	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Annotated Paper",
		OriginalFilename: "annotated.pdf",
		StoredPDFName:    "annotated.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	created, err := repo.PDFAnnotation.Create(paper.ID, PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "selected text",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 3, Left: 0.12, Top: 0.34, Width: 0.28, Height: 0.018},
			{Page: 2, Left: 0.20, Top: 0.40, Width: 0.10, Height: 0.020},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if created.ID == 0 || created.PaperID != paper.ID {
		t.Fatalf("created annotation identity = %+v, want paper_id %d with id", created, paper.ID)
	}
	if created.PageStart != 2 || created.PageEnd != 3 {
		t.Fatalf("created page range = %d-%d, want 2-3", created.PageStart, created.PageEnd)
	}
	if got := len(created.Fragments); got != 2 {
		t.Fatalf("created fragments = %d, want 2", got)
	}

	annotations, err := repo.PDFAnnotation.ListByPaperID(paper.ID)
	if err != nil {
		t.Fatalf("ListByPaperID() error = %v", err)
	}
	if len(annotations) != 1 || annotations[0].ID != created.ID {
		t.Fatalf("annotations = %+v, want created annotation", annotations)
	}
	if annotations[0].QuoteText != "selected text" || annotations[0].Color != "yellow" {
		t.Fatalf("annotation text/color = %q/%q", annotations[0].QuoteText, annotations[0].Color)
	}
}

func TestPDFAnnotationRepositoryDeleteRequiresPaperOwnership(t *testing.T) {
	repo := newTestRepository(t)
	first, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "First Paper",
		OriginalFilename: "first.pdf",
		StoredPDFName:    "first.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper(first) error = %v", err)
	}
	second, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Second Paper",
		OriginalFilename: "second.pdf",
		StoredPDFName:    "second.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper(second) error = %v", err)
	}
	created, err := repo.PDFAnnotation.Create(first.ID, PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "owned text",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 1, Left: 0.1, Top: 0.2, Width: 0.3, Height: 0.04},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.PDFAnnotation.Delete(second.ID, created.ID); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("Delete(wrong paper) code = %q, want %q (err=%v)", apperr.CodeOf(err), apperr.CodeNotFound, err)
	}

	annotations, err := repo.PDFAnnotation.ListByPaperID(first.ID)
	if err != nil {
		t.Fatalf("ListByPaperID() error = %v", err)
	}
	if len(annotations) != 1 {
		t.Fatalf("annotations after failed delete = %d, want 1", len(annotations))
	}
}

func TestPDFAnnotationRepositoryCascadesWhenPaperDeleted(t *testing.T) {
	repo := newTestRepository(t)
	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Cascade Paper",
		OriginalFilename: "cascade.pdf",
		StoredPDFName:    "cascade.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	if _, err := repo.PDFAnnotation.Create(paper.ID, PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "cascade text",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 1, Left: 0.1, Top: 0.2, Width: 0.3, Height: 0.04},
		},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.DeletePaper(paper.ID); err != nil {
		t.Fatalf("DeletePaper() error = %v", err)
	}
	annotations, err := repo.PDFAnnotation.ListByPaperID(paper.ID)
	if err != nil {
		t.Fatalf("ListByPaperID() error = %v", err)
	}
	if len(annotations) != 0 {
		t.Fatalf("annotations after paper delete = %d, want 0", len(annotations))
	}
}
