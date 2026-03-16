package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"

	sqliteDriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type LibraryRepository struct {
	db *sql.DB
}

type TagUpsertInput struct {
	Name  string
	Color string
}

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

type PaperUpsertInput struct {
	Title            string
	OriginalFilename string
	StoredPDFName    string
	FileSize         int64
	ContentType      string
	PDFText          string
	AbstractText     string
	NotesText        string
	BoxesJSON        string
	ExtractionStatus string
	ExtractorMessage string
	ExtractorJobID   string
	GroupID          *int64
	Tags             []TagUpsertInput
	Figures          []FigureUpsertInput
}

type PaperUpdateInput struct {
	Title        string
	AbstractText string
	NotesText    string
	GroupID      *int64
	Tags         []TagUpsertInput
}

func NewLibraryRepository(dbPath string) (*LibraryRepository, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "创建数据库目录失败", err)
	}

	dsn := dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "打开数据库失败", err)
	}

	db.SetMaxOpenConns(1)

	repo := &LibraryRepository{db: db}
	if err := repo.initSchema(); err != nil {
		db.Close()
		return nil, apperr.Wrap(apperr.CodeInternal, "初始化数据库结构失败", err)
	}

	return repo, nil
}

func (r *LibraryRepository) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL COLLATE NOCASE UNIQUE,
		description TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tags (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL COLLATE NOCASE UNIQUE,
		color TEXT DEFAULT '#A45C40',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS papers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		original_filename TEXT NOT NULL,
		stored_pdf_name TEXT NOT NULL,
		file_size INTEGER DEFAULT 0,
		content_type TEXT DEFAULT 'application/pdf',
		pdf_text TEXT DEFAULT '',
		abstract_text TEXT DEFAULT '',
		notes_text TEXT DEFAULT '',
		boxes_json TEXT DEFAULT '',
		extraction_status TEXT DEFAULT 'completed',
		extractor_message TEXT DEFAULT '',
		extractor_job_id TEXT DEFAULT '',
		group_id INTEGER REFERENCES groups(id) ON DELETE SET NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS paper_tags (
		paper_id INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
		tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
		PRIMARY KEY (paper_id, tag_id)
	);

	CREATE TABLE IF NOT EXISTS paper_figures (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		paper_id INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
		filename TEXT NOT NULL,
		original_name TEXT DEFAULT '',
		content_type TEXT DEFAULT '',
		page_number INTEGER DEFAULT 0,
		figure_index INTEGER DEFAULT 0,
		source TEXT DEFAULT 'auto',
		caption TEXT DEFAULT '',
		bbox_json TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_papers_group_id ON papers(group_id);
	CREATE INDEX IF NOT EXISTS idx_papers_created_at ON papers(created_at);
	CREATE INDEX IF NOT EXISTS idx_papers_status ON papers(extraction_status);
	CREATE INDEX IF NOT EXISTS idx_paper_figures_paper_id ON paper_figures(paper_id);
	CREATE INDEX IF NOT EXISTS idx_paper_tags_tag_id ON paper_tags(tag_id);
	`

	if _, err := r.db.Exec(schema); err != nil {
		return err
	}
	return r.ensureSchemaColumns()
}

func (r *LibraryRepository) ensureSchemaColumns() error {
	for _, column := range []struct {
		tableName  string
		name       string
		definition string
	}{
		{tableName: "papers", name: "extractor_job_id", definition: "TEXT DEFAULT ''"},
		{tableName: "papers", name: "abstract_text", definition: "TEXT DEFAULT ''"},
		{tableName: "papers", name: "notes_text", definition: "TEXT DEFAULT ''"},
		{tableName: "paper_figures", name: "source", definition: "TEXT DEFAULT 'auto'"},
	} {
		if err := r.ensureColumn(column.tableName, column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func (r *LibraryRepository) ensureColumn(tableName, columnName, definition string) error {
	rows, err := r.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			return err
		}
		if strings.EqualFold(name, columnName) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = r.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, definition))
	return err
}

func (r *LibraryRepository) Close() error {
	return r.db.Close()
}

func (r *LibraryRepository) GroupExists(id int64) (bool, error) {
	var count int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM groups WHERE id = ?", id).Scan(&count); err != nil {
		return false, wrapDBError(err, "查询分组失败")
	}
	return count > 0, nil
}

func (r *LibraryRepository) CreatePaper(input PaperUpsertInput) (*model.Paper, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, wrapDBError(err, "创建文献失败")
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		INSERT INTO papers (
			title, original_filename, stored_pdf_name, file_size, content_type,
			pdf_text, abstract_text, notes_text, boxes_json, extraction_status, extractor_message, extractor_job_id, group_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		input.Title,
		input.OriginalFilename,
		input.StoredPDFName,
		input.FileSize,
		input.ContentType,
		input.PDFText,
		input.AbstractText,
		input.NotesText,
		input.BoxesJSON,
		input.ExtractionStatus,
		input.ExtractorMessage,
		input.ExtractorJobID,
		input.GroupID,
	)
	if err != nil {
		return nil, wrapDBError(err, "创建文献失败")
	}

	paperID, err := result.LastInsertId()
	if err != nil {
		return nil, wrapDBError(err, "读取文献 ID 失败")
	}

	if err := r.syncPaperTags(tx, paperID, input.Tags); err != nil {
		return nil, wrapDBError(err, "保存文献标签失败")
	}

	for _, figure := range input.Figures {
		if _, err := tx.Exec(`
			INSERT INTO paper_figures (
				paper_id, filename, original_name, content_type, page_number, figure_index, source, caption, bbox_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			paperID,
			figure.Filename,
			figure.OriginalName,
			figure.ContentType,
			figure.PageNumber,
			figure.FigureIndex,
			firstNonEmpty(strings.TrimSpace(figure.Source), "auto"),
			figure.Caption,
			figure.BBoxJSON,
		); err != nil {
			return nil, wrapDBError(err, "保存文献图片失败")
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, wrapDBError(err, "提交文献事务失败")
	}

	return r.GetPaperDetail(paperID)
}

func (r *LibraryRepository) UpdatePaper(id int64, input PaperUpdateInput) (*model.Paper, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, wrapDBError(err, "更新文献失败")
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE papers
		SET title = ?, abstract_text = ?, notes_text = ?, group_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, input.Title, input.AbstractText, input.NotesText, input.GroupID, id)
	if err != nil {
		return nil, wrapDBError(err, "更新文献失败")
	}

	if err := ensureRowsAffected(result, "paper not found"); err != nil {
		return nil, err
	}

	if _, err := tx.Exec("DELETE FROM paper_tags WHERE paper_id = ?", id); err != nil {
		return nil, wrapDBError(err, "更新文献标签失败")
	}
	if err := r.syncPaperTags(tx, id, input.Tags); err != nil {
		return nil, wrapDBError(err, "更新文献标签失败")
	}

	if err := tx.Commit(); err != nil {
		return nil, wrapDBError(err, "提交文献事务失败")
	}

	return r.GetPaperDetail(id)
}

