package repository

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/model"
)

type ExternalIDRepository struct {
	db *sql.DB
}

func NewExternalIDRepository(db *sql.DB) *ExternalIDRepository {
	return &ExternalIDRepository{db: db}
}

func (r *ExternalIDRepository) GetBySourceKey(source, libraryID, itemKey string) (*model.PaperExternalID, error) {
	source = strings.TrimSpace(source)
	libraryID = strings.TrimSpace(libraryID)
	itemKey = strings.TrimSpace(itemKey)
	if source == "" || itemKey == "" {
		return nil, nil
	}
	var row model.PaperExternalID
	err := r.db.QueryRow(`
		SELECT id, paper_id, source, library_id, item_key, collection_path, created_at, updated_at
		FROM paper_external_ids
		WHERE source = ? AND library_id = ? AND item_key = ?
	`, source, libraryID, itemKey).Scan(
		&row.ID,
		&row.PaperID,
		&row.Source,
		&row.LibraryID,
		&row.ItemKey,
		&row.CollectionPath,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, wrapDBError(err, "查询外部文献 ID 失败")
	}
	return &row, nil
}

func (r *ExternalIDRepository) Upsert(paperID int64, source, libraryID, itemKey, collectionPath string) error {
	source = strings.TrimSpace(source)
	libraryID = strings.TrimSpace(libraryID)
	itemKey = strings.TrimSpace(itemKey)
	collectionPath = strings.TrimSpace(collectionPath)
	if paperID <= 0 || source == "" || itemKey == "" {
		return nil
	}
	_, err := r.db.Exec(`
		INSERT INTO paper_external_ids (paper_id, source, library_id, item_key, collection_path)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(source, library_id, item_key) DO UPDATE SET
			paper_id = excluded.paper_id,
			collection_path = excluded.collection_path,
			updated_at = CURRENT_TIMESTAMP
	`, paperID, source, libraryID, itemKey, collectionPath)
	return wrapDBError(err, "保存外部文献 ID 失败")
}

type ZoteroImportRunRepository struct {
	db *sql.DB
}

func NewZoteroImportRunRepository(db *sql.DB) *ZoteroImportRunRepository {
	return &ZoteroImportRunRepository{db: db}
}

func (r *ZoteroImportRunRepository) Save(run *model.ZoteroImportRun) error {
	if run == nil || strings.TrimSpace(run.ID) == "" {
		return nil
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now()
	}
	run.UpdatedAt = time.Now()
	if run.CollectionKeys == nil {
		run.CollectionKeys = []string{}
	}
	if run.Items == nil {
		run.Items = []model.ZoteroImportItem{}
	}
	keysJSON, err := json.Marshal(run.CollectionKeys)
	if err != nil {
		return wrapDBError(err, "序列化 Zotero 导入集合失败")
	}
	itemsJSON, err := json.Marshal(run.Items)
	if err != nil {
		return wrapDBError(err, "序列化 Zotero 导入条目失败")
	}
	summaryJSON, err := json.Marshal(run.Summary)
	if err != nil {
		return wrapDBError(err, "序列化 Zotero 导入摘要失败")
	}
	includeChildren := 0
	if run.IncludeChildren {
		includeChildren = 1
	}
	_, err = r.db.Exec(`
		INSERT INTO zotero_import_runs (
			id, status, include_children, collection_keys_json, summary_json, items_json, error_text, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			include_children = excluded.include_children,
			collection_keys_json = excluded.collection_keys_json,
			summary_json = excluded.summary_json,
			items_json = excluded.items_json,
			error_text = excluded.error_text,
			updated_at = excluded.updated_at
	`, run.ID, run.Status, includeChildren, string(keysJSON), string(summaryJSON), string(itemsJSON), run.ErrorText, run.CreatedAt, run.UpdatedAt)
	return wrapDBError(err, "保存 Zotero 导入任务失败")
}

func (r *ZoteroImportRunRepository) Get(id string) (*model.ZoteroImportRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	var (
		run             model.ZoteroImportRun
		includeChildren int
		keysJSON        string
		summaryJSON     string
		itemsJSON       string
	)
	err := r.db.QueryRow(`
		SELECT id, status, include_children, collection_keys_json, summary_json, items_json, error_text, created_at, updated_at
		FROM zotero_import_runs
		WHERE id = ?
	`, id).Scan(
		&run.ID,
		&run.Status,
		&includeChildren,
		&keysJSON,
		&summaryJSON,
		&itemsJSON,
		&run.ErrorText,
		&run.CreatedAt,
		&run.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, wrapDBError(err, "查询 Zotero 导入任务失败")
	}
	run.IncludeChildren = includeChildren == 1
	if err := json.Unmarshal([]byte(keysJSON), &run.CollectionKeys); err != nil {
		run.CollectionKeys = []string{}
	}
	if err := json.Unmarshal([]byte(summaryJSON), &run.Summary); err != nil {
		run.Summary = model.ZoteroImportSummary{}
	}
	if err := json.Unmarshal([]byte(itemsJSON), &run.Items); err != nil {
		run.Items = []model.ZoteroImportItem{}
	}
	return &run, nil
}

func (r *ZoteroImportRunRepository) HasRunning() (bool, error) {
	var count int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM zotero_import_runs WHERE status IN ('queued', 'running')`).Scan(&count); err != nil {
		return false, wrapDBError(err, "查询进行中的 Zotero 导入失败")
	}
	return count > 0, nil
}
