package ai_assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
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
	papers     PaperGetter
	planner    LibrarySearchPlanner
	classifier LibraryPaperClassifier
}

const (
	defaultLibrarySearchLimit = 12
	maxLibrarySearchLimit     = 50
	libraryClassifierParallel = 8
)

func NewLibrarySearchTool(papers PaperGetter) *LibrarySearchTool {
	return &LibrarySearchTool{papers: papers}
}

func NewLibrarySearchToolWithAgents(papers PaperGetter, planner LibrarySearchPlanner, classifier LibraryPaperClassifier) *LibrarySearchTool {
	return &LibrarySearchTool{papers: papers, planner: planner, classifier: classifier}
}

type LibrarySearchPlanner interface {
	PlanLibrarySearch(ctx context.Context, query string) (LibrarySearchPlan, error)
}

type LibrarySearchPlan struct {
	SearchTerms []string `json:"search_terms"`
	Rationale   string   `json:"rationale,omitempty"`
}

type LibraryPaperClassifier interface {
	ClassifyLibraryPaper(ctx context.Context, in LibraryPaperClassificationInput) (LibraryPaperClassificationResult, error)
}

type LibraryPaperClassificationInput struct {
	Query       string
	Plan        LibrarySearchPlan
	Terms       []string
	Paper       model.Paper
	Matches     []LocalEvidenceMatch
	MaxSnippets int
}

type LibraryPaperClassificationResult struct {
	Relevant bool
	Reason   string
}

