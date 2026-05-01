package ai_external

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var ErrNoSourcesEnabled = errors.New("ai_external: no external search sources enabled")

type SettingsProvider interface {
	EnabledExternalSources(ctx context.Context) ([]SourceID, error)
}

type SourceFailure struct {
	Source SourceID
	Err    error
}

type SearchResult struct {
	Papers   []Paper
	Results  []SourceResult
	Failures []SourceFailure
	Sources  []SourceID
}

type Service struct {
	settings  SettingsProvider
	searchers map[SourceID]Searcher
}

func NewService(settings SettingsProvider, searchers map[SourceID]Searcher) *Service {
	return &Service{settings: settings, searchers: searchers}
}

func (s *Service) Search(ctx context.Context, queries SourceQueries, opts SearchOptions) (SearchResult, error) {
	sources, err := s.enabledSources(ctx)
	if err != nil {
		return SearchResult{}, err
	}
	if len(sources) == 0 {
		return SearchResult{}, ErrNoSourcesEnabled
	}

	out := SearchResult{
		Sources: sources,
	}

	var wg sync.WaitGroup
	results := make(chan SourceResult, resultBufferSize(sources, queries))
	for _, source := range sources {
		source := source
		searcher := s.searcher(source)
		sourceQueries := nonblankQueries(queries[source])
		if searcher == nil {
			results <- SourceResult{
				Source: source,
				Err:    fmt.Errorf("source %s is not configured", source),
			}
			continue
		}
		if len(sourceQueries) == 0 {
			results <- SourceResult{
				Source: source,
				Err:    fmt.Errorf("source %s has no queries", source),
			}
			continue
		}

		for _, query := range sourceQueries {
			query := query
			wg.Add(1)
			go func() {
				defer wg.Done()
				papers, err := searcher.Search(ctx, query, opts)
				results <- SourceResult{
					Source: source,
					Query:  query,
					Papers: papers,
					Err:    err,
				}
			}()
		}
	}
	wg.Wait()
	close(results)

	successfulSources := make(map[SourceID]bool)
	for result := range results {
		out.Results = append(out.Results, result)
		if result.Err != nil {
			out.Failures = append(out.Failures, SourceFailure{
				Source: result.Source,
				Err:    result.Err,
			})
			continue
		}
		successfulSources[result.Source] = true
	}

	out.Papers = MergePapers(out.Results, sources, opts.Limit)
	if len(successfulSources) == 0 {
		return out, allSourcesFailedError(out.Failures)
	}
	return out, nil
}

func (s *Service) enabledSources(ctx context.Context) ([]SourceID, error) {
	if s == nil || s.settings == nil {
		return []SourceID{SourcePubMed}, nil
	}
	return s.settings.EnabledExternalSources(ctx)
}

func (s *Service) searcher(source SourceID) Searcher {
	if s == nil || s.searchers == nil {
		return nil
	}
	return s.searchers[source]
}

func nonblankQueries(queries []string) []string {
	out := make([]string, 0, len(queries))
	for _, query := range queries {
		if trimmed := strings.TrimSpace(query); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func resultBufferSize(sources []SourceID, queries SourceQueries) int {
	size := 0
	for _, source := range sources {
		if count := len(nonblankQueries(queries[source])); count > 0 {
			size += count
			continue
		}
		size++
	}
	return size
}

func allSourcesFailedError(failures []SourceFailure) error {
	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		if failure.Err == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %v", failure.Source, failure.Err))
	}
	return fmt.Errorf("ai_external: all external search sources failed: %s", strings.Join(parts, "; "))
}
