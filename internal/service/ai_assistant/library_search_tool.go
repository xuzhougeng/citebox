package ai_assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

type PaperGetter interface {
	GetPaperDetail(id int64) (*model.Paper, error)
}

type EvidenceCandidateLister interface {
	ListEvidenceCandidatePaperIDs(terms []string, limit int) ([]int64, error)
}

type LibrarySearchTool struct {
	papers PaperGetter
}

func NewLibrarySearchTool(papers PaperGetter) *LibrarySearchTool {
	return &LibrarySearchTool{papers: papers}
}

func (t *LibrarySearchTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 12
	}
	terms := EvidenceSearchTerms(in.Query)
	ids := candidateIDs(t.papers, terms, 120)
	cards := make([]ResultCard, 0, limit)
	citations := make([]Citation, 0, limit)

	for _, id := range ids {
		if len(cards) >= limit {
			break
		}
		if err := ctx.Err(); err != nil {
			return ToolResult{}, err
		}
		paper, err := t.papers.GetPaperDetail(id)
		if err != nil || paper == nil {
			continue
		}
		matches := FindLocalEvidenceMatches(*paper, terms, 3)
		if len(matches) == 0 {
			continue
		}

		match := matches[0]
		citation := Citation{
			I:       len(citations) + 1,
			PaperID: paper.ID,
			Title:   paper.Title,
			Source:  "local",
			Snippet: match.Snippet,
			Score:   match.Score,
		}
		citations = append(citations, citation)
		snippets := []PaperHitSnippet{{
			CitationIndex: citation.I,
			Location:      match.Location,
			Text:          match.Snippet.Text,
		}}

		card := PaperHitCard{
			PaperID:  paper.ID,
			Title:    paper.Title,
			DOI:      paper.DOI,
			Year:     paper.PublishedAt,
			Reason:   "命中 " + strings.Join(matchedLocations(snippets), "、"),
			Snippets: snippets,
		}
		cards = append(cards, ResultCard{Type: "paper_hit", Payload: card})
	}

	inputJSON, _ := json.Marshal(struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}{Query: in.Query, Limit: limit})
	outputJSON, _ := json.Marshal(struct {
		Candidates int `json:"candidates"`
		Hits       int `json:"hits"`
	}{Candidates: len(ids), Hits: len(cards)})

	return ToolResult{
		Process: ProcessSummary{
			Intent: IntentLibrarySearch,
			Stages: []ProcessStage{
				{Label: "全库检索", Count: len(ids), Unit: "篇", Status: "completed"},
				{Label: "命中", Count: len(cards), Unit: "篇", Status: "completed"},
			},
		},
		Cards:         cards,
		Citations:     citations,
		AnswerContext: libraryAnswerContext(cards),
		ToolCalls: []ToolCallSummary{{
			ToolName:          "library_search",
			InputJSON:         string(inputJSON),
			OutputSummaryJSON: string(outputJSON),
			Status:            "completed",
		}},
	}, nil
}

type LocalEvidenceMatch struct {
	Location string
	Snippet  research.Snippet
	Score    float64
}

type assistantLocalCandidate struct {
	paperID int64
	title   string
	field   string
	text    string
	start   int
	end     int
	score   float64
}

func EvidenceSearchTerms(query string) []string {
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

	if containsAnyEvidenceText(lower, "单细胞", "single-cell", "single cell", "scrna", "scatac") {
		add("single-cell", "single cell", "scRNA-seq", "single-cell RNA-seq", "single-cell RNA sequencing",
			"scATAC-seq", "single-cell ATAC-seq", "single-cell multiomics", "10x Genomics", "Seurat", "Scanpy")
	}
	if containsAnyEvidenceText(lower, "atac", "开放染色质", "染色质可及", "转座酶", "multiome", "multi-ome") {
		add("ATAC-seq", "ATAC seq", "ATAC sequencing", "assay for transposase-accessible chromatin",
			"chromatin accessibility", "accessible chromatin", "scATAC-seq", "single-cell ATAC-seq",
			"10x Multiome", "multiome", "multi-ome")
	}
	if containsAnyEvidenceText(lower, "测序", "sequenc", "rna-seq", "genome", "transcriptome") {
		add("sequencing", "RNA-seq", "RNA sequencing", "transcriptome", "genome sequencing",
			"whole-genome sequencing", "whole-exome sequencing", "resequencing", "ATAC-seq",
			"ChIP-seq", "long-read sequencing", "PacBio", "Nanopore", "Illumina")
	}
	if containsAnyEvidenceText(lower, "表皮", "epiderm") {
		add("epidermis", "epidermal")
	}
	if containsAnyEvidenceText(lower, "发育", "development") {
		add("development", "developmental")
	}
	if containsAnyEvidenceText(lower, "轨迹", "trajectory", "pseudotime") {
		add("trajectory", "trajectories", "pseudotime")
	}
	if containsAnyEvidenceText(lower, "染色质", "chromatin") {
		add("chromatin", "chromatin accessibility", "ATAC-seq")
	}
	if containsAnyEvidenceText(lower, "表达", "expression") {
		add("gene expression", "expression")
	}

	for _, match := range assistantASCIITermRe.FindAllString(q, -1) {
		if isEvidenceStopWord(match) {
			continue
		}
		add(match)
	}
	return dedupeEvidenceTerms(terms)
}

