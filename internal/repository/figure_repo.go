package repository

import (
	"database/sql"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// FigureRepository 负责图片相关的数据操作
type FigureRepository struct {
	db  *sql.DB
	tag *TagRepository
}

// NewFigureRepository 创建图片仓库
func NewFigureRepository(db *sql.DB, tagRepo *TagRepository) *FigureRepository {
	return &FigureRepository{db: db, tag: tagRepo}
}

// GetFigure 根据 ID 获取图片
func (r *FigureRepository) GetFigure(id int64) (*model.FigureListItem, error) {
	row := r.db.QueryRow(`
		SELECT
			pf.id, pf.paper_id, p.title, p.group_id, COALESCE(g.name, ''),
			pf.filename, pf.page_number, pf.figure_index, pf.parent_figure_id, pf.subfigure_label, pf.source, pf.caption, pf.notes_text,
			cp.id, COALESCE(cp.name, ''), COALESCE(cp.colors_json, ''),
			CASE WHEN cp.id IS NULL THEN 0 ELSE 1 END AS palette_count,
			pf.created_at, pf.updated_at
		FROM paper_figures pf
		JOIN papers p ON p.id = pf.paper_id
		LEFT JOIN groups g ON g.id = p.group_id
		LEFT JOIN color_palettes cp ON cp.figure_id = pf.id
		WHERE pf.id = ?
	`, id)

	var item model.FigureListItem
	var groupID sql.NullInt64
	var parentFigureID sql.NullInt64
	var paletteID sql.NullInt64
	var groupName string
	var paletteName string
	var paletteColorsJSON string
	if err := row.Scan(
		&item.ID,
		&item.PaperID,
		&item.PaperTitle,
		&groupID,
		&groupName,
		&item.Filename,
		&item.PageNumber,
		&item.FigureIndex,
		&parentFigureID,
		&item.SubfigureLabel,
		&item.Source,
		&item.Caption,
		&item.NotesText,
		&paletteID,
		&paletteName,
		&paletteColorsJSON,
		&item.PaletteCount,
		&item.CreatedAt,
		&item.UpdatedAt,
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
	if parentFigureID.Valid {
		item.ParentFigureID = &parentFigureID.Int64
	}
	paletteRef, paletteTitle, paletteColors, parseErr := parsePaletteSummary(paletteID, paletteName, paletteColorsJSON)
	if parseErr != nil {
		return nil, wrapDBError(parseErr, "解析图片配色失败")
	}
	item.PaletteID = paletteRef
	item.PaletteName = paletteTitle
	item.PaletteColors = paletteColors
	tagsByFigure, err := r.tag.LoadTagsByFigureIDs([]int64{id})
	if err != nil {
		return nil, wrapDBError(err, "查询图片标签失败")
	}
	item.Tags = tagsByFigure[id]
	if item.Tags == nil {
		item.Tags = []model.Tag{}
	}
	return &item, nil
}

// ListFigures 查询图片列表
func (r *FigureRepository) ListFigures(filter model.FigureFilter) ([]model.FigureListItem, int, error) {
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
			pf.filename, pf.page_number, pf.figure_index, pf.parent_figure_id, pf.subfigure_label, pf.source, pf.caption, pf.notes_text,
			cp.id, COALESCE(cp.name, ''), COALESCE(cp.colors_json, ''),
			CASE WHEN cp.id IS NULL THEN 0 ELSE 1 END AS palette_count,
			pf.created_at, pf.updated_at
		FROM paper_figures pf
		JOIN papers p ON p.id = pf.paper_id
		LEFT JOIN groups g ON g.id = p.group_id
		LEFT JOIN color_palettes cp ON cp.figure_id = pf.id
		` + whereClause + `
		` + buildFigureOrderBy(filter) + `
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
		var parentFigureID sql.NullInt64
		var paletteID sql.NullInt64
		var groupName string
		var paletteName string
		var paletteColorsJSON string
		if err := rows.Scan(
			&item.ID,
			&item.PaperID,
			&item.PaperTitle,
			&groupID,
			&groupName,
			&item.Filename,
			&item.PageNumber,
			&item.FigureIndex,
			&parentFigureID,
			&item.SubfigureLabel,
			&item.Source,
			&item.Caption,
			&item.NotesText,
			&paletteID,
			&paletteName,
			&paletteColorsJSON,
			&item.PaletteCount,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, 0, wrapDBError(err, "查询图片列表失败")
		}
		if groupID.Valid {
			item.GroupID = &groupID.Int64
			item.GroupName = groupName
		}
		if parentFigureID.Valid {
			item.ParentFigureID = &parentFigureID.Int64
		}
		paletteRef, paletteTitle, paletteColors, parseErr := parsePaletteSummary(paletteID, paletteName, paletteColorsJSON)
		if parseErr != nil {
			return nil, 0, wrapDBError(parseErr, "解析图片配色失败")
		}
		item.PaletteID = paletteRef
		item.PaletteName = paletteTitle
		item.PaletteColors = paletteColors
		item.Tags = []model.Tag{}
		figures = append(figures, item)
		figureIDs = append(figureIDs, item.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, wrapDBError(err, "查询图片列表失败")
	}

	tagsByFigure, err := r.tag.LoadTagsByFigureIDs(figureIDs)
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

// UpdateFigure 更新图片信息
func (r *FigureRepository) UpdateFigure(id int64, input FigureUpdateInput) (*model.Paper, error) {
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
		SET caption = ?, notes_text = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, input.Caption, input.NotesText, id)
	if err != nil {
		return nil, wrapDBError(err, "更新图片信息失败")
	}
	if err := ensureRowsAffected(result, "figure not found"); err != nil {
		return nil, err
	}

	if err := r.tag.SyncFigureTags(tx, id, input.Tags); err != nil {
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

	return nil, nil // 返回的 Paper 需要在外层查询
}

// UpdateFigureTags 更新图片标签（兼容旧接口）
func (r *FigureRepository) UpdateFigureTags(id int64, tags []TagUpsertInput) (*model.Paper, error) {
	figure, err := r.GetFigure(id)
	if err != nil {
		return nil, err
	}
	if figure == nil {
		return nil, apperr.New(apperr.CodeNotFound, "figure not found")
	}

	return r.UpdateFigure(id, FigureUpdateInput{
		Caption:   figure.Caption,
		NotesText: figure.NotesText,
		Tags:      tags,
	})
}

// DeletePaperFigure 删除图片
func (r *FigureRepository) DeletePaperFigure(id int64) error {
	result, err := r.db.Exec(`DELETE FROM paper_figures WHERE id = ?`, id)
	if err != nil {
		return wrapDBError(err, "删除图片失败")
	}
	return ensureRowsAffected(result, "figure not found")
}

// ApplyManualFigureChanges 应用人工图片修改（添加/删除）
func (r *FigureRepository) ApplyManualFigureChanges(id int64, addFigures []FigureUpsertInput, deleteFigureIDs []int64) error {
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
			`SELECT COUNT(*) FROM paper_figures WHERE paper_id = ? AND id IN (`+strings.Join(placeholders, ",")+")",
			args...,
		).Scan(&count); err != nil {
			return wrapDBError(err, "校验待删除图片失败")
		}
		if count != len(uniqueDeleteIDs) {
			return apperr.New(apperr.CodeNotFound, "待替换或删除的图片不存在")
		}

		if _, err := tx.Exec(
			`DELETE FROM paper_figures WHERE paper_id = ? AND id IN (`+strings.Join(placeholders, ",")+")",
			args...,
		); err != nil {
			return wrapDBError(err, "删除旧图片失败")
		}
	}

	for _, figure := range addFigures {
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
			firstNonEmpty(strings.TrimSpace(figure.Source), "manual"),
			figure.Caption,
			figure.BBoxJSON,
		); err != nil {
			return wrapConflictDBError(err, "图片文件已存在", "保存人工图片失败")
		}
	}

	return wrapDBError(tx.Commit(), "提交人工图片事务失败")
}

