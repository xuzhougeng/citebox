package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// PaperFiguresSource is the narrow surface /figures needs from the library
// service: the ability to load a paper (with its figures) by ID.
type PaperFiguresSource interface {
	GetPaper(id int64) (*model.Paper, error)
}

// FiguresCommand lists the figures attached to the currently-selected paper
// and pins their IDs as the surface's recent figure search results so /figure
// N can pick from them.
type FiguresCommand struct {
	Library PaperFiguresSource
}

func (*FiguresCommand) Name() string { return "/figures" }

func (c *FiguresCommand) Execute(_ context.Context, _ string, rt RuntimeCtx) (*Result, error) {
	var sc SurfaceContext
	if rt.State != nil {
		sc, _ = rt.State.Get(rt.Surface)
	}
	if sc.CurrentPaperID <= 0 {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先用 /paper N 选定文献。")
	}
	paper, err := c.Library.GetPaper(sc.CurrentPaperID)
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "目标文献不存在。")
	}

	figures := topLevelFigureItems(paper.Figures)
	if len(figures) == 0 {
		return &Result{UsedShortcut: true, ChunksText: []string{"该文献尚无已提取图片。"}}, nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("文献 #%d 共 %d 张图：\n", paper.ID, len(figures)))
	ids := make([]int64, 0, len(figures))
	for i, f := range figures {
		label := strings.TrimSpace(f.DisplayLabel)
		if label == "" {
			label = fmt.Sprintf("图 %d", f.FigureIndex)
		}
		caption := strings.TrimSpace(f.Caption)
		if caption == "" {
			caption = strings.TrimSpace(f.OriginalName)
		}
		line := fmt.Sprintf("%d. [#%d] %s · 第 %d 页", i+1, f.ID, label, f.PageNumber)
		if caption != "" {
			line += " — " + caption
		}
		b.WriteString(line + "\n")
		ids = append(ids, f.ID)
	}
	if rt.State != nil {
		_ = rt.State.SetSearchResults(rt.Surface, nil, ids)
	}
	b.WriteString("\n用 /figure N 选中一张。")
	return &Result{UsedShortcut: true, ChunksText: []string{strings.TrimRight(b.String(), "\n")}}, nil
}

// topLevelFigureItems mirrors service.topLevelFigures but operates on the
// *model.Paper's nested []model.Figure slice and returns only top-level
// (non-subfigure) entries.
func topLevelFigureItems(figures []model.Figure) []model.Figure {
	if len(figures) == 0 {
		return nil
	}
	out := make([]model.Figure, 0, len(figures))
	for _, f := range figures {
		if f.ParentFigureID != nil {
			continue
		}
		out = append(out, f)
	}
	return out
}