func FindLocalEvidenceMatches(paper model.Paper, terms []string, limit int) []LocalEvidenceMatch {
	if limit <= 0 || len(terms) == 0 {
		return nil
	}
	candidates := findAssistantLocalCandidates(paper, terms)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].paperID != candidates[j].paperID {
			return candidates[i].paperID < candidates[j].paperID
		}
		return candidates[i].start < candidates[j].start
	})

	out := make([]LocalEvidenceMatch, 0, limit)
	seen := map[string]bool{}
	for _, cand := range candidates {
		if len(out) >= limit {
			break
		}
		key := fmt.Sprintf("%s:%d", cand.field, cand.start/300)
		if seen[key] {
			continue
		}
		seen[key] = true
		section := localEvidenceSectionLabel(cand.field)
		out = append(out, LocalEvidenceMatch{
			Location: section,
			Snippet: research.Snippet{
				Text:          cand.text,
				SnippetKind:   cand.field,
				Section:       section,
				SnippetOffset: research.SnippetOffset{Start: cand.start, End: cand.end},
			},
			Score: cand.score,
		})
	}
	return out
}

func candidateIDs(papers PaperGetter, terms []string, limit int) []int64 {
	if papers == nil || len(terms) == 0 || limit <= 0 {
		return nil
	}
	lister, ok := papers.(EvidenceCandidateLister)
	if !ok {
		return nil
	}
	found, err := lister.ListEvidenceCandidatePaperIDs(terms, limit)
	if err != nil {
		return nil
	}
	seen := map[int64]bool{}
	ids := make([]int64, 0, len(found))
	for _, id := range found {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		if len(ids) >= limit {
			break
		}
	}
	return ids
}

func matchedLocations(snippets []PaperHitSnippet) []string {
	locations := make([]string, 0, len(snippets))
	seen := map[string]bool{}
	for _, snippet := range snippets {
		location := strings.TrimSpace(snippet.Location)
		if location == "" || seen[location] {
			continue
		}
		seen[location] = true
		locations = append(locations, location)
	}
	if len(locations) == 0 {
		return []string{"本地证据"}
	}
	return locations
}

func libraryAnswerContext(cards []ResultCard) string {
	var b strings.Builder
	for _, result := range cards {
		card, ok := result.Payload.(PaperHitCard)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "文献: %s", card.Title)
		if card.Year != "" {
			fmt.Fprintf(&b, " (%s)", card.Year)
		}
		if card.DOI != "" {
			fmt.Fprintf(&b, " DOI: %s", card.DOI)
		}
		b.WriteString("\n")
		for _, snippet := range card.Snippets {
			fmt.Fprintf(&b, "[%d] %s: %s\n", snippet.CitationIndex, snippet.Location, snippet.Text)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func findAssistantLocalCandidates(paper model.Paper, terms []string) []assistantLocalCandidate {
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

	candidates := make([]assistantLocalCandidate, 0)
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
				snippet, runeStart, runeEnd := snippetAroundEvidence(text, start, end, 420)
				candidates = append(candidates, assistantLocalCandidate{
					paperID: paper.ID,
					title:   paper.Title,
					field:   field.name,
					text:    snippet,
					start:   runeStart,
					end:     runeEnd,
					score:   localEvidenceScore(term, field.boost),
				})
				pos = end
				keptForTerm++
			}
		}
	}
	return candidates
}

func localEvidenceScore(term string, boost float64) float64 {
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

func localEvidenceSectionLabel(field string) string {
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

func snippetAroundEvidence(text string, startByte, endByte, windowRunes int) (string, int, int) {
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

func isEvidenceStopWord(term string) bool {
	switch strings.ToLower(strings.Trim(term, ".,;:!?()[]{}")) {
	case "the", "and", "for", "with", "that", "this", "from", "into", "about", "paper", "papers", "article", "articles", "related", "find", "search":
		return true
	default:
		return false
	}
}

func dedupeEvidenceTerms(terms []string) []string {
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

func containsAnyEvidenceText(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

var assistantASCIITermRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+&./-]{2,}`)
