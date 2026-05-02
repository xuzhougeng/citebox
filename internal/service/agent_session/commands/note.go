package commands

import (
	"context"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
)

// NotesAppender is the narrow surface the /note command needs from the library
// service. Defined as an interface so tests can fake it without spinning up the
// full LibraryService.
type NotesAppender interface {
	AppendPaperNote(paperID int64, text string) error
	AppendFigureNote(figureID int64, text string) error
}

// NoteCommand appends the user's text to the current figure's notes (if a
// figure is selected) or otherwise to the current paper's notes.
type NoteCommand struct {
	Library NotesAppender
}

func (*NoteCommand) Name() string { return "/note" }

func (c *NoteCommand) Execute(_ context.Context, arg string, rt RuntimeCtx) (*Result, error) {
	text := strings.TrimSpace(arg)
	if text == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "/note 后请跟笔记内容")
	}
	var sc SurfaceContext
	if rt.State != nil {
		sc, _ = rt.State.Get(rt.Surface)
	}
	switch {
	case sc.CurrentFigureID > 0:
		if err := c.Library.AppendFigureNote(sc.CurrentFigureID, text); err != nil {
			return nil, err
		}
		return &Result{UsedShortcut: true, ChunksText: []string{"已写入当前图片笔记。"}}, nil
	case sc.CurrentPaperID > 0:
		if err := c.Library.AppendPaperNote(sc.CurrentPaperID, text); err != nil {
			return nil, err
		}
		return &Result{UsedShortcut: true, ChunksText: []string{"已写入当前文献笔记。"}}, nil
	default:
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先选定一篇文献或一张图再 /note。")
	}
}
