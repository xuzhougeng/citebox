package repository

import (
	"strings"
	"testing"
)

func TestAppendAINotePreservesSuccessiveAppendsAndDeduplicates(t *testing.T) {
	repo := newTestRepository(t)
	p, err := repo.CreatePaper(PaperUpsertInput{Title: "Paper", OriginalFilename: "p.pdf", StoredPDFName: "p.pdf", ContentType: "application/pdf"})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		marker, body string
		saved        bool
	}{{"source-1", "source-1\nFirst", true}, {"source-2", "source-2\nSecond", true}, {"source-1", "source-1\nFirst", false}} {
		saved, err := repo.Paper.AppendAINote(p.ID, entry.marker, entry.body)
		if err != nil || saved != entry.saved {
			t.Fatalf("saved=%v err=%v", saved, err)
		}
	}
	p, err = repo.GetPaperDetail(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(p.PaperNotesText, "First") != 1 || !strings.Contains(p.PaperNotesText, "Second") {
		t.Fatal(p.PaperNotesText)
	}
}
