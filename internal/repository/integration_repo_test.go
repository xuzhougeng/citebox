package repository

import (
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
)

func TestIntegrationSchemaMigration(t *testing.T) {
	repo := newTestRepository(t)

	var tableCount int
	if err := repo.DB().QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'table' AND name = 'integration_tokens'
	`).Scan(&tableCount); err != nil {
		t.Fatalf("query integration_tokens table error = %v", err)
	}
	if tableCount != 1 {
		t.Fatalf("integration_tokens table count = %d, want 1", tableCount)
	}

	var indexCount int
	if err := repo.DB().QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'index' AND name = 'idx_integration_tokens_token_hash'
	`).Scan(&indexCount); err != nil {
		t.Fatalf("query integration_tokens index error = %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("idx_integration_tokens_token_hash count = %d, want 1", indexCount)
	}

	rows, err := repo.DB().Query("PRAGMA table_info(papers)")
	if err != nil {
		t.Fatalf("PRAGMA table_info(papers) error = %v", err)
	}
	defer rows.Close()

	hasPDFPageTexts := false
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal interface{}
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			t.Fatalf("scan table_info error = %v", err)
		}
		if name == "pdf_page_texts" {
			hasPDFPageTexts = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info error = %v", err)
	}
	if !hasPDFPageTexts {
		t.Fatal("papers.pdf_page_texts column missing after Initialize")
	}
}

func TestIntegrationTokenLifecycle(t *testing.T) {
	repo := newTestRepository(t)

	id, err := repo.Integration.Create("cli-client", "hash-1", []string{"papers:read", "figures:read"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if id <= 0 {
		t.Fatalf("Create() id = %d, want > 0", id)
	}

	token, err := repo.Integration.GetActiveByHash("hash-1")
	if err != nil {
		t.Fatalf("GetActiveByHash() error = %v", err)
	}
	if token.ID != id || token.Name != "cli-client" || token.TokenHash != "hash-1" {
		t.Fatalf("GetActiveByHash() token = %+v, want id %d name cli-client", token, id)
	}
	if len(token.Scopes) != 2 || token.Scopes[0] != "papers:read" || token.Scopes[1] != "figures:read" {
		t.Fatalf("GetActiveByHash() scopes = %v, want [papers:read figures:read]", token.Scopes)
	}
	if token.LastUsedAt != nil {
		t.Fatalf("GetActiveByHash() last_used_at = %v, want nil", token.LastUsedAt)
	}
	if token.RevokedAt != nil {
		t.Fatalf("GetActiveByHash() revoked_at = %v, want nil", token.RevokedAt)
	}

	if _, err := repo.Integration.GetActiveByHash("hash-unknown"); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("GetActiveByHash(unknown) code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}

	if err := repo.Integration.TouchLastUsed(id); err != nil {
		t.Fatalf("TouchLastUsed() error = %v", err)
	}
	token, err = repo.Integration.GetActiveByHash("hash-1")
	if err != nil {
		t.Fatalf("GetActiveByHash() after touch error = %v", err)
	}
	if token.LastUsedAt == nil {
		t.Fatal("GetActiveByHash() last_used_at = nil after TouchLastUsed, want set")
	}

	if err := repo.Integration.RevokeAll(); err != nil {
		t.Fatalf("RevokeAll() error = %v", err)
	}
	if _, err := repo.Integration.GetActiveByHash("hash-1"); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("GetActiveByHash() after revoke code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}

	var revokedAt interface{}
	if err := repo.DB().QueryRow("SELECT revoked_at FROM integration_tokens WHERE id = ?", id).Scan(&revokedAt); err != nil {
		t.Fatalf("query revoked_at error = %v", err)
	}
	if revokedAt == nil {
		t.Fatal("revoked_at = nil after RevokeAll, want set")
	}
}

func TestIntegrationTokenGetActive(t *testing.T) {
	repo := newTestRepository(t)

	if _, err := repo.Integration.GetActive(); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("GetActive() empty code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}

	firstID, err := repo.Integration.Create("first", "hash-first", []string{"library:read"})
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	secondID, err := repo.Integration.Create("second", "hash-second", []string{"library:read"})
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}

	// 多个未吊销令牌时返回最近创建的一个
	active, err := repo.Integration.GetActive()
	if err != nil {
		t.Fatalf("GetActive() error = %v", err)
	}
	if active.ID != secondID {
		t.Fatalf("GetActive() id = %d, want %d (most recent)", active.ID, secondID)
	}

	// 最近创建的被吊销后回落到上一个未吊销令牌
	if _, err := repo.DB().Exec("UPDATE integration_tokens SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", secondID); err != nil {
		t.Fatalf("revoke second error = %v", err)
	}
	active, err = repo.Integration.GetActive()
	if err != nil {
		t.Fatalf("GetActive() after revoke error = %v", err)
	}
	if active.ID != firstID {
		t.Fatalf("GetActive() id = %d, want %d after revoking newer token", active.ID, firstID)
	}

	if err := repo.Integration.RevokeAll(); err != nil {
		t.Fatalf("RevokeAll() error = %v", err)
	}
	if _, err := repo.Integration.GetActive(); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("GetActive() after RevokeAll code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
}

func TestIntegrationTokenCreateWithNilScopes(t *testing.T) {
	repo := newTestRepository(t)

	id, err := repo.Integration.Create("empty-scopes", "hash-nil-scopes", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	token, err := repo.Integration.GetActiveByHash("hash-nil-scopes")
	if err != nil {
		t.Fatalf("GetActiveByHash() error = %v", err)
	}
	if token.ID != id {
		t.Fatalf("GetActiveByHash() id = %d, want %d", token.ID, id)
	}
	if token.Scopes == nil || len(token.Scopes) != 0 {
		t.Fatalf("GetActiveByHash() scopes = %v, want empty non-nil slice", token.Scopes)
	}
}
