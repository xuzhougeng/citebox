// Package daily_figure provides a date-deterministic "today's figure" picker.
//
// Both the Overview page and the WeChat daily-recommendation ticker may
// independently ask "what figure does today belong to" — they must converge
// on the same answer for the same date. Determinism comes from hashing the
// calendar date and indexing into the ordered set of all eligible figure
// ids; the same date always picks the same figure as long as the library
// hasn't changed. (Adding or removing figures shifts the modulo, so the
// answer can change mid-day if the library mutates.)
package daily_figure

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// FigurePool is the narrow repository contract the Picker depends on.
// *repository.FigureRepository satisfies this via ListAllFigureIDs +
// GetFigure (the latter is method-receiver compatible with the
// (id int64) (*model.FigureListItem, error) shape after a small adapter).
type FigurePool interface {
	ListAllFigureIDs() ([]int64, error)
	LoadFigure(id int64) (*model.FigureListItem, error)
}

// Picker chooses the per-date figure deterministically from a FigurePool.
type Picker struct {
	pool FigurePool
}

// New constructs a Picker.
func New(pool FigurePool) *Picker { return &Picker{pool: pool} }

// PickForDate returns the figure assigned to the given calendar date in the
// pool's local timezone. Returns CodeFailedPrecondition when the library is
// empty.
func (p *Picker) PickForDate(date time.Time) (*model.FigureListItem, error) {
	return p.pickWithCtx(context.Background(), date)
}

func (p *Picker) pickWithCtx(_ context.Context, date time.Time) (*model.FigureListItem, error) {
	ids, err := p.pool.ListAllFigureIDs()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "图片库为空")
	}
	idx := dateIndex(date, len(ids))
	return p.pool.LoadFigure(ids[idx])
}

// dateIndex hashes the calendar date (YYYY-MM-DD) with FNV-1a and reduces it
// modulo n. Exposed for tests.
func dateIndex(date time.Time, n int) int {
	if n <= 0 {
		return 0
	}
	h := fnv.New64a()
	fmt.Fprintf(h, "%04d-%02d-%02d", date.Year(), int(date.Month()), date.Day())
	return int(h.Sum64() % uint64(n))
}
