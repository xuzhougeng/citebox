package repository

import "github.com/xuzhougeng/citebox/internal/model"

// PaperUpsertInput 文献创建/更新输入
type PaperUpsertInput struct {
	Title            string
	OriginalFilename string
	StoredPDFName    string
	PDFSHA256        string
	FileSize         int64
	ContentType      string
	PDFText          string
	AbstractText     string
	NotesText        string
	PaperNotesText   string
	BoxesJSON        string
	ExtractionStatus string
	ExtractorMessage string
	ExtractorJobID   string
	GroupID          *int64
	Tags             []TagUpsertInput
	Figures          []FigureUpsertInput
}

// PaperUpdateInput 文献更新输入
type PaperUpdateInput struct {
	Title          string
	PDFText        *string
	AbstractText   string
	NotesText      string
	PaperNotesText string
	GroupID        *int64
	Tags           []TagUpsertInput
}

// FigureUpsertInput 图片创建/更新输入
type FigureUpsertInput struct {
	Filename     string
	OriginalName string
	ContentType  string
	PageNumber   int
	FigureIndex  int
	Source       string
	Caption      string
	BBoxJSON     string
}

// FigureUpdateInput 图片更新输入
type FigureUpdateInput struct {
	Caption   string
	NotesText string
	Tags      []TagUpsertInput
}

// TagUpsertInput 标签创建/更新输入
type TagUpsertInput struct {
	Scope model.TagScope
	Name  string
	Color string
}

// PaperChecksumBackfillItem PDF SHA256 补全项
type PaperChecksumBackfillItem struct {
	ID            int64
	StoredPDFName string
}
