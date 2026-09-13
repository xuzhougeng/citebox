package repository

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func (r *LibraryRepository) CreateFigureAIJob(in model.AIFigureJobRequest, figures []model.Figure) (int64, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var active int64
	err = tx.QueryRow(`SELECT id FROM ai_figure_jobs WHERE paper_id=? AND status IN ('queued','running')`, in.PaperID).Scan(&active)
	if err == nil {
		return active, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := tx.Exec(`INSERT INTO ai_figure_jobs(paper_id,scope,note_mode,language,status) VALUES(?,?,?,?,'queued')`, in.PaperID, in.Scope, in.Mode, in.Language)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, f := range figures {
		if _, err := tx.Exec(`INSERT INTO ai_figure_job_items(job_id,figure_id,sort_order,original_notes) VALUES(?,?,?,?)`, id, f.ID, i, f.NotesText); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

func (r *LibraryRepository) GetFigureAIJob(id int64) (*model.AIFigureJob, error) {
	job := &model.AIFigureJob{Items: []model.AIFigureJobItem{}}
	err := r.db.QueryRow(`SELECT id,paper_id,scope,note_mode,language,status FROM ai_figure_jobs WHERE id=?`, id).Scan(&job.ID, &job.PaperID, &job.Scope, &job.Mode, &job.Language, &job.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.New(apperr.CodeNotFound, "AI task not found")
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(`SELECT figure_id,status,attempts,error FROM ai_figure_job_items WHERE job_id=? ORDER BY sort_order`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item model.AIFigureJobItem
		if err := rows.Scan(&item.FigureID, &item.Status, &item.Attempts, &item.Error); err != nil {
			return nil, err
		}
		job.Items = append(job.Items, item)
	}
	return job, rows.Err()
}

func (r *LibraryRepository) LatestFigureAIJob(paperID int64) (*model.AIFigureJob, error) {
	var id int64
	err := r.db.QueryRow(`SELECT id FROM ai_figure_jobs WHERE paper_id=? ORDER BY id DESC LIMIT 1`, paperID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.GetFigureAIJob(id)
}

func (r *LibraryRepository) RecoverFigureAIJobs() error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE ai_figure_job_items SET status='pending' WHERE status='running'`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE ai_figure_jobs SET status='stopped',updated_at=CURRENT_TIMESTAMP WHERE status IN ('queued','running')`); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *LibraryRepository) ResumeFigureAIJob(id int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE ai_figure_jobs SET status='queued',updated_at=CURRENT_TIMESTAMP WHERE id=? AND status IN ('failed','stopped')`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return apperr.New(apperr.CodeConflict, "AI task is not resumable")
	}
	if _, err := tx.Exec(`UPDATE ai_figure_job_items SET status='pending',error='' WHERE job_id=? AND status!='completed'`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *LibraryRepository) ClaimFigureAIJob(id int64) (bool, error) {
	res, err := r.db.Exec(`UPDATE ai_figure_jobs SET status='running',updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='queued'`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r *LibraryRepository) StartFigureAIJobItem(jobID, figureID int64) (bool, error) {
	res, err := r.db.Exec(`UPDATE ai_figure_job_items SET status='running',attempts=attempts+1,error='' WHERE job_id=? AND figure_id=? AND status='pending' AND EXISTS(SELECT 1 FROM ai_figure_jobs WHERE id=? AND status='running')`, jobID, figureID, jobID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// Note mutation and item completion commit together. A retry cannot append the
// same answer twice; append reads current notes and overwrite checks the snapshot.
func (r *LibraryRepository) CompleteFigureAIJobItem(jobID, figureID int64, answer string) error {
	if strings.TrimSpace(answer) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "empty figure answer")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var mode, original, current string
	err = tx.QueryRow(`SELECT j.note_mode,i.original_notes,COALESCE(f.notes_text,'') FROM ai_figure_jobs j JOIN ai_figure_job_items i ON i.job_id=j.id JOIN paper_figures f ON f.id=i.figure_id WHERE j.id=? AND i.figure_id=? AND j.status='running' AND i.status='running'`, jobID, figureID).Scan(&mode, &original, &current)
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.New(apperr.CodeConflict, "AI task item is no longer running")
	}
	if err != nil {
		return err
	}
	if mode == "overwrite" && current != original {
		return apperr.New(apperr.CodeConflict, "Figure notes changed; existing edits were preserved")
	}
	next := strings.TrimSpace(answer)
	if mode == "append" && strings.TrimSpace(current) != "" {
		next = current + "\n\n" + next
	}
	if _, err := tx.Exec(`UPDATE paper_figures SET notes_text=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, next, figureID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE papers SET updated_at=CURRENT_TIMESTAMP WHERE id=(SELECT paper_id FROM paper_figures WHERE id=?)`, figureID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE ai_figure_job_items SET status='completed',error='' WHERE job_id=? AND figure_id=?`, jobID, figureID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *LibraryRepository) FailFigureAIJobItem(jobID, figureID int64, message string) error {
	_, err := r.db.Exec(`UPDATE ai_figure_job_items SET status='failed',error=? WHERE job_id=? AND figure_id=? AND status='running'`, message, jobID, figureID)
	return err
}

func (r *LibraryRepository) FinishFigureAIJob(id int64, status string) error {
	if status != "completed" && status != "failed" && status != "stopped" {
		return apperr.New(apperr.CodeInvalidArgument, "invalid task status")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE ai_figure_jobs SET status=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status IN ('queued','running')`, status, id); err != nil {
		return err
	}
	if status == "stopped" {
		if _, err := tx.Exec(`UPDATE ai_figure_job_items SET status='pending' WHERE job_id=? AND status='running'`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *LibraryRepository) GetExtractionPageCheckpoint(paperID int64, page int, inputHash string) (string, error) {
	var result string
	err := r.db.QueryRow(`SELECT result_json FROM extraction_page_checkpoints WHERE paper_id=? AND page_number=? AND input_hash=?`, paperID, page, inputHash).Scan(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return result, err
}
func (r *LibraryRepository) SaveExtractionPageCheckpoint(paperID int64, page int, inputHash, result string) error {
	_, err := r.db.Exec(`INSERT INTO extraction_page_checkpoints(paper_id,page_number,input_hash,result_json) VALUES(?,?,?,?) ON CONFLICT(paper_id,page_number) DO UPDATE SET input_hash=excluded.input_hash,result_json=excluded.result_json`, paperID, page, inputHash, result)
	return err
}
func (r *LibraryRepository) ClearExtractionPageCheckpoints(paperID int64) error {
	_, err := r.db.Exec(`DELETE FROM extraction_page_checkpoints WHERE paper_id=?`, paperID)
	return err
}
