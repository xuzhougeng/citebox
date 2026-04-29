package research

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"
)

// ErrCacheRepoMiss signals "not in cache". The wrapping repo is expected to
// translate its own miss sentinel into this one before returning.
var ErrCacheRepoMiss = errors.New("research: cache repo miss")

// CacheRepo is the storage backend the Service uses for cache entries.
// Implementations should map their own miss sentinel to ErrCacheRepoMiss.
type CacheRepo interface {
	PutCache(key, payload string) error
	GetCache(key string) (string, time.Time, error)
}

// PaperFetcher is the inner client surface the Service depends on.
// Mirrors *Client.
type PaperFetcher interface {
	Search(ctx context.Context, query string, opts SearchOpts) (PaperList, error)
	Get(ctx context.Context, id string, fields []string) (Paper, error)
	References(ctx context.Context, paperID string, offset, limit int) (PaperList, error)
	Citations(ctx context.Context, paperID string, offset, limit int, opts CitationOpts) (PaperList, error)
	Recommendations(ctx context.Context, paperID string) ([]Paper, error)
	RecommendationsForList(ctx context.Context, positive, negative []string) ([]Paper, error)
}

// ServiceConfig sets cache TTLs.
type ServiceConfig struct {
	PaperTTL time.Duration // default 7 days
	RecTTL   time.Duration // default 1 day
}

// Service wraps a PaperFetcher with a CacheRepo and singleflight dedup.
type Service struct {
	client PaperFetcher
	cache  CacheRepo
	cfg    ServiceConfig
	group  singleflight.Group
}

// NewService constructs a Service. Zero TTLs become package defaults.
func NewService(client PaperFetcher, cache CacheRepo, cfg ServiceConfig) *Service {
	if cfg.PaperTTL == 0 {
		cfg.PaperTTL = 7 * 24 * time.Hour
	}
	if cfg.RecTTL == 0 {
		cfg.RecTTL = 24 * time.Hour
	}
	return &Service{client: client, cache: cache, cfg: cfg}
}

// GetPaper returns the paper, using the cache when fresh and falling back to
// stale cache on upstream errors.
func (s *Service) GetPaper(ctx context.Context, id string) (Paper, error) {
	key := paperCacheKey(id)
	v, err, _ := s.group.Do(key, func() (interface{}, error) {
		if payload, fetchedAt, err := s.cache.GetCache(key); err == nil {
			if time.Since(fetchedAt) <= s.cfg.PaperTTL {
				var p Paper
				if jsonErr := json.Unmarshal([]byte(payload), &p); jsonErr == nil {
					return p, nil
				}
			}
		}
		paper, upstreamErr := s.client.Get(ctx, id, nil)
		if upstreamErr != nil {
			// Fall back to stale cache if it exists.
			if payload, _, cacheErr := s.cache.GetCache(key); cacheErr == nil {
				var p Paper
				if jsonErr := json.Unmarshal([]byte(payload), &p); jsonErr == nil {
					return p, nil
				}
			}
			return Paper{}, upstreamErr
		}
		if buf, mErr := json.Marshal(paper); mErr == nil {
			_ = s.cache.PutCache(key, string(buf))
		}
		return paper, nil
	})
	if err != nil {
		return Paper{}, err
	}
	return v.(Paper), nil
}

// Search is a passthrough; not cached.
func (s *Service) Search(ctx context.Context, query string, opts SearchOpts) (PaperList, error) {
	return s.client.Search(ctx, query, opts)
}

// References / Citations: not cached, passthrough.
func (s *Service) References(ctx context.Context, paperID string, offset, limit int) (PaperList, error) {
	return s.client.References(ctx, paperID, offset, limit)
}

func (s *Service) Citations(ctx context.Context, paperID string, offset, limit int, opts CitationOpts) (PaperList, error) {
	return s.client.Citations(ctx, paperID, offset, limit, opts)
}

// Recommendations cached with RecTTL.
func (s *Service) Recommendations(ctx context.Context, paperID string) ([]Paper, error) {
	key := recCacheKey(paperID)
	v, err, _ := s.group.Do(key, func() (interface{}, error) {
		if payload, fetchedAt, err := s.cache.GetCache(key); err == nil {
			if time.Since(fetchedAt) <= s.cfg.RecTTL {
				var ps []Paper
				if jsonErr := json.Unmarshal([]byte(payload), &ps); jsonErr == nil {
					return ps, nil
				}
			}
		}
		papers, upstreamErr := s.client.Recommendations(ctx, paperID)
		if upstreamErr != nil {
			if payload, _, cacheErr := s.cache.GetCache(key); cacheErr == nil {
				var ps []Paper
				if jsonErr := json.Unmarshal([]byte(payload), &ps); jsonErr == nil {
					return ps, nil
				}
			}
			return nil, upstreamErr
		}
		if buf, mErr := json.Marshal(papers); mErr == nil {
			_ = s.cache.PutCache(key, string(buf))
		}
		return papers, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]Paper), nil
}

// RecommendationsForList: list responses for ad-hoc baskets aren't cached because
// the input set varies per request.
func (s *Service) RecommendationsForList(ctx context.Context, positive, negative []string) ([]Paper, error) {
	return s.client.RecommendationsForList(ctx, positive, negative)
}

func paperCacheKey(id string) string { return "paper:" + strings.TrimSpace(id) }
func recCacheKey(id string) string   { return "rec:" + strings.TrimSpace(id) }
