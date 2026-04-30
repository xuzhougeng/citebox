// Package research contains the Semantic Scholar Graph API client,
// response cache, and helpers backing the /research panel.
package research

// Paper is a normalised view of a Semantic Scholar paper. We only keep
// fields the panel actually renders; raw responses live in the cache.
type Paper struct {
	PaperID          string   `json:"paperId"`
	ExternalIDs      IDs      `json:"externalIds"`
	Title            string   `json:"title"`
	Abstract         string   `json:"abstract"`
	TLDR             string   `json:"tldr,omitempty"`
	Year             int      `json:"year"`
	Venue            string   `json:"venue"`
	Authors          []Author `json:"authors"`
	CitationCount    int      `json:"citationCount"`
	ReferenceCount   int      `json:"referenceCount"`
	InfluentialCount int      `json:"influentialCitationCount"`
	OpenAccessPDFURL string   `json:"openAccessPdfUrl,omitempty"`
	FieldsOfStudy    []string `json:"fieldsOfStudy,omitempty"`
}

// IDs holds the cross-source identifiers S2 attaches to a paper.
type IDs struct {
	DOI    string `json:"DOI,omitempty"`
	ArXiv  string `json:"ArXiv,omitempty"`
	PubMed string `json:"PubMed,omitempty"`
}

// Author is a minimal author record.
type Author struct {
	AuthorID string `json:"authorId,omitempty"`
	Name     string `json:"name"`
}

// SearchOpts narrows a paper/search query.
type SearchOpts struct {
	Year          string // e.g. "2018-2024", "2020-", "-2015"
	FieldsOfStudy string // comma-separated S2 fields-of-study labels
	Limit         int    // 1..100
	Offset        int
}

// PaperList is a single page of S2 paper-list responses (references / citations / search results).
type PaperList struct {
	Items  []Paper `json:"items"`
	Offset int     `json:"offset"`
	Next   int     `json:"next"` // 0 means no more
	Total  int     `json:"total,omitempty"`
}

// CitationOpts filters /paper/{id}/citations responses.
type CitationOpts struct {
	InfluentialOnly bool
}

// AutocompleteItem is a single suggestion from /paper/autocomplete.
type AutocompleteItem struct {
	PaperID     string `json:"paperId"`
	Title       string `json:"title"`
	AuthorsYear string `json:"authorsYear,omitempty"`
}

// SnippetSearchOpts narrows /snippet/search.
type SnippetSearchOpts struct {
	PaperIDs         []string // up to ~100
	Year             string
	Venue            string
	FieldsOfStudy    string
	MinCitationCount int
	Limit            int // 1..1000
}

// SnippetOffset marks the snippet's character span in the source paper.
type SnippetOffset struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Snippet is one matching text excerpt.
type Snippet struct {
	Text          string        `json:"text"`
	SnippetKind   string        `json:"snippetKind,omitempty"` // title | abstract | body
	Section       string        `json:"section,omitempty"`
	SnippetOffset SnippetOffset `json:"snippetOffset"`
}

// SnippetMatch is one row in /snippet/search results.
type SnippetMatch struct {
	PaperID string  `json:"paperId"`
	Paper   Paper   `json:"paper"`
	Snippet Snippet `json:"snippet"`
	Score   float64 `json:"score"`
}

// SnippetList wraps /snippet/search responses.
type SnippetList struct {
	Matches          []SnippetMatch `json:"matches"`
	RetrievalVersion string         `json:"retrievalVersion,omitempty"`
}
