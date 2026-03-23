package repository

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
)

type PaletteRepository struct {
	db *sql.DB
}

func NewPaletteRepository(db *sql.DB) *PaletteRepository {
	return &PaletteRepository{db: db}
}

func (r *PaletteRepository) UpsertPalette(input PaletteUpsertInput) (*model.Palette, error) {
	_, err := r.db.Exec(`
		INSERT INTO color_palettes (paper_id, figure_id, name, colors_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(figure_id) DO UPDATE SET
			name = excluded.name,
			colors_json = excluded.colors_json,
			updated_at = CURRENT_TIMESTAMP
	`, input.PaperID, input.FigureID, input.Name, input.ColorsJSON)
	if err != nil {
		return nil, wrapDBError(err, "保存配色失败")
	}

	return r.GetPaletteByFigureID(input.FigureID)
}

func (r *PaletteRepository) GetPalette(id int64) (*model.Palette, error) {
	row := r.db.QueryRow(paletteSelectSQL+` WHERE cp.id = ?`, id)
	return scanPalette(row)
}

func (r *PaletteRepository) GetPaletteByFigureID(figureID int64) (*model.Palette, error) {
	row := r.db.QueryRow(paletteSelectSQL+` WHERE cp.figure_id = ?`, figureID)
	return scanPalette(row)
}

func (r *PaletteRepository) DeletePalette(id int64) error {
	result, err := r.db.Exec(`DELETE FROM color_palettes WHERE id = ?`, id)
	if err != nil {
		return wrapDBError(err, "删除配色失败")
	}
	return ensureRowsAffected(result, "palette not found")
}

func (r *PaletteRepository) ListPalettes(filter model.PaletteFilter) ([]model.Palette, int, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 200 {
		filter.PageSize = 12
	}

	whereClause, args := buildPaletteWhere(filter)

	var total int
	if err := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM color_palettes cp
		JOIN papers p ON p.id = cp.paper_id
		JOIN paper_figures pf ON pf.id = cp.figure_id
		LEFT JOIN groups g ON g.id = p.group_id
	`+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, wrapDBError(err, "查询配色总数失败")
	}

	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(append([]interface{}{}, args...), filter.PageSize, offset)
	rows, err := r.db.Query(paletteSelectSQL+whereClause+` ORDER BY cp.updated_at DESC, cp.id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, wrapDBError(err, "查询配色列表失败")
	}
	defer rows.Close()

	palettes := []model.Palette{}
	for rows.Next() {
		palette, err := scanPaletteRows(rows)
		if err != nil {
			return nil, 0, err
		}
		palettes = append(palettes, *palette)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, wrapDBError(err, "查询配色列表失败")
	}

	return palettes, total, nil
}

const paletteSelectSQL = `
	SELECT
		cp.id, cp.paper_id, cp.figure_id, cp.name, cp.colors_json, cp.created_at, cp.updated_at,
		p.title, p.group_id, COALESCE(g.name, ''),
		pf.filename, pf.page_number, pf.figure_index, pf.parent_figure_id, pf.subfigure_label, pf.caption
	FROM color_palettes cp
	JOIN papers p ON p.id = cp.paper_id
	JOIN paper_figures pf ON pf.id = cp.figure_id
	LEFT JOIN groups g ON g.id = p.group_id
`

func buildPaletteWhere(filter model.PaletteFilter) (string, []interface{}) {
	conditions := []string{}
	args := []interface{}{}

	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		conditions = append(conditions, `(cp.name LIKE ? OR p.title LIKE ? OR pf.caption LIKE ? OR pf.subfigure_label LIKE ?)`)
		args = append(args, like, like, like, like)
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		conditions = append(conditions, "p.group_id = ?")
		args = append(args, *filter.GroupID)
	}

	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

type paletteScanner interface {
	Scan(dest ...interface{}) error
}

func scanPalette(scanner paletteScanner) (*model.Palette, error) {
	palette, err := scanPaletteFromScanner(scanner)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, wrapDBError(err, "查询配色失败")
	}
	return palette, nil
}

func scanPaletteRows(scanner paletteScanner) (*model.Palette, error) {
	palette, err := scanPaletteFromScanner(scanner)
	if err != nil {
		return nil, wrapDBError(err, "查询配色列表失败")
	}
	return palette, nil
}

func scanPaletteFromScanner(scanner paletteScanner) (*model.Palette, error) {
	var palette model.Palette
	var colorsJSON string
	var groupID sql.NullInt64
	var parentFigureID sql.NullInt64
	var groupName string
	if err := scanner.Scan(
		&palette.ID,
		&palette.PaperID,
		&palette.FigureID,
		&palette.Name,
		&colorsJSON,
		&palette.CreatedAt,
		&palette.UpdatedAt,
		&palette.PaperTitle,
		&groupID,
		&groupName,
		&palette.Filename,
		&palette.PageNumber,
		&palette.FigureIndex,
		&parentFigureID,
		&palette.SubfigureLabel,
		&palette.FigureCaption,
	); err != nil {
		return nil, err
	}

	if groupID.Valid {
		palette.GroupID = &groupID.Int64
		palette.GroupName = groupName
	}
	if parentFigureID.Valid {
		palette.ParentFigureID = &parentFigureID.Int64
	}
	if strings.TrimSpace(colorsJSON) == "" {
		palette.Colors = []string{}
		return &palette, nil
	}
	if err := json.Unmarshal([]byte(colorsJSON), &palette.Colors); err != nil {
		return nil, err
	}
	if palette.Colors == nil {
		palette.Colors = []string{}
	}
	return &palette, nil
}
