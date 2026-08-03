package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

type PDFAnnotationRepository struct {
	db *sql.DB
}

func NewPDFAnnotationRepository(db *sql.DB) *PDFAnnotationRepository {
	return &PDFAnnotationRepository{db: db}
}

func (r *PDFAnnotationRepository) Create(paperID int64, input PDFAnnotationCreateInput) (*model.PDFAnnotation, error) {
	pageStart, pageEnd := pdfAnnotationPageRange(input.Fragments)
	fragmentsJSON, err := json.Marshal(input.Fragments)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "编码 PDF 标注位置失败", err)
	}

	result, err := r.db.Exec(`
		INSERT INTO pdf_annotations (
			paper_id, type, page_start, page_end, quote_text, color, fragments_json, note_text, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`,
		paperID,
		input.Type,
		pageStart,
		pageEnd,
		input.QuoteText,
		input.Color,
		string(fragmentsJSON),
		input.NoteText,
	)
	if err != nil {
		return nil, wrapDBError(err, "创建 PDF 标注失败")
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, wrapDBError(err, "读取 PDF 标注 ID 失败")
	}

	return r.GetByID(paperID, id)
}

func (r *PDFAnnotationRepository) ListByPaperID(paperID int64) ([]model.PDFAnnotation, error) {
	rows, err := r.db.Query(`
		SELECT id, paper_id, type, page_start, page_end, quote_text, color, fragments_json, note_text, created_at, updated_at
		FROM pdf_annotations
		WHERE paper_id = ?
		ORDER BY page_start ASC, id ASC
	`, paperID)
	if err != nil {
		return nil, wrapDBError(err, "查询 PDF 标注失败")
	}
	defer rows.Close()

	annotations := []model.PDFAnnotation{}
	for rows.Next() {
		annotation, err := scanPDFAnnotation(rows)
		if err != nil {
			return nil, err
		}
		annotations = append(annotations, *annotation)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapDBError(err, "读取 PDF 标注失败")
	}
	return annotations, nil
}

