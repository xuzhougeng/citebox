package ai_conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

// ErrNoExternalIDs signals that none of the pinned papers had a DOI/arXiv id we
// could pass to /snippet/search.
var ErrNoExternalIDs = errors.New("ai_conversation: no usable external ids")

// SnippetSearcher is the surface we depend on. Satisfied by *research.Service.
type SnippetSearcher interface {
	SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error)
}

// Citation is one entry in the persisted citations_json array.
type Citation struct {
	I          int              `json:"i"`
	PaperID    int64            `json:"paper_id"`
	ExternalID string           `json:"external_id"`
	S2PaperID  string           `json:"s2_paper_id,omitempty"`
	Snippet    research.Snippet `json:"snippet"`
	Score      float64          `json:"score"`
}

// injectEvidence runs SnippetSearch over the pinned papers' external ids and
// returns (evidence-block prompt fragment, citations array, err).
func injectEvidence(ctx context.Context, searcher SnippetSearcher,
	userText string, pinned []repository.AIPinnedPaper) (string, []Citation, error) {

	idMap := map[string]int64{}
	idList := make([]string, 0, len(pinned))
	for _, p := range pinned {
		ext := externalIDFor(p)
		if ext == "" {
			continue
		}
		idMap[ext] = p.PaperID
		idList = append(idList, ext)
	}
	if len(idList) == 0 {
		return "", nil, ErrNoExternalIDs
	}

	q := userText
	if len([]rune(q)) > 200 {
		q = string([]rune(q)[:200])
	}
	res, err := searcher.SnippetSearch(ctx, q, research.SnippetSearchOpts{
		PaperIDs: idList,
		Limit:    8,
	})
	if err != nil {
		return "", nil, err
	}

	citations := make([]Citation, 0, len(res.Items))
	var b strings.Builder
	b.WriteString("你必须基于以下从已钉文献中检索到的证据片段回答。每个论断后用 [n] 标注引用。如果证据不足以支撑回答，请明确说明\"证据不足\"。\n\n证据：\n")
	for i, m := range res.Items {
		idx := i + 1
		ext := ""
		var paperID int64
		// Prefer DOI from the snippet's paper, fall back to ArXiv. Map back to
		// our local paper id via the idMap (built from pinned papers above).
		if m.Paper.ExternalIDs.DOI != "" {
			ext = "DOI:" + m.Paper.ExternalIDs.DOI
		} else if m.Paper.ExternalIDs.ArXiv != "" {
			ext = "ARXIV:" + m.Paper.ExternalIDs.ArXiv
		}
		if id, ok := idMap[ext]; ok {
			paperID = id
		}
		citations = append(citations, Citation{
			I: idx, PaperID: paperID, ExternalID: ext, S2PaperID: m.PaperID,
			Snippet: m.Snippet, Score: m.Score,
		})
		section := m.Snippet.Section
		if section == "" {
			section = m.Snippet.SnippetKind
		}
		fmt.Fprintf(&b, "[%d] (%s) %s\n", idx, section, m.Snippet.Text)
	}
	b.WriteString("\n用户问题：\n")
	b.WriteString(userText)
	return b.String(), citations, nil
}

func externalIDFor(p repository.AIPinnedPaper) string {
	if p.DOI != "" {
		return "DOI:" + p.DOI
	}
	return ""
}

// MarshalCitations is a tiny convenience wrapper.
func MarshalCitations(c []Citation) string {
	if len(c) == 0 {
		return ""
	}
	buf, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(buf)
}
