package repository

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// IntegrationTokenRepository 负责外部集成令牌的数据操作
type IntegrationTokenRepository struct {
	db *sql.DB
}

// NewIntegrationTokenRepository 创建外部集成令牌仓库
func NewIntegrationTokenRepository(db *sql.DB) *IntegrationTokenRepository {
	return &IntegrationTokenRepository{db: db}
}

// Create 创建集成令牌，返回令牌 ID
func (r *IntegrationTokenRepository) Create(name, tokenHash string, scopes []string) (int64, error) {
	if scopes == nil {
		scopes = []string{}
	}
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return 0, apperr.Wrap(apperr.CodeInternal, "编码集成令牌权限失败", err)
	}

	result, err := r.db.Exec(`
		INSERT INTO integration_tokens (name, token_hash, scopes)
		VALUES (?, ?, ?)
	`, strings.TrimSpace(name), strings.TrimSpace(tokenHash), string(scopesJSON))
	if err != nil {
		return 0, wrapConflictDBError(err, "集成令牌已存在", "创建集成令牌失败")
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, wrapDBError(err, "读取集成令牌 ID 失败")
	}
	return id, nil
}

// GetActiveByHash 按令牌哈希查询未吊销的集成令牌
func (r *IntegrationTokenRepository) GetActiveByHash(tokenHash string) (*model.IntegrationToken, error) {
	row := r.db.QueryRow(`
		SELECT id, name, token_hash, scopes, created_at, last_used_at, revoked_at
		FROM integration_tokens
		WHERE token_hash = ? AND revoked_at IS NULL
	`, strings.TrimSpace(tokenHash))

	token, err := scanIntegrationToken(row)
	if err == sql.ErrNoRows {
		return nil, notFoundError("integration token not found")
	}
	if err != nil {
		return nil, wrapDBError(err, "查询集成令牌失败")
	}
	return token, nil
}

// GetActive 查询最近创建且未吊销的集成令牌
func (r *IntegrationTokenRepository) GetActive() (*model.IntegrationToken, error) {
	row := r.db.QueryRow(`
		SELECT id, name, token_hash, scopes, created_at, last_used_at, revoked_at
		FROM integration_tokens
		WHERE revoked_at IS NULL
		ORDER BY id DESC
		LIMIT 1
	`)

	token, err := scanIntegrationToken(row)
	if err == sql.ErrNoRows {
		return nil, notFoundError("integration token not found")
	}
	if err != nil {
		return nil, wrapDBError(err, "查询集成令牌失败")
	}
	return token, nil
}

// RevokeAll 吊销所有集成令牌
func (r *IntegrationTokenRepository) RevokeAll() error {
	if _, err := r.db.Exec(`
		UPDATE integration_tokens
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE revoked_at IS NULL
	`); err != nil {
		return wrapDBError(err, "吊销集成令牌失败")
	}
	return nil
}

// TouchLastUsed 更新集成令牌的最近使用时间
func (r *IntegrationTokenRepository) TouchLastUsed(id int64) error {
	result, err := r.db.Exec(`
		UPDATE integration_tokens
		SET last_used_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)
	if err != nil {
		return wrapDBError(err, "更新集成令牌使用时间失败")
	}
	return ensureRowsAffected(result, "integration token not found")
}

func scanIntegrationToken(scanner interface {
	Scan(dest ...interface{}) error
}) (*model.IntegrationToken, error) {
	var token model.IntegrationToken
	var scopesJSON string
	var lastUsedAt sql.NullTime
	var revokedAt sql.NullTime
	if err := scanner.Scan(
		&token.ID,
		&token.Name,
		&token.TokenHash,
		&scopesJSON,
		&token.CreatedAt,
		&lastUsedAt,
		&revokedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(scopesJSON), &token.Scopes); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "解析集成令牌权限失败", err)
	}
	if token.Scopes == nil {
		token.Scopes = []string{}
	}
	if lastUsedAt.Valid {
		token.LastUsedAt = &lastUsedAt.Time
	}
	if revokedAt.Valid {
		token.RevokedAt = &revokedAt.Time
	}
	return &token, nil
}
