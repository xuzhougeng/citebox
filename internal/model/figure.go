package model

import "time"

type FigureListItem struct {
	ID          int64     `json:"id"`
	PaperID     int64     `json:"paper_id"`
	PaperTitle  string    `json:"paper_title"`
	GroupID     *int64    `json:"group_id,omitempty"`
	GroupName   string    `json:"group_name,omitempty"`
	Tags        []Tag     `json:"tags"`
	Filename    string    `json:"filename"`
	ImageURL    string    `json:"image_url,omitempty"`
	PageNumber  int       `json:"page_number"`
	FigureIndex int       `json:"figure_index"`
	Source      string    `json:"source,omitempty"`
	Caption     string    `json:"caption"`
	NotesText   string    `json:"notes_text,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type FigureFilter struct {
	Keyword  string `json:"keyword"`
	GroupID  *int64 `json:"group_id,omitempty"`
	TagID    *int64 `json:"tag_id,omitempty"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type FigureListResponse struct {
	Figures    []FigureListItem `json:"figures"`
	Total      int              `json:"total"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}
