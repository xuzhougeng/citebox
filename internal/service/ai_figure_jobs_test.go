package service

import (
	"context"
	"errors"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFigureJobRetriesOnlyFailedItemsAndPersistsNotes(t *testing.T) {
	_, repo, cfg := newTestService(t)
	svc := NewAIService(repo, cfg, nil)
	defer svc.Close()
	p, err := repo.CreatePaper(repository.PaperUpsertInput{Title: "Study", OriginalFilename: "study.pdf", StoredPDFName: "study.pdf", ExtractionStatus: "completed", Figures: []repository.FigureUpsertInput{{Filename: "one.png", FigureIndex: 1}, {Filename: "two.png", FigureIndex: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	calls := map[int64]int{}
	svc.jobReader = func(ctx context.Context, in model.AIReadRequest) (*model.AIReadResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		calls[in.FigureID]++
		if in.FigureID == p.Figures[1].ID && calls[in.FigureID] == 1 {
			return nil, errors.New("temporary provider failure")
		}
		return &model.AIReadResponse{Answer: "Saved result"}, nil
	}
	job, err := svc.StartFigureAIJob(model.AIFigureJobRequest{PaperID: p.ID, Scope: "all", Mode: "append", Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	svc.jobsWG.Wait()
	job, _ = svc.GetFigureAIJob(job.ID)
	if job.Status != "failed" {
		t.Fatalf("job: %+v", job)
	}
	if _, err := svc.ResumeFigureAIJob(job.ID); err != nil {
		t.Fatal(err)
	}
	svc.jobsWG.Wait()
	job, _ = svc.GetFigureAIJob(job.ID)
	if job.Status != "completed" {
		t.Fatalf("resumed: %+v", job)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls[p.Figures[0].ID] != 1 || calls[p.Figures[1].ID] != 2 {
		t.Fatalf("replayed successful model call: %v", calls)
	}
	updated, _ := repo.GetPaperDetail(p.ID)
	for _, f := range updated.Figures {
		if strings.Count(f.NotesText, "Saved result") != 1 {
			t.Fatal(f.NotesText)
		}
	}
}

func TestFigureJobStopCancelsInferenceAndKeepsItemResumable(t *testing.T) {
	_, repo, cfg := newTestService(t)
	svc := NewAIService(repo, cfg, nil)
	defer svc.Close()
	p, err := repo.CreatePaper(repository.PaperUpsertInput{Title: "Study", OriginalFilename: "study.pdf", StoredPDFName: "study.pdf", ExtractionStatus: "completed", Figures: []repository.FigureUpsertInput{{Filename: "one.png"}}})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	svc.jobReader = func(ctx context.Context, _ model.AIReadRequest) (*model.AIReadResponse, error) {
		close(started)
		<-ctx.Done()
		return &model.AIReadResponse{Answer: "Must not be saved"}, ctx.Err()
	}
	job, err := svc.StartFigureAIJob(model.AIFigureJobRequest{PaperID: p.ID, Scope: "all", Mode: "append"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	if _, err := svc.StopFigureAIJob(job.ID); err != nil {
		t.Fatal(err)
	}
	svc.jobsWG.Wait()
	job, _ = svc.GetFigureAIJob(job.ID)
	if job.Status != "stopped" || job.Items[0].Status != "pending" {
		t.Fatal(job.Status)
	}
	updated, _ := repo.GetPaperDetail(p.ID)
	if updated.Figures[0].NotesText != "" {
		t.Fatal("cancelled result saved")
	}
}
