package commands

import (
	"context"
	"fmt"
)

// RandomFigurePicker picks a random figure for the /random shortcut. The
// concrete implementation lives outside this package (Task 13/14 wires the
// daily-figure picker into it).
type RandomFigurePicker interface {
	PickRandomFigure(ctx context.Context) (id int64, path, caption string, err error)
}

// RandomCommand picks a random figure, sets it as the current selection, and
// returns a brief plus the image path for surfaces that can preview images.
type RandomCommand struct {
	Picker RandomFigurePicker
}

func (*RandomCommand) Name() string { return "/random" }

func (c *RandomCommand) Execute(ctx context.Context, _ string, rt RuntimeCtx) (*Result, error) {
	id, path, caption, err := c.Picker.PickRandomFigure(ctx)
	if err != nil {
		return nil, err
	}
	if rt.State != nil {
		_ = rt.State.SetCurrentFigure(rt.Surface, id)
	}
	msg := caption
	if msg == "" {
		msg = fmt.Sprintf("已挑选图片 #%d", id)
	}
	return &Result{UsedShortcut: true, ChunksText: []string{msg}, ImagePath: path}, nil
}