func (r *LibraryRepository) DeletePaper(id int64) error {
	result, err := r.db.Exec("DELETE FROM papers WHERE id = ?", id)
	if err != nil {
		return wrapDBError(err, "删除文献失败")
	}
	return ensureRowsAffected(result, "paper not found")
}

func (r *LibraryRepository) PurgeLibrary() error {
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

func (r *LibraryRepository) GetAppSetting(key string) (string, error) {
	var value string
	if err := r.db.QueryRow("SELECT value FROM app_settings WHERE key = ?", key).Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", wrapDBError(err, "读取应用设置失败")
	}
	return value, nil
}

func (r *LibraryRepository) UpsertAppSetting(key, value string) error {
	_, err := r.db.Exec(`
		INSERT INTO app_settings (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return wrapDBError(err, "保存应用设置失败")
}

func (r *LibraryRepository) UpdatePaperExtractionState(id int64, status, message, jobID string) error {
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

func (r *LibraryRepository) ApplyPaperExtractionResult(
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

	if _, err := tx.Exec("DELETE FROM paper_figures WHERE paper_id = ? AND COALESCE(source, 'auto') != 'manual'", id); err != nil {
		return wrapDBError(err, "更新文献图片失败")
	}

	for _, figure := range figures {
		if _, err := tx.Exec(`
			INSERT INTO paper_figures (
				paper_id, filename, original_name, content_type, page_number, figure_index, source, caption, bbox_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			id,
			figure.Filename,
			figure.OriginalName,
			figure.ContentType,
			figure.PageNumber,
			figure.FigureIndex,
			firstNonEmpty(strings.TrimSpace(figure.Source), "auto"),
			figure.Caption,
			figure.BBoxJSON,
		); err != nil {
			return wrapDBError(err, "更新文献图片失败")
		}
	}

	return wrapDBError(tx.Commit(), "提交文献解析结果失败")
}

