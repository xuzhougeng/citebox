package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
)

// PaperListSource is the narrow surface /recent and /search need from the
// library service: the ability to run a paginated paper listing with a
// filter. Defined as an interface so tests can fake it without spinning up
// the full LibraryService.
type PaperListSource interface {
	ListPapers(filter model.PaperFilter) (*model.PaperListResponse, error)
}

// RecentCommand renders the most-recently-updated papers and pins them as the
// surface's recent paper search results so /paper N can pick from them.
type RecentCommand struct {
	Lister PaperListSource
	Limit  int // default 5
}

func (*RecentCommand) Name() string { return "/recent" }

func (c *RecentCommand) Execute(_ context.Context, _ string, rt RuntimeCtx) (*Result, error) {
	n := c.Limit
	if n <= 0 {
		n = 5
	}
	resp, err := c.Lister.ListPapers(model.PaperFilter{
		Page:     1,
		PageSize: n,
		SortBy:   "created_at",
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Papers) == 0 {
		return &Result{UsedShortcut: true, ChunksText: []string{"文献库当前为空。"}}, nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("最近 %d 篇文献：\n", len(resp.Papers)))
	ids := make([]int64, 0, len(resp.Papers))
	for i, p := range resp.Papers {
		title := strings.TrimSpace(p.Title)
		if title == "" {
			title = strings.TrimSpace(p.OriginalFilename)
		}
		b.WriteString(fmt.Sprintf("%d. [#%d] %s\n", i+1, p.ID, title))
		ids = append(ids, p.ID)
	}
	if rt.State != nil {
		_ = rt.State.SetSearchResults(rt.Surface, ids, nil)
	}
	b.WriteString("\n用 /paper N 选定一篇。")
	return &Result{UsedShortcut: true, ChunksText: []string{strings.TrimRight(b.String(), "\n")}}, nil
}
