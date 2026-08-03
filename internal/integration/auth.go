// Package integration 实现面向外部工具（如 Wisp）的 CiteBox 研究上下文集成核心逻辑，
// 与传输层（HTTP MCP 适配器）无关
package integration

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

// 集成令牌权限范围
const (
	ScopeLibraryRead     = "library:read"
	ScopeNotesRead       = "notes:read"
	ScopeAnnotationsRead = "annotations:read"
	ScopeAssetsRead      = "assets:read"
)

// TokenPrefix 是集成令牌明文的前缀，便于识别和泄露扫描
const TokenPrefix = "cbx_"

// ReadScopes 返回集成令牌的全部只读权限范围
func ReadScopes() []string {
	return []string{ScopeLibraryRead, ScopeNotesRead, ScopeAnnotationsRead, ScopeAssetsRead}
}

// HasScope 检查令牌是否拥有指定权限范围
func HasScope(token *model.IntegrationToken, scope string) bool {
	if token == nil {
		return false
	}
	for _, granted := range token.Scopes {
		if granted == scope {
			return true
		}
	}
	return false
}

// GenerateToken 生成新的集成令牌明文及其哈希。数据库只保存哈希
func GenerateToken() (plaintext, hash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", apperr.Wrap(apperr.CodeInternal, "生成集成令牌失败", err)
	}
	plaintext = TokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	return plaintext, HashToken(plaintext), nil
}

// HashToken 计算令牌明文的 SHA-256 哈希（小写十六进制）
func HashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// TokenService 负责集成令牌的认证、轮换和吊销
type TokenService struct {
	repo *repository.IntegrationTokenRepository
}

// NewTokenService 创建集成令牌服务
func NewTokenService(repo *repository.IntegrationTokenRepository) *TokenService {
	return &TokenService{repo: repo}
}

// Authenticate 校验令牌明文，成功时返回令牌记录并尽力更新最近使用时间
func (s *TokenService) Authenticate(plaintext string) (*model.IntegrationToken, error) {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "集成令牌无效")
	}
	hash := HashToken(plaintext)
	token, err := s.repo.GetActiveByHash(hash)
	if err != nil {
		if apperr.IsCode(err, apperr.CodeNotFound) {
			return nil, apperr.New(apperr.CodeUnauthenticated, "集成令牌无效")
		}
		return nil, err
	}
	// 命中哈希后再做常量时间比较，避免时序侧信道
	if subtle.ConstantTimeCompare([]byte(token.TokenHash), []byte(hash)) != 1 {
		return nil, apperr.New(apperr.CodeUnauthenticated, "集成令牌无效")
	}
	// 最近使用时间只用于展示，更新失败不影响认证
	_ = s.repo.TouchLastUsed(token.ID)
	return token, nil
}

// Rotate 吊销所有现有令牌并创建新的默认令牌，返回明文（仅此一次可见）和令牌视图
func (s *TokenService) Rotate() (plaintext string, view *model.IntegrationToken, err error) {
	plaintext, hash, err := GenerateToken()
	if err != nil {
		return "", nil, err
	}
	if err := s.repo.RevokeAll(); err != nil {
		return "", nil, err
	}
	if _, err := s.repo.Create("default", hash, ReadScopes()); err != nil {
		return "", nil, err
	}
	view, err = s.repo.GetActiveByHash(hash)
	if err != nil {
		return "", nil, err
	}
	return plaintext, view, nil
}

// Revoke 吊销所有集成令牌
func (s *TokenService) Revoke() error {
	return s.repo.RevokeAll()
}

// Status 返回当前生效的集成令牌，没有时返回 CodeNotFound 错误
func (s *TokenService) Status() (*model.IntegrationToken, error) {
	return s.repo.GetActive()
}
