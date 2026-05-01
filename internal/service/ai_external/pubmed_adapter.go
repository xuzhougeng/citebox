package ai_external

import (
	"context"
	"errors"

	"github.com/xuzhougeng/citebox/internal/service/pubmed"
)

type PubMedSearcher interface {
	Search(ctx context.Context, query string, opts pubmed.SearchOptions) (pubmed.SearchResult, error)
}

type PubMedAdapter struct {
	Client PubMedSearcher
}

func (a PubMedAdapter) Search(ctx context.Context, query string, opts SearchOptions) ([]Paper, error) {
	if a.Client == nil {
		return nil, errors.New("pubmed client is not configured")
	}

	res, err := a.Client.Search(ctx, query, pubmed.SearchOptions{Limit: opts.Limit})
	if err != nil {
		return nil, err
	}
	out := make([]Paper, 0, len(res.Items))
	for _, p := range res.Items {
		out = append(out, Paper{
			Source:        SourcePubMed,
			SourcePaperID: p.PMID,
			PMID:          p.PMID,
			PMCID:         p.PMCID,
			DOI:           p.DOI,
			Title:         p.Title,
			Abstract:      p.Abstract,
			Venue:         p.Journal,
			Year:          p.Year,
			OnlineYear:    p.OnlineYear,
			IssueYear:     p.IssueYear,
			YearLabel:     p.YearLabel,
			Authors:       p.Authors,
			URL:           p.URL,
		})
	}
	return out, nil
}
