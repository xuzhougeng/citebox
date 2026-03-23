package model

import "time"

type Palette struct {
	ID                 int64     `json:"id"`
	PaperID            int64     `json:"paper_id"`
	FigureID           int64     `json:"figure_id"`
	PaperTitle         string    `json:"paper_title"`
	GroupID            *int64    `json:"group_id,omitempty"`
	GroupName          string    `json:"group_name,omitempty"`
	Filename           string    `json:"filename"`
	ImageURL           string    `json:"image_url,omitempty"`
	PageNumber         int       `json:"page_number"`
	FigureIndex        int       `json:"figure_index"`
	ParentFigureID     *int64    `json:"parent_figure_id,omitempty"`
	SubfigureLabel     string    `json:"subfigure_label,omitempty"`
	FigureDisplayLabel string    `json:"figure_display_label,omitempty"`
	ParentDisplayLabel string    `json:"parent_display_label,omitempty"`
	FigureCaption      string    `json:"figure_caption,omitempty"`
	Name               string    `json:"name"`
	Colors             []string  `json:"colors"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type PaletteFilter struct {
	Keyword  string `json:"keyword"`
	GroupID  *int64 `json:"group_id,omitempty"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type PaletteListResponse struct {
	Palettes   []Palette `json:"palettes"`
	Total      int       `json:"total"`
	Page       int       `json:"page"`
	PageSize   int       `json:"page_size"`
	TotalPages int       `json:"total_pages"`
}
