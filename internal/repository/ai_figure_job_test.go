package repository

import (
	"github.com/xuzhougeng/citebox/internal/model"
	"path/filepath"
	"strings"
	"testing"
)

func createJobPaper(t *testing.T, repo *LibraryRepository) *model.Paper {
	t.Helper()
	p, err := repo.CreatePaper(PaperUpsertInput{Title: "Job study", OriginalFilename: "job.pdf", StoredPDFName: "job.pdf", ExtractionStatus: "completed", Figures: []FigureUpsertInput{{Filename: "one.png", FigureIndex: 1}, {Filename: "two.png", FigureIndex: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFigureJobCompletionIsAtomicAndPreservesConcurrentAppend(t *testing.T) {
	repo := newTestRepository(t)
	p := createJobPaper(t, repo)
	id, err := repo.CreateFigureAIJob(model.AIFigureJobRequest{PaperID: p.ID, Scope: "all", Mode: "append", Language: "en"}, p.Figures)
	if err != nil {
		t.Fatal(err)
	}
	repo.ClaimFigureAIJob(id)
	repo.StartFigureAIJobItem(id, p.Figures[0].ID)
	if _, err := repo.DB().Exec(`UPDATE paper_figures SET notes_text='Concurrent human edit' WHERE id=?`, p.Figures[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.CompleteFigureAIJobItem(id, p.Figures[0].ID, "AI result"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CompleteFigureAIJobItem(id, p.Figures[0].ID, "AI result"); err == nil {
		t.Fatal("duplicate completion accepted")
	}
	updated, _ := repo.GetPaperDetail(p.ID)
	if updated.Figures[0].NotesText != "Concurrent human edit\n\nAI result" {
		t.Fatal(updated.Figures[0].NotesText)
	}
	job, _ := repo.GetFigureAIJob(id)
	if job.Items[0].Status != "completed" {
		t.Fatal("note and checkpoint diverged")
	}
}

func TestFigureJobOverwriteProtectsEditsAndRecoverySkipsCompletedItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.db")
	repo, err := NewLibraryRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	p := createJobPaper(t, repo)
	id, err := repo.CreateFigureAIJob(model.AIFigureJobRequest{PaperID: p.ID, Scope: "all", Mode: "overwrite", Language: "en"}, p.Figures)
	if err != nil {
		t.Fatal(err)
	}
	repo.ClaimFigureAIJob(id)
	repo.StartFigureAIJobItem(id, p.Figures[0].ID)
	if err := repo.CompleteFigureAIJobItem(id, p.Figures[0].ID, "Done"); err != nil {
		t.Fatal(err)
	}
	repo.StartFigureAIJobItem(id, p.Figures[1].ID)
	repo.DB().Exec(`UPDATE paper_figures SET notes_text='Human edit' WHERE id=?`, p.Figures[1].ID)
	if err := repo.CompleteFigureAIJobItem(id, p.Figures[1].ID, "Replace"); err == nil {
		t.Fatal("overwrite discarded a concurrent edit")
	}
	repo.Close()
	repo, err = NewLibraryRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.RecoverFigureAIJobs(); err != nil {
		t.Fatal(err)
	}
	job, _ := repo.GetFigureAIJob(id)
	if job.Status != "stopped" || job.Items[0].Status != "completed" || job.Items[1].Status != "pending" {
		t.Fatalf("restart: %+v", job)
	}
	if err := repo.ResumeFigureAIJob(id); err != nil {
		t.Fatal(err)
	}
	repo.ClaimFigureAIJob(id)
	if started, _ := repo.StartFigureAIJobItem(id, p.Figures[0].ID); started {
		t.Fatal("completed item replayed")
	}
}

func TestExtractionCheckpointInvalidationAndPaperDeletion(t *testing.T) {
	repo := newTestRepository(t)
	p := createJobPaper(t, repo)
	if err := repo.SaveExtractionPageCheckpoint(p.ID, 1, "old", `{"version":1}`); err != nil {
		t.Fatal(err)
	}
	if data, err := repo.GetExtractionPageCheckpoint(p.ID, 1, "new"); err != nil || data != "" {
		t.Fatal("stale checkpoint reused")
	}
	if data, _ := repo.GetExtractionPageCheckpoint(p.ID, 1, "old"); !strings.Contains(data, "version") {
		t.Fatal("checkpoint missing")
	}
	if _, err := repo.DB().Exec(`DELETE FROM papers WHERE id=?`, p.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	repo.DB().QueryRow(`SELECT count(*) FROM extraction_page_checkpoints`).Scan(&count)
	if count != 0 {
		t.Fatal("orphan checkpoint retained")
	}
}
