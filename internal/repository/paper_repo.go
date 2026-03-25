package repository

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// PaperRepository 负责文献相关的数据操作
type PaperRepository struct {
	db    *sql.DB
	tag   *TagRepository
	group *GroupRepository
}

// NewPaperRepository 创建文献仓库
func NewPaperRepository(db *sql.DB, tagRepo *TagRepository, groupRepo *GroupRepository) *PaperRepository {
	return &PaperRepository{db: db, tag: tagRepo, group: groupRepo}
}

// CreatePaper 创建文献
func (r *PaperRepository) CreatePaper(input PaperUpsertInput) (*model.Paper, error) {
	input.ExtractionStatus = strings.TrimSpace(input.ExtractionStatus)
	if input.ExtractionStatus == "" {
		input.ExtractionStatus = "completed"
	}
	if !isValidPaperExtractionStatus(input.ExtractionStatus) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "文献解析状态无效")
	}
	for _, figure := range input.Figures {
		source := strings.TrimSpace(figure.Source)
		if source != "" && !isValidFigureSource(source) {
			return nil, apperr.New(apperr.CodeInvalidArgument, "图片来源无效")
		}
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, wrapDBError(err, "创建文献失败")
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		INSERT INTO papers (
			title, original_filename, stored_pdf_name, pdf_sha256, file_size, content_type,
			pdf_text, abstract_text, notes_text, paper_notes_text, boxes_json, extraction_status, extractor_message, extractor_job_id, group_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		input.Title,
		input.OriginalFilename,
		input.StoredPDFName,
		input.PDFSHA256,
		input.FileSize,
		input.ContentType,
		input.PDFText,
		input.AbstractText,
		input.NotesText,
		input.PaperNotesText,
		input.BoxesJSON,
		input.ExtractionStatus,
		input.ExtractorMessage,
		input.ExtractorJobID,
		input.GroupID,
	)
	if err != nil {
		return nil, wrapConflictDBError(err, "文献文件已存在", "创建文献失败")
	}

	paperID, err := result.LastInsertId()
	if err != nil {
		return nil, wrapDBError(err, "读取文献 ID 失败")
	}

	if err := r.tag.SyncPaperTags(tx, paperID, input.Tags); err != nil {
		return nil, wrapDBError(err, "保存文献标签失败")
	}

	for _, figure := range input.Figures {
		if _, err := tx.Exec(`
				INSERT INTO paper_figures (
					paper_id, filename, original_name, content_type, page_number, figure_index, parent_figure_id, subfigure_label, source, caption, bbox_json, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			`,
			paperID,
			figure.Filename,
			figure.OriginalName,
			figure.ContentType,
			figure.PageNumber,
			figure.FigureIndex,
			figure.ParentFigureID,
			strings.TrimSpace(figure.SubfigureLabel),
			firstNonEmpty(strings.TrimSpace(figure.Source), "auto"),
			figure.Caption,
			figure.BBoxJSON,
		); err != nil {
			return nil, wrapConflictDBError(err, "图片文件已存在", "保存文献图片失败")
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, wrapDBError(err, "提交文献事务失败")
	}

	return r.GetPaperDetail(paperID)
}

// UpdatePaper 更新文献
func (r *PaperRepository) UpdatePaper(id int64, input PaperUpdateInput) (*model.Paper, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, wrapDBError(err, "更新文献失败")
	}
	defer tx.Rollback()

	var pdfTextValue interface{}
	if input.PDFText != nil {
		pdfTextValue = *input.PDFText
	}

	result, err := tx.Exec(`
		UPDATE papers
		SET title = ?, pdf_text = COALESCE(?, pdf_text), abstract_text = ?, notes_text = ?, paper_notes_text = ?, group_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, input.Title, pdfTextValue, input.AbstractText, input.NotesText, input.PaperNotesText, input.GroupID, id)
	if err != nil {
		return nil, wrapDBError(err, "更新文献失败")
	}

	if err := ensureRowsAffected(result, "paper not found"); err != nil {
		return nil, err
	}

	if err := r.tag.SyncPaperTags(tx, id, input.Tags); err != nil {
		return nil, wrapDBError(err, "更新文献标签失败")
	}

	if err := tx.Commit(); err != nil {
		return nil, wrapDBError(err, "提交文献事务失败")
	}

	return r.GetPaperDetail(id)
}

func (r *PaperRepository) UpdatePaperPDFText(id int64, pdfText string) (*model.Paper, error) {
	result, err := r.db.Exec(`
		UPDATE papers
		SET pdf_text = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, pdfText, id)
	if err != nil {
		return nil, wrapDBError(err, "更新 PDF 全文失败")
	}

	if err := ensureRowsAffected(result, "paper not found"); err != nil {
		return nil, err
	}

	return r.GetPaperDetail(id)
}