func (r *LibraryRepository) AddPaperFigures(id int64, figures []FigureUpsertInput) error {
	return r.ApplyManualFigureChanges(id, figures, nil)
}

func (r *LibraryRepository) ApplyManualFigureChanges(id int64, addFigures []FigureUpsertInput, deleteFigureIDs []int64) error {
	if len(addFigures) == 0 && len(deleteFigureIDs) == 0 {
		return nil
	}

	uniqueDeleteIDs := uniqueInt64s(deleteFigureIDs)
	tx, err := r.db.Begin()
	if err != nil {
		return wrapDBError(err, "更新人工图片失败")
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE papers
		SET updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)
	if err != nil {
		return wrapDBError(err, "更新人工图片失败")
	}
	if err := ensureRowsAffected(result, "paper not found"); err != nil {
		return err
	}

	if len(uniqueDeleteIDs) > 0 {
		placeholders := make([]string, len(uniqueDeleteIDs))
		args := make([]interface{}, 0, len(uniqueDeleteIDs)+1)
		args = append(args, id)
		for i, figureID := range uniqueDeleteIDs {
			placeholders[i] = "?"
			args = append(args, figureID)
		}

		var count int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM paper_figures WHERE paper_id = ? AND id IN (`+strings.Join(placeholders, ",")+`)`,
			args...,
		).Scan(&count); err != nil {
			return wrapDBError(err, "校验待删除图片失败")
		}
		if count != len(uniqueDeleteIDs) {
			return apperr.New(apperr.CodeNotFound, "待替换或删除的图片不存在")
		}

		if _, err := tx.Exec(
			`DELETE FROM paper_figures WHERE paper_id = ? AND id IN (`+strings.Join(placeholders, ",")+`)`,
			args...,
		); err != nil {
			return wrapDBError(err, "删除旧图片失败")
		}
	}

	for _, figure := range addFigures {
		if _, err := tx.Exec(`
			INSERT INTO paper_figures (
				paper_id, filename, original_name, content_type, page_number, figure_index, source, caption, bbox_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			id,
			figure.Filename,
			figure.OriginalName,
			figure.ContentType,
			figure.PageNumber,
			figure.FigureIndex,
			firstNonEmpty(strings.TrimSpace(figure.Source), "manual"),
			figure.Caption,
			figure.BBoxJSON,
		); err != nil {
			return wrapDBError(err, "保存人工图片失败")
		}
	}

	return wrapDBError(tx.Commit(), "提交人工图片事务失败")
}

