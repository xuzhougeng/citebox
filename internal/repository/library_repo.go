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
	Scope model.TagScope
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

type FigureUpdateInput struct {
	NotesText string
	Tags      []TagUpsertInput
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
		scope TEXT NOT NULL DEFAULT 'paper',
		name TEXT NOT NULL COLLATE NOCASE,
		color TEXT DEFAULT '#A45C40',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(scope, name)
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
		notes_text TEXT DEFAULT '',
		bbox_json TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS figure_tags (
		figure_id INTEGER NOT NULL REFERENCES paper_figures(id) ON DELETE CASCADE,
		tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
		PRIMARY KEY (figure_id, tag_id)
	);

	CREATE INDEX IF NOT EXISTS idx_papers_group_id ON papers(group_id);
	CREATE INDEX IF NOT EXISTS idx_papers_created_at ON papers(created_at);
	CREATE INDEX IF NOT EXISTS idx_papers_status ON papers(extraction_status);
	CREATE INDEX IF NOT EXISTS idx_paper_figures_paper_id ON paper_figures(paper_id);
	CREATE INDEX IF NOT EXISTS idx_paper_tags_tag_id ON paper_tags(tag_id);
	CREATE INDEX IF NOT EXISTS idx_figure_tags_tag_id ON figure_tags(tag_id);
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
		{tableName: "paper_figures", name: "notes_text", definition: "TEXT DEFAULT ''"},
	} {
		if err := r.ensureColumn(column.tableName, column.name, column.definition); err != nil {
			return err
		}
	}
	return r.ensureTagScopeSchema()
}

func (r *LibraryRepository) ensureColumn(tableName, columnName, definition string) error {
	hasColumn, err := r.hasColumn(tableName, columnName)
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}

	_, err = r.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, definition))
	return err
}

func (r *LibraryRepository) hasColumn(tableName, columnName string) (bool, error) {
	rows, err := r.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, err
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
			return false, err
		}
		if strings.EqualFold(name, columnName) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	return false, nil
}

func (r *LibraryRepository) Close() error {
	return r.db.Close()
}

func (r *LibraryRepository) ensureTagScopeSchema() (err error) {
	ready, err := r.tagScopeSchemaReady()
	if err != nil {
		return err
	}
	if ready {
		_, err = r.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_scope_name ON tags(scope, name COLLATE NOCASE)")
		if err != nil {
			return err
		}
		_, err = r.db.Exec("CREATE INDEX IF NOT EXISTS idx_tags_scope ON tags(scope)")
		return err
	}

	hasScope, err := r.hasColumn("tags", "scope")
	if err != nil {
		return err
	}

	if _, err = r.db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	defer func() {
		if _, pragmaErr := r.db.Exec("PRAGMA foreign_keys = ON"); err == nil && pragmaErr != nil {
			err = pragmaErr
		}
	}()

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := r.rebuildTagsWithScopes(tx, hasScope); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *LibraryRepository) tagScopeSchemaReady() (bool, error) {
	hasScope, err := r.hasColumn("tags", "scope")
	if err != nil || !hasScope {
		return false, err
	}

	rows, err := r.db.Query("PRAGMA index_list(tags)")
	if err != nil {
		return false, err
	}
	indexNames := []string{}
	indexUnique := map[string]bool{}
	for rows.Next() {
		var (
			seq     int
			name    string
			unique  int
			origin  string
			partial int
		)
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return false, err
		}
		indexNames = append(indexNames, name)
		indexUnique[name] = unique != 0
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	rows.Close()

	hasScopeNameUnique := false
	hasLegacyNameUnique := false
	for _, name := range indexNames {
		if !indexUnique[name] {
			continue
		}
		columns, err := r.indexColumns(name)
		if err != nil {
			return false, err
		}
		switch {
		case len(columns) == 2 && strings.EqualFold(columns[0], "scope") && strings.EqualFold(columns[1], "name"):
			hasScopeNameUnique = true
		case len(columns) == 1 && strings.EqualFold(columns[0], "name"):
			hasLegacyNameUnique = true
		}
	}
	return hasScopeNameUnique && !hasLegacyNameUnique, nil
}

