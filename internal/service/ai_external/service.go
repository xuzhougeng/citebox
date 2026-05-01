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

func (s *Service) EnabledExternalSources(ctx context.Context) ([]SourceID, error) {
	return s.enabledSources(ctx)
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

	type searchTask struct {
		resultIndex int
		source      SourceID
		query       string
		searcher    Searcher
	}

	results := make([]SourceResult, 0)
	tasks := make([]searchTask, 0)
	for _, source := range sources {
		source := source
		searcher := s.searcher(source)
		sourceQueries := nonblankQueries(queries[source])
		if searcher == nil {
			results = append(results, SourceResult{
				Source: source,
				Err:    fmt.Errorf("source %s is not configured", source),
			})
			continue
		}
		if len(sourceQueries) == 0 {
			results = append(results, SourceResult{
				Source: source,
				Err:    fmt.Errorf("source %s has no queries", source),
			})
			continue
		}

		for _, query := range sourceQueries {
			query := query
			resultIndex := len(results)
			results = append(results, SourceResult{
				Source: source,
				Query:  query,
			})
			tasks = append(tasks, searchTask{
				resultIndex: resultIndex,
				source:      source,
				query:       query,
				searcher:    searcher,
			})
		}
	}

	var wg sync.WaitGroup
	for _, task := range tasks {
		task := task
		wg.Add(1)
		go func() {
			defer wg.Done()
			papers, err := task.searcher.Search(ctx, task.query, opts)
			results[task.resultIndex] = SourceResult{
				Source: task.source,
				Query:  task.query,
				Papers: papers,
				Err:    err,
			}
		}()
	}
	wg.Wait()

	successfulSources := make(map[SourceID]bool)
	for _, result := range results {
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
