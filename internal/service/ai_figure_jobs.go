package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func (s *AIService) StartFigureAIJob(in model.AIFigureJobRequest) (*model.AIFigureJob, error) {
	if in.PaperID <= 0 || (in.Scope != "missing" && in.Scope != "all") || (in.Mode != "append" && in.Mode != "overwrite") {
		return nil, apperr.New(apperr.CodeInvalidArgument, "Invalid figure task options")
	}
	if in.Language != "en" {
		in.Language = "zh-CN"
	}
	paper, err := s.repo.GetPaperDetail(in.PaperID)
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "Paper not found")
	}
	var figures []model.Figure
	for _, f := range paper.Figures {
		if f.ParentFigureID == nil && (in.Scope == "all" || strings.TrimSpace(f.NotesText) == "") {
			figures = append(figures, f)
		}
	}
	if len(figures) == 0 {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "No figures need interpretation")
	}
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	if s.jobsClosed {
		return nil, apperr.New(apperr.CodeUnavailable, "AI service is closing")
	}
	id, err := s.repo.CreateFigureAIJob(in, figures)
	if err != nil {
		return nil, err
	}
	s.startFigureJobLocked(id)
	return s.repo.GetFigureAIJob(id)
}

func (s *AIService) GetFigureAIJob(id int64) (*model.AIFigureJob, error) {
	return s.repo.GetFigureAIJob(id)
}
func (s *AIService) LatestFigureAIJob(paperID int64) (*model.AIFigureJob, error) {
	return s.repo.LatestFigureAIJob(paperID)
}
func (s *AIService) RecoverFigureAIJobs() error { return s.repo.RecoverFigureAIJobs() }

func (s *AIService) ResumeFigureAIJob(id int64) (*model.AIFigureJob, error) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	if s.jobsClosed {
		return nil, apperr.New(apperr.CodeUnavailable, "AI service is closing")
	}
	if s.jobCancels[id] != nil {
		return nil, apperr.New(apperr.CodeConflict, "AI task is still stopping; retry shortly")
	}
	if err := s.repo.ResumeFigureAIJob(id); err != nil {
		return nil, err
	}
	s.startFigureJobLocked(id)
	return s.repo.GetFigureAIJob(id)
}

func (s *AIService) StopFigureAIJob(id int64) (*model.AIFigureJob, error) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	if cancel := s.jobCancels[id]; cancel != nil {
		cancel()
	}
	if err := s.repo.FinishFigureAIJob(id, "stopped"); err != nil {
		return nil, err
	}
	return s.repo.GetFigureAIJob(id)
}

func (s *AIService) startFigureJobLocked(id int64) {
	if s.jobCancels == nil {
		s.jobCancels = map[int64]context.CancelFunc{}
	}
	if s.jobCancels[id] != nil {
		return
	}
	if s.jobSlots == nil {
		s.jobSlots = make(chan struct{}, 2)
	}
	slots := s.jobSlots
	ctx, cancel := context.WithCancel(context.Background())
	s.jobCancels[id] = cancel
	s.jobsWG.Add(1)
	go func() {
		defer s.jobsWG.Done()
		defer func() { cancel(); s.jobsMu.Lock(); delete(s.jobCancels, id); s.jobsMu.Unlock() }()
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		case <-ctx.Done():
			_ = s.repo.FinishFigureAIJob(id, "stopped")
			return
		}
		s.runFigureAIJob(ctx, id)
	}()
}

func (s *AIService) runFigureAIJob(ctx context.Context, id int64) {
	claimed, err := s.repo.ClaimFigureAIJob(id)
	if err != nil {
		s.logger.Warn("claim figure task", "job_id", id, "error", err)
		_ = s.repo.FinishFigureAIJob(id, "failed")
		return
	}
	if !claimed {
		return
	}
	job, err := s.repo.GetFigureAIJob(id)
	if err != nil {
		_ = s.repo.FinishFigureAIJob(id, "failed")
		return
	}
	read := s.ReadPaper
	if s.jobReader != nil {
		read = s.jobReader
	}
	failed := false
	for _, item := range job.Items {
		if ctx.Err() != nil {
			_ = s.repo.FinishFigureAIJob(id, "stopped")
			return
		}
		if item.Status == "completed" {
			continue
		}
		started, err := s.repo.StartFigureAIJobItem(id, item.FigureID)
		if err != nil || !started {
			failed = true
			continue
		}
		question := fmt.Sprintf("请根据原文和图片解读 figure_id=%d，说明结果、方法和局限。区分图片观察与推断。", item.FigureID)
		if job.Language == "en" {
			question = fmt.Sprintf("Interpret figure_id=%d using its image and original paper. Explain results, methods and limitations; distinguish observations from inference.", item.FigureID)
		}
		itemCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		result, err := read(itemCtx, model.AIReadRequest{PaperID: job.PaperID, FigureID: item.FigureID, Action: model.AIActionFigureInterpretation, Question: question})
		cancel()
		if ctx.Err() != nil {
			_ = s.repo.FinishFigureAIJob(id, "stopped")
			return
		}
		if err == nil && (result == nil || strings.TrimSpace(result.Answer) == "") {
			err = apperr.New(apperr.CodeUnavailable, "Empty figure interpretation")
		}
		if err == nil {
			err = s.repo.CompleteFigureAIJobItem(id, item.FigureID, result.Answer)
		}
		if err != nil {
			failed = true
			message := []rune(err.Error())
			if len(message) > 2000 {
				message = message[:2000]
			}
			if saveErr := s.repo.FailFigureAIJobItem(id, item.FigureID, string(message)); saveErr != nil {
				s.logger.Warn("save figure task failure", "job_id", id, "error", saveErr)
			}
		}
	}
	status := "completed"
	if failed {
		status = "failed"
	}
	if err := s.repo.FinishFigureAIJob(id, status); err != nil {
		s.logger.Warn("finish figure task", "job_id", id, "error", err)
	}
}

func (s *AIService) StopFigureAIJobs() {
	if s == nil {
		return
	}
	s.jobsMu.Lock()
	s.jobsClosed = true
	for _, cancel := range s.jobCancels {
		cancel()
	}
	s.jobsMu.Unlock()
	s.jobsWG.Wait()
}
