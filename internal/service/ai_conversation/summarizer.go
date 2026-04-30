package ai_conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

const summarizerSystemPrompt = "你是对话摘要器。把下面的对话片段压缩成 ≤300 字的中文摘要，保留关键问题、结论、引用。如果有已有摘要，请把它合并进新摘要中。只返回摘要文本。"

// summarize compresses the oldest half of msgs together with the existing summary.
// Returns the new summary text and the largest message id included.
func summarize(ctx context.Context, caller NonStreamCaller, settings model.AISettings,
	existing string, msgs []repository.AIMessage) (string, int64, error) {
	if len(msgs) == 0 {
		return existing, 0, nil
	}
	half := len(msgs) / 2
	if half == 0 {
		half = 1
	}
	chunk := msgs[:half]
	through := chunk[len(chunk)-1].ID

	var buf strings.Builder
	if existing != "" {
		fmt.Fprintf(&buf, "已有摘要：\n%s\n\n新对话：\n", existing)
	}
	for _, m := range chunk {
		fmt.Fprintf(&buf, "%s: %s\n", m.Role, m.Content)
	}
	out, _, err := caller.CallProviderGeneric(ctx, settings, summarizerSystemPrompt, buf.String())
	if err != nil {
		return "", 0, err
	}
	return strings.TrimSpace(out), through, nil
}
