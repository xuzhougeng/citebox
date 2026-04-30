package ai_conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

const (
	evidenceSourceLocal    = "local"
	evidenceSourceExternal = "external"
)

// ExternalEvidenceSearcher is the external search surface we depend on. Satisfied by
// *research.Service.
type ExternalEvidenceSearcher interface {
	SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error)
	Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error)
}

// PaperDetailGetter is the local library surface used by internal search.
type PaperDetailGetter interface {
	GetPaperDetail(id int64) (*model.Paper, error)
}

// PaperEvidenceLister is optionally implemented by the local paper repository.
// It returns library-wide candidate IDs for literal term matching; no embeddings
// or vector search are involved.
type PaperEvidenceLister interface {
	ListEvidenceCandidatePaperIDs(terms []string, limit int) ([]int64, error)
}

// EvidenceOptions controls which evidence sources are used for one turn.
type EvidenceOptions struct {
	IncludeExternal bool
	DisableLocal    bool
}

// Citation is one entry in the persisted citations_json array.
type Citation struct {
	I          int              `json:"i"`
	PaperID    int64            `json:"paper_id"`
	ExternalID string           `json:"external_id"`
	S2PaperID  string           `json:"s2_paper_id,omitempty"`
	Title      string           `json:"title,omitempty"`
	Source     string           `json:"source,omitempty"`
	Snippet    research.Snippet `json:"snippet"`
	Score      float64          `json:"score"`
}

// injectEvidence builds an evidence prompt fragment from local library text
// first (with pinned papers prioritized), and optionally augments it with
// Semantic Scholar snippets. It deliberately never uses embeddings or vector
// retrieval.
func injectEvidence(ctx context.Context, papers PaperDetailGetter, searcher ExternalEvidenceSearcher,
	userText string, pinned []repository.AIPinnedPaper, opts EvidenceOptions) (string, []Citation, error) {

	var citations []Citation
	if !opts.DisableLocal {
		citations = localEvidence(papers, userText, pinned, 12)
	}
	var externalErr error
	if opts.IncludeExternal && searcher != nil {
		externalLimit := 8
		if !opts.DisableLocal && len(citations) < 8 {
			externalLimit = 8 - len(citations)
		}
		external, err := externalEvidence(ctx, searcher, userText, pinned, externalLimit)
		if err != nil {
			externalErr = err
		}
		citations = append(citations, external...)
	}

	for i := range citations {
		citations[i].I = i + 1
	}
	return buildEvidencePrompt(userText, citations, len(pinned), !opts.DisableLocal, opts.IncludeExternal, externalErr), citations, nil
}

func buildEvidencePrompt(userText string, citations []Citation, pinnedCount int, includeLocal, includeExternal bool, externalErr error) string {
	var b strings.Builder
	if includeLocal {
		b.WriteString("你处于内部搜索模式。")
	} else {
		b.WriteString("你处于外部搜索模式。")
	}
	b.WriteString("你必须只基于以下证据片段回答；每个由证据支撑的论断后用 [n] 标注引用。")
	b.WriteString("如果证据不足以回答或支持用户说法，请明确说明\"证据不足\"，不要凭常识补全。\n\n")
	sources := make([]string, 0, 2)
	if includeLocal {
		sources = append(sources, "本地文献库（已钉文献优先，包含本地已钉文献全文、标题、摘要、笔记和 PDF 全文）")
	}
	if includeExternal {
		sources = append(sources, "外部 Semantic Scholar 片段")
	}
	if len(sources) == 0 {
		sources = append(sources, "当前证据来源")
	}
	b.WriteString("证据来源：" + strings.Join(sources, "；"))
	b.WriteString("。\n")
	if externalErr != nil {
		if includeLocal {
			b.WriteString("外部搜索失败，本次仅使用可用的本地证据。\n")
		} else {
			b.WriteString("外部搜索失败，本次没有可用的外部搜索结果。\n")
		}
	}
	if includeLocal && pinnedCount == 0 {
		b.WriteString("当前没有已钉文献；本次会扫描本地文献库中的可匹配证据片段。\n")
	}

	b.WriteString("\n证据：\n")
	if len(citations) == 0 {
		b.WriteString("（未从当前证据来源找到匹配片段。请回答证据不足，不要按普通问答模式发挥。）\n")
	} else {
		for _, c := range citations {
			section := c.Snippet.Section
			if section == "" {
				section = c.Snippet.SnippetKind
			}
			source := "本地全文"
			if c.Source == evidenceSourceExternal {
				source = "外部 Semantic Scholar"
			}
			title := strings.TrimSpace(c.Title)
			if title == "" {
				title = fmt.Sprintf("paper %d", c.PaperID)
			}
			fmt.Fprintf(&b, "[%d] 来源: %s | 文献: %s | 位置: %s\n%s\n\n", c.I, source, title, section, c.Snippet.Text)
		}
	}
	b.WriteString("用户问题：\n")
	b.WriteString(userText)
	return b.String()
}