func (r *LibraryRepository) indexColumns(indexName string) ([]string, error) {
	query := fmt.Sprintf("PRAGMA index_info('%s')", strings.ReplaceAll(indexName, "'", "''"))
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := []string{}
	for rows.Next() {
		var (
			seqno int
			cid   int
			name  string
		)
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func (r *LibraryRepository) rebuildTagsWithScopes(tx *sql.Tx, hasScope bool) error {
	type legacyTag struct {
		ID        int64
		Scope     model.TagScope
		Name      string
		Color     string
		CreatedAt string
		UpdatedAt string
	}

	query := "SELECT id, name, color, created_at, updated_at FROM tags ORDER BY id"
	if hasScope {
		query = "SELECT id, scope, name, color, created_at, updated_at FROM tags ORDER BY id"
	}

	rows, err := tx.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	tags := []legacyTag{}
	for rows.Next() {
		var tag legacyTag
		if hasScope {
			if err := rows.Scan(&tag.ID, &tag.Scope, &tag.Name, &tag.Color, &tag.CreatedAt, &tag.UpdatedAt); err != nil {
				return err
			}
			tag.Scope = model.NormalizeTagScope(string(tag.Scope))
		} else {
			if err := rows.Scan(&tag.ID, &tag.Name, &tag.Color, &tag.CreatedAt, &tag.UpdatedAt); err != nil {
				return err
			}
			tag.Scope = model.TagScopePaper
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	paperUsage, err := r.loadTagUsage(tx, "paper_tags", "paper_id")
	if err != nil {
		return err
	}
	figureUsage, err := r.loadTagUsage(tx, "figure_tags", "figure_id")
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
		CREATE TABLE tags_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scope TEXT NOT NULL DEFAULT 'paper',
			name TEXT NOT NULL COLLATE NOCASE,
			color TEXT DEFAULT '#A45C40',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(scope, name)
		)
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		CREATE TABLE paper_tags_new (
			paper_id INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
			tag_id INTEGER NOT NULL REFERENCES tags_new(id) ON DELETE CASCADE,
			PRIMARY KEY (paper_id, tag_id)
		)
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		CREATE TABLE figure_tags_new (
			figure_id INTEGER NOT NULL REFERENCES paper_figures(id) ON DELETE CASCADE,
			tag_id INTEGER NOT NULL REFERENCES tags_new(id) ON DELETE CASCADE,
			PRIMARY KEY (figure_id, tag_id)
		)
	`); err != nil {
		return err
	}

	tagIDMap := map[int64]map[model.TagScope]int64{}
	for _, tag := range tags {
		scopes := desiredTagScopes(paperUsage[tag.ID], figureUsage[tag.ID], tag.Scope)
		preferred := tag.Scope
		if !preferred.Valid() {
			preferred = scopes[0]
		}

		tagIDMap[tag.ID] = map[model.TagScope]int64{}
		for _, scope := range scopes {
			var newID int64
			if scope == preferred {
				if _, err := tx.Exec(`
					INSERT INTO tags_new (id, scope, name, color, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?)
				`, tag.ID, scope, tag.Name, tag.Color, tag.CreatedAt, tag.UpdatedAt); err != nil {
					return err
				}
				newID = tag.ID
			} else {
				result, err := tx.Exec(`
					INSERT INTO tags_new (scope, name, color, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?)
				`, scope, tag.Name, tag.Color, tag.CreatedAt, tag.UpdatedAt)
				if err != nil {
					return err
				}
				newID, err = result.LastInsertId()
				if err != nil {
					return err
				}
			}
			tagIDMap[tag.ID][scope] = newID
		}
	}

	if err := copyScopedTagLinks(tx, "paper_tags", "paper_id", "paper_tags_new", model.TagScopePaper, tagIDMap); err != nil {
		return err
	}
	if err := copyScopedTagLinks(tx, "figure_tags", "figure_id", "figure_tags_new", model.TagScopeFigure, tagIDMap); err != nil {
		return err
	}

	for _, stmt := range []string{
		"DROP TABLE paper_tags",
		"DROP TABLE figure_tags",
		"DROP TABLE tags",
		"ALTER TABLE tags_new RENAME TO tags",
		"ALTER TABLE paper_tags_new RENAME TO paper_tags",
		"ALTER TABLE figure_tags_new RENAME TO figure_tags",
		"CREATE UNIQUE INDEX idx_tags_scope_name ON tags(scope, name COLLATE NOCASE)",
		"CREATE INDEX idx_tags_scope ON tags(scope)",
		"CREATE INDEX idx_paper_tags_tag_id ON paper_tags(tag_id)",
		"CREATE INDEX idx_figure_tags_tag_id ON figure_tags(tag_id)",
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}

func (r *LibraryRepository) loadTagUsage(tx *sql.Tx, tableName, ownerColumn string) (map[int64]bool, error) {
	query := fmt.Sprintf("SELECT DISTINCT tag_id FROM %s WHERE %s IS NOT NULL", tableName, ownerColumn)
	rows, err := tx.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	usage := map[int64]bool{}
	for rows.Next() {
		var tagID int64
		if err := rows.Scan(&tagID); err != nil {
			return nil, err
		}
		usage[tagID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return usage, nil
}

func desiredTagScopes(usedByPaper, usedByFigure bool, current model.TagScope) []model.TagScope {
	switch {
	case usedByPaper && usedByFigure:
		return []model.TagScope{model.TagScopePaper, model.TagScopeFigure}
	case usedByPaper:
		return []model.TagScope{model.TagScopePaper}
	case usedByFigure:
		return []model.TagScope{model.TagScopeFigure}
	case current.Valid():
		return []model.TagScope{current}
	default:
		return []model.TagScope{model.TagScopePaper}
	}
}

func copyScopedTagLinks(tx *sql.Tx, sourceTable, ownerColumn, targetTable string, scope model.TagScope, tagIDMap map[int64]map[model.TagScope]int64) error {
	query := fmt.Sprintf("SELECT %s, tag_id FROM %s", ownerColumn, sourceTable)
	rows, err := tx.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	insertStmt := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s, tag_id) VALUES (?, ?)", targetTable, ownerColumn)
	for rows.Next() {
		var ownerID, legacyTagID int64
		if err := rows.Scan(&ownerID, &legacyTagID); err != nil {
			return err
		}
		newID := tagIDMap[legacyTagID][scope]
		if newID == 0 {
			return fmt.Errorf("missing migrated tag for legacy tag %d and scope %s", legacyTagID, scope)
		}
		if _, err := tx.Exec(insertStmt, ownerID, newID); err != nil {
			return err
		}
	}
	return rows.Err()
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

	if err := r.syncPaperTags(tx, id, input.Tags); err != nil {
		return nil, wrapDBError(err, "更新文献标签失败")
	}

	if err := tx.Commit(); err != nil {
		return nil, wrapDBError(err, "提交文献事务失败")
	}

	return r.GetPaperDetail(id)
}

func (r *LibraryRepository) UpdateFigure(id int64, input FigureUpdateInput) (*model.Paper, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, wrapDBError(err, "更新图片信息失败")
	}
	defer tx.Rollback()

	var paperID int64
	if err := tx.QueryRow("SELECT paper_id FROM paper_figures WHERE id = ?", id).Scan(&paperID); err != nil {
		if err == sql.ErrNoRows {
			return nil, notFoundError("figure not found")
		}
		return nil, wrapDBError(err, "更新图片信息失败")
	}

	result, err := tx.Exec(`
		UPDATE paper_figures
		SET notes_text = ?
		WHERE id = ?
	`, input.NotesText, id)
	if err != nil {
		return nil, wrapDBError(err, "更新图片信息失败")
	}
	if err := ensureRowsAffected(result, "figure not found"); err != nil {
		return nil, err
	}

	if err := r.syncFigureTags(tx, id, input.Tags); err != nil {
		return nil, wrapDBError(err, "更新图片标签失败")
	}

	if _, err := tx.Exec(`
		UPDATE papers
		SET updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, paperID); err != nil {
		return nil, wrapDBError(err, "更新图片信息失败")
	}

	if err := tx.Commit(); err != nil {
		return nil, wrapDBError(err, "提交图片事务失败")
	}

	return r.GetPaperDetail(paperID)
}

func (r *LibraryRepository) UpdateFigureTags(id int64, tags []TagUpsertInput) (*model.Paper, error) {
	figure, err := r.GetFigure(id)
	if err != nil {
		return nil, err
	}
	if figure == nil {
		return nil, notFoundError("figure not found")
	}

	return r.UpdateFigure(id, FigureUpdateInput{
		NotesText: figure.NotesText,
		Tags:      tags,
	})
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
			pf.filename, pf.page_number, pf.figure_index, pf.source, pf.caption, pf.notes_text, pf.created_at
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
		&item.NotesText,
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
	tagsByFigure, err := r.loadTagsByFigureIDs([]int64{id})
	if err != nil {
		return nil, wrapDBError(err, "查询图片标签失败")
	}
	item.Tags = tagsByFigure[id]
	if item.Tags == nil {
		item.Tags = []model.Tag{}
	}
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
		SELECT id, filename, original_name, content_type, page_number, figure_index, source, caption, notes_text, bbox_json, created_at
		FROM paper_figures
		WHERE paper_id = ?
		ORDER BY page_number ASC, figure_index ASC, id ASC
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
		if err := rows.Scan(
			&figure.ID,
			&figure.Filename,
			&figure.OriginalName,
			&figure.ContentType,
			&figure.PageNumber,
			&figure.FigureIndex,
			&figure.Source,
			&figure.Caption,
			&figure.NotesText,
			&bboxJSON,
			&figure.CreatedAt,
		); err != nil {
			return nil, wrapDBError(err, "查询文献图片失败")
		}
		figure.BBox = rawJSON(bboxJSON)
		figure.Tags = []model.Tag{}
		paper.Figures = append(paper.Figures, figure)
		figureIDs = append(figureIDs, figure.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, wrapDBError(err, "查询文献图片失败")
	}

	tagsByFigure, err := r.loadTagsByFigureIDs(figureIDs)
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
			pf.filename, pf.page_number, pf.figure_index, pf.source, pf.caption, pf.notes_text, pf.created_at
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
	figureIDs := []int64{}
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
			&item.NotesText,
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
		figureIDs = append(figureIDs, item.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, wrapDBError(err, "查询图片列表失败")
	}

	tagsByFigure, err := r.loadTagsByFigureIDs(figureIDs)
	if err != nil {
		return nil, 0, wrapDBError(err, "查询图片标签失败")
	}
	for i := range figures {
		if tags := tagsByFigure[figures[i].ID]; tags != nil {
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

func (r *LibraryRepository) ListTags(scope model.TagScope) ([]model.Tag, error) {
	scope = model.NormalizeTagScope(string(scope))
	rows, err := r.db.Query(`
		SELECT
			t.id, t.scope, t.name, t.color, t.created_at, t.updated_at,
			(SELECT COUNT(*) FROM paper_tags pt WHERE pt.tag_id = t.id) AS paper_count,
			(SELECT COUNT(*) FROM figure_tags ft WHERE ft.tag_id = t.id) AS figure_count
		FROM tags t
		WHERE t.scope = ?
		ORDER BY t.name COLLATE NOCASE ASC
	`, scope)
	if err != nil {
		return nil, wrapDBError(err, "查询标签列表失败")
	}
	defer rows.Close()

	tags := []model.Tag{}
	for rows.Next() {
		var tag model.Tag
		if err := rows.Scan(
			&tag.ID,
			&tag.Scope,
			&tag.Name,
			&tag.Color,
			&tag.CreatedAt,
			&tag.UpdatedAt,
			&tag.PaperCount,
			&tag.FigureCount,
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

func (r *LibraryRepository) CreateTag(scope model.TagScope, name, color string) (*model.Tag, error) {
	scope = model.NormalizeTagScope(string(scope))
	result, err := r.db.Exec(`
		INSERT INTO tags (scope, name, color)
		VALUES (?, ?, ?)
	`, scope, name, color)
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
	return r.syncEntityTags(tx, "paper_tags", "paper_id", paperID, tags, model.TagScopePaper)
}

func (r *LibraryRepository) syncFigureTags(tx *sql.Tx, figureID int64, tags []TagUpsertInput) error {
	return r.syncEntityTags(tx, "figure_tags", "figure_id", figureID, tags, model.TagScopeFigure)
}

func (r *LibraryRepository) syncEntityTags(tx *sql.Tx, tableName, ownerColumn string, ownerID int64, tags []TagUpsertInput, scope model.TagScope) error {
	if _, err := tx.Exec(
		fmt.Sprintf("DELETE FROM %s WHERE %s = ?", tableName, ownerColumn),
		ownerID,
	); err != nil {
		return err
	}

	if len(tags) == 0 {
		return nil
	}

	tagIDs, err := r.upsertTagIDs(tx, scopedTagInputs(tags, scope))
	if err != nil {
		return err
	}

	for _, tagID := range tagIDs {
		if _, err := tx.Exec(
			fmt.Sprintf("INSERT OR IGNORE INTO %s (%s, tag_id) VALUES (?, ?)", tableName, ownerColumn),
			ownerID,
			tagID,
		); err != nil {
			return err
		}
	}

	return nil
}

func scopedTagInputs(tags []TagUpsertInput, scope model.TagScope) []TagUpsertInput {
	scope = model.NormalizeTagScope(string(scope))
	scoped := make([]TagUpsertInput, 0, len(tags))
	for _, tag := range tags {
		tag.Scope = scope
		scoped = append(scoped, tag)
	}
	return scoped
}

func (r *LibraryRepository) upsertTagIDs(tx *sql.Tx, tags []TagUpsertInput) ([]int64, error) {
	tagIDs := make([]int64, 0, len(tags))
	for _, tag := range tags {
		scope := model.NormalizeTagScope(string(tag.Scope))
		if _, err := tx.Exec(`
			INSERT INTO tags (scope, name, color)
			VALUES (?, ?, ?)
			ON CONFLICT(scope, name) DO UPDATE SET
				color = CASE
					WHEN excluded.color <> '' THEN excluded.color
					ELSE tags.color
				END,
				updated_at = CURRENT_TIMESTAMP
		`, scope, tag.Name, tag.Color); err != nil {
			return nil, err
		}

		var id int64
		if err := tx.QueryRow("SELECT id FROM tags WHERE scope = ? AND name = ?", scope, tag.Name).Scan(&id); err != nil {
			return nil, err
		}
		tagIDs = append(tagIDs, id)
	}

	sort.Slice(tagIDs, func(i, j int) bool {
		return tagIDs[i] < tagIDs[j]
	})

	return tagIDs, nil
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
			t.id, t.scope, t.name, t.color, t.created_at, t.updated_at,
			(SELECT COUNT(*) FROM paper_tags pt WHERE pt.tag_id = t.id),
			(SELECT COUNT(*) FROM figure_tags ft WHERE ft.tag_id = t.id)
		FROM tags t
		WHERE t.id = ?
	`, id)

	var tag model.Tag
	if err := row.Scan(
		&tag.ID,
		&tag.Scope,
		&tag.Name,
		&tag.Color,
		&tag.CreatedAt,
		&tag.UpdatedAt,
		&tag.PaperCount,
		&tag.FigureCount,
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
		SELECT pt.paper_id, t.id, t.scope, t.name, t.color, t.created_at, t.updated_at
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
			&tag.Scope,
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

func (r *LibraryRepository) loadTagsByFigureIDs(figureIDs []int64) (map[int64][]model.Tag, error) {
	result := make(map[int64][]model.Tag, len(figureIDs))
	if len(figureIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(figureIDs))
	args := make([]interface{}, len(figureIDs))
	for i, figureID := range figureIDs {
		placeholders[i] = "?"
		args[i] = figureID
	}

	query := `
		SELECT ft.figure_id, t.id, t.scope, t.name, t.color, t.created_at, t.updated_at
		FROM figure_tags ft
		JOIN tags t ON t.id = ft.tag_id
		WHERE ft.figure_id IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY t.name COLLATE NOCASE ASC
	`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, wrapDBError(err, "查询图片标签失败")
	}
	defer rows.Close()

	for rows.Next() {
		var figureID int64
		var tag model.Tag
		if err := rows.Scan(
			&figureID,
			&tag.ID,
			&tag.Scope,
			&tag.Name,
			&tag.Color,
			&tag.CreatedAt,
			&tag.UpdatedAt,
		); err != nil {
			return nil, wrapDBError(err, "查询图片标签失败")
		}
		result[figureID] = append(result[figureID], tag)
	}

	if err := rows.Err(); err != nil {
		return nil, wrapDBError(err, "查询图片标签失败")
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
			pf.notes_text LIKE ? OR
			pf.original_name LIKE ? OR
			EXISTS (
				SELECT 1
				FROM figure_tags ft
				JOIN tags t ON t.id = ft.tag_id
				WHERE ft.figure_id = pf.id AND t.name LIKE ?
			)
		)`)
		args = append(args, like, like, like, like, like)
	}

	if filter.GroupID != nil {
		conditions = append(conditions, "p.group_id = ?")
		args = append(args, *filter.GroupID)
	}

	if filter.TagID != nil {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM figure_tags ft WHERE ft.figure_id = pf.id AND ft.tag_id = ?)")
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
