package repository

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrCacheMiss is returned by GetCache when the key is not present.
var ErrCacheMiss = errors.New("research: cache miss")

// ResearchRepository owns the s2_paper_cache and research_basket_items tables.
type ResearchRepository struct {
	db *sql.DB
}

// NewResearchRepository builds a research repository.
func NewResearchRepository(db *sql.DB) *ResearchRepository {
	return &ResearchRepository{db: db}
}

// BasketItem mirrors a row in research_basket_items.
type BasketItem struct {
	ID        int64
	S2PaperID string
	Notes     string
	AddedAt   time.Time
}

// PutCache inserts or replaces a cache entry. Caller serialises payload as JSON.
func (r *ResearchRepository) PutCache(key, payload string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("research: cache key required")
	}
	_, err := r.db.Exec(`
		INSERT INTO s2_paper_cache (cache_key, payload, fetched_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(cache_key) DO UPDATE SET
			payload = excluded.payload,
			fetched_at = CURRENT_TIMESTAMP
	`, key, payload)
	return err
}

// GetCache returns the payload + fetched_at, or ErrCacheMiss.
func (r *ResearchRepository) GetCache(key string) (string, time.Time, error) {
	row := r.db.QueryRow(`SELECT payload, fetched_at FROM s2_paper_cache WHERE cache_key = ?`, key)
	var payload string
	var fetchedAt time.Time
	if err := row.Scan(&payload, &fetchedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", time.Time{}, ErrCacheMiss
		}
		return "", time.Time{}, err
	}
	return payload, fetchedAt, nil
}

// DeleteExpiredCache prunes cache entries older than maxAge.
func (r *ResearchRepository) DeleteExpiredCache(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	res, err := r.db.Exec(`DELETE FROM s2_paper_cache WHERE fetched_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// AddBasketItem inserts an item; if s2_paper_id already exists, the existing
// row's notes are updated to the supplied value.
func (r *ResearchRepository) AddBasketItem(s2PaperID, notes string) error {
	if strings.TrimSpace(s2PaperID) == "" {
		return errors.New("research: s2_paper_id required")
	}
	_, err := r.db.Exec(`
		INSERT INTO research_basket_items (s2_paper_id, notes, added_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(s2_paper_id) DO UPDATE SET notes = excluded.notes
	`, s2PaperID, notes)
	return err
}

// RemoveBasketItem deletes the item by s2_paper_id; not-found is not an error.
func (r *ResearchRepository) RemoveBasketItem(s2PaperID string) error {
	_, err := r.db.Exec(`DELETE FROM research_basket_items WHERE s2_paper_id = ?`, s2PaperID)
	return err
}

// ListBasketItems returns all basket items, newest first.
func (r *ResearchRepository) ListBasketItems() ([]BasketItem, error) {
	rows, err := r.db.Query(`
		SELECT id, s2_paper_id, notes, added_at
		FROM research_basket_items
		ORDER BY added_at DESC, id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]BasketItem, 0)
	for rows.Next() {
		var it BasketItem
		if err := rows.Scan(&it.ID, &it.S2PaperID, &it.Notes, &it.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
