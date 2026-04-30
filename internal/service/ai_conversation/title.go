package ai_conversation

import (
	"context"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
)

// NonStreamCaller is the non-streaming counterpart of StreamCaller. Used by
// the title generator and the summarizer (added in Task 3.1).
type NonStreamCaller interface {
	CallProviderGeneric(ctx context.Context, settings model.AISettings,
		systemPrompt, userPrompt string) (string, string, error)
}

const titleSystemPrompt = "你是一个会话标题生成器。给出一个简洁、不带引号、不超过 20 字的中文标题，概括下面这段对话的主题。只返回标题文本，不要任何前后缀。"

// generateTitle calls the LLM once with the first user message + first assistant
// reply. Returns the trimmed title text or "" if the call failed.
func generateTitle(ctx context.Context, caller NonStreamCaller, settings model.AISettings,
	userText, assistantText string) string {

	user := "用户：" + userText + "\n\nAI：" + assistantText
	out, _, err := caller.CallProviderGeneric(ctx, settings, titleSystemPrompt, user)
	if err != nil {
		return ""
	}
	t := strings.TrimSpace(out)
	t = strings.Trim(t, "「」\"'`")
	if len([]rune(t)) > 20 {
		t = string([]rune(t)[:20])
	}
	return t
}
