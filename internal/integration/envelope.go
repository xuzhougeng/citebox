package integration

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/buildinfo"
)

// ResearchContextSchema 是研究上下文信封的 schema 版本标识
const ResearchContextSchema = "citebox.research-context/v1"

// 实体类型
const (
	EntityTypePaper      = "paper"
	EntityTypeFigure     = "figure"
	EntityTypeNote       = "note"
	EntityTypeAnnotation = "annotation"
)

// Envelope 是研究上下文响应的统一信封
type Envelope struct {
	SchemaVersion string         `json:"schema_version"`
	SourceID      string         `json:"source_id"`
	EntityType    string         `json:"entity_type"`
	Revision      string         `json:"revision"`
	Data          any            `json:"data,omitempty"`
	Relations     []any          `json:"relations,omitempty"`
	Assets        []any          `json:"assets,omitempty"`
	Provenance    map[string]any `json:"provenance,omitempty"`
	Permissions   []string       `json:"permissions"`
	DeepLink      string         `json:"deep_link"`
}

// SourceRef 是解析后的 source_id
type SourceRef struct {
	Kind       string // paper | figure | annotation | note
	ID         int64  // 实体 ID；note 类型时为父实体 ID
	NoteParent string // note 类型时为父实体类型（paper | figure）
	NoteName   string // note 类型时为笔记名称（当前仅支持 main）
}

// String 把 SourceRef 格式化回 source_id
func (r SourceRef) String() string {
	switch r.Kind {
	case EntityTypeNote:
		return fmt.Sprintf("citebox:note:%s:%d:%s", r.NoteParent, r.ID, r.NoteName)
	default:
		return fmt.Sprintf("citebox:%s:%d", r.Kind, r.ID)
	}
}

// DeepLink 返回实体在 CiteBox 内的深链；note 类型链接到父实体
func (r SourceRef) DeepLink() string {
	kind := r.Kind
	if kind == EntityTypeNote {
		kind = r.NoteParent
	}
	return fmt.Sprintf("citebox://%s/%d", kind, r.ID)
}

// PaperSourceID 返回文献的 source_id
func PaperSourceID(id int64) string {
	return fmt.Sprintf("citebox:paper:%d", id)
}

// FigureSourceID 返回图片的 source_id
func FigureSourceID(id int64) string {
	return fmt.Sprintf("citebox:figure:%d", id)
}

// AnnotationSourceID 返回 PDF 标注的 source_id
func AnnotationSourceID(id int64) string {
	return fmt.Sprintf("citebox:annotation:%d", id)
}

// PaperNoteSourceID 返回文献笔记的 source_id
func PaperNoteSourceID(paperID int64) string {
	return fmt.Sprintf("citebox:note:paper:%d:main", paperID)
}

// FigureNoteSourceID 返回图片笔记的 source_id
func FigureNoteSourceID(figureID int64) string {
	return fmt.Sprintf("citebox:note:figure:%d:main", figureID)
}

// ParseSourceID 解析 source_id，格式错误时返回 CodeInvalidArgument
func ParseSourceID(raw string) (SourceRef, error) {
	invalid := func() (SourceRef, error) {
		return SourceRef{}, apperr.New(apperr.CodeInvalidArgument, "source_id 格式无效")
	}
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) < 3 || parts[0] != "citebox" {
		return invalid()
	}
	parseID := func(value string) (int64, error) {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return 0, apperr.New(apperr.CodeInvalidArgument, "source_id 格式无效")
		}
		return id, nil
	}
	switch parts[1] {
	case EntityTypePaper, EntityTypeFigure, EntityTypeAnnotation:
		if len(parts) != 3 {
			return invalid()
		}
		id, err := parseID(parts[2])
		if err != nil {
			return invalid()
		}
		return SourceRef{Kind: parts[1], ID: id}, nil
	case EntityTypeNote:
		if len(parts) != 5 || (parts[2] != EntityTypePaper && parts[2] != EntityTypeFigure) {
			return invalid()
		}
		id, err := parseID(parts[3])
		if err != nil || strings.TrimSpace(parts[4]) == "" {
			return invalid()
		}
		return SourceRef{Kind: EntityTypeNote, ID: id, NoteParent: parts[2], NoteName: parts[4]}, nil
	default:
		return invalid()
	}
}

// NewEnvelope 构造信封：revision 取实体 UpdatedAt（UTC RFC3339），权限恒为 read
func NewEnvelope(ref SourceRef, updatedAt time.Time, data any) *Envelope {
	return &Envelope{
		SchemaVersion: ResearchContextSchema,
		SourceID:      ref.String(),
		EntityType:    ref.Kind,
		Revision:      RevisionOf(updatedAt),
		Data:          data,
		Provenance:    map[string]any{"citebox_version": buildinfo.CurrentVersion()},
		Permissions:   []string{"read"},
		DeepLink:      ref.DeepLink(),
	}
}

// RevisionOf 把实体更新时间格式化为信封 revision（UTC RFC3339）
func RevisionOf(updatedAt time.Time) string {
	return updatedAt.UTC().Format(time.RFC3339)
}
