package ai_conversation

import (
	"fmt"
	"strings"
	"time"
)

// ExportMarkdown returns conversation content as a single Markdown string.
func (s *Service) ExportMarkdown(id int64) (string, string, error) {
	conv, err := s.repo.GetConversation(id)
	if err != nil {
		return "", "", mapRepoErr(err)
	}
	pinned, err := s.repo.ListPinnedPapers(id)
	if err != nil {
		return "", "", err
	}
	msgs, err := s.repo.ListMessages(id, 0, 1000)
	if err != nil {
		return "", "", err
	}

	var b strings.Builder
	title := conv.Title
	if title == "" {
		title = fmt.Sprintf("会话 #%d", id)
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "_创建于 %s_\n\n", conv.CreatedAt.Format(time.RFC3339))
	if len(pinned) > 0 {
		b.WriteString("**已钉文献**:\n")
		for _, p := range pinned {
			fmt.Fprintf(&b, "- %s", p.Title)
			if p.DOI != "" {
				fmt.Fprintf(&b, " · DOI: %s", p.DOI)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	for _, m := range msgs {
		who := "用户"
		if m.Role == "assistant" {
			who = fmt.Sprintf("AI (%s/%s)", m.Provider, m.Model)
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", who, m.Content)
	}
	filename := safeFilename(title) + ".md"
	return b.String(), filename, nil
}

func safeFilename(title string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	out := r.Replace(strings.TrimSpace(title))
	if out == "" {
		out = "conversation"
	}
	if len([]rune(out)) > 60 {
		out = string([]rune(out)[:60])
	}
	return out
}
