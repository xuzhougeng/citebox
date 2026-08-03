package model

import "time"

// IntegrationToken 外部集成访问令牌，数据库只保存令牌哈希
type IntegrationToken struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	TokenHash  string     `json:"token_hash"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}
