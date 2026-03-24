package repository

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository/schema"

	_ "modernc.org/sqlite" // 注册 sqlite 驱动
)

// LibraryRepository 是图书馆仓库的门面，组合了各领域的子仓库
type LibraryRepository struct {
	db *sql.DB

	// 子仓库
	Paper   *PaperRepository
	Figure  *FigureRepository
	Palette *PaletteRepository
	Group   *GroupRepository
	Tag     *TagRepository
	Setting *SettingRepository
}

// NewLibraryRepository 创建图书馆仓库
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

	// 初始化 Schema
	schemaManager := schema.NewManager(db)
	if err := schemaManager.Initialize(); err != nil {
		db.Close()
		var appErr *apperr.Error
		if errors.As(err, &appErr) {
			return nil, err
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "初始化数据库结构失败", err)
	}

	// 创建子仓库（按依赖顺序）
	tagRepo := NewTagRepository(db)
	groupRepo := NewGroupRepository(db)
	paperRepo := NewPaperRepository(db, tagRepo, groupRepo)
	figureRepo := NewFigureRepository(db, tagRepo)
	paletteRepo := NewPaletteRepository(db)
	settingRepo := NewSettingRepository(db)

	repo := &LibraryRepository{
		db:      db,
		Paper:   paperRepo,
		Figure:  figureRepo,
		Palette: paletteRepo,
		Group:   groupRepo,
		Tag:     tagRepo,
		Setting: settingRepo,
	}

	return repo, nil
}

// Close 关闭数据库连接
func (r *LibraryRepository) Close() error {
	return r.db.Close()
}

// ========== 兼容旧接口的委托方法 ==========
// 以下方法为了保持向后兼容，委托给相应的子仓库

// CreatePaper 创建文献（委托给 Paper 仓库）
func (r *LibraryRepository) CreatePaper(input PaperUpsertInput) (*model.Paper, error) {
	return r.Paper.CreatePaper(input)
}

// UpdatePaper 更新文献（委托给 Paper 仓库）
func (r *LibraryRepository) UpdatePaper(id int64, input PaperUpdateInput) (*model.Paper, error) {
	return r.Paper.UpdatePaper(id, input)
}

// DeletePaper 删除文献（委托给 Paper 仓库）
func (r *LibraryRepository) DeletePaper(id int64) error {
	return r.Paper.DeletePaper(id)
}

// GetPaperDetail 获取文献详情（委托给 Paper 仓库）
func (r *LibraryRepository) GetPaperDetail(id int64) (*model.Paper, error) {
	return r.Paper.GetPaperDetail(id)
}

// ListPapers 查询文献列表（委托给 Paper 仓库）
func (r *LibraryRepository) ListPapers(filter model.PaperFilter) ([]model.Paper, int, error) {
	return r.Paper.ListPapers(filter)
}

// ListPapersByExtractionStatuses 根据解析状态查询文献（委托给 Paper 仓库）
func (r *LibraryRepository) ListPapersByExtractionStatuses(statuses []string) ([]model.Paper, error) {
	return r.Paper.ListPapersByExtractionStatuses(statuses)
}

// FindPaperByPDFSHA256 根据 PDF SHA256 查找文献（委托给 Paper 仓库）
func (r *LibraryRepository) FindPaperByPDFSHA256(pdfSHA256 string) (*model.Paper, error) {
	return r.Paper.FindPaperByPDFSHA256(pdfSHA256)
}

// ListPapersMissingPDFSHA256 查询缺少 PDF SHA256 的文献（委托给 Paper 仓库）
func (r *LibraryRepository) ListPapersMissingPDFSHA256() ([]PaperChecksumBackfillItem, error) {
	return r.Paper.ListPapersMissingPDFSHA256()
}

// UpdatePaperPDFSHA256 更新文献 PDF SHA256（委托给 Paper 仓库）
func (r *LibraryRepository) UpdatePaperPDFSHA256(id int64, pdfSHA256 string) error {
	return r.Paper.UpdatePaperPDFSHA256(id, pdfSHA256)
}

// UpdatePaperExtractionState 更新文献解析状态（委托给 Paper 仓库）
func (r *LibraryRepository) UpdatePaperExtractionState(id int64, status, message, jobID string) error {
	return r.Paper.UpdatePaperExtractionState(id, status, message, jobID)
}

