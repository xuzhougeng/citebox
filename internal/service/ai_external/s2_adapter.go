package ai_external

import (
	"context"
	"errors"

	"github.com/xuzhougeng/citebox/internal/service/research"
)

type SemanticScholarSearcher interface {
	Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error)
}

type SemanticScholarAdapter struct {
	SearchService SemanticScholarSearcher
}

func (a SemanticScholarAdapter) Search(ctx context.Context, query string, opts SearchOptions) ([]Paper, error) {
	if a.SearchService == nil {
		return nil, errors.New("semantic scholar search service is not configured")
	}

	list, err := a.SearchService.Search(ctx, query, research.SearchOpts{Limit: opts.Limit})
	if err != nil {
		return nil, err
	}
	out := make([]Paper, 0, len(list.Items))
	for _, p := range list.Items {
		authors := make([]string, 0, len(p.Authors))
		for _, author := range p.Authors {
			if author.Name != "" {
				authors = append(authors, author.Name)
			}
		}
		out = append(out, Paper{
			Source:        SourceSemanticScholar,
			SourcePaperID: p.PaperID,
			DOI:           p.ExternalIDs.DOI,
			ArXiv:         p.ExternalIDs.ArXiv,
			PMID:          p.ExternalIDs.PubMed,
			Title:         p.Title,
			Abstract:      p.Abstract,
			TLDR:          p.TLDR,
			Venue:         p.Venue,
			Year:          p.Year,
			Authors:       authors,
			OpenAccessURL: p.OpenAccessPDFURL,
			CitationCount: p.CitationCount,
		})
	}
	return out, nil
}
