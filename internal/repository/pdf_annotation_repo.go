package repository

import (
	"database/sql"
	"encoding/json"

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
