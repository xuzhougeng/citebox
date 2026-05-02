package daily_figure

import (
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/model"
)

type fakePool struct {
	ids   []int64
	idsErr error
	loads map[int64]*model.FigureListItem
}

func (f *fakePool) ListAllFigureIDs() ([]int64, error) {
	if f.idsErr != nil {
		return nil, f.idsErr
	}
	return f.ids, nil
}
func (f *fakePool) GetFigure(id int64) (*model.FigureListItem, error) {
	if fig := f.loads[id]; fig != nil {
		return fig, nil
	}
	return &model.FigureListItem{ID: id}, nil
}

func TestPickForDateIsDeterministic(t *testing.T) {
	p := New(&fakePool{ids: []int64{1, 2, 3, 4, 5}})
	date := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	a, err := p.PickForDate(date)
	if err != nil {
		t.Fatalf("first PickForDate: %v", err)
	}
	b, err := p.PickForDate(date)
	if err != nil {
		t.Fatalf("second PickForDate: %v", err)
	}
	if a.ID != b.ID {
		t.Fatalf("same date should yield same id, got %d vs %d", a.ID, b.ID)
	}
}

func TestPickForDateChangesAcrossDays(t *testing.T) {
	p := New(&fakePool{ids: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}})
	seen := map[int64]struct{}{}
	for d := 1; d <= 7; d++ {
		fig, err := p.PickForDate(time.Date(2026, 5, d, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("day %d: %v", d, err)
		}
		seen[fig.ID] = struct{}{}
	}
	// With 7 days over a 10-figure pool we expect more than one distinct id.
	if len(seen) < 2 {
		t.Fatalf("expected variation across 7 days, got %d distinct ids", len(seen))
	}
}

func TestPickForDateEmptyPoolReturnsError(t *testing.T) {
	p := New(&fakePool{ids: nil})
	if _, err := p.PickForDate(time.Now()); err == nil {
		t.Fatal("want error for empty pool")
	}
}

func TestDateIndexIsStableForSameDate(t *testing.T) {
	d1 := time.Date(2026, 5, 2, 1, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 5, 2, 23, 59, 59, 0, time.UTC)
	if dateIndex(d1, 7) != dateIndex(d2, 7) {
		t.Fatal("dateIndex should ignore time-of-day")
	}
}
