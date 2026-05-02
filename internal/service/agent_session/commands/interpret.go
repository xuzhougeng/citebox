package commands

import (
	"context"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// FigureOwnerLookup returns just enough about a figure to feed an
// AIReadRequest (paper id + figure id). It mirrors the signature of
// FigureLookup in figure.go (the underlying repository's GetFigure does not
// take a context); kept as a separate interface to keep /interpret
// independently mockable in tests.
type FigureOwnerLookup interface {
	GetFigure(id int64) (*model.FigureListItem, error)
}

// InterpretCommand routes a question (optionally empty) to the constrained
// figure-interpretation AI path against the surface's currently-selected
// figure. It looks up the figure to recover its owning paper id, since
// AIReadRequest needs both.
type InterpretCommand struct {
	AI     PaperQAService
	Lookup FigureOwnerLookup
}

func (*InterpretCommand) Name() string { return "/interpret" }

func (c *InterpretCommand) Execute(ctx context.Context, arg string, rt RuntimeCtx) (*Result, error) {
	if rt.State == nil {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "缺少 surface state")
	}
	sc, _ := rt.State.Get(rt.Surface)
	if sc.CurrentFigureID <= 0 {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先选定一张图再 /interpret。")
	}
	if c.Lookup == nil {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "图片查找服务未配置")
	}
	figure, err := c.Lookup.GetFigure(sc.CurrentFigureID)
	if err != nil {
		return nil, err
	}
	if figure == nil {
		return nil, apperr.New(apperr.CodeNotFound, "图片不存在")
	}
	resp, err := c.AI.ReadPaper(ctx, model.AIReadRequest{
		PaperID:  figure.PaperID,
		FigureID: figure.ID,
		Action:   model.AIActionFigureInterpretation,
		Question: strings.TrimSpace(arg),
	})
	if err != nil {
		return nil, err
	}
	answer := ""
	if resp != nil {
		answer = strings.TrimSpace(resp.Answer)
	}
	if answer == "" {
		answer = "（模型未返回解读）"
	}
	return &Result{ChunksText: []string{answer}}, nil
}