// DeletePaper 删除文献
func (r *PaperRepository) DeletePaper(id int64) error {
	result, err := r.db.Exec("DELETE FROM papers WHERE id = ?", id)
	if err != nil {
		return wrapDBError(err, "删除文献失败")
	}
	return ensureRowsAffected(result, "paper not found")
}

// PurgeLibrary 清空文献库
func (r *PaperRepository) PurgeLibrary() error {
	tx, err := r.db.Begin()
	if err != nil {
		return wrapDBError(err, "清空数据库失败")
	}
	defer tx.Rollback()

	for _, stmt := range []string{
		"DELETE FROM papers",
		"DELETE FROM groups",
		"DELETE FROM tags",
		"DELETE FROM sqlite_sequence WHERE name IN ('papers', 'paper_figures', 'groups', 'tags')",
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return wrapDBError(err, "清空数据库失败")
		}
	}

	return wrapDBError(tx.Commit(), "提交清空数据库事务失败")
}

// GetPaperDetail 获取文献详情
func (r *PaperRepository) GetPaperDetail(id int64) (*model.Paper, error) {
	row := r.db.QueryRow(`
			SELECT
				p.id, p.title, p.original_filename, p.stored_pdf_name, p.file_size, p.content_type,
				p.pdf_text, p.abstract_text, p.notes_text, p.paper_notes_text, p.boxes_json, p.extraction_status, p.extractor_message, p.extractor_job_id,
				p.group_id, COALESCE(g.name, ''),
				p.created_at, p.updated_at,
				(SELECT COUNT(*) FROM paper_figures pf WHERE pf.paper_id = p.id AND pf.parent_figure_id IS NULL)
		FROM papers p
		LEFT JOIN groups g ON g.id = p.group_id
		WHERE p.id = ?
	`, id)

	paper, err := scanPaper(row, true)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, wrapDBError(err, "查询文献失败")
	}

	tagsByPaper, err := r.tag.LoadTagsByPaperIDs([]int64{id})
	if err != nil {
		return nil, wrapDBError(err, "查询文献标签失败")
	}
	paper.Tags = tagsByPaper[id]
	if paper.Tags == nil {
		paper.Tags = []model.Tag{}
	}

	rows, err := r.db.Query(`
		SELECT
			pf.id, pf.filename, pf.original_name, pf.content_type, pf.page_number, pf.figure_index,
			pf.parent_figure_id, pf.subfigure_label, pf.source, pf.caption, pf.notes_text, pf.bbox_json,
			cp.id, COALESCE(cp.name, ''), COALESCE(cp.colors_json, ''),
			CASE WHEN cp.id IS NULL THEN 0 ELSE 1 END AS palette_count,
			pf.created_at, pf.updated_at
		FROM paper_figures pf
		LEFT JOIN color_palettes cp ON cp.figure_id = pf.id
		WHERE pf.paper_id = ?
		ORDER BY pf.page_number ASC, pf.figure_index ASC, CASE WHEN pf.parent_figure_id IS NULL THEN 0 ELSE 1 END ASC, pf.subfigure_label ASC, pf.id ASC
	`, id)
	if err != nil {
		return nil, wrapDBError(err, "查询文献图片失败")
	}
	defer rows.Close()

	paper.Figures = []model.Figure{}
	figureIDs := []int64{}
	for rows.Next() {
		var figure model.Figure
		var bboxJSON string
		var parentFigureID sql.NullInt64
		var paletteID sql.NullInt64
		var paletteName string
		var paletteColorsJSON string
		if err := rows.Scan(
			&figure.ID,
			&figure.Filename,
			&figure.OriginalName,
			&figure.ContentType,
			&figure.PageNumber,
			&figure.FigureIndex,
			&parentFigureID,
			&figure.SubfigureLabel,
			&figure.Source,
			&figure.Caption,
			&figure.NotesText,
			&bboxJSON,
			&paletteID,
			&paletteName,
			&paletteColorsJSON,
			&figure.PaletteCount,
			&figure.CreatedAt,
			&figure.UpdatedAt,
		); err != nil {
			return nil, wrapDBError(err, "查询文献图片失败")
		}
		if parentFigureID.Valid {
			figure.ParentFigureID = &parentFigureID.Int64
		}
		figure.PaletteID, figure.PaletteName, figure.PaletteColors, err = parsePaletteSummary(paletteID, paletteName, paletteColorsJSON)
		if err != nil {
			return nil, wrapDBError(err, "解析图片配色失败")
		}
		figure.BBox = rawJSON(bboxJSON)
		figure.Tags = []model.Tag{}
		paper.Figures = append(paper.Figures, figure)
		figureIDs = append(figureIDs, figure.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, wrapDBError(err, "查询文献图片失败")
	}

	tagsByFigure, err := r.tag.LoadTagsByFigureIDs(figureIDs)
	if err != nil {
		return nil, wrapDBError(err, "查询图片标签失败")
	}
	for i := range paper.Figures {
		if tags := tagsByFigure[paper.Figures[i].ID]; tags != nil {
			paper.Figures[i].Tags = tags
		}
	}

	return paper, nil
}

