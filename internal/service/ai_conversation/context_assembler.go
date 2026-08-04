package ai_conversation

import (
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
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
// attachmentBlock carries user-attached context (PDF excerpts, checked figure
// summaries) and is inserted right before the final user question.
//
// Sliding-window only — summarization & evidence injection happen in sibling
// files later (Tasks 3.2 / 3.4).
func (s *Service) assembleForTurn(conv repository.AIConversation,
	pinned []repository.AIPinnedPaper, history []repository.AIMessage,
	userText string, attachmentBlock string, settings model.AISettings) (assembledContext, error) {

	pinnedBlock := ""
	if !conv.StrictEvidence {
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
		if len(paperBlocks) > 0 {
			pinnedBlock = "已钉文献：\n\n" + strings.Join(paperBlocks, "\n\n---\n\n") + "\n\n"
		}
	}

	// Sliding-window: keep newest history while estimated total stays within budget.
	budget := settings.ContextBudgetTokens
	if budget <= 0 {
		budget = 32000
	}
	systemPrompt := strings.TrimSpace(settings.SystemPrompt)

	var historyLines []string
	staticBudget := estimateTokens(systemPrompt) + estimateTokens(pinnedBlock) + estimateTokens(attachmentBlock) + estimateTokens(userText) + 200
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
	if attachmentBlock != "" {
		userPrompt += attachmentBlock
	}
	userPrompt += "用户问题：\n" + userText

	return assembledContext{
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
	}, nil
}

// Limits for user-attached context from the AI page PDF panel.
const (
	maxExcerptsPerTurn = 8
	maxExcerptRunes    = 2000
)

// buildExcerptBlock renders user-selected PDF text snippets as a prompt block.
// Empty when no excerpts were attached.
func buildExcerptBlock(excerpts []ai_assistant.ContextExcerpt) string {
	if len(excerpts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("用户引用的原文片段（来自 PDF 预览划选，优先结合这些片段回答）：\n")
	count := 0
	for _, ex := range excerpts {
		text := strings.TrimSpace(ex.Text)
		if text == "" {
			continue
		}
		count++
		if count > maxExcerptsPerTurn {
			break
		}
		if ex.Page > 0 {
			fmt.Fprintf(&b, "[%d] (第 %d 页) \"%s\"\n", count, ex.Page, truncateRunes(text, maxExcerptRunes))
		} else {
			fmt.Fprintf(&b, "[%d] \"%s\"\n", count, truncateRunes(text, maxExcerptRunes))
		}
	}
	if count == 0 {
		return ""
	}
	b.WriteString("\n")
	return b.String()
}

// buildFigureContextBlock renders checked-figure summaries. When
// imagesIncluded is false the master model cannot see images, so the block
// explains the text-only degradation.
func buildFigureContextBlock(summaries []string, imagesIncluded bool) string {
	if len(summaries) == 0 {
		return ""
	}
	var b strings.Builder
	if imagesIncluded {
		fmt.Fprintf(&b, "本轮随附图片（共 %d 张，已作为图片输入提供，与用户问题相关）：\n", len(summaries))
	} else {
		fmt.Fprintf(&b, "本轮用户勾选了 %d 张图片；当前模型不支持图片输入，仅以文字信息代替：\n", len(summaries))
	}
	for _, summary := range summaries {
		b.WriteString(summary)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
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