type localCandidate struct {
	paperID int64
	title   string
	source  string
	field   string
	text    string
	start   int
	end     int
	score   float64
}

func localEvidence(papers PaperDetailGetter, userText string, pinned []repository.AIPinnedPaper, limit int) []Citation {
	if papers == nil || limit <= 0 {
		return nil
	}
	terms := evidenceSearchTerms(userText)
	if len(terms) == 0 {
		return nil
	}

	candidates := make([]localCandidate, 0)
	pinnedIDs := make(map[int64]bool, len(pinned))
	for _, pin := range pinned {
		pinnedIDs[pin.PaperID] = true
	}
	for _, paperID := range localEvidenceCandidateIDs(papers, terms, pinned, 120) {
		paper, err := papers.GetPaperDetail(paperID)
		if err != nil || paper == nil {
			continue
		}
		found := findLocalCandidates(*paper, terms)
		if pinnedIDs[paperID] {
			for i := range found {
				found[i].score += 0.25
			}
		}
		candidates = append(candidates, found...)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].paperID != candidates[j].paperID {
			return candidates[i].paperID < candidates[j].paperID
		}
		return candidates[i].start < candidates[j].start
	})

	out := make([]Citation, 0, limit)
	seen := map[string]bool{}
	perPaper := map[int64]int{}
	for _, cand := range candidates {
		if len(out) >= limit {
			break
		}
		if perPaper[cand.paperID] >= 4 {
			continue
		}
		key := fmt.Sprintf("%d:%s:%d", cand.paperID, cand.field, cand.start/300)
		if seen[key] {
			continue
		}
		seen[key] = true
		perPaper[cand.paperID]++
		out = append(out, Citation{
			PaperID: cand.paperID,
			Title:   cand.title,
			Source:  evidenceSourceLocal,
			Snippet: research.Snippet{
				Text:          cand.text,
				SnippetKind:   cand.field,
				Section:       localSectionLabel(cand.field),
				SnippetOffset: research.SnippetOffset{Start: cand.start, End: cand.end},
			},
			Score: cand.score,
		})
	}
	return out
}

