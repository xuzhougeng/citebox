package model

import (
	"encoding/json"
	"time"
)

type Figure struct {
	ID                 int64           `json:"id"`
	Filename           string          `json:"filename"`
	OriginalName       string          `json:"original_name"`
	ContentType        string          `json:"content_type"`
	PageNumber         int             `json:"page_number"`
	FigureIndex        int             `json:"figure_index"`
	ParentFigureID     *int64          `json:"parent_figure_id,omitempty"`
	SubfigureLabel     string          `json:"subfigure_label,omitempty"`
	DisplayLabel       string          `json:"display_label,omitempty"`
	ParentDisplayLabel string          `json:"parent_display_label,omitempty"`
	Source             string          `json:"source,omitempty"`
	Caption            string          `json:"caption"`
	NotesText          string          `json:"notes_text,omitempty"`
	Tags               []Tag           `json:"tags"`
	BBox               json.RawMessage `json:"bbox,omitempty"`
	ImageURL           string          `json:"image_url,omitempty"`
	Subfigures         []Figure        `json:"subfigures,omitempty"`
	PaletteID          *int64          `json:"palette_id,omitempty"`
	PaletteName        string          `json:"palette_name,omitempty"`
	PaletteColors      []string        `json:"palette_colors,omitempty"`
	PaletteCount       int             `json:"palette_count,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type Paper struct {
	ID               int64           `json:"id"`
	Title            string          `json:"title"`
	DOI              string          `json:"doi,omitempty"`
	AuthorsText      string          `json:"authors_text,omitempty"`
	Journal          string          `json:"journal,omitempty"`
	PublishedAt      string          `json:"published_at,omitempty"`
	OriginalFilename string          `json:"original_filename"`
	StoredPDFName    string          `json:"stored_pdf_name,omitempty"`
	PDFURL           string          `json:"pdf_url,omitempty"`
	FileSize         int64           `json:"file_size"`
	ContentType      string          `json:"content_type"`
	PDFText          string          `json:"pdf_text,omitempty"`
	AbstractText     string          `json:"abstract_text,omitempty"`
	NotesText        string          `json:"notes_text,omitempty"`
	PaperNotesText   string          `json:"paper_notes_text,omitempty"`
	Boxes            json.RawMessage `json:"boxes,omitempty"`
	ExtractionStatus string          `json:"extraction_status"`
	ExtractorMessage string          `json:"extractor_message,omitempty"`
	ExtractorJobID   string          `json:"extractor_job_id,omitempty"`
	GroupID          *int64          `json:"group_id,omitempty"`
	GroupName        string          `json:"group_name,omitempty"`
	Tags             []Tag           `json:"tags"`
	Figures          []Figure        `json:"figures,omitempty"`
	FigureCount      int             `json:"figure_count"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type PDFAnnotationFragment struct {
	Page   int     `json:"page"`
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type PDFAnnotation struct {
	ID        int64                   `json:"id"`
	PaperID   int64                   `json:"paper_id"`
	Type      string                  `json:"type"`
	PageStart int                     `json:"page_start"`
	PageEnd   int                     `json:"page_end"`
	QuoteText string                  `json:"quote_text"`
	Color     string                  `json:"color"`
	Fragments []PDFAnnotationFragment `json:"fragments"`
	NoteText  string                  `json:"note_text"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
}

type PDFAnnotationListItem struct {
	ID                    int64                   `json:"id"`
	PaperID               int64                   `json:"paper_id"`
	PaperTitle            string                  `json:"paper_title"`
	PaperOriginalFilename string                  `json:"paper_original_filename"`
	PaperStoredPDFName    string                  `json:"-"`
	PaperPDFURL           string                  `json:"paper_pdf_url,omitempty"`
	Type                  string                  `json:"type"`
	PageStart             int                     `json:"page_start"`
	PageEnd               int                     `json:"page_end"`
	QuoteText             string                  `json:"quote_text"`
	Color                 string                  `json:"color"`
	Fragments             []PDFAnnotationFragment `json:"fragments"`
	NoteText              string                  `json:"note_text"`
	CreatedAt             time.Time               `json:"created_at"`
	UpdatedAt             time.Time               `json:"updated_at"`
}

type PDFAnnotationListPagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type PDFAnnotationListResponse struct {
	Annotations []PDFAnnotationListItem     `json:"annotations"`
	Pagination  PDFAnnotationListPagination `json:"pagination"`
}

type PaperListResponse struct {
	Papers     []Paper `json:"papers"`
	Total      int     `json:"total"`
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
	TotalPages int     `json:"total_pages"`
}

type PaperFilter struct {
	Keyword       string `json:"keyword"`
	Author        string `json:"author,omitempty"`
	KeywordScope  string `json:"keyword_scope,omitempty"`
	GroupID       *int64 `json:"group_id,omitempty"`
	TagID         *int64 `json:"tag_id,omitempty"`
	Status        string `json:"status"`
	HasPaperNotes bool   `json:"has_paper_notes,omitempty"`
	SortBy        string `json:"sort_by,omitempty"`
	Page          int    `json:"page"`
	PageSize      int    `json:"page_size"`
}
