package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// FigureLookup optionally enriches the /figure N response with the figure's
// caption. When nil, the command falls back to a generic "已选定图片 #N" string.
type FigureLookup interface {
	GetFigure(id int64) (*model.FigureListItem, error)
}

// FigureCommand selects a figure from the surface's recent figure search
// results by 1-based index.
type FigureCommand struct {
	Lookup FigureLookup
}

func (*FigureCommand) Name() string { return "/figure" }

func (c *FigureCommand) Execute(_ context.Context, arg string, rt RuntimeCtx) (*Result, error) {
	n, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil || n <= 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "/figure 后请跟序号，例如 /figure 3")
	}
	var sc SurfaceContext
	if rt.State != nil {
		sc, _ = rt.State.Get(rt.Surface)
	}
	if n > len(sc.RecentSearchFigureIDs) {
		return nil, apperr.New(apperr.CodeFailedPrecondition,
			fmt.Sprintf("最近图片结果只有 %d 项。", len(sc.RecentSearchFigureIDs)))
	}
	fid := sc.RecentSearchFigureIDs[n-1]

	brief := fmt.Sprintf("已选定图片 #%d", fid)
	if c.Lookup != nil {
		if fig, err := c.Lookup.GetFigure(fid); err == nil && fig != nil {
			caption := strings.TrimSpace(fig.Caption)
			if caption == "" {
				caption = strings.TrimSpace(fig.DisplayLabel)
			}
			if caption == "" {
				caption = strings.TrimSpace(fig.Filename)
			}
			if caption != "" {
				brief = fmt.Sprintf("已选定图片 [#%d] %s", fid, caption)
			}
		}
	}
	if rt.State != nil {
		if err := rt.State.SetCurrentFigure(rt.Surface, fid); err != nil {
			return nil, err
		}
	}
	return &Result{UsedShortcut: true, ChunksText: []string{brief}}, nil
}
