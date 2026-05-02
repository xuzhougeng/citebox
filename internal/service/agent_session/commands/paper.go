package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// PaperLookup optionally enriches the /paper N response with the paper's
// title. When nil, the command falls back to a generic "已选定文献 #N" string.
type PaperLookup interface {
	GetPaper(id int64) (*model.Paper, error)
}

// PaperCommand selects a paper from the surface's recent paper search results
// by 1-based index.
type PaperCommand struct {
	Lookup PaperLookup
}

func (*PaperCommand) Name() string { return "/paper" }

func (c *PaperCommand) Execute(_ context.Context, arg string, rt RuntimeCtx) (*Result, error) {
	n, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil || n <= 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "/paper 后请跟序号，例如 /paper 2")
	}
	var sc SurfaceContext
	if rt.State != nil {
		sc, _ = rt.State.Get(rt.Surface)
	}
	if n > len(sc.RecentSearchPaperIDs) {
		return nil, apperr.New(apperr.CodeFailedPrecondition,
			fmt.Sprintf("最近文献结果只有 %d 项。", len(sc.RecentSearchPaperIDs)))
	}
	pid := sc.RecentSearchPaperIDs[n-1]

	title := ""
	brief := fmt.Sprintf("已选定文献 #%d", pid)
	if c.Lookup != nil {
		if paper, err := c.Lookup.GetPaper(pid); err == nil && paper != nil {
			title = strings.TrimSpace(paper.Title)
			if title == "" {
				title = strings.TrimSpace(paper.OriginalFilename)
			}
			if title != "" {
				brief = fmt.Sprintf("已选定文献 [#%d] %s", pid, title)
			}
		}
	}
	if rt.State != nil {
		if err := rt.State.SetCurrentPaper(rt.Surface, pid, title); err != nil {
			return nil, err
		}
	}
	return &Result{UsedShortcut: true, ChunksText: []string{brief}}, nil
}
