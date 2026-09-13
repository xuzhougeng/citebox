package ai_assistant

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
)

type paperPageTextGetter interface{ GetPaperPDFPageTexts(int64) ([]string, error) }

// Resolve locations at retrieval time, never when an old answer is saved.
// A page is assigned only when the complete normalized excerpt matches exactly
// one stored page. Cross-page or ambiguous excerpts keep an unknown page.
func EnrichCitationSources(papers PaperGetter, citations []Citation) {
	cache := map[int64]*model.Paper{}
	pages := map[int64][]string{}
	for i := range citations {
		c := &citations[i]
		if c.Source != "local" || c.PaperID <= 0 || papers == nil {
			if c.SourceURL == "" && c.S2PaperID != "" {
				c.SourceURL = "https://www.semanticscholar.org/paper/" + url.PathEscape(c.S2PaperID)
			}
			continue
		}
		p, ok := cache[c.PaperID]
		if !ok {
			p, _ = papers.GetPaperDetail(c.PaperID)
			cache[c.PaperID] = p
			if getter, ok := papers.(paperPageTextGetter); ok {
				pages[c.PaperID], _ = getter.GetPaperPDFPageTexts(c.PaperID)
			}
		}
		if p == nil {
			continue
		}
		c.Title = p.Title
		text := p.PDFText
		switch c.Snippet.SnippetKind {
		case "title":
			text = p.Title
		case "abstract":
			text = p.AbstractText
		case "notes":
			text = p.NotesText + "\n" + p.PaperNotesText
		}
		c.SourceRevision = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(strings.TrimSpace(text))))
		if c.Snippet.SnippetKind == "body" {
			needle := normalizeEvidenceWhitespace(strings.Trim(strings.TrimSpace(c.Snippet.Text), "."))
			if needle != "" {
				found := 0
				for page, body := range pages[c.PaperID] {
					if strings.Contains(normalizeEvidenceWhitespace(body), needle) {
						if found > 0 {
							found = 0
							break
						}
						found = page + 1
					}
				}
				if found > 0 {
					c.Page = &found
				}
			}
		}
		c.SourceURL = fmt.Sprintf("/library?paper=%d", p.ID)
		if p.StoredPDFName != "" && c.Snippet.SnippetKind == "body" {
			params := url.Values{"kind": {"pdf"}, "src": {"/files/papers/" + url.PathEscape(p.StoredPDFName)}, "paper_id": {fmt.Sprint(p.ID)}}
			if c.Page != nil {
				params.Set("page", fmt.Sprint(*c.Page))
			}
			c.SourceURL = "/viewer?" + params.Encode()
		}
	}
}