// ListPapers 查询文献列表
func (r *PaperRepository) ListPapers(filter model.PaperFilter) ([]model.Paper, int, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 12
	}

	whereClause, args := buildPaperWhere(filter)

	var total int
	countQuery := "SELECT COUNT(*) FROM papers p" + whereClause
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, wrapDBError(err, "查询文献总数失败")
	}

	query := `
			SELECT
				p.id, p.title, p.original_filename, p.stored_pdf_name, p.file_size, p.content_type,
				'', p.abstract_text, p.notes_text, p.paper_notes_text, '', p.extraction_status, p.extractor_message, p.extractor_job_id,
				p.group_id, COALESCE(g.name, ''),
				p.created_at, p.updated_at,
				(SELECT COUNT(*) FROM paper_figures pf WHERE pf.paper_id = p.id AND pf.parent_figure_id IS NULL)
		FROM papers p
		LEFT JOIN groups g ON g.id = p.group_id
	` + whereClause + `
		` + buildPaperOrderBy(filter) + `
		LIMIT ? OFFSET ?
	`

	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(append([]interface{}{}, args...), filter.PageSize, offset)
	rows, err := r.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, wrapDBError(err, "查询文献列表失败")
	}
	defer rows.Close()

	papers := []model.Paper{}
	paperIDs := []int64{}
	for rows.Next() {
		paper, err := scanPaper(rows, false)
		if err != nil {
			return nil, 0, wrapDBError(err, "查询文献列表失败")
		}
		paper.Tags = []model.Tag{}
		paper.Figures = nil
		papers = append(papers, *paper)
		paperIDs = append(paperIDs, paper.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, wrapDBError(err, "查询文献列表失败")
	}

	tagsByPaper, err := r.tag.LoadTagsByPaperIDs(paperIDs)
	if err != nil {
		return nil, 0, wrapDBError(err, "查询文献标签失败")
	}
	for i := range papers {
		if tags := tagsByPaper[papers[i].ID]; tags != nil {
			papers[i].Tags = tags
		}
	}

	return papers, total, nil
}

// ListPapersByExtractionStatuses 根据解析状态查询文献
func (r *PaperRepository) ListPapersByExtractionStatuses(statuses []string) ([]model.Paper, error) {
	if len(statuses) == 0 {
		return []model.Paper{}, nil
	}

	placeholders := make([]string, len(statuses))
	args := make([]interface{}, len(statuses))
	for i, status := range statuses {
		placeholders[i] = "?"
		args[i] = status
	}

	rows, err := r.db.Query(`
		SELECT
			p.id, p.title, p.original_filename, p.stored_pdf_name, p.file_size, p.content_type,
			'', p.abstract_text, p.notes_text, p.paper_notes_text, '', p.extraction_status, p.extractor_message, p.extractor_job_id,
			p.group_id, COALESCE(g.name, ''),
			p.created_at, p.updated_at,
			(SELECT COUNT(*) FROM paper_figures pf WHERE pf.paper_id = p.id AND pf.parent_figure_id IS NULL)
		FROM papers p
		LEFT JOIN groups g ON g.id = p.group_id
		WHERE p.extraction_status IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY p.updated_at DESC, p.id DESC
	`, args...)
	if err != nil {
		return nil, wrapDBError(err, "查询待恢复文献失败")
	}
	defer rows.Close()

	papers := []model.Paper{}
	for rows.Next() {
		paper, err := scanPaper(rows, false)
		if err != nil {
			return nil, wrapDBError(err, "查询待恢复文献失败")
		}
		papers = append(papers, *paper)
	}

	if err := rows.Err(); err != nil {
		return nil, wrapDBError(err, "查询待恢复文献失败")
	}

	return papers, nil
}

