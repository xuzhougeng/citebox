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

func TestPDFAnnotationRepositoryListGlobalIncludesPaperFieldsAndSorts(t *testing.T) {
	repo := newTestRepository(t)
	first, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Alpha Signaling",
		DOI:              "10.1000/alpha",
		OriginalFilename: "alpha-original.pdf",
		StoredPDFName:    "alpha stored.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper(first) error = %v", err)
	}
	second, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Beta Metabolism",
		DOI:              "10.1000/beta",
		OriginalFilename: "beta-original.pdf",
		StoredPDFName:    "beta.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper(second) error = %v", err)
	}
	firstAnnotation, err := repo.PDFAnnotation.Create(first.ID, PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "alpha selected text",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 4, Left: 0.10, Top: 0.20, Width: 0.30, Height: 0.04},
		},
	})
	if err != nil {
		t.Fatalf("Create(first annotation) error = %v", err)
	}
	secondAnnotation, err := repo.PDFAnnotation.Create(second.ID, PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "beta selected text",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 2, Left: 0.11, Top: 0.21, Width: 0.31, Height: 0.04},
		},
	})
	if err != nil {
		t.Fatalf("Create(second annotation) error = %v", err)
	}

	items, total, err := repo.PDFAnnotation.ListGlobal(PDFAnnotationListFilter{
		Sort:     "updated_desc",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListGlobal() error = %v", err)
	}

	if total != 2 || len(items) != 2 {
		t.Fatalf("total/items = %d/%d, want 2/2", total, len(items))
	}
	if items[0].ID != secondAnnotation.ID || items[1].ID != firstAnnotation.ID {
		t.Fatalf("ids = %d,%d want %d,%d", items[0].ID, items[1].ID, secondAnnotation.ID, firstAnnotation.ID)
	}
	if items[0].PaperTitle != "Beta Metabolism" || items[0].PaperOriginalFilename != "beta-original.pdf" || items[0].PaperStoredPDFName != "beta.pdf" {
		t.Fatalf("paper fields = %+v", items[0])
	}
	if items[1].PageStart != 4 || items[1].QuoteText != "alpha selected text" {
		t.Fatalf("annotation fields = %+v", items[1])
	}
}

func TestPDFAnnotationRepositoryListGlobalSearchesQuoteAndPaper(t *testing.T) {
	repo := newTestRepository(t)
	first, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Immune Atlas",
		DOI:              "10.1000/immune",
		OriginalFilename: "immune.pdf",
		StoredPDFName:    "immune.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper(first) error = %v", err)
	}
	second, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Metabolic Paper",
		DOI:              "10.1000/metabolic",
		OriginalFilename: "metabolic.pdf",
		StoredPDFName:    "metabolic.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper(second) error = %v", err)
	}
	if _, err := repo.PDFAnnotation.Create(first.ID, PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "selected immune evidence",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{{Page: 1, Left: 0.1, Top: 0.2, Width: 0.3, Height: 0.04}},
	}); err != nil {
		t.Fatalf("Create(first annotation) error = %v", err)
	}
	if _, err := repo.PDFAnnotation.Create(second.ID, PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "selected metabolic evidence",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{{Page: 2, Left: 0.1, Top: 0.2, Width: 0.3, Height: 0.04}},
	}); err != nil {
		t.Fatalf("Create(second annotation) error = %v", err)
	}

	items, total, err := repo.PDFAnnotation.ListGlobal(PDFAnnotationListFilter{
		Query:    "immune",
		Sort:     "created_desc",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListGlobal(query) error = %v", err)
	}

	if total != 1 || len(items) != 1 || items[0].PaperID != first.ID {
		t.Fatalf("query result total/items = %d/%+v, want first paper only", total, items)
	}

	items, total, err = repo.PDFAnnotation.ListGlobal(PDFAnnotationListFilter{
		Query:    "10.1000/metabolic",
		Sort:     "created_desc",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListGlobal(doi query) error = %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].PaperID != second.ID {
		t.Fatalf("doi query result total/items = %d/%+v, want second paper only", total, items)
	}
}
