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
	systemPrompt           string
	userPrompt             string
	images                 []model.AIImageInput
	papers                 []PinnedPaperContextUsage
	evidenceText           string
	historyMessages        int
	omittedHistoryMessages int
}

// PinnedPaperContextUsage describes the exact body spans added to this turn.
type PinnedPaperContextUsage struct {
	PaperID           int64  `json:"paper_id"`
	Title             string `json:"title"`
	IncludedBodyRunes int    `json:"included_body_runes"`
	TotalBodyRunes    int    `json:"total_body_runes"`
	ExcerptCount      int    `json:"excerpt_count"`
}

// assembleForTurn returns prompts ready for the LLM call. Pinned papers'
// abstract + bounded samples across pdf_text are included as context.
// Recent messages are concatenated; oldest are dropped if estimated tokens >
// budget. attachmentBlock carries user-attached context (PDF excerpts, checked
// figure summaries) and is inserted right before the final user question.
//
// History uses a sliding window; summary generation remains in the service.
func (s *Service) assembleForTurn(conv repository.AIConversation,
	pinned []repository.AIPinnedPaper, history []repository.AIMessage,
	userText string, attachmentBlock string, settings model.AISettings) (assembledContext, error) {
	return s.assembleWithEvidence(conv, pinned, history, userText, attachmentBlock, "", settings)
}

func (s *Service) assembleWithEvidence(conv repository.AIConversation,
	pinned []repository.AIPinnedPaper, history []repository.AIMessage,
	userText, attachmentBlock, evidenceBlock string, settings model.AISettings) (assembledContext, error) {

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
	reservedHistory := min(historyTokenCost(history), max(0, budget-fixedTokens)/2)
	evidenceBudget := max(0, budget-fixedTokens-reservedHistory)
	if !conv.StrictEvidence && len(pinned) > 0 {
		evidenceBudget /= 2
	}
	evidenceBlock = budgetedEvidenceBlock(evidenceBlock, evidenceBudget)
	fixedTokens += estimateTokens(evidenceBlock)
	pinnedBudget := budget - fixedTokens - reservedHistory
	pinnedBlock := ""
	var paperUsage []PinnedPaperContextUsage
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
		share := remaining/(len(pinned)-i) - 10
		if conv.StrictEvidence {
			share = 0
		}
		block, usage := budgetedPinnedPaperBlock(*paper, share)
		paperUsage = append(paperUsage, usage)
		if block == "" {
			continue
		}
		paperBlocks = append(paperBlocks, block)
		remaining -= estimateTokens(block) + 10
	}
	if len(paperBlocks) > 0 {
		pinnedBlock = "已钉文献：\n\n" + strings.Join(paperBlocks, "\n\n---\n\n") + "\n\n"
	}

	historyBlock, keptMessages := budgetedHistoryBlock(history, max(0, budget-fixedTokens-estimateTokens(pinnedBlock)))

	userPrompt := pinnedBlock + summaryBlock
	userPrompt += historyBlock
	if attachmentBlock != "" {
		userPrompt += attachmentBlock
	}
	userPrompt += evidenceBlock
	userPrompt += "用户问题：\n" + userText

	return assembledContext{
		systemPrompt:    systemPrompt,
		userPrompt:      userPrompt,
		papers:          paperUsage,
		evidenceText:    evidenceBlock,
		historyMessages: keptMessages, omittedHistoryMessages: len(history) - keptMessages,
	}, nil
}

// Reserve tool evidence before optional pinned text and history. Exceptionally
// large tool output is disclosed as sampled rather than silently overrunning
// the text budget or losing all late evidence to a prefix cut.
func budgetedEvidenceBlock(text string, budget int) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if estimateTokens(text) <= budget {
		return text
	}
	render := func(n int) string {
		var b strings.Builder
		b.WriteString("工具结果受本轮预算限制，以下仅为抽样片段；未展示部分不能作为已读证据：\n")
		for _, excerpt := range ai_assistant.SamplePaperText(text, n) {
			b.WriteString(excerpt.Text)
			b.WriteString("\n…\n")
		}
		return b.String()
	}
	n := fitPinnedLength(len([]rune(text)), budget, render)
	if n < 0 {
		return ""
	}
	return render(n)
}

// Abstracts retain a ceiling; body coverage is limited only by the turn budget.
const maxPinnedAbstractRunes = 4000

// budgetedPinnedPaperBlock retains the abstract first, then includes as much
// body text as the paper's share permits. Truncation disclosure is budgeted too.
func budgetedPinnedPaperBlock(paper model.Paper, budget int) (string, PinnedPaperContextUsage) {
	abstract := []rune(strings.TrimSpace(paper.AbstractText))
	body := strings.TrimSpace(paper.PDFText)
	totalBody := len([]rune(body))
	usage := PinnedPaperContextUsage{PaperID: paper.ID, Title: paper.Title, TotalBodyRunes: totalBody}
	abstractLimit := min(len(abstract), maxPinnedAbstractRunes)
	bodyLimit := totalBody
	render := func(abstractLength, bodyLength int) string {
		abstractText := string(abstract[:abstractLength])
		if abstractLength < len(abstract) {
			abstractText += "…"
		}
		excerpts := ai_assistant.SamplePaperText(body, bodyLength)
		var text strings.Builder
		included := 0
		for _, excerpt := range excerpts {
			if bodyLength < totalBody {
				fmt.Fprintf(&text, "[正文字符 %d–%d / %d；%s]\n", excerpt.StartRune+1, excerpt.EndRune, totalBody, excerpt.Heading)
			}
			text.WriteString(excerpt.Text)
			text.WriteString("\n")
			included += excerpt.EndRune - excerpt.StartRune
		}
		block := fmt.Sprintf("### %s\nDOI: %s\n摘要: %s\n正文片段:\n%s",
			paper.Title, paper.DOI, abstractText, text.String())
		if included < totalBody {
			block += fmt.Sprintf("\n（注意：以上为跨章节或分布式抽样，非完整全文；已带入 %d / %d 字符，共 %d 段。未展示的部分仍可能存在于库中，可通过文献检索工具查询；不要据此判断用户未提供全文。）", included, totalBody, len(excerpts))
		} else if totalBody == 0 {
			block += "\n（当前没有可用的已提取正文；摘要和笔记不能代替全文。）"
		}
		return block
	}
	abstractLength := fitPinnedLength(abstractLimit, budget, func(n int) string { return render(n, 0) })
	if abstractLength < 0 {
		return "", usage
	}
	bodyLength := fitPinnedLength(bodyLimit, budget, func(n int) string { return render(abstractLength, n) })
	for _, excerpt := range ai_assistant.SamplePaperText(body, bodyLength) {
		usage.IncludedBodyRunes += excerpt.EndRune - excerpt.StartRune
		usage.ExcerptCount++
	}
	return render(abstractLength, bodyLength), usage
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
