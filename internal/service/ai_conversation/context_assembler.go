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
	papers       []PaperContextCoverage
}

// assembleForTurn returns prompts ready for the LLM call. Pinned papers'
// full text or question-aware passages across the document are included.
// Recent messages are concatenated; oldest are dropped if estimated tokens >
// budget. attachmentBlock carries user-attached context (PDF excerpts, checked
// figure summaries) and is inserted right before the final user question.
//
// Sliding-window only — summarization & evidence injection happen in sibling
// files later (Tasks 3.2 / 3.4).
func (s *Service) assembleForTurn(conv repository.AIConversation,
	pinned []repository.AIPinnedPaper, history []repository.AIMessage,
	userText string, attachmentBlock string, settings model.AISettings) (assembledContext, error) {

	budget := settings.ContextBudgetTokens
	if budget <= 0 {
		budget = 32000
	}
	systemPrompt := strings.TrimSpace(settings.SystemPrompt)
	summaryBlock := ""
	if conv.SummaryText != "" {
		summaryBlock = "对话摘要（更早的内容）：\n" + conv.SummaryText + "\n\n"
	}
	// Reserve mandatory input and framing before adding optional paper text.
	// The same estimate is used below to allocate the remaining history window.
	fixedTokens := estimateTokens(systemPrompt) + estimateTokens(summaryBlock) + estimateTokens(attachmentBlock) + estimateTokens(userText) + 200
	pinnedBudget := budget - fixedTokens
	pinnedBlock := ""
	var coverage []PaperContextCoverage
	if !conv.StrictEvidence {
		var paperBlocks []string
		remaining := pinnedBudget - estimateTokens("已钉文献：\n\n")
		for i, pp := range pinned {
			paper, err := s.papers.GetPaperDetail(pp.PaperID)
			if err != nil {
				s.logger.Warn("ai_conversation: pinned paper missing", "paper_id", pp.PaperID, "error", err)
				continue
			}
			// Share the budget across papers so the first long PDF cannot crowd
			// out every other pinned source. Unused shares remain available.
			block, selected := selectPinnedPaperBlock(*paper, userText, remaining/(len(pinned)-i)-10)
			coverage = append(coverage, selected)
			if block == "" {
				continue
			}
			paperBlocks = append(paperBlocks, block)
			remaining -= estimateTokens(block) + 10
		}
		if len(paperBlocks) > 0 {
			pinnedBlock = "已钉文献：\n\n" + strings.Join(paperBlocks, "\n\n---\n\n") + "\n\n"
		}
	}

	var historyLines []string
	staticBudget := fixedTokens + estimateTokens(pinnedBlock)
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

	userPrompt := pinnedBlock + summaryBlock
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
		papers:       coverage,
	}, nil
}

// PaperContextCoverage describes only text actually included in the provider prompt.
type PaperContextCoverage struct {
	PaperID int64  `json:"paper_id"`
	Title   string `json:"title"`
	ai_assistant.PaperTextSelection
}

func budgetedPinnedPaperBlock(paper model.Paper, budget int) string {
	block, _ := selectPinnedPaperBlock(paper, "", budget)
	return block
}

func selectPinnedPaperBlock(paper model.Paper, query string, budget int) (string, PaperContextCoverage) {
	abstract := []rune(strings.TrimSpace(paper.AbstractText))
	total := len([]rune(paper.PDFText))
	coverage := PaperContextCoverage{PaperID: paper.ID, Title: paper.Title,
		PaperTextSelection: ai_assistant.PaperTextSelection{Total: total}}
	render := func(abstractLength, bodyLength int) (string, ai_assistant.PaperTextSelection) {
		selected := ai_assistant.SelectPaperText(paper.PDFText, query, bodyLength)
		abstractText := string(abstract[:abstractLength])
		if abstractLength < len(abstract) {
			abstractText += "…"
		}
		block := fmt.Sprintf("### %s\nDOI: %s\n摘要: %s\n正文（本轮 %d / 库内 %d 字符）:\n%s",
			paper.Title, paper.DOI, abstractText, selected.Included, total, selected.Text)
		if selected.Included < total {
			block += "\n正文为结合问题与全文位置选取的片段，不是完整全文。未选中的内容仍可能在库内；不能把本轮未见等同于原文不存在。"
		} else if total == 0 {
			block += "\n库内尚无提取正文；摘要不能替代全文。"
		}
		return block, selected
	}
	abstractLength := fitPinnedLength(min(len(abstract), 4000), budget, func(n int) string { block, _ := render(n, 0); return block })
	if abstractLength < 0 {
		return "", coverage
	}
	bodyLength := fitPinnedLength(total, budget, func(n int) string { block, _ := render(abstractLength, n); return block })
	if bodyLength < 0 {
		return "", coverage
	}
	block, selected := render(abstractLength, bodyLength)
	// Sampling locations can change at width thresholds. Verify the final rendered budget.
	for estimateTokens(block) > budget && bodyLength > 0 {
		bodyLength = bodyLength * 9 / 10
		block, selected = render(abstractLength, bodyLength)
	}
	coverage.PaperTextSelection = selected
	return block, coverage
}

// Return -1 when even the metadata and disclosure cannot fit.
func fitPinnedLength(limit, budget int, render func(int) string) int {
	if budget <= 0 {
		return -1
	}
	if estimateTokens(render(limit)) <= budget {
		return limit
	}
	if estimateTokens(render(0)) > budget {
		return -1
	}
	low, high := 0, limit
	for low < high {
		mid := low + (high-low+1)/2
		if estimateTokens(render(mid)) <= budget {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return low
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
		fmt.Fprintf(&b, "本轮用户勾选了 %d 张图片；当前模型未确认支持图片输入，仅以文字信息代替：\n", len(summaries))
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
