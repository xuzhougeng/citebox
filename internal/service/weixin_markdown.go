package service

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	weixinMarkdownFencePattern             = regexp.MustCompile(`^\s*([` + "`" + `~]{3,})(.*)$`)
	weixinMarkdownHeadingLinePattern       = regexp.MustCompile(`^\s*(#{1,6})\s+(.*)$`)
	weixinMarkdownBulletLinePattern        = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	weixinMarkdownOrderedLinePattern       = regexp.MustCompile(`^(\s*)(\d+)\.\s+(.*)$`)
	weixinMarkdownBlockquoteLinePattern    = regexp.MustCompile(`^\s*(>+)\s?(.*)$`)
	weixinMarkdownRuleLinePattern          = regexp.MustCompile(`^\s{0,3}(?:(?:\*\s*){3,}|(?:-\s*){3,}|(?:_\s*){3,})\s*$`)
	weixinMarkdownTableSeparatorPattern    = regexp.MustCompile(`^\s*\|?(?:\s*:?-{3,}:?\s*\|)+\s*:?-{3,}:?\s*\|?\s*$`)
	weixinMarkdownCodeSpanPattern          = regexp.MustCompile("`([^`\n]+)`")
	weixinMarkdownImageOnlyPattern         = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]*)\)`)
	weixinMarkdownLinkOnlyPattern          = regexp.MustCompile(`\[(.*?)\]\(([^)]*)\)`)
	weixinMarkdownStrongAsteriskPattern    = regexp.MustCompile(`\*\*([^\n]+?)\*\*`)
	weixinMarkdownStrongUnderlinePattern   = regexp.MustCompile(`__([^\n]+?)__`)
	weixinMarkdownEmphasisAsteriskPattern  = regexp.MustCompile(`\*([^\*\n]+?)\*`)
	weixinMarkdownEmphasisUnderlinePattern = regexp.MustCompile(`_([^_\n]+?)_`)
	weixinMarkdownStrikePattern            = regexp.MustCompile(`~~([^\n]+?)~~`)
	weixinMarkdownEscapePattern            = regexp.MustCompile(`\\([\\` + "`" + `*_{}\[\]()#+\-.!|])`)
	weixinMarkdownBlankLinePattern         = regexp.MustCompile(`\n{3,}`)
)

var weixinMarkdownHeadingLabels = []string{
	"",
	"【一级标题】",
	"【二级标题】",
	"【三级标题】",
	"【四级标题】",
	"【五级标题】",
	"【六级标题】",
}

const weixinMarkdownCodeDivider = "────────────────"

func renderWeixinMarkdown(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSpace(html.UnescapeString(text))
	if text == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	rendered := make([]string, 0, len(lines))

	for index := 0; index < len(lines); index++ {
		line := strings.TrimRight(lines[index], " \t")
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			rendered = append(rendered, "")
			continue
		}

		if marker, info, ok := parseWeixinMarkdownFence(trimmed); ok {
			codeLines := make([]string, 0, 8)
			closed := false
			for index+1 < len(lines) {
				index++
				next := strings.TrimRight(lines[index], "\r")
				if isWeixinMarkdownFenceClose(strings.TrimSpace(next), marker) {
					closed = true
					break
				}
				codeLines = append(codeLines, next)
			}
			if !closed {
				rendered = append(rendered, line)
				rendered = append(rendered, codeLines...)
				break
			}
			rendered = appendBlock(rendered, renderWeixinMarkdownCodeBlock(info, codeLines))
			continue
		}

		if isWeixinMarkdownTableStart(lines, index) {
			tableLines := []string{line}
			for index+1 < len(lines) {
				next := strings.TrimRight(lines[index+1], " \t")
				if strings.TrimSpace(next) == "" || !strings.Contains(next, "|") {
					break
				}
				index++
				tableLines = append(tableLines, next)
			}
			rendered = appendBlock(rendered, strings.Join(tableLines, "\n"))
			continue
		}

		if matches := weixinMarkdownHeadingLinePattern.FindStringSubmatch(line); len(matches) == 3 {
			level := len(matches[1])
			label := weixinMarkdownHeadingLabels[level]
			content := strings.TrimSpace(renderWeixinMarkdownInline(matches[2]))
			if content == "" {
				rendered = appendBlock(rendered, label)
			} else {
				rendered = appendBlock(rendered, label+" "+content)
			}
			continue
		}

		if weixinMarkdownRuleLinePattern.MatchString(line) {
			rendered = appendBlock(rendered, weixinMarkdownCodeDivider)
			continue
		}

		if matches := weixinMarkdownBlockquoteLinePattern.FindStringSubmatch(line); len(matches) == 3 {
			prefix := strings.Repeat("│ ", len(matches[1]))
			content := strings.TrimSpace(renderWeixinMarkdownInline(matches[2]))
			if content == "" {
				rendered = append(rendered, strings.TrimRight(prefix, " "))
			} else {
				rendered = append(rendered, prefix+content)
			}
			continue
		}

		if matches := weixinMarkdownBulletLinePattern.FindStringSubmatch(line); len(matches) == 3 {
			indent := weixinMarkdownListIndent(matches[1])
			rendered = append(rendered, indent+"• "+strings.TrimSpace(renderWeixinMarkdownInline(matches[2])))
			continue
		}

		if matches := weixinMarkdownOrderedLinePattern.FindStringSubmatch(line); len(matches) == 4 {
			indent := weixinMarkdownListIndent(matches[1])
			rendered = append(rendered, indent+matches[2]+". "+strings.TrimSpace(renderWeixinMarkdownInline(matches[3])))
			continue
		}

		rendered = append(rendered, renderWeixinMarkdownInline(line))
	}

	return strings.TrimSpace(normalizeWeixinMarkdownSpacing(strings.Join(rendered, "\n")))
}

