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

// ListBasketItems implements BasketRepo.
func (a *RepoAdapter) ListBasketItems() ([]BasketRow, error) {
	rows, err := a.Repo.ListBasketItems()
	if err != nil {
		return nil, err
	}
	out := make([]BasketRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, BasketRow{S2PaperID: r.S2PaperID, Notes: r.Notes, AddedAt: r.AddedAt})
	}
	return out, nil
}

// AddBasketItem implements BasketRepo.
func (a *RepoAdapter) AddBasketItem(id, notes string) error {
	return a.Repo.AddBasketItem(id, notes)
}

// RemoveBasketItem implements BasketRepo.
func (a *RepoAdapter) RemoveBasketItem(id string) error {
	return a.Repo.RemoveBasketItem(id)
}
