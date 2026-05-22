package service

import (
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestCreatePDFAnnotationDefaultsAndComputesPageRange(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	annotation, err := svc.CreatePDFAnnotation(paper.ID, CreatePDFAnnotationParams{
		QuoteText: "  selected text  ",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 4, Left: 0.12, Top: 0.34, Width: 0.28, Height: 0.018},
			{Page: 2, Left: 0.20, Top: 0.40, Width: 0.10, Height: 0.020},
		},
	})
	if err != nil {
		t.Fatalf("CreatePDFAnnotation() error = %v", err)
	}

	if annotation.Type != "highlight" || annotation.Color != "yellow" {
		t.Fatalf("type/color = %q/%q, want highlight/yellow", annotation.Type, annotation.Color)
	}
	if annotation.QuoteText != "selected text" {
		t.Fatalf("quote_text = %q, want trimmed selected text", annotation.QuoteText)
	}
	if annotation.PageStart != 2 || annotation.PageEnd != 4 {
		t.Fatalf("page range = %d-%d, want 2-4", annotation.PageStart, annotation.PageEnd)
	}
}

func TestCreatePDFAnnotationValidation(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)
	validFragment := model.PDFAnnotationFragment{Page: 1, Left: 0.1, Top: 0.2, Width: 0.3, Height: 0.04}

	tests := []struct {
		name   string
		params CreatePDFAnnotationParams
	}{
		{
			name:   "empty quote",
			params: CreatePDFAnnotationParams{QuoteText: "   ", Fragments: []model.PDFAnnotationFragment{validFragment}},
		},
		{
			name:   "quote too long",
			params: CreatePDFAnnotationParams{QuoteText: strings.Repeat("x", 10001), Fragments: []model.PDFAnnotationFragment{validFragment}},
		},
		{
			name:   "unsupported type",
			params: CreatePDFAnnotationParams{Type: "note", QuoteText: "selected", Fragments: []model.PDFAnnotationFragment{validFragment}},
		},
		{
			name:   "unsupported color",
			params: CreatePDFAnnotationParams{QuoteText: "selected", Color: "blue", Fragments: []model.PDFAnnotationFragment{validFragment}},
		},
		{
			name:   "missing fragments",
			params: CreatePDFAnnotationParams{QuoteText: "selected"},
		},
		{
			name:   "fragment outside page",
			params: CreatePDFAnnotationParams{QuoteText: "selected", Fragments: []model.PDFAnnotationFragment{{Page: 1, Left: 0.9, Top: 0.2, Width: 0.2, Height: 0.04}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.CreatePDFAnnotation(paper.ID, tt.params); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
				t.Fatalf("CreatePDFAnnotation() code = %q, want %q (err=%v)", apperr.CodeOf(err), apperr.CodeInvalidArgument, err)
			}
		})
	}
}

func TestPDFAnnotationServiceRequiresExistingPaper(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.CreatePDFAnnotation(999, CreatePDFAnnotationParams{
		QuoteText: "selected",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 1, Left: 0.1, Top: 0.2, Width: 0.3, Height: 0.04},
		},
	})
	if !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("CreatePDFAnnotation(missing paper) code = %q, want %q (err=%v)", apperr.CodeOf(err), apperr.CodeNotFound, err)
	}

	if _, err := svc.ListPDFAnnotations(999); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("ListPDFAnnotations(missing paper) code = %q, want %q (err=%v)", apperr.CodeOf(err), apperr.CodeNotFound, err)
	}
}

func TestDeletePDFAnnotationRequiresPaperOwnership(t *testing.T) {
	svc, repo, _ := newTestService(t)
	first := createTestPaper(t, repo)
	second, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Second Paper",
		OriginalFilename: "second.pdf",
		StoredPDFName:    "second.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
	})
	if err != nil {
		t.Fatalf("CreatePaper(second) error = %v", err)
	}
	annotation, err := svc.CreatePDFAnnotation(first.ID, CreatePDFAnnotationParams{
		QuoteText: "owned text",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 1, Left: 0.1, Top: 0.2, Width: 0.3, Height: 0.04},
		},
	})
	if err != nil {
		t.Fatalf("CreatePDFAnnotation() error = %v", err)
	}

	if err := svc.DeletePDFAnnotation(second.ID, annotation.ID); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("DeletePDFAnnotation(wrong paper) code = %q, want %q (err=%v)", apperr.CodeOf(err), apperr.CodeNotFound, err)
	}

	annotations, err := svc.ListPDFAnnotations(first.ID)
	if err != nil {
		t.Fatalf("ListPDFAnnotations() error = %v", err)
	}
	if len(annotations) != 1 {
		t.Fatalf("annotations after failed delete = %d, want 1", len(annotations))
	}
}
