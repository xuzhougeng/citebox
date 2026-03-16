package model

import "time"

type Tag struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Color      string    `json:"color"`
	PaperCount int       `json:"paper_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
