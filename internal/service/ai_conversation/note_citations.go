package ai_conversation

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
)

var noteCitationRE = regexp.MustCompile(`\[(\d+)\]`)

func answerWithCitationSnapshots(answer, raw, language string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return answer, nil
	}
	var citations []ai_assistant.Citation
	if err := json.Unmarshal([]byte(raw), &citations); err != nil {
		return "", fmt.Errorf("invalid saved citation data: %w", err)
	}
	byIndex := map[int]ai_assistant.Citation{}
	for _, c := range citations {
		if c.I > 0 {
			byIndex[c.I] = c
		}
	}
	used := map[int]bool{}
	lines := strings.Split(answer, "\n")
	fence := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			mark := trimmed[:3]
			if fence == "" {
				fence = mark
			} else if mark == fence {
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		// Inline code is preserved; citation syntax inside code is not a source.
		parts := strings.Split(line, "`")
		for j := 0; j < len(parts); j += 2 {
			segment := parts[j]
			var rewritten strings.Builder
			last := 0
			for _, span := range noteCitationRE.FindAllStringIndex(segment, -1) {
				rewritten.WriteString(segment[last:span[0]])
				match := segment[span[0]:span[1]]
				n, _ := strconv.Atoi(match[1 : len(match)-1])
				c, ok := byIndex[n]
				// Existing Markdown links/references and image syntax remain intact.
				linked := span[1] < len(segment) && strings.ContainsRune("([:", rune(segment[span[1]]))
				image := span[0] > 0 && segment[span[0]-1] == '!'
				if ok && !linked && !image {
					used[n] = true
					if href := safeCitationURL(c.SourceURL); href != "" {
						match += "(" + href + ")"
					}
				}
				rewritten.WriteString(match)
				last = span[1]
			}
			rewritten.WriteString(segment[last:])
			parts[j] = rewritten.String()
		}
		lines[i] = strings.Join(parts, "`")
	}
	if len(used) == 0 {
		return answer, nil
	}
	heading, pageLabel, revisionLabel, unknown := "引用证据快照", "页码", "来源版本", "页码未知"
	if language == "en" {
		heading, pageLabel, revisionLabel, unknown = "Citation evidence snapshots", "Page", "Source revision", "Page unknown"
	}
	var b strings.Builder
	b.WriteString(strings.Join(lines, "\n"))
	fmt.Fprintf(&b, "\n\n### %s\n", heading)
	seen := map[int]bool{}
	for _, c := range citations {
		if !used[c.I] || seen[c.I] {
			continue
		}
		seen[c.I] = true
		title := escapeNoteText(c.Title)
		if title == "" {
			title = fmt.Sprintf("#%d", c.PaperID)
		}
		if href := safeCitationURL(c.SourceURL); href != "" {
			title = "[" + title + "](" + href + ")"
		}
		fmt.Fprintf(&b, "\n**[%d]** %s", c.I, title)
		if c.Page != nil {
			fmt.Fprintf(&b, " · %s %d", pageLabel, *c.Page)
		} else {
			fmt.Fprintf(&b, " · %s", unknown)
		}
		if c.Snippet.Section != "" {
			fmt.Fprintf(&b, " · %s", escapeNoteText(c.Snippet.Section))
		}
		b.WriteString("\n\n")
		for _, line := range strings.Split(c.Snippet.Text, "\n") {
			b.WriteString("> " + escapeNoteText(line) + "\n")
		}
		if c.SourceRevision != "" {
			fmt.Fprintf(&b, "\n%s: `%s`\n", revisionLabel, escapeNoteText(c.SourceRevision))
		}
		if c.Snippet.Origin != "" {
			fmt.Fprintf(&b, "\n`%s`", escapeNoteText(c.Snippet.Origin))
		}
		if c.Verdict != "" {
			fmt.Fprintf(&b, " · `%s`", escapeNoteText(c.Verdict))
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), nil
}

func safeCitationURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if u.IsAbs() {
		if u.Scheme != "https" && u.Scheme != "http" {
			return ""
		}
	} else if u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return ""
	}
	return strings.NewReplacer("(", "%28", ")", "%29", "<", "%3C", ">", "%3E").Replace(u.String())
}

func escapeNoteText(text string) string {
	return strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`").Replace(html.EscapeString(text))
}
