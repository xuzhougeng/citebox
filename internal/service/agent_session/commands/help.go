package commands

import "context"

// HelpCommand renders the list of available commands. Both surfaces share the
// same text so users see the same vocabulary regardless of where they are.
type HelpCommand struct{}

func (*HelpCommand) Name() string { return "/help" }

func (*HelpCommand) Execute(_ context.Context, _ string, _ RuntimeCtx) (*Result, error) {
	return &Result{UsedShortcut: true, ChunksText: []string{HelpText()}}, nil
}

// HelpText is exported so /status and other surfaces can quote it.
func HelpText() string {
	return `可用命令：
状态：/clear /reset /help /status /note <内容>
查询：/recent /figures /random /paper N /figure N /search <关键词>
追问：/ask <问题>（针对当前文献） /interpret（解读当前图片）
直接发自然语言或 DOI 链接也可以。`
}
