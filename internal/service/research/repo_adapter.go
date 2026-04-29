package research

import (
	"errors"
	"time"

	"github.com/xuzhougeng/citebox/internal/repository"
)

// RepoAdapter wraps *repository.ResearchRepository to satisfy CacheRepo.
type RepoAdapter struct {
	Repo *repository.ResearchRepository
}

// PutCache forwards to the underlying repo.
func (a *RepoAdapter) PutCache(key, payload string) error {
	return a.Repo.PutCache(key, payload)
}

// GetCache forwards to the underlying repo, translating the miss sentinel.
func (a *RepoAdapter) GetCache(key string) (string, time.Time, error) {
	payload, fetchedAt, err := a.Repo.GetCache(key)
	if errors.Is(err, repository.ErrCacheMiss) {
		return "", time.Time{}, ErrCacheRepoMiss
	}
	return payload, fetchedAt, err
}