// FindPaperByPDFSHA256 根据 PDF SHA256 查找文献
func (r *PaperRepository) FindPaperByPDFSHA256(pdfSHA256 string) (*model.Paper, error) {
	pdfSHA256 = strings.TrimSpace(pdfSHA256)
	if pdfSHA256 == "" {
		return nil, nil
	}

	var paperID int64
	err := r.db.QueryRow(`
		SELECT id
		FROM papers
		WHERE pdf_sha256 = ?
		LIMIT 1
	`, pdfSHA256).Scan(&paperID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, wrapDBError(err, "查询重复文献失败")
	}

	return r.GetPaperDetail(paperID)
}

// ListPapersMissingPDFSHA256 查询缺少 PDF SHA256 的文献
func (r *PaperRepository) ListPapersMissingPDFSHA256() ([]PaperChecksumBackfillItem, error) {
	rows, err := r.db.Query(`
		SELECT id, stored_pdf_name
		FROM papers
		WHERE COALESCE(TRIM(pdf_sha256), '') = ''
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, wrapDBError(err, "查询待补全文献指纹失败")
	}
	defer rows.Close()

	items := []PaperChecksumBackfillItem{}
	for rows.Next() {
		var item PaperChecksumBackfillItem
		if err := rows.Scan(&item.ID, &item.StoredPDFName); err != nil {
			return nil, wrapDBError(err, "查询待补全文献指纹失败")
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapDBError(err, "查询待补全文献指纹失败")
	}
	return items, nil
}

// UpdatePaperPDFSHA256 更新文献 PDF SHA256
func (r *PaperRepository) UpdatePaperPDFSHA256(id int64, pdfSHA256 string) error {
	result, err := r.db.Exec(`
		UPDATE papers
		SET pdf_sha256 = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, strings.TrimSpace(pdfSHA256), id)
	if err != nil {
		return wrapConflictDBError(err, "文献 PDF 指纹已存在", "更新文献 PDF 指纹失败")
	}
	return ensureRowsAffected(result, "paper not found")
}

// UpdatePaperExtractionState 更新文献解析状态
func (r *PaperRepository) UpdatePaperExtractionState(id int64, status, message, jobID string) error {
	result, err := r.db.Exec(`
		UPDATE papers
		SET extraction_status = ?, extractor_message = ?, extractor_job_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, message, jobID, id)
	if err != nil {
		return wrapDBError(err, "更新文献解析状态失败")
	}

	return ensureRowsAffected(result, "paper not found")
}

// ApplyPaperExtractionResult 应用文献解析结果
func (r *PaperRepository) ApplyPaperExtractionResult(
	id int64,
	pdfText string,
	boxesJSON string,
	status string,
	message string,
	jobID string,
	figures []FigureUpsertInput,
) error {
	tx, err := r.db.Begin()
	if err != nil {
		return wrapDBError(err, "写入文献解析结果失败")
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE papers
		SET pdf_text = ?, boxes_json = ?, extraction_status = ?, extractor_message = ?, extractor_job_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, pdfText, boxesJSON, status, message, jobID, id)
	if err != nil {
		return wrapDBError(err, "写入文献解析结果失败")
	}

	if err := ensureRowsAffected(result, "paper not found"); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		DELETE FROM paper_figures
		WHERE paper_id = ?
		  AND COALESCE(source, 'auto') != 'manual'
		  AND parent_figure_id IS NULL
		  AND NOT EXISTS (
		  	SELECT 1
		  	FROM paper_figures child
		  	WHERE child.parent_figure_id = paper_figures.id
		  )
	`, id); err != nil {
		return wrapDBError(err, "更新文献图片失败")
	}

	for _, figure := range figures {
		if _, err := tx.Exec(`
			INSERT INTO paper_figures (
				paper_id, filename, original_name, content_type, page_number, figure_index, parent_figure_id, subfigure_label, source, caption, bbox_json, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`,
			id,
			figure.Filename,
			figure.OriginalName,
			figure.ContentType,
			figure.PageNumber,
			figure.FigureIndex,
			figure.ParentFigureID,
			strings.TrimSpace(figure.SubfigureLabel),
			firstNonEmpty(strings.TrimSpace(figure.Source), "auto"),
			figure.Caption,
			figure.BBoxJSON,
		); err != nil {
			return wrapConflictDBError(err, "图片文件已存在", "更新文献图片失败")
		}
	}

	return wrapDBError(tx.Commit(), "提交文献解析结果失败")
}

