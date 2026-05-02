package repository

import (
	"path/filepath"
	"testing"
)

// NewTestAIConversationRepo returns a repository wired to an isolated SQLite
// database with the full schema migrated. It is intended for tests in other
// packages that need a real *AIConversationRepository.
func NewTestAIConversationRepo(t *testing.T) *AIConversationRepository {
	t.Helper()
	libRepo, err := NewLibraryRepository(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("NewLibraryRepository: %v", err)
	}
	t.Cleanup(func() { _ = libRepo.Close() })
	return libRepo.AIConversation
}