func localEvidenceCandidateIDs(papers PaperDetailGetter, terms []string, pinned []repository.AIPinnedPaper, limit int) []int64 {
	seen := map[int64]bool{}
	ids := make([]int64, 0, limit)
	add := func(id int64) {
		if id <= 0 || seen[id] {
			return
		}
		if limit > 0 && len(ids) >= limit {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}

	for _, pin := range pinned {
		add(pin.PaperID)
	}
	if lister, ok := papers.(PaperEvidenceLister); ok {
		found, err := lister.ListEvidenceCandidatePaperIDs(terms, limit)
		if err == nil {
			for _, id := range found {
				add(id)
			}
		}
	}
	return ids
}

func findLocalCandidates(paper model.Paper, terms []string) []localCandidate {
	fields := []struct {
		name  string
		text  string
		boost float64
	}{
		{name: "title", text: paper.Title, boost: 1.4},
		{name: "abstract", text: paper.AbstractText, boost: 1.25},
		{name: "notes", text: paper.NotesText + "\n" + paper.PaperNotesText, boost: 1.15},
		{name: "body", text: paper.PDFText, boost: 1.0},
	}

	candidates := make([]localCandidate, 0)
	for _, field := range fields {
		text := strings.TrimSpace(field.text)
		if text == "" {
			continue
		}
		lowerText := strings.ToLower(text)
		for _, term := range terms {
			lowerTerm := strings.ToLower(term)
			if lowerTerm == "" {
				continue
			}
			pos := 0
			keptForTerm := 0
			for keptForTerm < 6 {
				idx := strings.Index(lowerText[pos:], lowerTerm)
				if idx < 0 {
					break
				}
				start := pos + idx
				end := start + len(lowerTerm)
				snippet, runeStart, runeEnd := snippetAround(text, start, end, 420)
				candidates = append(candidates, localCandidate{
					paperID: paper.ID,
					title:   paper.Title,
					source:  evidenceSourceLocal,
					field:   field.name,
					text:    snippet,
					start:   runeStart,
					end:     runeEnd,
					score:   localScore(term, field.boost),
				})
				pos = end
				keptForTerm++
			}
		}
	}
	return candidates
}

func localScore(term string, boost float64) float64 {
	runes := utf8.RuneCountInString(term)
	score := 0.45 + float64(runes)/40
	if strings.Contains(term, "-") || strings.Contains(term, " ") {
		score += 0.18
	}
	if score > 0.98 {
		score = 0.98
	}
	return score * boost
}

func localSectionLabel(field string) string {
	switch field {
	case "title":
		return "标题"
	case "abstract":
		return "摘要"
	case "notes":
		return "笔记"
	default:
		return "本地全文"
	}
}

func snippetAround(text string, startByte, endByte, windowRunes int) (string, int, int) {
	startRune := utf8.RuneCountInString(text[:startByte])
	endRune := utf8.RuneCountInString(text[:endByte])
	runes := []rune(text)
	left := startRune - windowRunes
	if left < 0 {
		left = 0
	}
	right := endRune + windowRunes
	if right > len(runes) {
		right = len(runes)
	}
	prefix := ""
	if left > 0 {
		prefix = "..."
	}
	suffix := ""
	if right < len(runes) {
		suffix = "..."
	}
	return prefix + normalizeEvidenceWhitespace(string(runes[left:right])) + suffix, startRune, endRune
}

func normalizeEvidenceWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func externalEvidence(ctx context.Context, searcher ExternalEvidenceSearcher, userText string,
	pinned []repository.AIPinnedPaper, limit int) ([]Citation, error) {

	if searcher == nil || limit <= 0 {
		return nil, nil
	}
	idMap := map[string]repository.AIPinnedPaper{}
	for _, p := range pinned {
		ext := externalIDFor(p)
		if ext == "" {
			continue
		}
		idMap[ext] = p
	}

	q := externalEvidenceQuery(userText)
	if len([]rune(q)) > 200 {
		q = string([]rune(q)[:200])
	}
	res, err := searcher.SnippetSearch(ctx, q, research.SnippetSearchOpts{
		Limit: limit,
	})
	if err == nil {
		citations := citationsFromSnippetMatches(res.Items, idMap)
		if len(citations) > 0 {
			return citations, nil
		}
	}

	fallback, fallbackErr := externalPaperSearchEvidence(ctx, searcher, q, idMap, limit)
	if fallbackErr != nil {
		if err != nil {
			return nil, fallbackErr
		}
		return nil, nil
	}
	return fallback, nil
}

func citationsFromSnippetMatches(items []research.SnippetMatch, idMap map[string]repository.AIPinnedPaper) []Citation {
	citations := make([]Citation, 0, len(items))
	for _, m := range items {
		ext := externalIDForResearchPaper(m.Paper)
		var paperID int64
		title := m.Paper.Title
		if title == "" {
			title = m.PaperID
		}
		if pin, ok := idMap[ext]; ok {
			paperID = pin.PaperID
			if title == "" {
				title = pin.Title
			}
		}
		citations = append(citations, Citation{
			PaperID: paperID, ExternalID: ext, S2PaperID: m.PaperID, Title: title,
			Source: evidenceSourceExternal, Snippet: m.Snippet, Score: m.Score,
		})
	}
	return citations
}

func externalPaperSearchEvidence(ctx context.Context, searcher ExternalEvidenceSearcher, query string,
	idMap map[string]repository.AIPinnedPaper, limit int) ([]Citation, error) {

	res, err := searcher.Search(ctx, query, research.SearchOpts{Limit: limit})
	if err != nil {
		return nil, err
	}
	citations := make([]Citation, 0, len(res.Items))
	for i, p := range res.Items {
		snippet, kind := evidenceSnippetFromSearchPaper(p)
		if snippet == "" {
			continue
		}
		ext := externalIDForResearchPaper(p)
		var paperID int64
		title := p.Title
		if title == "" {
			title = p.PaperID
		}
		if pin, ok := idMap[ext]; ok {
			paperID = pin.PaperID
			if title == "" {
				title = pin.Title
			}
		}
		score := 0.7
		if i < 10 {
			score -= float64(i) * 0.03
		}
		citations = append(citations, Citation{
			PaperID: paperID, ExternalID: ext, S2PaperID: p.PaperID, Title: title,
			Source: evidenceSourceExternal,
			Snippet: research.Snippet{
				Text:        snippet,
				SnippetKind: kind,
				Section:     externalSearchSectionLabel(kind, p),
			},
			Score: score,
		})
	}
	return citations, nil
}

func evidenceSnippetFromSearchPaper(p research.Paper) (string, string) {
	parts := make([]string, 0, 3)
	if p.Title != "" {
		parts = append(parts, "Title: "+p.Title)
	}
	kind := "title"
	if p.TLDR != "" {
		parts = append(parts, "TLDR: "+p.TLDR)
		kind = "tldr"
	}
	if p.Abstract != "" {
		parts = append(parts, "Abstract: "+p.Abstract)
		kind = "abstract"
	}
	text := normalizeEvidenceWhitespace(strings.Join(parts, " "))
	if len([]rune(text)) > 900 {
		text = string([]rune(text)[:900]) + "..."
	}
	return text, kind
}

func externalSearchSectionLabel(kind string, p research.Paper) string {
	switch kind {
	case "abstract":
		return "Semantic Scholar 摘要"
	case "tldr":
		return "Semantic Scholar TLDR"
	default:
		if p.Year > 0 && p.Venue != "" {
			return fmt.Sprintf("Semantic Scholar 搜索结果（%s, %d）", p.Venue, p.Year)
		}
		return "Semantic Scholar 搜索结果"
	}
}

func externalIDForResearchPaper(p research.Paper) string {
	if p.ExternalIDs.DOI != "" {
		return "DOI:" + p.ExternalIDs.DOI
	}
	if p.ExternalIDs.ArXiv != "" {
		return "ARXIV:" + p.ExternalIDs.ArXiv
	}
	if p.ExternalIDs.PubMed != "" {
		return "PMID:" + p.ExternalIDs.PubMed
	}
	return ""
}

func externalEvidenceQuery(userText string) string {
	terms := evidenceSearchTerms(userText)
	selected := make([]string, 0, 12)
	seen := map[string]bool{}
	add := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" {
			return
		}
		key := strings.ToLower(term)
		if seen[key] {
			return
		}
		seen[key] = true
		selected = append(selected, term)
	}

	for _, term := range terms {
		if len(selected) >= 12 {
			break
		}
		if containsASCIIWord(term) {
			add(term)
		}
	}
	if len(selected) == 0 {
		add(userText)
	}
	return strings.Join(selected, " ")
}

