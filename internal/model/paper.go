package model

import (
	"encoding/json"
	"time"
)

type Figure struct {
	ID           int64           `json:"id"`
	Filename     string          `json:"filename"`
	OriginalName string          `json:"original_name"`
	ContentType  string          `json:"content_type"`
	PageNumber   int             `json:"page_number"`
	FigureIndex  int             `json:"figure_index"`
	Source       string          `json:"source,omitempty"`
	Caption      string          `json:"caption"`
	Tags         []Tag           `json:"tags"`
	BBox         json.RawMessage `json:"bbox,omitempty"`
	ImageURL     string          `json:"image_url,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

type Paper struct {
	ID               int64           `json:"id"`
	Title            string          `json:"title"`
	OriginalFilename string          `json:"original_filename"`
	StoredPDFName    string          `json:"stored_pdf_name,omitempty"`
	PDFURL           string          `json:"pdf_url,omitempty"`
	FileSize         int64           `json:"file_size"`
	ContentType      string          `json:"content_type"`
	PDFText          string          `json:"pdf_text,omitempty"`
	AbstractText     string          `json:"abstract_text,omitempty"`
	NotesText        string          `json:"notes_text,omitempty"`
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

type PaperListResponse struct {
	Papers     []Paper `json:"papers"`
	Total      int     `json:"total"`
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
	TotalPages int     `json:"total_pages"`
}

type PaperFilter struct {
	Keyword  string `json:"keyword"`
	GroupID  *int64 `json:"group_id,omitempty"`
	TagID    *int64 `json:"tag_id,omitempty"`
	Status   string `json:"status"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}