// buildFigureWhere 构建图片查询条件
func buildFigureWhere(filter model.FigureFilter) (string, []interface{}) {
	conditions := []string{"pf.parent_figure_id IS NULL"}
	args := []interface{}{}

	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		conditions = append(conditions, `(p.title LIKE ? OR pf.original_name LIKE ? OR pf.caption LIKE ? OR pf.notes_text LIKE ? OR EXISTS (
			SELECT 1
			FROM figure_tags ft
			JOIN tags t ON t.id = ft.tag_id
			WHERE ft.figure_id = pf.id AND t.name LIKE ?
		))`)
		args = append(args, like, like, like, like, like)
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		conditions = append(conditions, "p.group_id = ?")
		args = append(args, *filter.GroupID)
	}
	if filter.TagID != nil && *filter.TagID > 0 {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM figure_tags ft WHERE ft.figure_id = pf.id AND ft.tag_id = ?)")
		args = append(args, *filter.TagID)
	}
	if filter.HasNotes {
		conditions = append(conditions, "TRIM(COALESCE(pf.notes_text, '')) <> ''")
	}

	return " WHERE " + strings.Join(conditions, " AND "), args
}

// buildFigureOrderBy 构建图片排序
func buildFigureOrderBy(filter model.FigureFilter) string {
	if filter.HasNotes {
		return "ORDER BY pf.updated_at DESC, pf.id DESC"
	}
	return "ORDER BY pf.created_at DESC, pf.id DESC"
}

// firstNonEmpty 返回第一个非空字符串
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
