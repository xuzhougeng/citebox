package repository

import (
	"testing"
	"time"
)

func newTestResearchRepo(t *testing.T) *ResearchRepository {
	t.Helper()
	repo := newTestRepository(t)
	return repo.Research
}

func TestResearchRepoCachePutAndGet(t *testing.T) {
	repo := newTestResearchRepo(t)

	if err := repo.PutCache("paper:DOI:10.1/abc", `{"title":"foo"}`); err != nil {
		t.Fatalf("PutCache error: %v", err)
	}

	payload, fetchedAt, err := repo.GetCache("paper:DOI:10.1/abc")
	if err != nil {
		t.Fatalf("GetCache error: %v", err)
	}
	if payload != `{"title":"foo"}` {
		t.Fatalf("payload = %q, want %q", payload, `{"title":"foo"}`)
	}
	if time.Since(fetchedAt) > time.Minute {
		t.Fatalf("fetchedAt suspiciously old: %v", fetchedAt)
	}
}

func TestResearchRepoCacheMiss(t *testing.T) {
	repo := newTestResearchRepo(t)

	_, _, err := repo.GetCache("paper:DOI:nonexistent")
	if err != ErrCacheMiss {
		t.Fatalf("expected ErrCacheMiss, got %v", err)
	}
}

func TestResearchRepoBasketAddListRemove(t *testing.T) {
	repo := newTestResearchRepo(t)

	if err := repo.AddBasketItem("S2:abc", "my note"); err != nil {
		t.Fatalf("AddBasketItem error: %v", err)
	}
	if err := repo.AddBasketItem("S2:def", ""); err != nil {
		t.Fatalf("AddBasketItem error: %v", err)
	}
	if err := repo.AddBasketItem("S2:abc", "dup note"); err != nil {
		t.Fatalf("AddBasketItem dup error: %v", err)
	}

	items, err := repo.ListBasketItems()
	if err != nil {
		t.Fatalf("ListBasketItems error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}

	if err := repo.RemoveBasketItem("S2:abc"); err != nil {
		t.Fatalf("RemoveBasketItem error: %v", err)
	}
	items, _ = repo.ListBasketItems()
	if len(items) != 1 || items[0].S2PaperID != "S2:def" {
		t.Fatalf("after remove items = %+v", items)
	}
}

func TestResearchRepoDeleteExpiredCache(t *testing.T) {
	repo := newTestResearchRepo(t)

	// Insert two entries.
	if err := repo.PutCache("paper:fresh", `{}`); err != nil {
		t.Fatalf("PutCache fresh: %v", err)
	}
	if err := repo.PutCache("paper:stale", `{}`); err != nil {
		t.Fatalf("PutCache stale: %v", err)
	}

	// Backdate the "stale" row by 2 hours so it falls outside the prune window.
	now := time.Now()
	staleTime := now.Add(-2 * time.Hour)
	if _, err := repo.db.Exec(
		`UPDATE s2_paper_cache SET fetched_at = ? WHERE cache_key = ?`,
		staleTime, "paper:stale",
	); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// Also update the fresh entry to be recent (10 seconds ago) to ensure it's clearly fresh.
	freshTime := now.Add(-10 * time.Second)
	if _, err := repo.db.Exec(
		`UPDATE s2_paper_cache SET fetched_at = ? WHERE cache_key = ?`,
		freshTime, "paper:fresh",
	); err != nil {
		t.Fatalf("update fresh: %v", err)
	}

	deleted, err := repo.DeleteExpiredCache(time.Hour)
	if err != nil {
		t.Fatalf("DeleteExpiredCache: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	// Fresh row still present.
	if _, _, err := repo.GetCache("paper:fresh"); err != nil {
		t.Fatalf("fresh row should be retained, got %v", err)
	}
	// Stale row gone.
	if _, _, err := repo.GetCache("paper:stale"); err != ErrCacheMiss {
		t.Fatalf("stale row should be deleted, got %v", err)
	}
}
