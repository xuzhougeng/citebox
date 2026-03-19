package repository

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
)

// TagRepository 负责标签相关的数据操作
type TagRepository struct {
	db *sql.DB
}

// NewTagRepository 创建标签仓库
func NewTagRepository(db *sql.DB) *TagRepository {
	return &TagRepository{db: db}
}

// ListTags 查询标签列表
func (r *TagRepository) ListTags(scope model.TagScope) ([]model.Tag, error) {
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

// CreateTag 创建标签
func (r *TagRepository) CreateTag(scope model.TagScope, name, color string) (*model.Tag, error) {
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

// UpdateTag 更新标签
func (r *TagRepository) UpdateTag(id int64, name, color string) (*model.Tag, error) {
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

// DeleteTag 删除标签
func (r *TagRepository) DeleteTag(id int64) error {
	result, err := r.db.Exec("DELETE FROM tags WHERE id = ?", id)
	if err != nil {
		return wrapDBError(err, "删除标签失败")
	}
	return ensureRowsAffected(result, "tag not found")
}

// getTagByID 根据 ID 获取标签
func (r *TagRepository) getTagByID(id int64) (*model.Tag, error) {
	var tag model.Tag
	if err := r.db.QueryRow(`
		SELECT
			t.id, t.scope, t.name, t.color, t.created_at, t.updated_at,
			(SELECT COUNT(*) FROM paper_tags pt WHERE pt.tag_id = t.id) AS paper_count,
			(SELECT COUNT(*) FROM figure_tags ft WHERE ft.tag_id = t.id) AS figure_count
		FROM tags t
		WHERE t.id = ?
	`, id).Scan(
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
			return nil, nil
		}
		return nil, wrapDBError(err, "查询标签失败")
	}
	return &tag, nil
}

// SyncPaperTags 同步文献标签
func (r *TagRepository) SyncPaperTags(tx *sql.Tx, paperID int64, tags []TagUpsertInput) error {
	return r.syncEntityTags(tx, "paper_tags", "paper_id", paperID, tags, model.TagScopePaper)
}

// SyncFigureTags 同步图片标签
func (r *TagRepository) SyncFigureTags(tx *sql.Tx, figureID int64, tags []TagUpsertInput) error {
	return r.syncEntityTags(tx, "figure_tags", "figure_id", figureID, tags, model.TagScopeFigure)
}

// syncEntityTags 同步实体标签的通用方法
func (r *TagRepository) syncEntityTags(tx *sql.Tx, tableName, ownerColumn string, ownerID int64, tags []TagUpsertInput, scope model.TagScope) error {
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

// upsertTagIDs 批量插入或更新标签并返回标签 ID 列表
func (r *TagRepository) upsertTagIDs(tx *sql.Tx, tags []TagUpsertInput) ([]int64, error) {
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

// LoadTagsByPaperIDs 批量加载文献标签
func (r *TagRepository) LoadTagsByPaperIDs(paperIDs []int64) (map[int64][]model.Tag, error) {
	return r.loadTagsByEntityIDs("paper_tags", "paper_id", paperIDs)
}

// LoadTagsByFigureIDs 批量加载图片标签
func (r *TagRepository) LoadTagsByFigureIDs(figureIDs []int64) (map[int64][]model.Tag, error) {
	return r.loadTagsByEntityIDs("figure_tags", "figure_id", figureIDs)
}

// loadTagsByEntityIDs 批量加载实体标签的通用方法
func (r *TagRepository) loadTagsByEntityIDs(linkTable, ownerColumn string, ownerIDs []int64) (map[int64][]model.Tag, error) {
	result := map[int64][]model.Tag{}
	if len(ownerIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(ownerIDs))
	args := make([]interface{}, 0, len(ownerIDs))
	for i, id := range ownerIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		SELECT lt.%s, t.id, t.scope, t.name, t.color, t.created_at, t.updated_at
		FROM %s lt
		JOIN tags t ON t.id = lt.tag_id
		WHERE lt.%s IN (%s)
	`, ownerColumn, linkTable, ownerColumn, strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ownerID int64
		var tag model.Tag
		if err := rows.Scan(
			&ownerID,
			&tag.ID,
			&tag.Scope,
			&tag.Name,
			&tag.Color,
			&tag.CreatedAt,
			&tag.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if result[ownerID] == nil {
			result[ownerID] = []model.Tag{}
		}
		result[ownerID] = append(result[ownerID], tag)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// scopedTagInputs 将标签输入转换为指定 scope
func scopedTagInputs(tags []TagUpsertInput, scope model.TagScope) []TagUpsertInput {
	scope = model.NormalizeTagScope(string(scope))
	scoped := make([]TagUpsertInput, 0, len(tags))
	for _, tag := range tags {
		tag.Scope = scope
		scoped = append(scoped, tag)
	}
	return scoped
}

// uniqueInt64s 去重 int64 切片
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
