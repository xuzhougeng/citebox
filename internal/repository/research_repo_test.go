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