// ApplyPaperExtractionResult 应用文献解析结果（委托给 Paper 仓库）
func (r *LibraryRepository) ApplyPaperExtractionResult(
	id int64,
	pdfText string,
	boxesJSON string,
	status string,
	message string,
	jobID string,
	figures []FigureUpsertInput,
) error {
	return r.Paper.ApplyPaperExtractionResult(id, pdfText, boxesJSON, status, message, jobID, figures)
}

// ApplyManualFigureChanges 应用人工图片修改（委托给 Figure 仓库）
func (r *LibraryRepository) ApplyManualFigureChanges(id int64, addFigures []FigureUpsertInput, deleteFigureIDs []int64) error {
	return r.Figure.ApplyManualFigureChanges(id, addFigures, deleteFigureIDs)
}

// AddPaperFigures 添加文献图片（委托给 Figure 仓库）
func (r *LibraryRepository) AddPaperFigures(id int64, figures []FigureUpsertInput) error {
	return r.Figure.ApplyManualFigureChanges(id, figures, nil)
}

// PurgeLibrary 清空文献库（委托给 Paper 仓库）
func (r *LibraryRepository) PurgeLibrary() error {
	return r.Paper.PurgeLibrary()
}

// GetFigure 获取图片（委托给 Figure 仓库）
func (r *LibraryRepository) GetFigure(id int64) (*model.FigureListItem, error) {
	return r.Figure.GetFigure(id)
}

// ListFigures 查询图片列表（委托给 Figure 仓库）
func (r *LibraryRepository) ListFigures(filter model.FigureFilter) ([]model.FigureListItem, int, error) {
	return r.Figure.ListFigures(filter)
}

// UpdateFigure 更新图片（委托给 Figure 仓库）
func (r *LibraryRepository) UpdateFigure(id int64, input FigureUpdateInput) (*model.Paper, error) {
	// 先执行更新
	if _, err := r.Figure.UpdateFigure(id, input); err != nil {
		return nil, err
	}
	// 获取关联的 Paper ID 并返回详情
	figure, err := r.Figure.GetFigure(id)
	if err != nil {
		return nil, err
	}
	if figure == nil {
		return nil, apperr.New(apperr.CodeNotFound, "figure not found")
	}
	return r.Paper.GetPaperDetail(figure.PaperID)
}

// UpdateFigureTags 更新图片标签（委托给 Figure 仓库）
func (r *LibraryRepository) UpdateFigureTags(id int64, tags []TagUpsertInput) (*model.Paper, error) {
	if _, err := r.Figure.UpdateFigureTags(id, tags); err != nil {
		return nil, err
	}
	figure, err := r.Figure.GetFigure(id)
	if err != nil {
		return nil, err
	}
	if figure == nil {
		return nil, apperr.New(apperr.CodeNotFound, "figure not found")
	}
	return r.Paper.GetPaperDetail(figure.PaperID)
}

// DeletePaperFigure 删除图片（委托给 Figure 仓库）
func (r *LibraryRepository) DeletePaperFigure(id int64) error {
	return r.Figure.DeletePaperFigure(id)
}

func (r *LibraryRepository) UpsertPalette(input PaletteUpsertInput) (*model.Palette, error) {
	return r.Palette.UpsertPalette(input)
}

func (r *LibraryRepository) GetPalette(id int64) (*model.Palette, error) {
	return r.Palette.GetPalette(id)
}

func (r *LibraryRepository) GetPaletteByFigureID(figureID int64) (*model.Palette, error) {
	return r.Palette.GetPaletteByFigureID(figureID)
}

func (r *LibraryRepository) ListPalettes(filter model.PaletteFilter) ([]model.Palette, int, error) {
	return r.Palette.ListPalettes(filter)
}

func (r *LibraryRepository) DeletePalette(id int64) error {
	return r.Palette.DeletePalette(id)
}

// ListGroups 查询分组列表（委托给 Group 仓库）
func (r *LibraryRepository) ListGroups() ([]model.Group, error) {
	return r.Group.ListGroups()
}

// CreateGroup 创建分组（委托给 Group 仓库）
func (r *LibraryRepository) CreateGroup(name, description string) (*model.Group, error) {
	return r.Group.CreateGroup(name, description)
}

// UpdateGroup 更新分组（委托给 Group 仓库）
func (r *LibraryRepository) UpdateGroup(id int64, name, description string) (*model.Group, error) {
	return r.Group.UpdateGroup(id, name, description)
}

