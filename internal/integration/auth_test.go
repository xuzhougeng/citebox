package integration

import (
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func newTestTokenService(t *testing.T) (*TokenService, *repository.LibraryRepository) {
	t.Helper()
	repo, err := repository.NewLibraryRepository(t.TempDir() + "/library.db")
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return NewTokenService(repo.Integration), repo
}

func TestGenerateTokenFormat(t *testing.T) {
	plaintext, hash, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if !strings.HasPrefix(plaintext, TokenPrefix) {
		t.Fatalf("GenerateToken() plaintext = %q, want prefix %q", plaintext, TokenPrefix)
	}
	// 32 字节随机数的 RawURLEncoding 编码长度为 43
	if len(plaintext) != len(TokenPrefix)+43 {
		t.Fatalf("GenerateToken() plaintext length = %d, want %d", len(plaintext), len(TokenPrefix)+43)
	}
	if len(hash) != 64 {
		t.Fatalf("GenerateToken() hash length = %d, want 64", len(hash))
	}
	if hash != HashToken(plaintext) {
		t.Fatal("HashToken() 与 GenerateToken() 返回的哈希不一致")
	}
}

func TestTokenServiceAuthenticateRoundTrip(t *testing.T) {
	svc, _ := newTestTokenService(t)

	plaintext, view, err := svc.Rotate()
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	if view == nil || view.Name != "default" {
		t.Fatalf("Rotate() view = %+v, want name default", view)
	}
	if len(view.Scopes) != len(ReadScopes()) {
		t.Fatalf("Rotate() scopes = %v, want %v", view.Scopes, ReadScopes())
	}

	token, err := svc.Authenticate(plaintext)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if token.ID != view.ID {
		t.Fatalf("Authenticate() id = %d, want %d", token.ID, view.ID)
	}

	// 认证后 last_used_at 被尽力更新
	status, err := svc.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.LastUsedAt == nil {
		t.Fatal("Status() last_used_at = nil after Authenticate, want set")
	}
}

func TestTokenServiceWrongTokenFails(t *testing.T) {
	svc, _ := newTestTokenService(t)

	if _, _, err := svc.Rotate(); err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	if _, err := svc.Authenticate("cbx_wrong-token-value"); !apperr.IsCode(err, apperr.CodeUnauthenticated) {
		t.Fatalf("Authenticate(wrong) code = %q, want %q", apperr.CodeOf(err), apperr.CodeUnauthenticated)
	}
	if _, err := svc.Authenticate(""); !apperr.IsCode(err, apperr.CodeUnauthenticated) {
		t.Fatalf("Authenticate(empty) code = %q, want %q", apperr.CodeOf(err), apperr.CodeUnauthenticated)
	}
}

func TestTokenServiceRotateInvalidatesOld(t *testing.T) {
	svc, _ := newTestTokenService(t)

	oldPlaintext, _, err := svc.Rotate()
	if err != nil {
		t.Fatalf("Rotate() #1 error = %v", err)
	}
	newPlaintext, _, err := svc.Rotate()
	if err != nil {
		t.Fatalf("Rotate() #2 error = %v", err)
	}
	if oldPlaintext == newPlaintext {
		t.Fatal("Rotate() 两次生成的令牌明文相同")
	}
	if _, err := svc.Authenticate(oldPlaintext); !apperr.IsCode(err, apperr.CodeUnauthenticated) {
		t.Fatalf("Authenticate(old) code = %q, want %q", apperr.CodeOf(err), apperr.CodeUnauthenticated)
	}
	if _, err := svc.Authenticate(newPlaintext); err != nil {
		t.Fatalf("Authenticate(new) error = %v", err)
	}
}

func TestTokenServiceRevokeBlocksAuthenticate(t *testing.T) {
	svc, _ := newTestTokenService(t)

	plaintext, _, err := svc.Rotate()
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	if err := svc.Revoke(); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if _, err := svc.Authenticate(plaintext); !apperr.IsCode(err, apperr.CodeUnauthenticated) {
		t.Fatalf("Authenticate() after Revoke code = %q, want %q", apperr.CodeOf(err), apperr.CodeUnauthenticated)
	}
	if _, err := svc.Status(); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("Status() after Revoke code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
}

func TestHasScope(t *testing.T) {
	svc, _ := newTestTokenService(t)
	_, view, err := svc.Rotate()
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	for _, scope := range ReadScopes() {
		if !HasScope(view, scope) {
			t.Fatalf("HasScope(%q) = false, want true", scope)
		}
	}
	if HasScope(view, "library:write") {
		t.Fatal("HasScope(library:write) = true, want false")
	}
	if HasScope(nil, ScopeLibraryRead) {
		t.Fatal("HasScope(nil token) = true, want false")
	}
}

func TestSettingsServiceDefaultsAndUpdate(t *testing.T) {
	_, repo := newTestTokenService(t)
	svc := NewService(repo.Setting)

	settings := svc.Get()
	if settings.Enabled || settings.Port != DefaultPort {
		t.Fatalf("Get() = %+v, want defaults {false %d}", settings, DefaultPort)
	}
	if got := svc.BaseURL(); got != "http://127.0.0.1:19831" {
		t.Fatalf("BaseURL() = %q, want http://127.0.0.1:19831", got)
	}

	if err := svc.Update(true, 20001); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	settings = svc.Get()
	if !settings.Enabled || settings.Port != 20001 {
		t.Fatalf("Get() after Update = %+v, want {true 20001}", settings)
	}
	if got := svc.BaseURL(); got != "http://127.0.0.1:20001" {
		t.Fatalf("BaseURL() = %q, want http://127.0.0.1:20001", got)
	}

	if err := svc.Update(true, 0); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("Update(port 0) code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
	if err := svc.Update(true, 65536); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("Update(port 65536) code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}

	// 存储内容损坏时回落到默认设置
	if err := repo.UpsertAppSetting(settingsKey, "{not-json"); err != nil {
		t.Fatalf("UpsertAppSetting() error = %v", err)
	}
	if settings := svc.Get(); settings.Enabled || settings.Port != DefaultPort {
		t.Fatalf("Get() with corrupt value = %+v, want defaults", settings)
	}
}
