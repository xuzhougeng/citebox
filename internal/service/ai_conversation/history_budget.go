package ai_conversation

import (
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/repository"
)

func historyTokenCost(history []repository.AIMessage) int {
	if len(history) == 0 {
		return 0
	}
	cost := 20
	for _, m := range history {
		cost += estimateTokens(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}
	return cost
}

// Select complete recent turns. If the latest turn alone exceeds its share,
// keep a disclosed excerpt of each message instead of losing the entire turn.
func budgetedHistoryBlock(history []repository.AIMessage, budget int) (string, int) {
	if len(history) == 0 || budget <= 0 {
		return "", 0
	}
	starts := []int{}
	for i, m := range history {
		if m.Role == "user" {
			starts = append(starts, i)
		}
	}
	if len(starts) == 0 {
		starts = append(starts, 0)
	}
	render := func(start int) string {
		var b strings.Builder
		b.WriteString("近期对话：\n")
		for _, m := range history[start:] {
			fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Content)
		}
		b.WriteString("\n")
		return b.String()
	}
	selected := -1
	for i := len(starts) - 1; i >= 0; i-- {
		if estimateTokens(render(starts[i])) > budget {
			break
		}
		selected = starts[i]
	}
	if selected >= 0 {
		return render(selected), len(history) - selected
	}
	latest := history[starts[len(starts)-1]:]
	header := "近期对话（受预算限制，以下为最近一轮的摘录，非完整历史）：\n"
	remaining := budget - estimateTokens(header) - 10
	if remaining <= 0 {
		return "", 0
	}
	var b strings.Builder
	b.WriteString(header)
	kept := 0
	for i, m := range latest {
		share := remaining / (len(latest) - i)
		runes := []rune(m.Content)
		line := func(n int) string {
			text := string(runes[:n])
			if n < len(runes) {
				text += "… [truncated]"
			}
			return fmt.Sprintf("%s: %s\n", m.Role, text)
		}
		n := fitPinnedLength(len(runes), share, line)
		if n <= 0 {
			continue
		}
		text := line(n)
		b.WriteString(text)
		remaining -= estimateTokens(text)
		kept++
	}
	if kept == 0 {
		return "", 0
	}
	b.WriteString("\n")
	return b.String(), kept
}
