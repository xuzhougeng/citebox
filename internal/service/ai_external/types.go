package ai_external

import "context"

type SourceID string

const (
	SourcePubMed          SourceID = "pubmed"
	SourceSemanticScholar SourceID = "semantic_scholar"
)

type Paper struct {
	Source         SourceID
	SourcePaperID  string
	Sources        []SourceID
	SourcePaperIDs map[SourceID]string
	PMID           string
	PMCID          string
	DOI            string
	ArXiv          string
	Title          string
	Abstract       string
	TLDR           string
	Venue          string
	Year           int
	Authors        []string
	URL            string
	OpenAccessURL  string
	CitationCount  int
	MatchedQuery   string
}

type SearchOptions struct {
	Limit int
}

type Searcher interface {
	Search(ctx context.Context, query string, opts SearchOptions) ([]Paper, error)
}

type SourceResult struct {
	Source SourceID
	Query  string
	Papers []Paper
	Err    error
}

type SourceQueries map[SourceID][]string
