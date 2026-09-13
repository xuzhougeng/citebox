package ai_assistant

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// TextRange uses zero-based, half-open Unicode character offsets into PDFText.
type TextRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type PaperTextSelection struct {
	Total    int         `json:"total_characters"`
	Included int         `json:"included_characters"`
	Ranges   []TextRange `json:"ranges"`
	Text     string      `json:"-"`
}

// Major headings are an extra hint, never a prerequisite for reaching the tail.
// Uniform anchors cover documents whose extraction lost all heading structure.
var majorPaperHeading = regexp.MustCompile(`(?im)^[\t #*0-9.\-]*(STAR\s*METHODS|MATERIALS\s+AND\s+METHODS|METHODS|RESULTS|DISCUSSION|CONCLUSIONS?|INTRODUCTION|DATA\s+AND\s+CODE\s+AVAILABILITY|QUANTIFICATION\s+AND\s+STATISTICAL\s+ANALYSIS|SUPPLEMENTAL\s+INFORMATION|方法|材料与方法|结果|讨论|结论|数据与代码可用性)[\t :：#*\r]*$`)

// SelectPaperText includes the entire stored text when it fits. Otherwise it
// combines question matches, major section starts and positions throughout the
// document. It never presents sampled passages as a complete reading.
func SelectPaperText(text, query string, maxRunes int) PaperTextSelection {
	body := []rune(text)
	selection := PaperTextSelection{Total: len(body)}
	if maxRunes <= 0 || len(body) == 0 {
		return selection
	}
	if maxRunes >= len(body) {
		selection.Ranges = []TextRange{{0, len(body)}}
		selection.Included, selection.Text = len(body), text
		return selection
	}
	count := min(12, max(2, maxRunes/200))
	anchors := []int{0, len(body) - 1}
	add := func(position int) {
		if len(anchors) >= count {
			return
		}
		for _, existing := range anchors {
			if absInt(position-existing) < max(1, maxRunes/count/2) {
				return
			}
		}
		anchors = append(anchors, position)
	}
	// A few precise query hits complement, rather than replace, broad coverage.
	lower := strings.ToLower(text)
	terms := EvidenceSearchTerms(query)
	for _, term := range terms {
		if len(anchors) >= min(count, 5) {
			break
		}
		if index := strings.Index(lower, strings.ToLower(term)); index >= 0 {
			add(utf8.RuneCountInString(lower[:index]))
		}
	}
	headings := majorPaperHeading.FindAllStringIndex(text, -1)
	// Read headings from the end first so long front matter cannot crowd out methods.
	for i := len(headings) - 1; i >= 0 && len(anchors) < count-2; i-- {
		add(utf8.RuneCountInString(text[:headings[i][0]]))
	}
	for i := 1; i < count && len(anchors) < count; i++ {
		add(len(body) * i / count)
	}
	width := maxRunes / len(anchors)
	for i, anchor := range anchors {
		size := width
		if i < maxRunes%len(anchors) {
			size++
		}
		if size == 0 {
			continue
		}
		start := max(0, anchor-size/5)
		end := min(len(body), start+size)
		start = max(0, end-size)
		selection.Ranges = append(selection.Ranges, TextRange{start, end})
	}
	sort.Slice(selection.Ranges, func(i, j int) bool { return selection.Ranges[i].Start < selection.Ranges[j].Start })
	merged := []TextRange{}
	for _, span := range selection.Ranges {
		if len(merged) > 0 && span.Start <= merged[len(merged)-1].End {
			merged[len(merged)-1].End = max(merged[len(merged)-1].End, span.End)
		} else {
			merged = append(merged, span)
		}
	}
	selection.Ranges = merged
	var b strings.Builder
	for _, span := range merged {
		selection.Included += span.End - span.Start
		fmt.Fprintf(&b, "[正文字符 %d–%d / %d]\n%s\n\n", span.Start+1, span.End, len(body), string(body[span.Start:span.End]))
	}
	selection.Text = strings.TrimSpace(b.String())
	return selection
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
