package ai_conversation

import (
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

// assembleContext is the prompt-ready output of one turn's prompt build.
type assembledContext struct {
	systemPrompt string
	userPrompt   string
	images       []model.AIImageInput
}

// assembleForTurn returns prompts ready for the LLM call. Pinned papers'
// abstract + first ~6 KB of pdf_text are included as context. Recent messages
// are concatenated; oldest are dropped if estimated tokens > budget.
//
// Sliding-window only — summarization & evidence injection happen in sibling
// files later (Tasks 3.2 / 3.4).
func (s *Service) assembleForTurn(conv repository.AIConversation,
	pinned []repository.AIPinnedPaper, history []repository.AIMessage,
	userText string, settings model.AISettings) (assembledContext, error) {

	var paperBlocks []string
	for _, pp := range pinned {
		paper, err := s.papers.GetPaperDetail(pp.PaperID)
		if err != nil {
			s.logger.Warn("ai_conversation: pinned paper missing", "paper_id", pp.PaperID, "error", err)
			continue
		}
		body := truncateRunes(paper.PDFText, 6000)
		paperBlocks = append(paperBlocks, fmt.Sprintf(
			"### %s\nDOI: %s\n摘要: %s\n正文片段:\n%s",
			paper.Title, paper.DOI,
			truncateRunes(paper.AbstractText, 800),
			body))
	}
	pinnedBlock := ""
	if len(paperBlocks) > 0 {
		pinnedBlock = "已钉文献：\n\n" + strings.Join(paperBlocks, "\n\n---\n\n") + "\n\n"
	}

	// Sliding-window: keep newest history while estimated total stays within budget.
	budget := settings.ContextBudgetTokens
	if budget <= 0 {
		budget = 32000
	}
	systemPrompt := strings.TrimSpace(settings.SystemPrompt)

	var historyLines []string
	staticBudget := estimateTokens(systemPrompt) + estimateTokens(pinnedBlock) + estimateTokens(userText) + 200
	available := budget - staticBudget
	if available < 0 {
		available = 0
	}
	cumulative := 0
	for i := len(history) - 1; i >= 0; i-- {
		m := history[i]
		line := fmt.Sprintf("%s: %s", m.Role, m.Content)
		cost := estimateTokens(line)
		if cumulative+cost > available {
			break
		}
		historyLines = append([]string{line}, historyLines...)
		cumulative += cost
	}

	userPrompt := pinnedBlock
	if conv.SummaryText != "" {
		userPrompt += "对话摘要（更早的内容）：\n" + conv.SummaryText + "\n\n"
	}
	if len(historyLines) > 0 {
		userPrompt += "近期对话：\n" + strings.Join(historyLines, "\n") + "\n\n"
	}
	userPrompt += "用户问题：\n" + userText

	return assembledContext{
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
	}, nil
}

// estimateTokens returns a heuristic token count: ASCII chars/4, CJK chars/2.
func estimateTokens(s string) int {
	cjk, ascii := 0, 0
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			cjk++
		} else {
			ascii++
		}
	}
	return cjk/2 + ascii/4
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
