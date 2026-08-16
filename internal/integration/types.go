package integration

import (
	"time"
)

// SearchLibraryParams 是跨实体检索参数
type SearchLibraryParams struct {
	Query        string     `json:"query"`
	EntityTypes  []string   `json:"entity_types"`
	GroupID      *int64     `json:"group_id"`
	Tags         []int64    `json:"tags"`
	UpdatedAfter *time.Time `json:"updated_after"`
	Cursor       string     `json:"cursor"`
	Limit        int        `json:"limit"`
}

// SearchLibraryItem 是跨实体检索的单个命中项（data 为轻量数据）
type SearchLibraryItem struct {
	SourceID   string `json:"source_id"`
	EntityType string `json:"entity_type"`
	Revision   string `json:"revision"`
	Title      string `json:"title"`
	Snippet    string `json:"snippet"`
	Data       any    `json:"data,omitempty"`
}

// SearchLibraryResult 是跨实体检索结果
type SearchLibraryResult struct {
	Items      []SearchLibraryItem `json:"items"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

// GetPaperContextParams 是文献上下文打包参数
type GetPaperContextParams struct {
	PaperID         int64    `json:"paper_id"`
	Include         []string `json:"include"`
	FigureLimit     int      `json:"figure_limit"`
	AnnotationLimit int      `json:"annotation_limit"`
}

// 文献上下文可包含的部分；Include 为空表示全部包含
const (
	IncludeMetadata    = "metadata"
	IncludeAbstract    = "abstract"
	IncludePaperNotes  = "paper_notes"
	IncludeFigureNotes = "figure_notes"
	IncludeAnnotations = "annotations"
	IncludeFigures     = "figures"
	IncludeTags        = "tags"
	IncludeGroup       = "group"
)

// GetFigureHandoffParams 是图级研究交接参数。figure_id 与 source_id 至少提供一个。
type GetFigureHandoffParams struct {
	FigureID int64  `json:"figure_id"`
	SourceID string `json:"source_id"`
}

// SearchPaperTextParams 是 PDF 全文检索参数
type SearchPaperTextParams struct {
	PaperIDs     []int64 `json:"paper_ids"`
	Query        string  `json:"query"`
	Limit        int     `json:"limit"`
	ContextChars int     `json:"context_chars"`
}

// PaperTextMatch 是 PDF 全文检索的单个命中；Page 为 1 起始页码，无逐页文本时为 nil
type PaperTextMatch struct {
	PaperID  int64  `json:"paper_id"`
	Page     *int   `json:"page"`
	Snippet  string `json:"snippet"`
	SourceID string `json:"source_id"`
	Revision string `json:"revision"`
}

// SearchPaperTextResult 是 PDF 全文检索结果
type SearchPaperTextResult struct {
	Matches []PaperTextMatch `json:"matches"`
}

// 可导出的资产类型
const (
	AssetKindFigureImage           = "figure_image"
	AssetKindFigureTransferPackage = "figure_transfer_package"
)

// AssetExport 是导出资产的描述（资产通过 URL 下载）
type AssetExport struct {
	URL       string    `json:"url"`
	ByteSize  int       `json:"byte_size"`
	MediaType string    `json:"media_type"`
	SHA256    string    `json:"sha256"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ListChangesParams 是增量变更同步参数
type ListChangesParams struct {
	Cursor      string   `json:"cursor"`
	EntityTypes []string `json:"entity_types"`
	Limit       int      `json:"limit"`
}

// ChangeItem 是单条增量变更
type ChangeItem struct {
	Operation string `json:"operation"`
	SourceID  string `json:"source_id"`
	Revision  string `json:"revision"`
}

// ListChangesResult 是增量变更同步结果
type ListChangesResult struct {
	Changes    []ChangeItem `json:"changes"`
	NextCursor string       `json:"next_cursor,omitempty"`
}
