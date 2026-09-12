package ai_assistant

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// PaperTextExcerpt is an exact span of the supplied text. Rune offsets use an
// inclusive start and exclusive end; an excerpt is not the complete paper.
type PaperTextExcerpt struct {
	Text      string
	Heading   string
	StartRune int
	EndRune   int
}

const maxPaperTextExcerpts = 8

// SamplePaperText selects bounded, deterministic coverage of a paper, including
// late methods sections and the tail. The combined Text fields never exceed
// runeBudget. Short papers are returned intact; unstructured text is sampled at
// distributed positions. Headings are a heuristic, not a structural PDF parser.
func SamplePaperText(text string, runeBudget int) []PaperTextExcerpt {
	return samplePaperText(text, runeBudget, maxPaperTextExcerpts)
}

type paperTextHeading struct {
	text     string
	start    int
	class    string
	priority int
}

func samplePaperText(text string, runeBudget, limit int) []PaperTextExcerpt {
	if runeBudget <= 0 || limit <= 0 || strings.TrimSpace(text) == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) <= runeBudget {
		return []PaperTextExcerpt{{Text: text, EndRune: len(runes)}}
	}
	if limit > maxPaperTextExcerpts {
		limit = maxPaperTextExcerpts
	}
	// Keep very small budgets useful instead of spending them on many headings.
	if n := max(1, runeBudget/160); limit > n {
		limit = n
	}
	headings := findPaperTextHeadings(text)
	share := runeBudget / limit
	selected := []int{0}
	add := func(start int) {
		if len(selected) >= limit {
			return
		}
		for _, current := range selected {
			if current == start {
				return
			}
		}
		selected = append(selected, start)
	}
	if limit > 1 {
		add(len(runes) - share)
	}
	// Select each scientific section class once before filling gaps, so
	// a paper with many Results subheadings cannot crowd out STAR Methods.
	preferred := append([]paperTextHeading(nil), headings...)
	sort.SliceStable(preferred, func(i, j int) bool {
		if preferred[i].priority != preferred[j].priority {
			return preferred[i].priority > preferred[j].priority
		}
		// Prefer the actual later section to a possible contents-page entry.
		return preferred[i].start > preferred[j].start
	})
	seenClasses := map[string]bool{}
	for _, heading := range preferred {
		if heading.priority == 0 || seenClasses[heading.class] {
			continue
		}
		seenClasses[heading.class] = true
		add(heading.start)
	}
	// Fill remaining slots with the positions furthest from existing samples.
	// This also gives documents without recognizable headings even coverage.
	for len(selected) < limit {
		sort.Ints(selected)
		bestStart, largestGap := -1, 0
		for i, start := range selected {
			end := len(runes) - share
			if i+1 < len(selected) {
				end = selected[i+1]
			}
			if end-start > largestGap {
				bestStart, largestGap = start+(end-start)/2, end-start
			}
		}
		if largestGap < 2 {
			break
		}
		add(bestStart)
	}
	sort.Ints(selected)
	// Partition at the selected starts to avoid duplicate source text. Reuse
	// unused allocations from short sections while preserving the total cap.
	lengths := make([]int, len(selected))
	capacities := make([]int, len(selected))
	for i, start := range selected {
		end := len(runes)
		if i+1 < len(selected) {
			end = selected[i+1]
		}
		capacities[i] = end - start
	}
	remaining := runeBudget
	for remaining > 0 {
		active := 0
		for i, capacity := range capacities {
			if lengths[i] < capacity {
				active++
			}
		}
		if active == 0 {
			break
		}
		allocation := max(1, remaining/active)
		for i, capacity := range capacities {
			n := min(allocation, capacity-lengths[i], remaining)
			lengths[i] += n
			remaining -= n
		}
	}
	excerpts := make([]PaperTextExcerpt, 0, len(selected))
	for i, start := range selected {
		if lengths[i] == 0 {
			continue
		}
		end := start + lengths[i]
		heading := ""
		for _, candidate := range headings {
			if candidate.start > start {
				break
			}
			heading = candidate.text
		}
		excerpts = append(excerpts, PaperTextExcerpt{
			Text: string(runes[start:end]), Heading: heading, StartRune: start, EndRune: end,
		})
	}
	return excerpts
}

func findPaperTextHeadings(text string) []paperTextHeading {
	headings := make([]paperTextHeading, 0)
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimSpace(line)
		markdown := strings.HasPrefix(trimmed, "#")
		label := strings.TrimSpace(strings.Trim(trimmed, "#*"))
		label = paperHeadingNumberRE.ReplaceAllString(label, "")
		if utf8.RuneCountInString(label) <= 120 {
			class, priority := paperHeadingClass(label)
			if label != "" && (markdown || priority > 0) {
				headings = append(headings, paperTextHeading{text: label, start: offset, class: class, priority: priority})
			}
		}
		offset += utf8.RuneCountInString(line)
	}
	return headings
}

func paperHeadingClass(label string) (string, int) {
	label = strings.ToLower(strings.TrimSpace(strings.Trim(label, ":：")))
	label = strings.NewReplacer("★", " ", "⋆", " ", "*", " ", "&", "and").Replace(label)
	label = strings.Join(strings.Fields(label), " ")
	switch label {
	case "star methods", "starmethods":
		return "methods", 8
	case "methods", "materials and methods", "experimental procedures", "method details", "methodology", "experimental model and subject details", "方法", "材料与方法", "研究方法", "实验方法":
		return "methods", 7
	case "quantification and statistical analysis", "statistical analysis", "statistics", "统计分析", "定量和统计分析":
		return "statistics", 6
	case "data and code availability", "data availability", "code availability", "availability of data and materials", "数据与代码可用性", "数据可用性", "代码可用性":
		return "availability", 5
	case "results", "results and discussion", "结果", "研究结果", "结果与讨论":
		return "results", 4
	case "discussion", "讨论":
		return "discussion", 4
	case "conclusion", "conclusions", "concluding remarks", "结论":
		return "conclusion", 3
	case "limitations", "limitations of the study", "limitations of this study", "研究局限", "研究局限性":
		return "limitations", 3
	case "introduction", "background", "引言", "背景":
		return "introduction", 2
	case "abstract", "summary", "摘要":
		return "abstract", 1
	default:
		return "", 0
	}
}

var paperHeadingNumberRE = regexp.MustCompile(`^\d+(?:\.\d+)*[.)]?\s+`)