// DeleteGroup 删除分组（委托给 Group 仓库）
func (r *LibraryRepository) DeleteGroup(id int64) error {
	return r.Group.DeleteGroup(id)
}

// GroupExists 检查分组是否存在（委托给 Group 仓库）
func (r *LibraryRepository) GroupExists(id int64) (bool, error) {
	return r.Group.GroupExists(id)
}

// ListTags 查询标签列表（委托给 Tag 仓库）
func (r *LibraryRepository) ListTags(scope model.TagScope) ([]model.Tag, error) {
	return r.Tag.ListTags(scope)
}

// CreateTag 创建标签（委托给 Tag 仓库）
func (r *LibraryRepository) CreateTag(scope model.TagScope, name, color string) (*model.Tag, error) {
	return r.Tag.CreateTag(scope, name, color)
}

// UpdateTag 更新标签（委托给 Tag 仓库）
func (r *LibraryRepository) UpdateTag(id int64, name, color string) (*model.Tag, error) {
	return r.Tag.UpdateTag(id, name, color)
}

// DeleteTag 删除标签（委托给 Tag 仓库）
func (r *LibraryRepository) DeleteTag(id int64) error {
	return r.Tag.DeleteTag(id)
}

// syncPaperTags 同步文献标签（委托给 Tag 仓库）
func (r *LibraryRepository) syncPaperTags(tx *sql.Tx, paperID int64, tags []TagUpsertInput) error {
	return r.Tag.SyncPaperTags(tx, paperID, tags)
}

// syncFigureTags 同步图片标签（委托给 Tag 仓库）
func (r *LibraryRepository) syncFigureTags(tx *sql.Tx, figureID int64, tags []TagUpsertInput) error {
	return r.Tag.SyncFigureTags(tx, figureID, tags)
}

// loadTagsByPaperIDs 批量加载文献标签（委托给 Tag 仓库）
func (r *LibraryRepository) loadTagsByPaperIDs(paperIDs []int64) (map[int64][]model.Tag, error) {
	return r.Tag.LoadTagsByPaperIDs(paperIDs)
}

// loadTagsByFigureIDs 批量加载图片标签（委托给 Tag 仓库）
func (r *LibraryRepository) loadTagsByFigureIDs(figureIDs []int64) (map[int64][]model.Tag, error) {
	return r.Tag.LoadTagsByFigureIDs(figureIDs)
}

// GetAppSetting 获取应用设置（委托给 Setting 仓库）
func (r *LibraryRepository) GetAppSetting(key string) (string, error) {
	return r.Setting.GetAppSetting(key)
}

// UpsertAppSetting 插入或更新应用设置（委托给 Setting 仓库）
func (r *LibraryRepository) UpsertAppSetting(key, value string) error {
	return r.Setting.UpsertAppSetting(key, value)
}

// DeleteAppSetting 删除应用设置（委托给 Setting 仓库）
func (r *LibraryRepository) DeleteAppSetting(key string) error {
	return r.Setting.DeleteAppSetting(key)
}

// 辅助函数（从原文件保留）

func wrapDBError(err error, message string) error {
	if err == nil {
		return nil
	}
	return apperr.Wrap(apperr.CodeInternal, message, err)
}

func wrapConflictDBError(err error, conflictMessage, fallbackMessage string) error {
	if err == nil {
		return nil
	}
	var sqliteErr interface{ Error() string }
	if errors.As(err, &sqliteErr) {
		errStr := sqliteErr.Error()
		if containsAny(errStr, "UNIQUE", "unique") {
			return apperr.New(apperr.CodeConflict, conflictMessage)
		}
	}
	return apperr.Wrap(apperr.CodeInternal, fallbackMessage, err)
}

func ensureRowsAffected(result sql.Result, notFoundMessage string) error {
	n, err := result.RowsAffected()
	if err != nil {
		return wrapDBError(err, "检查操作结果失败")
	}
	if n == 0 {
		return apperr.New(apperr.CodeNotFound, notFoundMessage)
	}
	return nil
}

func notFoundError(message string) error {
	return apperr.New(apperr.CodeNotFound, message)
}

// ftsEscapeKeyword escapes special FTS5 characters and wraps the keyword
// so it is treated as a literal substring match (prefix token).
func ftsEscapeKeyword(keyword string) string {
	// Quote the keyword to escape special FTS5 operators (* : ^ etc.)
	escaped := strings.ReplaceAll(keyword, `"`, `""`)
	return `"` + escaped + `"`
}

func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if contains(s, substr) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