func (r *LibraryRepository) GetFigure(id int64) (*model.FigureListItem, error) {
	row := r.db.QueryRow(`
		SELECT
			pf.id, pf.paper_id, p.title, p.group_id, COALESCE(g.name, ''),
			pf.filename, pf.page_number, pf.figure_index, pf.source, pf.caption, pf.created_at
		FROM paper_figures pf
		JOIN papers p ON p.id = pf.paper_id
		LEFT JOIN groups g ON g.id = p.group_id
		WHERE pf.id = ?
	`, id)

	var item model.FigureListItem
	var groupID sql.NullInt64
	var groupName string
	if err := row.Scan(
		&item.ID,
		&item.PaperID,
		&item.PaperTitle,
		&groupID,
		&groupName,
		&item.Filename,
		&item.PageNumber,
		&item.FigureIndex,
		&item.Source,
		&item.Caption,
		&item.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, wrapDBError(err, "查询图片失败")
	}

	if groupID.Valid {
		item.GroupID = &groupID.Int64
		item.GroupName = groupName
	}
	item.Tags = []model.Tag{}
	return &item, nil
}

func uniqueInt64s(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (r *LibraryRepository) DeletePaperFigure(id int64) error {
	result, err := r.db.Exec(`DELETE FROM paper_figures WHERE id = ?`, id)
	if err != nil {
		return wrapDBError(err, "删除图片失败")
	}
	return ensureRowsAffected(result, "figure not found")
}

func (r *LibraryRepository) GetPaperDetail(id int64) (*model.Paper, error) {
	row := r.db.QueryRow(`
			SELECT
				p.id, p.title, p.original_filename, p.stored_pdf_name, p.file_size, p.content_type,
				p.pdf_text, p.abstract_text, p.notes_text, p.boxes_json, p.extraction_status, p.extractor_message, p.extractor_job_id,
				p.group_id, COALESCE(g.name, ''),
				p.created_at, p.updated_at,
				(SELECT COUNT(*) FROM paper_figures pf WHERE pf.paper_id = p.id)
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

	tagsByPaper, err := r.loadTagsByPaperIDs([]int64{id})
	if err != nil {
		return nil, wrapDBError(err, "查询文献标签失败")
	}
	paper.Tags = tagsByPaper[id]
	if paper.Tags == nil {
		paper.Tags = []model.Tag{}
	}

	rows, err := r.db.Query(`
		SELECT id, filename, original_name, content_type, page_number, figure_index, source, caption, bbox_json, created_at
		FROM paper_figures
		WHERE paper_id = ?
		ORDER BY page_number ASC, figure_index ASC, id ASC
	`, id)
	if err != nil {
		return nil, wrapDBError(err, "查询文献图片失败")
	}
	defer rows.Close()

	paper.Figures = []model.Figure{}
	for rows.Next() {
		var figure model.Figure
		var bboxJSON string
		if err := rows.Scan(
			&figure.ID,
			&figure.Filename,
			&figure.OriginalName,
			&figure.ContentType,
			&figure.PageNumber,
			&figure.FigureIndex,
			&figure.Source,
			&figure.Caption,
			&bboxJSON,
			&figure.CreatedAt,
		); err != nil {
			return nil, wrapDBError(err, "查询文献图片失败")
		}
		figure.BBox = rawJSON(bboxJSON)
		paper.Figures = append(paper.Figures, figure)
	}

	if err := rows.Err(); err != nil {
		return nil, wrapDBError(err, "查询文献图片失败")
	}

	return paper, nil
}

func (r *LibraryRepository) ListPapers(filter model.PaperFilter) ([]model.Paper, int, error) {
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
				'', p.abstract_text, p.notes_text, '', p.extraction_status, p.extractor_message, p.extractor_job_id,
				p.group_id, COALESCE(g.name, ''),
				p.created_at, p.updated_at,
				(SELECT COUNT(*) FROM paper_figures pf WHERE pf.paper_id = p.id)
		FROM papers p
		LEFT JOIN groups g ON g.id = p.group_id
	` + whereClause + `
		ORDER BY p.created_at DESC, p.id DESC
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

	tagsByPaper, err := r.loadTagsByPaperIDs(paperIDs)
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

func (r *LibraryRepository) ListPapersByExtractionStatuses(statuses []string) ([]model.Paper, error) {
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
			'', p.abstract_text, p.notes_text, '', p.extraction_status, p.extractor_message, p.extractor_job_id,
			p.group_id, COALESCE(g.name, ''),
			p.created_at, p.updated_at,
			(SELECT COUNT(*) FROM paper_figures pf WHERE pf.paper_id = p.id)
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

func (r *LibraryRepository) ListFigures(filter model.FigureFilter) ([]model.FigureListItem, int, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 200 {
		filter.PageSize = 8
	}

	whereClause, args := buildFigureWhere(filter)

	var total int
	countQuery := `
		SELECT COUNT(*)
		FROM paper_figures pf
		JOIN papers p ON p.id = pf.paper_id
	` + whereClause
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, wrapDBError(err, "查询图片总数失败")
	}

	query := `
		SELECT
			pf.id, pf.paper_id, p.title, p.group_id, COALESCE(g.name, ''),
			pf.filename, pf.page_number, pf.figure_index, pf.source, pf.caption, pf.created_at
		FROM paper_figures pf
		JOIN papers p ON p.id = pf.paper_id
		LEFT JOIN groups g ON g.id = p.group_id
	` + whereClause + `
		ORDER BY p.created_at DESC, pf.page_number ASC, pf.figure_index ASC, pf.id ASC
		LIMIT ? OFFSET ?
	`

	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(append([]interface{}{}, args...), filter.PageSize, offset)
	rows, err := r.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, wrapDBError(err, "查询图片列表失败")
	}
	defer rows.Close()

	figures := []model.FigureListItem{}
	paperIDs := []int64{}
	seenPaperIDs := map[int64]bool{}
	for rows.Next() {
		var item model.FigureListItem
		var groupID sql.NullInt64
		var groupName string
		if err := rows.Scan(
			&item.ID,
			&item.PaperID,
			&item.PaperTitle,
			&groupID,
			&groupName,
			&item.Filename,
			&item.PageNumber,
			&item.FigureIndex,
			&item.Source,
			&item.Caption,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, wrapDBError(err, "查询图片列表失败")
		}
		if groupID.Valid {
			item.GroupID = &groupID.Int64
			item.GroupName = groupName
		}
		item.Tags = []model.Tag{}
		figures = append(figures, item)
		if !seenPaperIDs[item.PaperID] {
			seenPaperIDs[item.PaperID] = true
			paperIDs = append(paperIDs, item.PaperID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, wrapDBError(err, "查询图片列表失败")
	}

	tagsByPaper, err := r.loadTagsByPaperIDs(paperIDs)
	if err != nil {
		return nil, 0, wrapDBError(err, "查询文献标签失败")
	}
	for i := range figures {
		if tags := tagsByPaper[figures[i].PaperID]; tags != nil {
			figures[i].Tags = tags
		}
	}

	return figures, total, nil
}

func (r *LibraryRepository) ListGroups() ([]model.Group, error) {
	rows, err := r.db.Query(`
		SELECT
			g.id, g.name, g.description, g.created_at, g.updated_at,
			COUNT(p.id) AS paper_count
		FROM groups g
		LEFT JOIN papers p ON p.group_id = g.id
		GROUP BY g.id
		ORDER BY g.name COLLATE NOCASE ASC
	`)
	if err != nil {
		return nil, wrapDBError(err, "查询分组列表失败")
	}
	defer rows.Close()

	groups := []model.Group{}
	for rows.Next() {
		var group model.Group
		if err := rows.Scan(
			&group.ID,
			&group.Name,
			&group.Description,
			&group.CreatedAt,
			&group.UpdatedAt,
			&group.PaperCount,
		); err != nil {
			return nil, wrapDBError(err, "查询分组列表失败")
		}
		groups = append(groups, group)
	}

	if err := rows.Err(); err != nil {
		return nil, wrapDBError(err, "查询分组列表失败")
	}

	return groups, nil
}

func (r *LibraryRepository) CreateGroup(name, description string) (*model.Group, error) {
	result, err := r.db.Exec(`
		INSERT INTO groups (name, description)
		VALUES (?, ?)
	`, name, description)
	if err != nil {
		return nil, wrapConflictDBError(err, "分组名称已存在", "创建分组失败")
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, wrapDBError(err, "读取分组 ID 失败")
	}

	return r.getGroupByID(id)
}

func (r *LibraryRepository) UpdateGroup(id int64, name, description string) (*model.Group, error) {
	result, err := r.db.Exec(`
		UPDATE groups
		SET name = ?, description = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, name, description, id)
	if err != nil {
		return nil, wrapConflictDBError(err, "分组名称已存在", "更新分组失败")
	}

	if err := ensureRowsAffected(result, "group not found"); err != nil {
		return nil, err
	}

	return r.getGroupByID(id)
}

func (r *LibraryRepository) DeleteGroup(id int64) error {
	result, err := r.db.Exec("DELETE FROM groups WHERE id = ?", id)
	if err != nil {
		return wrapDBError(err, "删除分组失败")
	}
	return ensureRowsAffected(result, "group not found")
}

func (r *LibraryRepository) ListTags() ([]model.Tag, error) {
	rows, err := r.db.Query(`
		SELECT
			t.id, t.name, t.color, t.created_at, t.updated_at,
			COUNT(pt.paper_id) AS paper_count
		FROM tags t
		LEFT JOIN paper_tags pt ON pt.tag_id = t.id
		GROUP BY t.id
		ORDER BY t.name COLLATE NOCASE ASC
	`)
	if err != nil {
		return nil, wrapDBError(err, "查询标签列表失败")
	}
	defer rows.Close()

	tags := []model.Tag{}
	for rows.Next() {
		var tag model.Tag
		if err := rows.Scan(
			&tag.ID,
			&tag.Name,
			&tag.Color,
			&tag.CreatedAt,
			&tag.UpdatedAt,
			&tag.PaperCount,
		); err != nil {
			return nil, wrapDBError(err, "查询标签列表失败")
		}
		tags = append(tags, tag)
	}

	if err := rows.Err(); err != nil {
		return nil, wrapDBError(err, "查询标签列表失败")
	}

	return tags, nil
}

func (r *LibraryRepository) CreateTag(name, color string) (*model.Tag, error) {
	result, err := r.db.Exec(`
		INSERT INTO tags (name, color)
		VALUES (?, ?)
	`, name, color)
	if err != nil {
		return nil, wrapConflictDBError(err, "标签名称已存在", "创建标签失败")
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, wrapDBError(err, "读取标签 ID 失败")
	}

	return r.getTagByID(id)
}

func (r *LibraryRepository) UpdateTag(id int64, name, color string) (*model.Tag, error) {
	result, err := r.db.Exec(`
		UPDATE tags
		SET name = ?, color = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, name, color, id)
	if err != nil {
		return nil, wrapConflictDBError(err, "标签名称已存在", "更新标签失败")
	}

	if err := ensureRowsAffected(result, "tag not found"); err != nil {
		return nil, err
	}

	return r.getTagByID(id)
}

func (r *LibraryRepository) DeleteTag(id int64) error {
	result, err := r.db.Exec("DELETE FROM tags WHERE id = ?", id)
	if err != nil {
		return wrapDBError(err, "删除标签失败")
	}
	return ensureRowsAffected(result, "tag not found")
}

func (r *LibraryRepository) syncPaperTags(tx *sql.Tx, paperID int64, tags []TagUpsertInput) error {
	if len(tags) == 0 {
		return nil
	}

	tagIDs := make([]int64, 0, len(tags))
	for _, tag := range tags {
		result, err := tx.Exec(`
			INSERT INTO tags (name, color)
			VALUES (?, ?)
			ON CONFLICT(name) DO UPDATE SET
				color = CASE
					WHEN excluded.color <> '' THEN excluded.color
					ELSE tags.color
				END,
				updated_at = CURRENT_TIMESTAMP
		`, tag.Name, tag.Color)
		if err != nil {
			return err
		}

		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		if id == 0 {
			if err := tx.QueryRow("SELECT id FROM tags WHERE name = ?", tag.Name).Scan(&id); err != nil {
				return err
			}
		}
		tagIDs = append(tagIDs, id)
	}

	sort.Slice(tagIDs, func(i, j int) bool {
		return tagIDs[i] < tagIDs[j]
	})

	for _, tagID := range tagIDs {
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO paper_tags (paper_id, tag_id)
			VALUES (?, ?)
		`, paperID, tagID); err != nil {
			return err
		}
	}

	return nil
}

