package model

import "time"

type TagScope string

const (
	TagScopePaper  TagScope = "paper"
	TagScopeFigure TagScope = "figure"
)

func (s TagScope) Valid() bool {
	switch s {
	case TagScopePaper, TagScopeFigure:
		return true
	default:
		return false
	}
}

func NormalizeTagScope(raw string) TagScope {
	scope := TagScope(raw)
	if scope.Valid() {
		return scope
	}
	return TagScopePaper
}

type Tag struct {
	ID          int64     `json:"id"`
	Scope       TagScope  `json:"scope"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	PaperCount  int       `json:"paper_count"`
	FigureCount int       `json:"figure_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
