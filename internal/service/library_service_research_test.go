package service

import (
	"context"
	"testing"

	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

func TestImportPaperFromS2CreatesNewMetadataOnlyPaper(t *testing.T) {
	svc, _, _ := newTestService(t)
	p, err := svc.ImportPaperFromS2(context.Background(), research.Paper{
		PaperID:     "S2:p1",
		Title:       "Attention Is All You Need",
		ExternalIDs: research.IDs{DOI: "10.5555/aiayn"},
		Year:        2017,
		Venue:       "NeurIPS",
		Authors:     []research.Author{{Name: "Vaswani"}},
		Abstract:    "...",
	})
	if err != nil {
		t.Fatalf("ImportPaperFromS2 error: %v", err)
	}
	if p == nil {
		t.Fatal("expected paper, got nil")
	}
	if p.Title != "Attention Is All You Need" {
		t.Fatalf("title = %q", p.Title)
	}
	if p.DOI != "10.5555/aiayn" {
		t.Fatalf("DOI = %q", p.DOI)
	}
}

func TestImportPaperFromS2DeduplicatesByDOI(t *testing.T) {
	svc, repo, _ := newTestService(t)

	first, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Already Here",
		DOI:              "10.5555/dup",
		OriginalFilename: "x.pdf",
		StoredPDFName:    "x.pdf",
	})
	if err != nil {
		t.Fatalf("seed CreatePaper: %v", err)
	}

	got, err := svc.ImportPaperFromS2(context.Background(), research.Paper{
		PaperID:     "S2:other",
		Title:       "Different Title",
		ExternalIDs: research.IDs{DOI: "10.5555/dup"},
	})
	if err != nil {
		t.Fatalf("import error: %v", err)
	}
	if got.ID != first.ID {
		t.Fatalf("expected dedup → existing id %d, got %d", first.ID, got.ID)
	}
}