func (r *LibraryRepository) getGroupByID(id int64) (*model.Group, error) {
	row := r.db.QueryRow(`
		SELECT
			g.id, g.name, g.description, g.created_at, g.updated_at,
			COUNT(p.id)
		FROM groups g
		LEFT JOIN papers p ON p.group_id = g.id
		WHERE g.id = ?
		GROUP BY g.id
	`, id)

	var group model.Group
	if err := row.Scan(
		&group.ID,
		&group.Name,
		&group.Description,
		&group.CreatedAt,
		&group.UpdatedAt,
		&group.PaperCount,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, notFoundError("group not found")
		}
		return nil, wrapDBError(err, "查询分组失败")
	}

	return &group, nil
}

func (r *LibraryRepository) getTagByID(id int64) (*model.Tag, error) {
	row := r.db.QueryRow(`
		SELECT
			t.id, t.name, t.color, t.created_at, t.updated_at,
			COUNT(pt.paper_id)
		FROM tags t
		LEFT JOIN paper_tags pt ON pt.tag_id = t.id
		WHERE t.id = ?
		GROUP BY t.id
	`, id)

	var tag model.Tag
	if err := row.Scan(
		&tag.ID,
		&tag.Name,
		&tag.Color,
		&tag.CreatedAt,
		&tag.UpdatedAt,
		&tag.PaperCount,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, notFoundError("tag not found")
		}
		return nil, wrapDBError(err, "查询标签失败")
	}

	return &tag, nil
}

