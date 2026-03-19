package repository

import (
	"database/sql"
)

// SettingRepository 负责应用设置相关的数据操作
type SettingRepository struct {
	db *sql.DB
}

// NewSettingRepository 创建设置仓库
func NewSettingRepository(db *sql.DB) *SettingRepository {
	return &SettingRepository{db: db}
}

// GetAppSetting 获取应用设置
func (r *SettingRepository) GetAppSetting(key string) (string, error) {
	var value string
	if err := r.db.QueryRow("SELECT value FROM app_settings WHERE key = ?", key).Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", wrapDBError(err, "读取应用设置失败")
	}
	return value, nil
}

// UpsertAppSetting 插入或更新应用设置
func (r *SettingRepository) UpsertAppSetting(key, value string) error {
	_, err := r.db.Exec(`
		INSERT INTO app_settings (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return wrapDBError(err, "保存应用设置失败")
}

// DeleteAppSetting 删除应用设置
func (r *SettingRepository) DeleteAppSetting(key string) error {
	_, err := r.db.Exec("DELETE FROM app_settings WHERE key = ?", key)
	return wrapDBError(err, "删除应用设置失败")
}

// ListAppSettings 列出所有应用设置
func (r *SettingRepository) ListAppSettings() (map[string]string, error) {
	rows, err := r.db.Query("SELECT key, value FROM app_settings")
	if err != nil {
		return nil, wrapDBError(err, "查询应用设置失败")
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, wrapDBError(err, "查询应用设置失败")
		}
		settings[key] = value
	}

	if err := rows.Err(); err != nil {
		return nil, wrapDBError(err, "查询应用设置失败")
	}

	return settings, nil
}
