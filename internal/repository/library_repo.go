package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"paper_image_db/internal/model"

	_ "modernc.org/sqlite"
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
	BoxesJSON        string
	ExtractionStatus string
	ExtractorMessage string
	ExtractorJobID   string
	GroupID          *int64
	Tags             []TagUpsertInput
	Figures          []FigureUpsertInput
}

func NewLibraryRepository(dbPath string) (*LibraryRepository, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	dsn := dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1)

	repo := &LibraryRepository{db: db}
	if err := repo.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
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

	CREATE TABLE IF NOT EXISTS papers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		original_filename TEXT NOT NULL,
		stored_pdf_name TEXT NOT NULL,
		file_size INTEGER DEFAULT 0,
		content_type TEXT DEFAULT 'application/pdf',
		pdf_text TEXT DEFAULT '',
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
	return r.ensureColumn("papers", "extractor_job_id", "TEXT DEFAULT ''")
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
		return false, err
	}
	return count > 0, nil
}

func (r *LibraryRepository) CreatePaper(input PaperUpsertInput) (*model.Paper, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		INSERT INTO papers (
			title, original_filename, stored_pdf_name, file_size, content_type,
			pdf_text, boxes_json, extraction_status, extractor_message, extractor_job_id, group_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		input.Title,
		input.OriginalFilename,
		input.StoredPDFName,
		input.FileSize,
		input.ContentType,
		input.PDFText,
		input.BoxesJSON,
		input.ExtractionStatus,
		input.ExtractorMessage,
		input.ExtractorJobID,
		input.GroupID,
	)
	if err != nil {
		return nil, err
	}

	paperID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	if err := r.syncPaperTags(tx, paperID, input.Tags); err != nil {
		return nil, err
	}

	for _, figure := range input.Figures {
		if _, err := tx.Exec(`
			INSERT INTO paper_figures (
				paper_id, filename, original_name, content_type, page_number, figure_index, caption, bbox_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`,
			paperID,
			figure.Filename,
			figure.OriginalName,
			figure.ContentType,
			figure.PageNumber,
			figure.FigureIndex,
			figure.Caption,
			figure.BBoxJSON,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetPaperDetail(paperID)
}

func (r *LibraryRepository) UpdatePaper(id int64, title string, groupID *int64, tags []TagUpsertInput) (*model.Paper, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE papers
		SET title = ?, group_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, title, groupID, id)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, sql.ErrNoRows
	}

	if _, err := tx.Exec("DELETE FROM paper_tags WHERE paper_id = ?", id); err != nil {
		return nil, err
	}
	if err := r.syncPaperTags(tx, id, tags); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetPaperDetail(id)
}

func (r *LibraryRepository) DeletePaper(id int64) error {
	_, err := r.db.Exec("DELETE FROM papers WHERE id = ?", id)
	return err
}