func (r *LibraryRepository) loadTagsByPaperIDs(paperIDs []int64) (map[int64][]model.Tag, error) {
	result := make(map[int64][]model.Tag, len(paperIDs))
	if len(paperIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(paperIDs))
	args := make([]interface{}, len(paperIDs))
	for i, paperID := range paperIDs {
		placeholders[i] = "?"
		args[i] = paperID
	}

	query := `
		SELECT pt.paper_id, t.id, t.name, t.color, t.created_at, t.updated_at
		FROM paper_tags pt
		JOIN tags t ON t.id = pt.tag_id
		WHERE pt.paper_id IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY t.name COLLATE NOCASE ASC
	`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, wrapDBError(err, "查询文献标签失败")
	}
	defer rows.Close()

	for rows.Next() {
		var paperID int64
		var tag model.Tag
		if err := rows.Scan(
			&paperID,
			&tag.ID,
			&tag.Name,
			&tag.Color,
			&tag.CreatedAt,
			&tag.UpdatedAt,
		); err != nil {
			return nil, wrapDBError(err, "查询文献标签失败")
		}
		result[paperID] = append(result[paperID], tag)
	}

	if err := rows.Err(); err != nil {
		return nil, wrapDBError(err, "查询文献标签失败")
	}

	return result, nil
}

