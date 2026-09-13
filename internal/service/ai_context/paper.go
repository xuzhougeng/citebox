package ai_context

import (
	"fmt"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
	"strings"
)

// PaperUsage describes the exact body spans added to this turn.
type PaperUsage struct {
	PaperID           int64  `json:"paper_id"`
	Title             string `json:"title"`
	IncludedBodyRunes int    `json:"included_body_runes"`
	TotalBodyRunes    int    `json:"total_body_runes"`
	ExcerptCount      int    `json:"excerpt_count"`
}

// Abstracts retain a ceiling; body coverage is limited only by the turn budget.
const maxPinnedAbstractRunes = 4000

// PaperBlock retains the abstract first, then includes as much
// body text as the paper's share permits. Truncation disclosure is budgeted too.
func PaperBlock(paper model.Paper, budget int) (string, PaperUsage) {
	abstract := []rune(strings.TrimSpace(paper.AbstractText))
	body := strings.TrimSpace(paper.PDFText)
	totalBody := len([]rune(body))
	usage := PaperUsage{PaperID: paper.ID, Title: paper.Title, TotalBodyRunes: totalBody}
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
	abstractLength := FitLength(abstractLimit, budget, func(n int) string { return render(n, 0) })
	if abstractLength < 0 {
		return "", usage
	}
	bodyLength := FitLength(bodyLimit, budget, func(n int) string { return render(abstractLength, n) })
	for _, excerpt := range ai_assistant.SamplePaperText(body, bodyLength) {
		usage.IncludedBodyRunes += excerpt.EndRune - excerpt.StartRune
		usage.ExcerptCount++
	}
	return render(abstractLength, bodyLength), usage
}

// Return -1 when even the metadata and disclosure cannot fit.
func FitLength(limit, budget int, render func(int) string) int {
	if budget <= 0 {
		return -1
	}
	if EstimateTokens(render(limit)) <= budget {
		return limit
	}
	if EstimateTokens(render(0)) > budget {
		return -1
	}
	low, high := 0, limit
	for low < high {
		mid := low + (high-low+1)/2
		if EstimateTokens(render(mid)) <= budget {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return low
}

func EstimateTokens(s string) int {
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

// BoundText clips optional prompt metadata with an explicit disclosure.
func BoundText(text string, budget int) string {
	if EstimateTokens(text) <= budget {
		return text
	}
	runes := []rune(text)
	render := func(n int) string { return string(runes[:n]) + "… [truncated]" }
	n := FitLength(len(runes), budget, render)
	if n < 0 {
		return ""
	}
	return render(n)
}

// FigureBody reserves evidence near question/caption terms before broad section
// coverage. Notes never substitute for the original paper in this block.
func FigureBody(paper model.Paper, query string, budget int) string {
	original := paper
	original.NotesText = ""
	original.PaperNotesText = ""
	original.AbstractText = ""
	original.Title = ""
	matches := ai_assistant.FindLocalEvidenceMatches(original, ai_assistant.EvidenceSearchTerms(query), 3)
	var evidence strings.Builder
	for _, m := range matches {
		evidence.WriteString(m.Snippet.Text + "\n")
	}
	targeted := BoundText(evidence.String(), budget/3)
	block, _ := PaperBlock(paper, max(0, budget-EstimateTokens(targeted)-40))
	if targeted != "" {
		return "相关正文摘录（非完整全文）：\n" + targeted + "\n" + block
	}
	return block
}