func containsASCIIWord(s string) bool {
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			return true
		}
	}
	return false
}

func externalIDFor(p repository.AIPinnedPaper) string {
	if p.DOI != "" {
		return "DOI:" + p.DOI
	}
	return ""
}

var asciiTermRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+&./-]{2,}`)

func evidenceSearchTerms(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	lower := strings.ToLower(q)
	terms := make([]string, 0, 32)
	add := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			terms = append(terms, value)
		}
	}

	if containsAnyText(lower, "单细胞", "single-cell", "single cell", "scrna", "scatac") {
		add("single-cell", "single cell", "scRNA-seq", "single-cell RNA-seq", "single-cell RNA sequencing",
			"scATAC-seq", "single-cell ATAC-seq", "single-cell multiomics", "10x Genomics", "Seurat", "Scanpy")
	}
	if containsAnyText(lower, "atac", "开放染色质", "染色质可及", "转座酶", "multiome", "multi-ome") {
		add("ATAC-seq", "ATAC seq", "ATAC sequencing", "assay for transposase-accessible chromatin",
			"chromatin accessibility", "accessible chromatin", "scATAC-seq", "single-cell ATAC-seq",
			"10x Multiome", "multiome", "multi-ome")
	}
	if containsAnyText(lower, "测序", "sequenc", "rna-seq", "genome", "transcriptome") {
		add("sequencing", "RNA-seq", "RNA sequencing", "transcriptome", "genome sequencing",
			"whole-genome sequencing", "whole-exome sequencing", "resequencing", "ATAC-seq",
			"ChIP-seq", "long-read sequencing", "PacBio", "Nanopore", "Illumina")
	}
	if containsAnyText(lower, "表皮", "epiderm") {
		add("epidermis", "epidermal")
	}
	if containsAnyText(lower, "发育", "development") {
		add("development", "developmental")
	}
	if containsAnyText(lower, "轨迹", "trajectory", "pseudotime") {
		add("trajectory", "trajectories", "pseudotime")
	}
	if containsAnyText(lower, "染色质", "chromatin") {
		add("chromatin", "chromatin accessibility", "ATAC-seq")
	}
	if containsAnyText(lower, "表达", "expression") {
		add("gene expression", "expression")
	}

	for _, match := range asciiTermRe.FindAllString(q, -1) {
		if isEvidenceStopWord(match) {
			continue
		}
		add(match)
	}
	return dedupeTerms(terms)
}

func isEvidenceStopWord(term string) bool {
	switch strings.ToLower(strings.Trim(term, ".,;:!?()[]{}")) {
	case "the", "and", "for", "with", "that", "this", "from", "into", "about", "paper", "papers", "article", "articles", "related", "find", "search":
		return true
	default:
		return false
	}
}

func dedupeTerms(terms []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		key := strings.ToLower(strings.TrimSpace(term))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, term)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return utf8.RuneCountInString(out[i]) > utf8.RuneCountInString(out[j])
	})
	if len(out) > 40 {
		return out[:40]
	}
	return out
}

func containsAnyText(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
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
