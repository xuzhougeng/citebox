package model

import "time"

const (
	FigureTypeFigure            = "figure"
	FigureTypeGraphicalAbstract = "graphical_abstract"
)

// NormalizeFigureType maps unknown figure types to the plain figure default.
func NormalizeFigureType(figureType string) string {
	switch figureType {
	case FigureTypeGraphicalAbstract:
		return FigureTypeGraphicalAbstract
	default:
		return FigureTypeFigure
	}
}

type FigureListItem struct {
	ID                 int64     `json:"id"`
	PaperID            int64     `json:"paper_id"`
	PaperTitle         string    `json:"paper_title"`
	GroupID            *int64    `json:"group_id,omitempty"`
	GroupName          string    `json:"group_name,omitempty"`
	Tags               []Tag     `json:"tags"`
	Filename           string    `json:"filename"`
	ImageURL           string    `json:"image_url,omitempty"`
	PageNumber         int       `json:"page_number"`
	FigureIndex        int       `json:"figure_index"`
	ParentFigureID     *int64    `json:"parent_figure_id,omitempty"`
	SubfigureLabel     string    `json:"subfigure_label,omitempty"`
	DisplayLabel       string    `json:"display_label,omitempty"`
	ParentDisplayLabel string    `json:"parent_display_label,omitempty"`
	Source             string    `json:"source,omitempty"`
	FigureType         string    `json:"figure_type,omitempty"`
	Caption            string    `json:"caption"`
	NotesText          string    `json:"notes_text,omitempty"`
	PaletteID          *int64    `json:"palette_id,omitempty"`
	PaletteName        string    `json:"palette_name,omitempty"`
	PaletteColors      []string  `json:"palette_colors,omitempty"`
	PaletteCount       int       `json:"palette_count,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type SubfigureExtractionRegion struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	ImageData string  `json:"image_data,omitempty"`
	Caption   string  `json:"caption,omitempty"`
	Label     string  `json:"label,omitempty"`
}

const (
	FigureTagModeAll = "all"
	FigureTagModeAny = "any"
)

// MaxFigureFilterTags caps how many tags one figure query may combine.
const MaxFigureFilterTags = 20

type FigureFilter struct {
	Keyword string `json:"keyword"`
	PaperID *int64 `json:"paper_id,omitempty"`
	GroupID *int64 `json:"group_id,omitempty"`
	// TagIDs filters by figure tags; TagMode decides whether a figure must
	// carry every tag (FigureTagModeAll, the default) or any one of them.
	TagIDs     []int64 `json:"tag_ids,omitempty"`
	TagMode    string  `json:"tag_mode,omitempty"`
	FigureType string  `json:"figure_type,omitempty"`
	HasNotes   bool    `json:"has_notes,omitempty"`
	SortBy     string  `json:"sort_by,omitempty"`
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
}

type FigureListResponse struct {
	Figures    []FigureListItem `json:"figures"`
	Total      int              `json:"total"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}
