package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// SearchCommand runs a direct keyword search against the library and pins the
// matching paper IDs as the surface's recent search results so the user can
// follow up with /paper N.
type SearchCommand struct {
	Lister PaperListSource
	Limit  int // default 5
}

func (*SearchCommand) Name() string { return "/search" }

func (c *SearchCommand) Execute(_ context.Context, arg string, rt RuntimeCtx) (*Result, error) {
	q := strings.TrimSpace(arg)
	if q == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "/search 后请跟检索词")
	}
	limit := c.Limit
	if limit <= 0 {
		limit = 5
	}
	resp, err := c.Lister.ListPapers(model.PaperFilter{
		Page:     1,
		PageSize: limit,
		Keyword:  q,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Papers) == 0 {
		return &Result{UsedShortcut: true, ChunksText: []string{"未找到匹配的文献。"}}, nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("找到 %d 条结果：\n", len(resp.Papers)))
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
