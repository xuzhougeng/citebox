package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var ErrAIGeneratedImageNotFound = errors.New("ai_generated_image not found")

type AIGeneratedImage struct {
	ID              int64
	TurnRunID       int64
	ConversationID  int64
	FilePath        string
	Prompt          string
	Model           string
	Size            string
	Quality         string
	SourcePaperIDs  []int64
	SourceFigureIDs []int64
	CostEstimateUSD sql.NullFloat64
	CreatedAt       time.Time
}

type AIGeneratedImageRepository struct {
	db *sql.DB
}

func NewAIGeneratedImageRepository(db *sql.DB) *AIGeneratedImageRepository {
	return &AIGeneratedImageRepository{db: db}
}

func (r *AIGeneratedImageRepository) Insert(img AIGeneratedImage) (int64, error) {
	paperIDs, err := json.Marshal(nonNilInt64Slice(img.SourcePaperIDs))
	if err != nil {
		return 0, err
	}
	figureIDs, err := json.Marshal(nonNilInt64Slice(img.SourceFigureIDs))
	if err != nil {
		return 0, err
	}
	res, err := r.db.Exec(`
		INSERT INTO ai_generated_images (
			turn_run_id, conversation_id, file_path, prompt,
			model, size, quality,
			source_paper_ids, source_figure_ids, cost_estimate_usd
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		img.TurnRunID, img.ConversationID, img.FilePath, img.Prompt,
		img.Model, img.Size, img.Quality,
		string(paperIDs), string(figureIDs), nullFloatToInterface(img.CostEstimateUSD),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *AIGeneratedImageRepository) GetByID(id int64) (AIGeneratedImage, error) {
	var img AIGeneratedImage
	var paperIDs, figureIDs string
	err := r.db.QueryRow(`
		SELECT id, turn_run_id, conversation_id, file_path, prompt,
		       model, size, quality, source_paper_ids, source_figure_ids,
		       cost_estimate_usd, created_at
		FROM ai_generated_images
		WHERE id = ?
	`, id).Scan(
		&img.ID, &img.TurnRunID, &img.ConversationID, &img.FilePath, &img.Prompt,
		&img.Model, &img.Size, &img.Quality, &paperIDs, &figureIDs,
		&img.CostEstimateUSD, &img.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AIGeneratedImage{}, ErrAIGeneratedImageNotFound
	}
	if err != nil {
		return AIGeneratedImage{}, err
	}
	if err := json.Unmarshal([]byte(paperIDs), &img.SourcePaperIDs); err != nil {
		return AIGeneratedImage{}, err
	}
	if err := json.Unmarshal([]byte(figureIDs), &img.SourceFigureIDs); err != nil {
		return AIGeneratedImage{}, err
	}
	return img, nil
}

func (r *AIGeneratedImageRepository) ListByConversation(conversationID int64) ([]AIGeneratedImage, error) {
	rows, err := r.db.Query(`
		SELECT id, turn_run_id, conversation_id, file_path, prompt,
		       model, size, quality, source_paper_ids, source_figure_ids,
		       cost_estimate_usd, created_at
		FROM ai_generated_images
		WHERE conversation_id = ?
		ORDER BY id ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AIGeneratedImage, 0)
	for rows.Next() {
		var img AIGeneratedImage
		var paperIDs, figureIDs string
		if err := rows.Scan(
			&img.ID, &img.TurnRunID, &img.ConversationID, &img.FilePath, &img.Prompt,
			&img.Model, &img.Size, &img.Quality, &paperIDs, &figureIDs,
			&img.CostEstimateUSD, &img.CreatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(paperIDs), &img.SourcePaperIDs)
		_ = json.Unmarshal([]byte(figureIDs), &img.SourceFigureIDs)
		out = append(out, img)
	}
	return out, rows.Err()
}

func nonNilInt64Slice(in []int64) []int64 {
	if in == nil {
		return []int64{}
	}
	return in
}

func nullFloatToInterface(v sql.NullFloat64) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Float64
}
