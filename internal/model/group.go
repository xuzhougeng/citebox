package model

import "time"

type Group struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	PaperCount  int       `json:"paper_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GroupStats 分组统计信息
type GroupStats struct {
	PaperCount  int `json:"paper_count"`
	FigureCount int `json:"figure_count"`
}
