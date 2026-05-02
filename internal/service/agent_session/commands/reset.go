package commands

import "context"

// ResetCommand clears the surface-scoped selection (current paper, figure, last
// search results) without touching the persisted conversation history.
type ResetCommand struct{}

func (*ResetCommand) Name() string { return "/reset" }

func (*ResetCommand) Execute(_ context.Context, _ string, rt RuntimeCtx) (*Result, error) {
	if rt.State != nil {
		if err := rt.State.Reset(rt.Surface); err != nil {
			return nil, err
		}
	}
	return &Result{
		UsedShortcut: true,
		ChunksText:   []string{"已重置当前选择（论文 / 图片 / 上次检索结果）。对话历史保留。"},
	}, nil
}