// buildPaperWhere 构建文献查询条件
func buildPaperWhere(filter model.PaperFilter) (string, []interface{}) {
	conditions := []string{}
	args := []interface{}{}

	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		ftsKeyword := ftsEscapeKeyword(keyword)
		if strings.TrimSpace(filter.KeywordScope) == "title_abstract" {
			conditions = append(conditions, "p.id IN (SELECT rowid FROM papers_fts WHERE papers_fts MATCH '{title abstract_text}: ' || ?)")
			args = append(args, ftsKeyword)
		} else {
			conditions = append(conditions, "p.id IN (SELECT rowid FROM papers_fts WHERE papers_fts MATCH ?)")
			args = append(args, ftsKeyword)
		}
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		conditions = append(conditions, "p.group_id = ?")
		args = append(args, *filter.GroupID)
	}
	if filter.TagID != nil && *filter.TagID > 0 {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM paper_tags pt WHERE pt.paper_id = p.id AND pt.tag_id = ?)")
		args = append(args, *filter.TagID)
	}
	if filter.Status != "" {
		conditions = append(conditions, "p.extraction_status = ?")
		args = append(args, filter.Status)
	}
	if filter.HasPaperNotes {
		conditions = append(conditions, "TRIM(COALESCE(p.paper_notes_text, '')) <> ''")
	}

	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func buildPaperOrderBy(filter model.PaperFilter) string {
	switch strings.TrimSpace(filter.SortBy) {
	case "updated_at":
		return "ORDER BY p.updated_at DESC, p.id DESC"
	case "created_at":
		fallthrough
	default:
		return "ORDER BY p.created_at DESC, p.id DESC"
	}
}

func isValidPaperExtractionStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "queued", "running", "manual_pending", "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func isValidFigureSource(source string) bool {
	switch strings.TrimSpace(source) {
	case "auto", "manual":
		return true
	default:
		return false
	}
}

// scanPaper 扫描文献数据
func scanPaper(scanner interface {
	Scan(dest ...interface{}) error
}, withFullText bool) (*model.Paper, error) {
	var paper model.Paper
	var groupID sql.NullInt64
	var groupName string
	var pdfText string
	var boxesJSON string
	var figureCount int

	args := []interface{}{
		&paper.ID,
		&paper.Title,
		&paper.OriginalFilename,
		&paper.StoredPDFName,
		&paper.FileSize,
		&paper.ContentType,
	}
	if withFullText {
		args = append(args, &pdfText)
	} else {
		var dummy string
		args = append(args, &dummy)
	}
	args = append(args,
		&paper.AbstractText,
		&paper.NotesText,
		&paper.PaperNotesText,
	)
	if withFullText {
		args = append(args, &boxesJSON)
	} else {
		var dummy string
		args = append(args, &dummy)
	}
	args = append(args,
		&paper.ExtractionStatus,
		&paper.ExtractorMessage,
		&paper.ExtractorJobID,
		&groupID,
		&groupName,
		&paper.CreatedAt,
		&paper.UpdatedAt,
		&figureCount,
	)

	if err := scanner.Scan(args...); err != nil {
		return nil, err
	}

	if groupID.Valid {
		paper.GroupID = &groupID.Int64
		paper.GroupName = groupName
	}
	paper.FigureCount = figureCount
	if withFullText {
		paper.PDFText = pdfText
		paper.Boxes = rawJSON(boxesJSON)
	}

	return &paper, nil
}

// rawJSON 返回原始 JSON 字节
func rawJSON(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
}