func (t *LibrarySearchTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = defaultLibrarySearchLimit
	}
	if limit > maxLibrarySearchLimit {
		limit = maxLibrarySearchLimit
	}
	terms := EvidenceSearchTerms(in.Query)
	plan := LibrarySearchPlan{}
	processStages := make([]ProcessStage, 0, 4)
	if t.planner != nil {
		planned, planErr := t.planner.PlanLibrarySearch(ctx, in.Query)
		if planErr == nil {
			plan = planned
			plannedTerms := sanitizeEvidenceTerms(planned.SearchTerms)
			if len(plannedTerms) > 0 {
				terms = plannedTerms
			}
			processStages = append(processStages, ProcessStage{Label: "Master规划", Count: len(terms), Unit: "词", Status: "completed"})
		} else {
			processStages = append(processStages, ProcessStage{Label: "Master规划", Status: "failed"})
		}
	}
	inputJSON, _ := json.Marshal(struct {
		Query string   `json:"query"`
		Terms []string `json:"terms,omitempty"`
		Limit int      `json:"limit"`
	}{Query: in.Query, Terms: terms, Limit: limit})
	ids, candidateErr := candidateIDs(t.papers, terms, 120)
	if candidateErr != nil {
		processStages = append(processStages, ProcessStage{Label: "全文扫描", Status: "failed"})
		return ToolResult{
			Process: ProcessSummary{
				Intent: IntentLibrarySearch,
				Stages: processStages,
			},
			ToolCalls: []ToolCallSummary{{
				ToolName:  "library_search",
				InputJSON: string(inputJSON),
				Status:    "failed",
				Error:     candidateErr.Error(),
			}},
		}, nil
	}
	processStages = append(processStages, ProcessStage{Label: "全文扫描", Count: len(ids), Unit: "篇", Status: "completed"})
	rawHitLimit := limit
	if t.classifier != nil {
		rawHitLimit = limit * 4
		if rawHitLimit > maxLibrarySearchLimit {
			rawHitLimit = maxLibrarySearchLimit
		}
	}
	hits := make([]librarySearchHit, 0, rawHitLimit)
	skipped := 0

	for _, id := range ids {
		if len(hits) >= rawHitLimit {
			break
		}
		if err := ctx.Err(); err != nil {
			return ToolResult{}, err
		}
		paper, err := t.papers.GetPaperDetail(id)
		if err != nil || paper == nil {
			skipped++
			continue
		}
		matches := FindLocalEvidenceMatches(*paper, terms, 3)
		if len(matches) == 0 {
			continue
		}
		hits = append(hits, librarySearchHit{Paper: *paper, Matches: matches})
	}

	classified := 0
	if t.classifier != nil && len(hits) > 0 {
		hits, classified = t.classifyHits(ctx, in.Query, plan, terms, hits)
		processStages = append(processStages, ProcessStage{Label: "Sub-Agent判定", Count: classified, Unit: "篇", Status: "completed"})
	}

	cards := make([]ResultCard, 0, limit)
	citations := make([]Citation, 0, limit*3)
	for _, hit := range hits {
		if len(cards) >= limit {
			break
		}
		snippets := make([]PaperHitSnippet, 0, len(hit.Matches))
		for _, match := range hit.Matches {
			citation := Citation{
				I:       len(citations) + 1,
				PaperID: hit.Paper.ID,
				Title:   hit.Paper.Title,
				Source:  "local",
				Snippet: match.Snippet,
				Score:   match.Score,
			}
			citations = append(citations, citation)
			snippets = append(snippets, PaperHitSnippet{
				CitationIndex: citation.I,
				Location:      match.Location,
				Text:          match.Snippet.Text,
			})
		}

		reason := "命中 " + strings.Join(matchedLocations(snippets), "、")
		if strings.TrimSpace(hit.ClassifierReason) != "" {
			reason = "Sub-Agent判定：" + strings.TrimSpace(hit.ClassifierReason)
		}
		card := PaperHitCard{
			PaperID:  hit.Paper.ID,
			Title:    hit.Paper.Title,
			DOI:      hit.Paper.DOI,
			Year:     hit.Paper.PublishedAt,
			Reason:   reason,
			Snippets: snippets,
		}
		cards = append(cards, ResultCard{Type: "paper_hit", Payload: card})
	}
	processStages = append(processStages, ProcessStage{Label: "命中", Count: len(cards), Unit: "篇", Status: "completed"})

	outputJSON, _ := json.Marshal(struct {
		Candidates int `json:"candidates"`
		Hits       int `json:"hits"`
		Skipped    int `json:"skipped,omitempty"`
		Classified int `json:"classified,omitempty"`
	}{Candidates: len(ids), Hits: len(cards), Skipped: skipped, Classified: classified})

	return ToolResult{
		Process: ProcessSummary{
			Intent: IntentLibrarySearch,
			Stages: processStages,
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

type librarySearchHit struct {
	Paper            model.Paper
	Matches          []LocalEvidenceMatch
	ClassifierReason string
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
	add(chineseLiteralEvidenceTerms(q)...)
	return sanitizeEvidenceTerms(terms)
}

func (t *LibrarySearchTool) classifyHits(ctx context.Context, query string, plan LibrarySearchPlan, terms []string, hits []librarySearchHit) ([]librarySearchHit, int) {
	if t.classifier == nil || len(hits) == 0 {
		return hits, 0
	}
	type result struct {
		index int
		ok    bool
		res   LibraryPaperClassificationResult
	}
	parallel := libraryClassifierParallel
	if parallel > len(hits) {
		parallel = len(hits)
	}
	sem := make(chan struct{}, parallel)
	out := make(chan result, len(hits))
	var wg sync.WaitGroup
	for i, hit := range hits {
		if err := ctx.Err(); err != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(index int, candidate librarySearchHit) {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := t.classifier.ClassifyLibraryPaper(ctx, LibraryPaperClassificationInput{
				Query:       query,
				Plan:        plan,
				Terms:       terms,
				Paper:       candidate.Paper,
				Matches:     candidate.Matches,
				MaxSnippets: 3,
			})
			out <- result{index: index, ok: err == nil, res: res}
		}(i, hit)
	}
	wg.Wait()
	close(out)

	accepted := make([]bool, len(hits))
	reasons := make([]string, len(hits))
	classified := 0
	for res := range out {
		if !res.ok {
			accepted[res.index] = true
			continue
		}
		classified++
		accepted[res.index] = res.res.Relevant
		reasons[res.index] = res.res.Reason
	}
	filtered := make([]librarySearchHit, 0, len(hits))
	for i, hit := range hits {
		if !accepted[i] {
			continue
		}
		hit.ClassifierReason = reasons[i]
		filtered = append(filtered, hit)
	}
	return filtered, classified
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

func candidateIDs(papers PaperGetter, terms []string, limit int) ([]int64, error) {
	if papers == nil || len(terms) == 0 || limit <= 0 {
		return nil, nil
	}
	lister, ok := papers.(EvidenceCandidateLister)
	if !ok {
		return nil, nil
	}
	found, err := lister.ListEvidenceCandidatePaperIDs(terms, limit)
	if err != nil {
		return nil, err
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
	return ids, nil
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

func sanitizeEvidenceTerms(terms []string) []string {
	cleaned := make([]string, 0, len(terms))
	for _, term := range terms {
		if isGenericEvidenceTerm(term) {
			continue
		}
		cleaned = append(cleaned, term)
	}
	return dedupeEvidenceTerms(cleaned)
}

func isGenericEvidenceTerm(term string) bool {
	switch strings.ToLower(strings.TrimSpace(term)) {
	case "", "数据", "文章", "论文", "文献", "资料", "信息", "内容", "data", "dataset", "paper", "papers", "article", "articles":
		return true
	default:
		return false
	}
}

func containsAnyEvidenceText(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func chineseLiteralEvidenceTerms(query string) []string {
	cleaned := strings.TrimSpace(query)
	if cleaned == "" {
		return nil
	}
	for _, phrase := range evidenceQueryPhrases {
		cleaned = strings.ReplaceAll(cleaned, phrase, " ")
	}

	terms := make([]string, 0, 16)
	var runes []rune
	flush := func() {
		if len(runes) < 2 {
			runes = runes[:0]
			return
		}
		terms = append(terms, string(runes))
		for n := 4; n >= 2; n-- {
			if len(runes) <= n {
				continue
			}
			for i := 0; i+n <= len(runes); i++ {
				terms = append(terms, string(runes[i:i+n]))
			}
		}
		runes = runes[:0]
	}
	for _, r := range cleaned {
		if unicode.Is(unicode.Han, r) {
			runes = append(runes, r)
			continue
		}
		flush()
	}
	flush()
	return terms
}

var evidenceQueryPhrases = []string{
	"相关的文章",
	"相关文献",
	"相关论文",
	"的文章",
	"的文献",
	"的论文",
	"帮我查找",
	"帮我查询",
	"帮我搜索",
	"帮我寻找",
	"帮忙查找",
	"帮忙查询",
	"帮忙搜索",
	"帮忙寻找",
	"查找",
	"查询",
	"搜索",
	"寻找",
	"关于",
	"有关",
	"相关",
	"包括",
	"包含",
	"用到",
	"使用",
	"涉及",
	"是否",
	"有没有",
	"哪些",
	"文章",
	"论文",
	"文献",
	"信息",
	"内容",
	"资料",
	"帮我",
	"帮忙",
}

var assistantASCIITermRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+&./-]{2,}`)
