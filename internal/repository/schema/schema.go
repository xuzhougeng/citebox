package schema

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// Manager 负责数据库 Schema 的初始化和迁移
type Manager struct {
	db *sql.DB
}

// NewManager 创建 Schema 管理器
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// Initialize 初始化数据库 Schema
func (m *Manager) Initialize() error {
	if err := m.initSchema(); err != nil {
		return err
	}
	return m.ensureSchemaColumns()
}

func (m *Manager) initSchema() error {
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
		scope TEXT NOT NULL DEFAULT 'paper' CHECK (scope IN ('paper', 'figure')),
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
		doi TEXT DEFAULT '',
		original_filename TEXT NOT NULL,
		stored_pdf_name TEXT NOT NULL,
		pdf_sha256 TEXT DEFAULT '',
		file_size INTEGER DEFAULT 0,
		content_type TEXT DEFAULT 'application/pdf',
		pdf_text TEXT DEFAULT '',
		abstract_text TEXT DEFAULT '',
		notes_text TEXT DEFAULT '',
		paper_notes_text TEXT DEFAULT '',
		boxes_json TEXT DEFAULT '',
		extraction_status TEXT DEFAULT 'completed' CHECK (extraction_status IN ('queued', 'running', 'manual_pending', 'completed', 'failed', 'cancelled')),
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
		parent_figure_id INTEGER REFERENCES paper_figures(id) ON DELETE CASCADE,
		subfigure_label TEXT DEFAULT '',
		source TEXT DEFAULT 'auto' CHECK (source IN ('auto', 'manual')),
		caption TEXT DEFAULT '',
		notes_text TEXT DEFAULT '',
		bbox_json TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS figure_tags (
		figure_id INTEGER NOT NULL REFERENCES paper_figures(id) ON DELETE CASCADE,
		tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
		PRIMARY KEY (figure_id, tag_id)
	);

	CREATE TABLE IF NOT EXISTS color_palettes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		paper_id INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
		figure_id INTEGER NOT NULL UNIQUE REFERENCES paper_figures(id) ON DELETE CASCADE,
		name TEXT DEFAULT '',
		colors_json TEXT NOT NULL DEFAULT '[]',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_papers_group_id ON papers(group_id);
	CREATE INDEX IF NOT EXISTS idx_papers_created_at ON papers(created_at);
	CREATE INDEX IF NOT EXISTS idx_papers_status ON papers(extraction_status);
	CREATE INDEX IF NOT EXISTS idx_paper_figures_paper_id ON paper_figures(paper_id);
	CREATE INDEX IF NOT EXISTS idx_color_palettes_paper_id ON color_palettes(paper_id);
	CREATE INDEX IF NOT EXISTS idx_paper_tags_tag_id ON paper_tags(tag_id);
	CREATE INDEX IF NOT EXISTS idx_figure_tags_tag_id ON figure_tags(tag_id);
	`

	if _, err := m.db.Exec(schema); err != nil {
		return err
	}
	return nil
}

func (m *Manager) ensureSchemaColumns() error {
	for _, column := range []struct {
		tableName  string
		name       string
		definition string
	}{
		{tableName: "papers", name: "doi", definition: "TEXT DEFAULT ''"},
		{tableName: "papers", name: "extractor_job_id", definition: "TEXT DEFAULT ''"},
		{tableName: "papers", name: "abstract_text", definition: "TEXT DEFAULT ''"},
		{tableName: "papers", name: "notes_text", definition: "TEXT DEFAULT ''"},
		{tableName: "papers", name: "pdf_sha256", definition: "TEXT DEFAULT ''"},
		{tableName: "paper_figures", name: "source", definition: "TEXT DEFAULT 'auto'"},
		{tableName: "paper_figures", name: "notes_text", definition: "TEXT DEFAULT ''"},
		{tableName: "paper_figures", name: "updated_at", definition: "DATETIME"},
		{tableName: "paper_figures", name: "parent_figure_id", definition: "INTEGER REFERENCES paper_figures(id) ON DELETE CASCADE"},
		{tableName: "paper_figures", name: "subfigure_label", definition: "TEXT DEFAULT ''"},
	} {
		if err := m.ensureColumn(column.tableName, column.name, column.definition); err != nil {
			return err
		}
	}
	if err := m.ensurePaperNotesSchema(); err != nil {
		return err
	}
	if err := m.ensureTagScopeSchema(); err != nil {
		return err
	}
	if err := m.ensureFigureUpdatedAtValues(); err != nil {
		return err
	}
	if err := m.ensureValidationTriggers(); err != nil {
		return err
	}
	if err := m.ensureIndexes(); err != nil {
		return err
	}
	return m.ensureFTSSchema()
}

func (m *Manager) ensurePaperNotesSchema() error {
	hasColumn, err := m.hasColumn("papers", "paper_notes_text")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}

	if _, err := m.db.Exec("ALTER TABLE papers ADD COLUMN paper_notes_text TEXT DEFAULT ''"); err != nil {
		return err
	}
	_, err = m.db.Exec(`
		UPDATE papers
		SET paper_notes_text = COALESCE(notes_text, ''),
			notes_text = ''
		WHERE TRIM(COALESCE(notes_text, '')) <> ''
		  AND TRIM(COALESCE(paper_notes_text, '')) = ''
	`)
	return err
}

func (m *Manager) ensureColumn(tableName, columnName, definition string) error {
	hasColumn, err := m.hasColumn(tableName, columnName)
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}

	_, err = m.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, definition))
	return err
}

func (m *Manager) hasColumn(tableName, columnName string) (bool, error) {
	rows, err := m.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
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

func (m *Manager) hasSchemaObject(objectType, name string) (bool, error) {
	var count int
	if err := m.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = ? AND name = ?",
		objectType,
		name,
	).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (m *Manager) hasTable(tableName string) (bool, error) {
	return m.hasSchemaObject("table", tableName)
}

func (m *Manager) hasIndex(indexName string) (bool, error) {
	return m.hasSchemaObject("index", indexName)
}

func (m *Manager) ensureFigureUpdatedAtValues() error {
	_, err := m.db.Exec(`
		UPDATE paper_figures
		SET updated_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP)
		WHERE updated_at IS NULL
	`)
	return err
}

func (m *Manager) ensureValidationTriggers() error {
	for _, statement := range []string{
		`CREATE TRIGGER IF NOT EXISTS validate_tags_scope_insert
		BEFORE INSERT ON tags
		FOR EACH ROW
		WHEN NEW.scope NOT IN ('paper', 'figure')
		BEGIN
			SELECT RAISE(ABORT, 'invalid tag scope');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS validate_tags_scope_update
		BEFORE UPDATE OF scope ON tags
		FOR EACH ROW
		WHEN NEW.scope NOT IN ('paper', 'figure')
		BEGIN
			SELECT RAISE(ABORT, 'invalid tag scope');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS validate_papers_status_insert
		BEFORE INSERT ON papers
		FOR EACH ROW
		WHEN NEW.extraction_status NOT IN ('queued', 'running', 'manual_pending', 'completed', 'failed', 'cancelled')
		BEGIN
			SELECT RAISE(ABORT, 'invalid paper extraction status');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS validate_papers_status_update
		BEFORE UPDATE OF extraction_status ON papers
		FOR EACH ROW
		WHEN NEW.extraction_status NOT IN ('queued', 'running', 'manual_pending', 'completed', 'failed', 'cancelled')
		BEGIN
			SELECT RAISE(ABORT, 'invalid paper extraction status');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS validate_paper_figures_source_insert
		BEFORE INSERT ON paper_figures
		FOR EACH ROW
		WHEN COALESCE(NEW.source, 'auto') NOT IN ('auto', 'manual')
		BEGIN
			SELECT RAISE(ABORT, 'invalid figure source');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS validate_paper_figures_source_update
		BEFORE UPDATE OF source ON paper_figures
		FOR EACH ROW
		WHEN COALESCE(NEW.source, 'auto') NOT IN ('auto', 'manual')
		BEGIN
			SELECT RAISE(ABORT, 'invalid figure source');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS validate_subfigure_label_insert
		BEFORE INSERT ON paper_figures
		FOR EACH ROW
		WHEN NEW.parent_figure_id IS NOT NULL AND COALESCE(TRIM(NEW.subfigure_label), '') = ''
		BEGIN
			SELECT RAISE(ABORT, 'subfigure label required');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS validate_subfigure_label_update
		BEFORE UPDATE OF parent_figure_id, subfigure_label ON paper_figures
		FOR EACH ROW
		WHEN NEW.parent_figure_id IS NOT NULL AND COALESCE(TRIM(NEW.subfigure_label), '') = ''
		BEGIN
			SELECT RAISE(ABORT, 'subfigure label required');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS validate_subfigure_parent_paper_insert
		BEFORE INSERT ON paper_figures
		FOR EACH ROW
		WHEN NEW.parent_figure_id IS NOT NULL AND EXISTS (
			SELECT 1
			FROM paper_figures parent
			WHERE parent.id = NEW.parent_figure_id
			  AND parent.paper_id != NEW.paper_id
		)
		BEGIN
			SELECT RAISE(ABORT, 'subfigure parent paper mismatch');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS validate_subfigure_parent_paper_update
		BEFORE UPDATE OF parent_figure_id, paper_id ON paper_figures
		FOR EACH ROW
		WHEN NEW.parent_figure_id IS NOT NULL AND EXISTS (
			SELECT 1
			FROM paper_figures parent
			WHERE parent.id = NEW.parent_figure_id
			  AND parent.paper_id != NEW.paper_id
		)
		BEGIN
			SELECT RAISE(ABORT, 'subfigure parent paper mismatch');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS validate_subfigure_depth_insert
		BEFORE INSERT ON paper_figures
		FOR EACH ROW
		WHEN NEW.parent_figure_id IS NOT NULL AND EXISTS (
			SELECT 1
			FROM paper_figures parent
			WHERE parent.id = NEW.parent_figure_id
			  AND parent.parent_figure_id IS NOT NULL
		)
		BEGIN
			SELECT RAISE(ABORT, 'subfigure depth exceeded');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS validate_subfigure_depth_update
		BEFORE UPDATE OF parent_figure_id ON paper_figures
		FOR EACH ROW
		WHEN NEW.parent_figure_id IS NOT NULL AND EXISTS (
			SELECT 1
			FROM paper_figures parent
			WHERE parent.id = NEW.parent_figure_id
			  AND parent.parent_figure_id IS NOT NULL
		)
		BEGIN
			SELECT RAISE(ABORT, 'subfigure depth exceeded');
		END;`,
	} {
		if _, err := m.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) ensureIndexes() error {
	if err := m.ensureUniqueIndex(
		"idx_papers_stored_pdf_name_unique",
		"papers",
		"stored_pdf_name",
		"数据库中存在重复的文献文件名，无法完成升级，请先清理重复数据",
	); err != nil {
		return err
	}
	if err := m.ensureUniqueNonEmptyIndex(
		"idx_papers_pdf_sha256_unique",
		"papers",
		"pdf_sha256",
		"数据库中存在重复的 PDF 指纹，无法完成升级，请先清理重复数据",
	); err != nil {
		return err
	}
	if err := m.ensureUniqueNonEmptyIndex(
		"idx_papers_doi_unique",
		"papers",
		"doi",
		"数据库中存在重复的 DOI，无法完成升级，请先清理重复数据",
	); err != nil {
		return err
	}
	if err := m.ensureUniqueIndex(
		"idx_paper_figures_filename_unique",
		"paper_figures",
		"filename",
		"数据库中存在重复的图片文件名，无法完成升级，请先清理重复数据",
	); err != nil {
		return err
	}

	for _, statement := range []string{
		"CREATE INDEX IF NOT EXISTS idx_paper_figures_updated_at ON paper_figures(updated_at)",
		"CREATE INDEX IF NOT EXISTS idx_paper_figures_parent_figure_id ON paper_figures(parent_figure_id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_paper_figures_parent_label_unique ON paper_figures(parent_figure_id, subfigure_label) WHERE parent_figure_id IS NOT NULL AND COALESCE(TRIM(subfigure_label), '') <> ''",
		"CREATE INDEX IF NOT EXISTS idx_color_palettes_paper_id ON color_palettes(paper_id)",
		"CREATE INDEX IF NOT EXISTS idx_tags_scope ON tags(scope)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_scope_name ON tags(scope, name)",
	} {
		if _, err := m.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) ensureUniqueIndex(indexName, tableName, columnName, duplicateMessage string) error {
	hasIndex, err := m.hasIndex(indexName)
	if err != nil {
		return err
	}
	if hasIndex {
		return nil
	}

	var duplicateValue string
	query := fmt.Sprintf(`
		SELECT %s
		FROM %s
		GROUP BY %s
		HAVING COUNT(*) > 1
		LIMIT 1
	`, columnName, tableName, columnName)
	err = m.db.QueryRow(query).Scan(&duplicateValue)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		return apperr.New(apperr.CodeFailedPrecondition, duplicateMessage)
	}

	_, err = m.db.Exec(fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s(%s)", indexName, tableName, columnName))
	return err
}

func (m *Manager) ensureUniqueNonEmptyIndex(indexName, tableName, columnName, duplicateMessage string) error {
	hasIndex, err := m.hasIndex(indexName)
	if err != nil {
		return err
	}
	if hasIndex {
		return nil
	}

	var duplicateValue string
	query := fmt.Sprintf(`
		SELECT %s
		FROM %s
		WHERE COALESCE(TRIM(%s), '') <> ''
		GROUP BY %s
		HAVING COUNT(*) > 1
		LIMIT 1
	`, columnName, tableName, columnName, columnName)
	err = m.db.QueryRow(query).Scan(&duplicateValue)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		return apperr.New(apperr.CodeFailedPrecondition, duplicateMessage)
	}

	_, err = m.db.Exec(fmt.Sprintf(
		"CREATE UNIQUE INDEX %s ON %s(%s) WHERE COALESCE(TRIM(%s), '') <> ''",
		indexName,
		tableName,
		columnName,
		columnName,
	))
	return err
}

func (m *Manager) ensureFTSSchema() error {
	papersFTSCreated, err := m.ensureFTSTable("papers_fts", `
		CREATE VIRTUAL TABLE papers_fts USING fts5(
			title,
			original_filename,
			abstract_text,
			notes_text,
			pdf_text,
			tokenize='trigram'
		)
	`)
	if err != nil {
		return err
	}
	figuresFTSCreated, err := m.ensureFTSTable("figures_fts", `
		CREATE VIRTUAL TABLE figures_fts USING fts5(
			original_name,
			caption,
			notes_text,
			tokenize='trigram'
		)
	`)
	if err != nil {
		return err
	}

	for _, statement := range []string{
		`CREATE TRIGGER IF NOT EXISTS papers_fts_insert AFTER INSERT ON papers BEGIN
			INSERT INTO papers_fts(rowid, title, original_filename, abstract_text, notes_text, pdf_text)
			VALUES (NEW.id, NEW.title, NEW.original_filename, NEW.abstract_text, NEW.notes_text, NEW.pdf_text);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS papers_fts_update AFTER UPDATE ON papers BEGIN
			DELETE FROM papers_fts WHERE rowid = OLD.id;
			INSERT INTO papers_fts(rowid, title, original_filename, abstract_text, notes_text, pdf_text)
			VALUES (NEW.id, NEW.title, NEW.original_filename, NEW.abstract_text, NEW.notes_text, NEW.pdf_text);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS papers_fts_delete AFTER DELETE ON papers BEGIN
			DELETE FROM papers_fts WHERE rowid = OLD.id;
		END;`,
		`CREATE TRIGGER IF NOT EXISTS figures_fts_insert AFTER INSERT ON paper_figures BEGIN
			INSERT INTO figures_fts(rowid, original_name, caption, notes_text)
			VALUES (NEW.id, NEW.original_name, NEW.caption, NEW.notes_text);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS figures_fts_update AFTER UPDATE ON paper_figures BEGIN
			DELETE FROM figures_fts WHERE rowid = OLD.id;
			INSERT INTO figures_fts(rowid, original_name, caption, notes_text)
			VALUES (NEW.id, NEW.original_name, NEW.caption, NEW.notes_text);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS figures_fts_delete AFTER DELETE ON paper_figures BEGIN
			DELETE FROM figures_fts WHERE rowid = OLD.id;
		END;`,
	} {
		if _, err := m.db.Exec(statement); err != nil {
			return err
		}
	}

	paperCount, err := m.countRows("papers")
	if err != nil {
		return err
	}
	papersFTSCount, err := m.countRows("papers_fts")
	if err != nil {
		return err
	}
	if papersFTSCreated || paperCount != papersFTSCount {
		if err := m.rebuildPapersFTS(); err != nil {
			return err
		}
	}

	figureCount, err := m.countRows("paper_figures")
	if err != nil {
		return err
	}
	figuresFTSCount, err := m.countRows("figures_fts")
	if err != nil {
		return err
	}
	if figuresFTSCreated || figureCount != figuresFTSCount {
		if err := m.rebuildFiguresFTS(); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) ensureFTSTable(tableName, createSQL string) (bool, error) {
	hasTable, err := m.hasTable(tableName)
	if err != nil {
		return false, err
	}
	if hasTable {
		return false, nil
	}
	if _, err := m.db.Exec(createSQL); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) countRows(tableName string) (int, error) {
	var count int
	if err := m.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (m *Manager) rebuildPapersFTS() error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM papers_fts"); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO papers_fts(rowid, title, original_filename, abstract_text, notes_text, pdf_text)
		SELECT id, title, original_filename, abstract_text, notes_text, pdf_text
		FROM papers
	`); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *Manager) rebuildFiguresFTS() error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM figures_fts"); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO figures_fts(rowid, original_name, caption, notes_text)
		SELECT id, original_name, caption, notes_text
		FROM paper_figures
	`); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *Manager) ensureTagScopeSchema() (err error) {
	ready, err := m.tagScopeSchemaReady()
	if err != nil {
		return err
	}
	if ready {
		_, err = m.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_scope_name ON tags(scope, name COLLATE NOCASE)")
		if err != nil {
			return err
		}
		_, err = m.db.Exec("CREATE INDEX IF NOT EXISTS idx_tags_scope ON tags(scope)")
		return err
	}

	hasScope, err := m.hasColumn("tags", "scope")
	if err != nil {
		return err
	}

	if _, err = m.db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	defer func() {
		if _, pragmaErr := m.db.Exec("PRAGMA foreign_keys = ON"); err == nil && pragmaErr != nil {
			err = pragmaErr
		}
	}()

	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := m.rebuildTagsWithScopes(tx, hasScope); err != nil {
		return err
	}

	return tx.Commit()
}

func (m *Manager) tagScopeSchemaReady() (bool, error) {
	hasScope, err := m.hasColumn("tags", "scope")
	if err != nil || !hasScope {
		return false, err
	}

	rows, err := m.db.Query("PRAGMA index_list(tags)")
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
		columns, err := m.indexColumns(name)
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

func (m *Manager) indexColumns(indexName string) ([]string, error) {
	query := fmt.Sprintf("PRAGMA index_info('%s')", strings.ReplaceAll(indexName, "'", "''"))
	rows, err := m.db.Query(query)
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

func (m *Manager) rebuildTagsWithScopes(tx *sql.Tx, hasScope bool) error {
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

	paperUsage, err := m.loadTagUsage(tx, "paper_tags", "paper_id")
	if err != nil {
		return err
	}
	figureUsage, err := m.loadTagUsage(tx, "figure_tags", "figure_id")
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

func (m *Manager) loadTagUsage(tx *sql.Tx, tableName, ownerColumn string) (map[int64]bool, error) {
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