func buildPaperWhere(filter model.PaperFilter) (string, []interface{}) {
	conditions := []string{}
	args := []interface{}{}

	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		conditions = append(conditions, `(
			p.title LIKE ? OR
			p.original_filename LIKE ? OR
			p.pdf_text LIKE ? OR
			p.abstract_text LIKE ? OR
			p.notes_text LIKE ? OR
			EXISTS (
				SELECT 1
				FROM paper_tags pt
				JOIN tags t ON t.id = pt.tag_id
				WHERE pt.paper_id = p.id AND t.name LIKE ?
			) OR
			EXISTS (
				SELECT 1
				FROM groups g2
				WHERE g2.id = p.group_id AND g2.name LIKE ?
			)
		)`)
		args = append(args, like, like, like, like, like, like, like)
	}

	if filter.GroupID != nil {
		conditions = append(conditions, "p.group_id = ?")
		args = append(args, *filter.GroupID)
	}

	if filter.TagID != nil {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM paper_tags pt WHERE pt.paper_id = p.id AND pt.tag_id = ?)")
		args = append(args, *filter.TagID)
	}

	if status := strings.TrimSpace(filter.Status); status != "" {
		conditions = append(conditions, "p.extraction_status = ?")
		args = append(args, status)
	}

	if len(conditions) == 0 {
		return "", args
	}

	return " WHERE " + strings.Join(conditions, " AND "), args
}

