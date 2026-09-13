package repository

import (
	"fmt"
	"testing"
)

func TestEvidenceCandidateBudgetIncludesLaterPreciseQueries(t *testing.T) {
	repo := newTestRepository(t)
	var preciseID int64
	for i := 0; i < 8; i++ {
		title := fmt.Sprintf("Broadterm study %d", i)
		if i == 0 {
			title = "Precisemarker study"
		}
		p, err := repo.CreatePaper(PaperUpsertInput{Title: title, OriginalFilename: fmt.Sprintf("p%d.pdf", i), StoredPDFName: fmt.Sprintf("p%d.pdf", i), ContentType: "application/pdf", ExtractionStatus: "completed"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			preciseID = p.ID
		}
	}
	ids, err := repo.Paper.ListEvidenceCandidatePaperIDs([]string{"Broadterm", "Precisemarker"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if id == preciseID {
			return
		}
	}
	t.Fatalf("later precise query starved: %v", ids)
}
