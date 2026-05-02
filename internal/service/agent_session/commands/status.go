package commands

import (
	"context"
	"fmt"
)

// StatusCommand renders a snapshot of the current surface state so users can
// confirm what /ask, /interpret, /note, etc. will operate on.
type StatusCommand struct{}

func (*StatusCommand) Name() string { return "/status" }

func (*StatusCommand) Execute(_ context.Context, _ string, rt RuntimeCtx) (*Result, error) {
	var sc SurfaceContext
	if rt.State != nil {
		sc, _ = rt.State.Get(rt.Surface)
	}
	msg := fmt.Sprintf(`当前会话：#%d
当前论文：%d %s
当前图片：%d
最近论文检索结果数：%d
最近图片检索结果数：%d`,
		rt.ConversationID,
		sc.CurrentPaperID, sc.CurrentPaperTitle,
		sc.CurrentFigureID,
		len(sc.RecentSearchPaperIDs),
		len(sc.RecentSearchFigureIDs))
	return &Result{UsedShortcut: true, ChunksText: []string{msg}}, nil
}