func buildFigureWhere(filter model.FigureFilter) (string, []interface{}) {
	conditions := []string{}
	args := []interface{}{}

	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		conditions = append(conditions, `(
			p.title LIKE ? OR
			pf.caption LIKE ? OR
			pf.original_name LIKE ? OR
			EXISTS (
				SELECT 1
				FROM paper_tags pt
				JOIN tags t ON t.id = pt.tag_id
				WHERE pt.paper_id = p.id AND t.name LIKE ?
			)
		)`)
		args = append(args, like, like, like, like)
	}

	if filter.GroupID != nil {
		conditions = append(conditions, "p.group_id = ?")
		args = append(args, *filter.GroupID)
	}

	if filter.TagID != nil {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM paper_tags pt WHERE pt.paper_id = p.id AND pt.tag_id = ?)")
		args = append(args, *filter.TagID)
	}

	if len(conditions) == 0 {
		return "", args
	}

	return " WHERE " + strings.Join(conditions, " AND "), args
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanPaper(s scanner, includeHeavyFields bool) (*model.Paper, error) {
	var paper model.Paper
	var groupID sql.NullInt64
	var groupName string
	var boxesJSON string
	var pdfText string
	var abstractText string
	var notesText string

	if err := s.Scan(
		&paper.ID,
		&paper.Title,
		&paper.OriginalFilename,
		&paper.StoredPDFName,
		&paper.FileSize,
		&paper.ContentType,
		&pdfText,
		&abstractText,
		&notesText,
		&boxesJSON,
		&paper.ExtractionStatus,
		&paper.ExtractorMessage,
		&paper.ExtractorJobID,
		&groupID,
		&groupName,
		&paper.CreatedAt,
		&paper.UpdatedAt,
		&paper.FigureCount,
	); err != nil {
		return nil, err
	}

	if groupID.Valid {
		paper.GroupID = &groupID.Int64
		paper.GroupName = groupName
	}

	if includeHeavyFields {
		paper.PDFText = pdfText
		paper.Boxes = rawJSON(boxesJSON)
	}
	paper.AbstractText = abstractText
	paper.NotesText = notesText

	return &paper, nil
}

func rawJSON(value string) json.RawMessage {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return json.RawMessage(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func ensureRowsAffected(result sql.Result, notFoundMessage string) error {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return wrapDBError(err, "读取数据库影响行数失败")
	}
	if rowsAffected == 0 {
		return notFoundError(notFoundMessage)
	}
	return nil
}

func notFoundError(message string) error {
	return apperr.Wrap(apperr.CodeNotFound, message, sql.ErrNoRows)
}

func wrapConflictDBError(err error, conflictMessage, defaultMessage string) error {
	if err == nil {
		return nil
	}

	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return err
	}
	if sqliteCode(err) == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return apperr.Wrap(apperr.CodeConflict, conflictMessage, err)
	}
	return wrapDBError(err, defaultMessage)
}

func wrapDBError(err error, message string) error {
	if err == nil {
		return nil
	}

	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return err
	}
	return apperr.Wrap(apperr.CodeInternal, message, err)
}

func sqliteCode(err error) int {
	var sqliteErr *sqliteDriver.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code()
	}
	return 0
}
