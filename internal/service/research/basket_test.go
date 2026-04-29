package research

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeBasketRepo struct {
	items map[string]string
}

func newFakeBasketRepo() *fakeBasketRepo { return &fakeBasketRepo{items: map[string]string{}} }

func (f *fakeBasketRepo) AddBasketItem(id, notes string) error { f.items[id] = notes; return nil }
func (f *fakeBasketRepo) RemoveBasketItem(id string) error     { delete(f.items, id); return nil }
func (f *fakeBasketRepo) ListBasketItems() ([]BasketRow, error) {
	out := make([]BasketRow, 0, len(f.items))
	for k, v := range f.items {
		out = append(out, BasketRow{S2PaperID: k, Notes: v, AddedAt: time.Now()})
	}
	return out, nil
}

type fakePaperLookup struct {
	store map[string]Paper
}

func (f *fakePaperLookup) GetPaper(ctx context.Context, id string) (Paper, error) {
	if p, ok := f.store[id]; ok {
		return p, nil
	}
	return Paper{}, ErrPaperNotFound
}

func TestBasketAddListExport(t *testing.T) {
	repo := newFakeBasketRepo()
	lookup := &fakePaperLookup{store: map[string]Paper{
		"S2:p1": {PaperID: "S2:p1", Title: "First", ExternalIDs: IDs{DOI: "10.1/a"}, Year: 2020, Authors: []Author{{Name: "Alice"}}},
	}}
	b := NewBasket(repo, lookup, nil)

	if err := b.Add(context.Background(), "S2:p1", "interesting"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	items, err := b.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].Title != "First" {
		t.Fatalf("items = %+v", items)
	}

	md, err := b.ExportMarkdown(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !strings.Contains(md, "First") || !strings.Contains(md, "10.1/a") {
		t.Fatalf("markdown = %q", md)
	}
}
