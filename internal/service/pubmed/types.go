package pubmed

type Paper struct {
	PMID     string
	PMCID    string
	DOI      string
	Title    string
	Abstract string
	Journal  string
	Year     int
	Authors  []string
	URL      string
}

type SearchOptions struct {
	Limit int
}

type SearchResult struct {
	Items []Paper
}