func (r *LibraryRepository) UpdatePaperExtractionState(id int64, status, message, jobID string) error {
	result, err := r.db.Exec(`
		UPDATE papers
		SET extraction_status = ?, extractor_message = ?, extractor_job_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, message, jobID, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
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
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE papers
		SET pdf_text = ?, boxes_json = ?, extraction_status = ?, extractor_message = ?, extractor_job_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, pdfText, boxesJSON, status, message, jobID, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	if _, err := tx.Exec("DELETE FROM paper_figures WHERE paper_id = ?", id); err != nil {
		return err
	}

	for _, figure := range figures {
		if _, err := tx.Exec(`
			INSERT INTO paper_figures (
				paper_id, filename, original_name, content_type, page_number, figure_index, caption, bbox_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`,
			id,
			figure.Filename,
			figure.OriginalName,
			figure.ContentType,
			figure.PageNumber,
			figure.FigureIndex,
			figure.Caption,
			figure.BBoxJSON,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *LibraryRepository) GetPaperDetail(id int64) (*model.Paper, error) {
	row := r.db.QueryRow(`
		SELECT
			p.id, p.title, p.original_filename, p.stored_pdf_name, p.file_size, p.content_type,
			p.pdf_text, p.boxes_json, p.extraction_status, p.extractor_message, p.extractor_job_id,
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
		return nil, err
	}

	tagsByPaper, err := r.loadTagsByPaperIDs([]int64{id})
	if err != nil {
		return nil, err
	}
	paper.Tags = tagsByPaper[id]
	if paper.Tags == nil {
		paper.Tags = []model.Tag{}
	}

	rows, err := r.db.Query(`
		SELECT id, filename, original_name, content_type, page_number, figure_index, caption, bbox_json, created_at
		FROM paper_figures
		WHERE paper_id = ?
		ORDER BY page_number ASC, figure_index ASC, id ASC
	`, id)
	if err != nil {
		return nil, err
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
			&figure.Caption,
			&bboxJSON,
			&figure.CreatedAt,
		); err != nil {
			return nil, err
		}
		figure.BBox = rawJSON(bboxJSON)
		paper.Figures = append(paper.Figures, figure)
	}

	return paper, rows.Err()
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
		return nil, 0, err
	}

	query := `
		SELECT
			p.id, p.title, p.original_filename, p.stored_pdf_name, p.file_size, p.content_type,
			'', '', p.extraction_status, p.extractor_message, p.extractor_job_id,
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
		return nil, 0, err
	}
	defer rows.Close()

	papers := []model.Paper{}
	paperIDs := []int64{}
	for rows.Next() {
		paper, err := scanPaper(rows, false)
		if err != nil {
			return nil, 0, err
		}
		paper.Tags = []model.Tag{}
		paper.Figures = nil
		papers = append(papers, *paper)
		paperIDs = append(paperIDs, paper.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	tagsByPaper, err := r.loadTagsByPaperIDs(paperIDs)
	if err != nil {
		return nil, 0, err
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
			'', '', p.extraction_status, p.extractor_message, p.extractor_job_id,
			p.group_id, COALESCE(g.name, ''),
			p.created_at, p.updated_at,
			(SELECT COUNT(*) FROM paper_figures pf WHERE pf.paper_id = p.id)
		FROM papers p
		LEFT JOIN groups g ON g.id = p.group_id
		WHERE p.extraction_status IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY p.updated_at DESC, p.id DESC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	papers := []model.Paper{}
	for rows.Next() {
		paper, err := scanPaper(rows, false)
		if err != nil {
			return nil, err
		}
		papers = append(papers, *paper)
	}

	return papers, rows.Err()
}

func (r *LibraryRepository) ListFigures(filter model.FigureFilter) ([]model.FigureListItem, int, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 200 {
		filter.PageSize = 24
	}

	whereClause, args := buildFigureWhere(filter)

	var total int
	countQuery := `
		SELECT COUNT(*)
		FROM paper_figures pf
		JOIN papers p ON p.id = pf.paper_id
	` + whereClause
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT
			pf.id, pf.paper_id, p.title, p.group_id, COALESCE(g.name, ''),
			pf.filename, pf.page_number, pf.figure_index, pf.caption, pf.created_at
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
		return nil, 0, err
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
			&item.Caption,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
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
		return nil, 0, err
	}

	tagsByPaper, err := r.loadTagsByPaperIDs(paperIDs)
	if err != nil {
		return nil, 0, err
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
		return nil, err
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
			return nil, err
		}
		groups = append(groups, group)
	}

	return groups, rows.Err()
}

func (r *LibraryRepository) CreateGroup(name, description string) (*model.Group, error) {
	result, err := r.db.Exec(`
		INSERT INTO groups (name, description)
		VALUES (?, ?)
	`, name, description)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
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
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, sql.ErrNoRows
	}

	return r.getGroupByID(id)
}

func (r *LibraryRepository) DeleteGroup(id int64) error {
	_, err := r.db.Exec("DELETE FROM groups WHERE id = ?", id)
	return err
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
		return nil, err
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
			return nil, err
		}
		tags = append(tags, tag)
	}

	return tags, rows.Err()
}

func (r *LibraryRepository) CreateTag(name, color string) (*model.Tag, error) {
	result, err := r.db.Exec(`
		INSERT INTO tags (name, color)
		VALUES (?, ?)
	`, name, color)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
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
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, sql.ErrNoRows
	}

	return r.getTagByID(id)
}

func (r *LibraryRepository) DeleteTag(id int64) error {
	_, err := r.db.Exec("DELETE FROM tags WHERE id = ?", id)
	return err
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
		return nil, err
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
		return nil, err
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
		return nil, err
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
			return nil, err
		}
		result[paperID] = append(result[paperID], tag)
	}

	return result, rows.Err()
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
		args = append(args, like, like, like, like, like)
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

	if err := s.Scan(
		&paper.ID,
		&paper.Title,
		&paper.OriginalFilename,
		&paper.StoredPDFName,
		&paper.FileSize,
		&paper.ContentType,
		&pdfText,
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

	return &paper, nil
}

func rawJSON(value string) json.RawMessage {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return json.RawMessage(value)
}
