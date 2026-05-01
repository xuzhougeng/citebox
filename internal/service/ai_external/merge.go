package ai_external

import (
	"strings"
	"unicode"
)

const defaultMergeLimit = 20

func MergePapers(results []SourceResult, sourceOrder []SourceID, limit int) []Paper {
	if limit <= 0 {
		limit = defaultMergeLimit
	}

	bySource := make(map[SourceID][]Paper)
	seenSources := make(map[SourceID]bool)
	for _, result := range results {
		if result.Err != nil {
			continue
		}
		source := result.Source
		seenSources[source] = true
		for _, paper := range result.Papers {
			if paper.Source == "" {
				paper.Source = source
			}
			bySource[source] = append(bySource[source], paper)
		}
	}

	order := mergeSourceOrder(sourceOrder, results, seenSources)
	out := make([]Paper, 0)
	index := make(map[string]int)
	for row := 0; ; row++ {
		added := false
		for _, source := range order {
			papers := bySource[source]
			if row >= len(papers) {
				continue
			}
			added = true
			paper := ensureSourceMetadata(papers[row])
			if existing, ok := findExistingPaper(index, paper); ok {
				out[existing] = mergePaper(out[existing], paper)
				indexPaper(index, out[existing], existing)
				continue
			}
			out = append(out, paper)
			indexPaper(index, paper, len(out)-1)
		}
		if !added {
			break
		}
	}

	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func mergeSourceOrder(sourceOrder []SourceID, results []SourceResult, seenSources map[SourceID]bool) []SourceID {
	order := make([]SourceID, 0, len(sourceOrder))
	added := make(map[SourceID]bool)
	for _, source := range sourceOrder {
		if seenSources[source] && !added[source] {
			order = append(order, source)
			added[source] = true
		}
	}
	for _, result := range results {
		if result.Err != nil || added[result.Source] {
			continue
		}
		order = append(order, result.Source)
		added[result.Source] = true
	}
	return order
}

func findExistingPaper(index map[string]int, paper Paper) (int, bool) {
	for _, key := range paperKeys(paper) {
		if existing, ok := index[key]; ok {
			return existing, true
		}
	}
	return 0, false
}

func indexPaper(index map[string]int, paper Paper, position int) {
	for _, key := range paperKeys(paper) {
		index[key] = position
	}
}

func paperKeys(paper Paper) []string {
	keys := make([]string, 0, 3)
	if doi := normalizeDOI(paper.DOI); doi != "" {
		keys = append(keys, "doi:"+doi)
	}
	if pmid := strings.TrimSpace(paper.PMID); pmid != "" {
		keys = append(keys, "pmid:"+pmid)
	}
	if title := normalizeTitle(paper.Title); title != "" {
		keys = append(keys, "title:"+title)
	}
	return keys
}

func ensureSourceMetadata(paper Paper) Paper {
	if paper.SourcePaperIDs == nil {
		paper.SourcePaperIDs = make(map[SourceID]string)
	}
	paper.Sources = uniqueSources(paper.Sources)
	if paper.Source != "" {
		if !hasSource(paper.Sources, paper.Source) {
			paper.Sources = append(paper.Sources, paper.Source)
		}
		if paper.SourcePaperID != "" && paper.SourcePaperIDs[paper.Source] == "" {
			paper.SourcePaperIDs[paper.Source] = paper.SourcePaperID
		}
	}
	return paper
}

func mergePaper(dst Paper, src Paper) Paper {
	dst = ensureSourceMetadata(dst)
	src = ensureSourceMetadata(src)

	for _, source := range src.Sources {
		if source != "" && !hasSource(dst.Sources, source) {
			dst.Sources = append(dst.Sources, source)
		}
	}
	if dst.SourcePaperIDs == nil {
		dst.SourcePaperIDs = make(map[SourceID]string)
	}
	for source, id := range src.SourcePaperIDs {
		if id != "" && dst.SourcePaperIDs[source] == "" {
			dst.SourcePaperIDs[source] = id
		}
	}

	if dst.DOI == "" {
		dst.DOI = src.DOI
	}
	if dst.PMID == "" {
		dst.PMID = src.PMID
	}
	if dst.PMCID == "" {
		dst.PMCID = src.PMCID
	}
	if dst.ArXiv == "" {
		dst.ArXiv = src.ArXiv
	}
	if dst.Title == "" {
		dst.Title = src.Title
	}
	if len(src.Abstract) > len(dst.Abstract) {
		dst.Abstract = src.Abstract
	}
	if dst.TLDR == "" {
		dst.TLDR = src.TLDR
	}
	if dst.Venue == "" {
		dst.Venue = src.Venue
	}
	if dst.Year == 0 {
		dst.Year = src.Year
	}
	if len(dst.Authors) == 0 && len(src.Authors) > 0 {
		dst.Authors = src.Authors
	}
	if dst.URL == "" {
		dst.URL = src.URL
	}
	if dst.OpenAccessURL == "" {
		dst.OpenAccessURL = src.OpenAccessURL
	}
	if dst.CitationCount == 0 {
		dst.CitationCount = src.CitationCount
	} else if src.CitationCount > dst.CitationCount {
		dst.CitationCount = src.CitationCount
	}
	if dst.MatchedQuery == "" {
		dst.MatchedQuery = src.MatchedQuery
	}
	return dst
}

func uniqueSources(sources []SourceID) []SourceID {
	unique := make([]SourceID, 0, len(sources))
	for _, source := range sources {
		if source != "" && !hasSource(unique, source) {
			unique = append(unique, source)
		}
	}
	return unique
}

func hasSource(sources []SourceID, source SourceID) bool {
	for _, existing := range sources {
		if existing == source {
			return true
		}
	}
	return false
}

func normalizeDOI(doi string) string {
	doi = strings.ToLower(strings.TrimSpace(doi))
	doi = strings.TrimPrefix(doi, "https://doi.org/")
	doi = strings.TrimPrefix(doi, "http://doi.org/")
	doi = strings.TrimPrefix(doi, "doi:")
	return strings.TrimSpace(doi)
}

func normalizeTitle(title string) string {
	title = strings.ToLower(title)
	var b strings.Builder
	lastSpace := true
	for _, r := range title {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if unicode.IsSpace(r) || !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}