func appendBlock(lines []string, block string) []string {
	block = strings.TrimSpace(block)
	if block == "" {
		return lines
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	lines = append(lines, strings.Split(block, "\n")...)
	return lines
}

func parseWeixinMarkdownFence(line string) (string, string, bool) {
	matches := weixinMarkdownFencePattern.FindStringSubmatch(line)
	if len(matches) != 3 {
		return "", "", false
	}
	marker := strings.TrimSpace(matches[1])
	if len(marker) < 3 {
		return "", "", false
	}
	info := strings.TrimSpace(matches[2])
	return marker, info, true
}

func isWeixinMarkdownFenceClose(line, marker string) bool {
	line = strings.TrimSpace(line)
	if len(line) < len(marker) {
		return false
	}
	return strings.HasPrefix(line, marker)
}

func isWeixinMarkdownTableStart(lines []string, index int) bool {
	if index+1 >= len(lines) {
		return false
	}
	header := strings.TrimSpace(lines[index])
	separator := strings.TrimSpace(lines[index+1])
	if header == "" || separator == "" {
		return false
	}
	if !strings.Contains(header, "|") {
		return false
	}
	return weixinMarkdownTableSeparatorPattern.MatchString(separator)
}

func renderWeixinMarkdownCodeBlock(info string, codeLines []string) string {
	info = strings.TrimSpace(info)
	header := "─── 代码块 ───"
	if info != "" {
		header = fmt.Sprintf("─── %s ───", info)
	}
	footerLength := utf8.RuneCountInString(header)
	if footerLength < utf8.RuneCountInString(weixinMarkdownCodeDivider) {
		footerLength = utf8.RuneCountInString(weixinMarkdownCodeDivider)
	}
	footer := strings.Repeat("─", footerLength)

	lines := make([]string, 0, len(codeLines)+2)
	lines = append(lines, header)
	if len(codeLines) == 0 {
		lines = append(lines, "    ")
	} else {
		for _, line := range codeLines {
			lines = append(lines, "    "+line)
		}
	}
	lines = append(lines, footer)
	return strings.Join(lines, "\n")
}

func weixinMarkdownListIndent(raw string) string {
	spaces := len(raw)
	if spaces <= 0 {
		return ""
	}
	return strings.Repeat("  ", spaces/2)
}

func renderWeixinMarkdownInline(text string) string {
	text = html.UnescapeString(text)
	if strings.TrimSpace(text) == "" {
		return ""
	}

	text, placeholders := protectWeixinMarkdownCodeSpans(text)
	text = replaceWeixinMarkdownImages(text)
	text = replaceWeixinMarkdownLinks(text)
	text = weixinMarkdownStrikePattern.ReplaceAllString(text, "$1")
	text = replaceWeixinMarkdownStrong(text)
	text = replaceWeixinMarkdownEmphasis(text)
	text = restoreWeixinMarkdownCodeSpans(text, placeholders)
	text = weixinMarkdownEscapePattern.ReplaceAllString(text, "$1")
	return strings.TrimSpace(text)
}

func protectWeixinMarkdownCodeSpans(text string) (string, []string) {
	placeholders := make([]string, 0, 4)
	protected := weixinMarkdownCodeSpanPattern.ReplaceAllStringFunc(text, func(match string) string {
		submatches := weixinMarkdownCodeSpanPattern.FindStringSubmatch(match)
		if len(submatches) != 2 {
			return match
		}
		token := fmt.Sprintf("\x00WXCODE%d\x00", len(placeholders))
		placeholders = append(placeholders, strings.TrimSpace(submatches[1]))
		return token
	})
	return protected, placeholders
}

func restoreWeixinMarkdownCodeSpans(text string, placeholders []string) string {
	for index, value := range placeholders {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00WXCODE%d\x00", index), value)
	}
	return text
}

func replaceWeixinMarkdownImages(text string) string {
	return weixinMarkdownImageOnlyPattern.ReplaceAllStringFunc(text, func(match string) string {
		submatches := weixinMarkdownImageOnlyPattern.FindStringSubmatch(match)
		if len(submatches) != 3 {
			return match
		}
		alt := strings.TrimSpace(submatches[1])
		target := strings.TrimSpace(submatches[2])
		if alt == "" {
			alt = "图片"
		}
		if target == "" || strings.HasPrefix(target, "figure://") {
			return "【图片】" + alt
		}
		return fmt.Sprintf("【图片】%s (%s)", alt, target)
	})
}

func replaceWeixinMarkdownLinks(text string) string {
	return weixinMarkdownLinkOnlyPattern.ReplaceAllStringFunc(text, func(match string) string {
		submatches := weixinMarkdownLinkOnlyPattern.FindStringSubmatch(match)
		if len(submatches) != 3 {
			return match
		}
		label := strings.TrimSpace(submatches[1])
		target := strings.TrimSpace(submatches[2])
		if label == "" {
			return target
		}
		if target == "" || strings.HasPrefix(target, "figure://") || label == target {
			return label
		}
		return fmt.Sprintf("%s (%s)", label, target)
	})
}

func replaceWeixinMarkdownStrong(text string) string {
	text = weixinMarkdownStrongAsteriskPattern.ReplaceAllStringFunc(text, func(match string) string {
		submatches := weixinMarkdownStrongAsteriskPattern.FindStringSubmatch(match)
		if len(submatches) != 2 {
			return match
		}
		return toUnicodeSansSerifBold(submatches[1])
	})
	return weixinMarkdownStrongUnderlinePattern.ReplaceAllStringFunc(text, func(match string) string {
		submatches := weixinMarkdownStrongUnderlinePattern.FindStringSubmatch(match)
		if len(submatches) != 2 {
			return match
		}
		return toUnicodeSansSerifBold(submatches[1])
	})
}

func replaceWeixinMarkdownEmphasis(text string) string {
	text = weixinMarkdownEmphasisAsteriskPattern.ReplaceAllStringFunc(text, func(match string) string {
		submatches := weixinMarkdownEmphasisAsteriskPattern.FindStringSubmatch(match)
		if len(submatches) != 2 {
			return match
		}
		return toUnicodeSansSerifItalic(submatches[1])
	})
	return weixinMarkdownEmphasisUnderlinePattern.ReplaceAllStringFunc(text, func(match string) string {
		submatches := weixinMarkdownEmphasisUnderlinePattern.FindStringSubmatch(match)
		if len(submatches) != 2 {
			return match
		}
		return toUnicodeSansSerifItalic(submatches[1])
	})
}

func toUnicodeSansSerifBold(text string) string {
	return mapUnicodeRunes(text, func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
			return rune(0x1D5D4 + (r - 'A'))
		case r >= 'a' && r <= 'z':
			return rune(0x1D5EE + (r - 'a'))
		case r >= '0' && r <= '9':
			return rune(0x1D7EC + (r - '0'))
		default:
			return r
		}
	})
}

func toUnicodeSansSerifItalic(text string) string {
	return mapUnicodeRunes(text, func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
			return rune(0x1D608 + (r - 'A'))
		case r >= 'a' && r <= 'z':
			return rune(0x1D622 + (r - 'a'))
		default:
			return r
		}
	})
}

func mapUnicodeRunes(text string, mapper func(rune) rune) string {
	var builder strings.Builder
	builder.Grow(len(text) * 2)
	for _, r := range text {
		builder.WriteRune(mapper(r))
	}
	return builder.String()
}

func normalizeWeixinMarkdownSpacing(text string) string {
	text = weixinMarkdownBlankLinePattern.ReplaceAllString(text, "\n\n")
	return text
}