func (r *PDFAnnotationRepository) ListGlobal(filter PDFAnnotationListFilter) ([]model.PDFAnnotationListItem, int, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	whereSQL, args := pdfAnnotationGlobalWhere(filter.Query)
	countSQL := `
		SELECT COUNT(*)
		FROM pdf_annotations a
		JOIN papers p ON p.id = a.paper_id
	` + whereSQL
	var total int
	if err := r.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, wrapDBError(err, "统计 PDF 标注失败")
	}

	orderSQL := pdfAnnotationGlobalOrder(filter.Sort)
	queryArgs := append([]interface{}{}, args...)
	queryArgs = append(queryArgs, pageSize, offset)
	rows, err := r.db.Query(fmt.Sprintf(`
		SELECT
			a.id, a.paper_id, p.title, p.original_filename, p.stored_pdf_name,
			a.type, a.page_start, a.page_end, a.quote_text, a.color,
			a.fragments_json, a.note_text, a.created_at, a.updated_at
		FROM pdf_annotations a
		JOIN papers p ON p.id = a.paper_id
		%s
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, whereSQL, orderSQL), queryArgs...)
	if err != nil {
		return nil, 0, wrapDBError(err, "查询 PDF 标注库失败")
	}
	defer rows.Close()

	items := []model.PDFAnnotationListItem{}
	for rows.Next() {
		item, err := scanPDFAnnotationListItem(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, wrapDBError(err, "读取 PDF 标注库失败")
	}
	return items, total, nil
}

func pdfAnnotationGlobalWhere(query string) (string, []interface{}) {
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return "", nil
	}
	like := "%" + normalized + "%"
	return `
		WHERE lower(a.quote_text) LIKE ?
			OR lower(p.title) LIKE ?
			OR lower(p.original_filename) LIKE ?
			OR lower(COALESCE(p.doi, '')) LIKE ?
	`, []interface{}{like, like, like, like}
}

func pdfAnnotationGlobalOrder(sort string) string {
	switch strings.TrimSpace(sort) {
	case "updated_asc":
		return "a.updated_at ASC, a.id ASC"
	case "created_desc":
		return "a.created_at DESC, a.id DESC"
	case "created_asc":
		return "a.created_at ASC, a.id ASC"
	default:
		return "a.updated_at DESC, a.id DESC"
	}
}

func (r *PDFAnnotationRepository) GetByID(paperID, annotationID int64) (*model.PDFAnnotation, error) {
	row := r.db.QueryRow(`
		SELECT id, paper_id, type, page_start, page_end, quote_text, color, fragments_json, note_text, created_at, updated_at
		FROM pdf_annotations
		WHERE paper_id = ? AND id = ?
	`, paperID, annotationID)

	annotation, err := scanPDFAnnotation(row)
	if err == sql.ErrNoRows {
		return nil, apperr.New(apperr.CodeNotFound, "pdf annotation not found")
	}
	if err != nil {
		return nil, err
	}
	return annotation, nil
}

// GetByAnnotationID 按标注 ID 查询单条 PDF 标注（不限定文献）
func (r *PDFAnnotationRepository) GetByAnnotationID(annotationID int64) (*model.PDFAnnotation, error) {
	row := r.db.QueryRow(`
		SELECT id, paper_id, type, page_start, page_end, quote_text, color, fragments_json, note_text, created_at, updated_at
		FROM pdf_annotations
		WHERE id = ?
	`, annotationID)

	annotation, err := scanPDFAnnotation(row)
	if err == sql.ErrNoRows {
		return nil, apperr.New(apperr.CodeNotFound, "pdf annotation not found")
	}
	if err != nil {
		return nil, err
	}
	return annotation, nil
}

// ListChangedSince 按更新时间增量查询 PDF 标注（keyset 分页，按 updated_at、id 升序）
func (r *PDFAnnotationRepository) ListChangedSince(since time.Time, afterID int64, limit int) ([]model.PDFAnnotation, error) {
	if limit <= 0 {
		limit = 100
	}
	sinceValue := changedSinceValue(since)

	rows, err := r.db.Query(`
		SELECT id, paper_id, type, page_start, page_end, quote_text, color, fragments_json, note_text, created_at, updated_at
		FROM pdf_annotations
		WHERE updated_at > ? OR (updated_at = ? AND id > ?)
		ORDER BY updated_at ASC, id ASC
		LIMIT ?
	`, sinceValue, sinceValue, afterID, limit)
	if err != nil {
		return nil, wrapDBError(err, "增量查询 PDF 标注失败")
	}
	defer rows.Close()

	annotations := []model.PDFAnnotation{}
	for rows.Next() {
		annotation, err := scanPDFAnnotation(rows)
		if err != nil {
			return nil, err
		}
		annotations = append(annotations, *annotation)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapDBError(err, "读取 PDF 标注失败")
	}
	return annotations, nil
}

func (r *PDFAnnotationRepository) Delete(paperID, annotationID int64) error {
	result, err := r.db.Exec("DELETE FROM pdf_annotations WHERE paper_id = ? AND id = ?", paperID, annotationID)
	if err != nil {
		return wrapDBError(err, "删除 PDF 标注失败")
	}
	return ensureRowsAffected(result, "pdf annotation not found")
}

func scanPDFAnnotation(scanner interface {
	Scan(dest ...interface{}) error
}) (*model.PDFAnnotation, error) {
	var annotation model.PDFAnnotation
	var fragmentsJSON string
	if err := scanner.Scan(
		&annotation.ID,
		&annotation.PaperID,
		&annotation.Type,
		&annotation.PageStart,
		&annotation.PageEnd,
		&annotation.QuoteText,
		&annotation.Color,
		&fragmentsJSON,
		&annotation.NoteText,
		&annotation.CreatedAt,
		&annotation.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(fragmentsJSON), &annotation.Fragments); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "解析 PDF 标注位置失败", err)
	}
	if annotation.Fragments == nil {
		annotation.Fragments = []model.PDFAnnotationFragment{}
	}
	return &annotation, nil
}

func scanPDFAnnotationListItem(scanner interface {
	Scan(dest ...interface{}) error
}) (*model.PDFAnnotationListItem, error) {
	var item model.PDFAnnotationListItem
	var fragmentsJSON string
	if err := scanner.Scan(
		&item.ID,
		&item.PaperID,
		&item.PaperTitle,
		&item.PaperOriginalFilename,
		&item.PaperStoredPDFName,
		&item.Type,
		&item.PageStart,
		&item.PageEnd,
		&item.QuoteText,
		&item.Color,
		&fragmentsJSON,
		&item.NoteText,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(fragmentsJSON), &item.Fragments); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "解析 PDF 标注位置失败", err)
	}
	if item.Fragments == nil {
		item.Fragments = []model.PDFAnnotationFragment{}
	}
	return &item, nil
}

func pdfAnnotationPageRange(fragments []model.PDFAnnotationFragment) (int, int) {
	if len(fragments) == 0 {
		return 1, 1
	}
	pageStart := fragments[0].Page
	pageEnd := fragments[0].Page
	for _, fragment := range fragments[1:] {
		if fragment.Page < pageStart {
			pageStart = fragment.Page
		}
		if fragment.Page > pageEnd {
			pageEnd = fragment.Page
		}
	}
	return pageStart, pageEnd
}
