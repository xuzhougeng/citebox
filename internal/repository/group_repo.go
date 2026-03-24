package repository

import (
	"database/sql"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// GroupRepository 负责分组相关的数据操作
type GroupRepository struct {
	db *sql.DB
}

// NewGroupRepository 创建分组仓库
func NewGroupRepository(db *sql.DB) *GroupRepository {
	return &GroupRepository{db: db}
}

// ListGroups 查询分组列表
func (r *GroupRepository) ListGroups() ([]model.Group, error) {
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

// GetGroupByID 根据 ID 获取分组
func (r *GroupRepository) GetGroupByID(id int64) (*model.Group, error) {
	var group model.Group
	if err := r.db.QueryRow(`
		SELECT
			g.id, g.name, g.description, g.created_at, g.updated_at,
			COUNT(p.id) AS paper_count
		FROM groups g
		LEFT JOIN papers p ON p.group_id = g.id
		WHERE g.id = ?
		GROUP BY g.id
	`, id).Scan(
		&group.ID,
		&group.Name,
		&group.Description,
		&group.CreatedAt,
		&group.UpdatedAt,
		&group.PaperCount,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, wrapDBError(err, "查询分组失败")
	}
	return &group, nil
}

// CreateGroup 创建分组
func (r *GroupRepository) CreateGroup(name, description string) (*model.Group, error) {
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

	return r.GetGroupByID(id)
}

// UpdateGroup 更新分组
func (r *GroupRepository) UpdateGroup(id int64, name, description string) (*model.Group, error) {
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

	return r.GetGroupByID(id)
}

// DeleteGroup 删除分组
func (r *GroupRepository) DeleteGroup(id int64) error {
	result, err := r.db.Exec("DELETE FROM groups WHERE id = ?", id)
	if err != nil {
		return wrapDBError(err, "删除分组失败")
	}
	return ensureRowsAffected(result, "group not found")
}

// GroupExists 检查分组是否存在
func (r *GroupRepository) GroupExists(id int64) (bool, error) {
	var count int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM groups WHERE id = ?", id).Scan(&count); err != nil {
		return false, wrapDBError(err, "查询分组失败")
	}
	return count > 0, nil
}

// GroupExistsByName 检查分组名称是否已存在（排除指定 ID）
func (r *GroupRepository) GroupExistsByName(name string, excludeID *int64) (bool, error) {
	query := "SELECT COUNT(*) FROM groups WHERE name = ?"
	args := []interface{}{name}
	if excludeID != nil {
		query += " AND id != ?"
		args = append(args, *excludeID)
	}

	var count int
	if err := r.db.QueryRow(query, args...).Scan(&count); err != nil {
		return false, wrapDBError(err, "查询分组失败")
	}
	return count > 0, nil
}

// EnsureDefaultGroup 确保存在默认分组
func (r *GroupRepository) EnsureDefaultGroup() (int64, error) {
	var id int64
	err := r.db.QueryRow("SELECT id FROM groups WHERE name = ?", "默认分组").Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, wrapDBError(err, "查询默认分组失败")
	}

	// 创建默认分组
	result, err := r.db.Exec(`
		INSERT INTO groups (name, description)
		VALUES (?, ?)
	`, "默认分组", "系统自动创建的默认分组")
	if err != nil {
		return 0, wrapDBError(err, "创建默认分组失败")
	}

	id, err = result.LastInsertId()
	if err != nil {
		return 0, wrapDBError(err, "读取默认分组 ID 失败")
	}
	return id, nil
}

// GetGroupStats 获取分组统计信息
func (r *GroupRepository) GetGroupStats(id int64) (*model.GroupStats, error) {
	var stats model.GroupStats
	if err := r.db.QueryRow(`
		SELECT
			COUNT(DISTINCT p.id) AS paper_count,
			COUNT(DISTINCT pf.id) AS figure_count
		FROM groups g
		LEFT JOIN papers p ON p.group_id = g.id
		LEFT JOIN paper_figures pf ON pf.paper_id = p.id AND pf.parent_figure_id IS NULL
		WHERE g.id = ?
	`, id).Scan(&stats.PaperCount, &stats.FigureCount); err != nil {
		if err == sql.ErrNoRows {
			return nil, apperr.New(apperr.CodeNotFound, "分组不存在")
		}
		return nil, wrapDBError(err, "查询分组统计失败")
	}
	return &stats, nil
}
