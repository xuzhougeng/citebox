package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
)

const (
	RememberLoginCookieName = "citebox_remember_login"
	RememberLoginTokenTTL   = 365 * 24 * time.Hour
	rememberLoginTokensKey  = "remember_login_tokens"
	rememberLoginTokenBytes = 32
)

type rememberLoginTokenRecord struct {
	Hash          string `json:"hash"`
	CreatedAtUnix int64  `json:"created_at_unix"`
	ExpiresAtUnix int64  `json:"expires_at_unix"`
}

func (s *LibraryService) AdminUsername() string {
	return s.config.AdminUsername
}

func (s *LibraryService) IssueRememberLoginToken() (string, time.Time, error) {
	s.rememberLoginMu.Lock()
	defer s.rememberLoginMu.Unlock()

	records, err := s.loadRememberLoginTokensLocked()
	if err != nil {
		return "", time.Time{}, apperr.Wrap(apperr.CodeInternal, "保存记住登录状态失败", err)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(RememberLoginTokenTTL)
	token, err := generateRememberLoginToken()
	if err != nil {
		return "", time.Time{}, apperr.Wrap(apperr.CodeInternal, "生成记住登录令牌失败", err)
	}

	records = append(records, rememberLoginTokenRecord{
		Hash:          hashRememberLoginToken(token),
		CreatedAtUnix: now.Unix(),
		ExpiresAtUnix: expiresAt.Unix(),
	})

	if err := s.saveRememberLoginTokensLocked(records); err != nil {
		return "", time.Time{}, apperr.Wrap(apperr.CodeInternal, "保存记住登录状态失败", err)
	}

	return token, expiresAt, nil
}

func (s *LibraryService) HasRememberLoginToken(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}

	s.rememberLoginMu.Lock()
	defer s.rememberLoginMu.Unlock()

	records, err := s.loadRememberLoginTokensLocked()
	if err != nil {
		s.logger.Warn("failed to load remember login tokens", "error", err)
		return false
	}

	targetHash := hashRememberLoginToken(token)
	for _, record := range records {
		if record.Hash == targetHash {
			return true
		}
	}

	return false
}

func (s *LibraryService) RevokeRememberLoginToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}

	s.rememberLoginMu.Lock()
	defer s.rememberLoginMu.Unlock()

	records, err := s.loadRememberLoginTokensLocked()
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "取消记住登录状态失败", err)
	}

	targetHash := hashRememberLoginToken(token)
	filtered := make([]rememberLoginTokenRecord, 0, len(records))
	for _, record := range records {
		if record.Hash == targetHash {
			continue
		}
		filtered = append(filtered, record)
	}

	if err := s.saveRememberLoginTokensLocked(filtered); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "取消记住登录状态失败", err)
	}

	return nil
}

func (s *LibraryService) RevokeAllRememberLoginTokens() error {
	s.rememberLoginMu.Lock()
	defer s.rememberLoginMu.Unlock()

	if err := s.repo.DeleteAppSetting(rememberLoginTokensKey); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "清除记住登录状态失败", err)
	}

	return nil
}

func (s *LibraryService) loadRememberLoginTokensLocked() ([]rememberLoginTokenRecord, error) {
	raw, err := s.repo.GetAppSetting(rememberLoginTokensKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	var records []rememberLoginTokenRecord
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, fmt.Errorf("decode remember login tokens: %w", err)
	}

	now := time.Now().UTC().Unix()
	filtered := make([]rememberLoginTokenRecord, 0, len(records))
	pruned := false
	for _, record := range records {
		if strings.TrimSpace(record.Hash) == "" || record.ExpiresAtUnix <= now {
			pruned = true
			continue
		}
		filtered = append(filtered, record)
	}

	if pruned {
		if err := s.saveRememberLoginTokensLocked(filtered); err != nil {
			return nil, err
		}
	}

	return filtered, nil
}

func (s *LibraryService) saveRememberLoginTokensLocked(records []rememberLoginTokenRecord) error {
	if len(records) == 0 {
		return s.repo.DeleteAppSetting(rememberLoginTokensKey)
	}

	payload, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("marshal remember login tokens: %w", err)
	}

	return s.repo.UpsertAppSetting(rememberLoginTokensKey, string(payload))
}

func generateRememberLoginToken() (string, error) {
	buf := make([]byte, rememberLoginTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashRememberLoginToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
