package app

import (
	"context"
	"strings"

	"github.com/xuzhougeng/citebox/internal/service"
	"github.com/xuzhougeng/citebox/internal/service/ai_external"
)

type aiExternalSettingsShim struct {
	librarySvc *service.LibraryService
}

// PubMed is always enabled (anonymous access works out of the box). Semantic
// Scholar is enabled when s2_api_key is non-empty. The legacy Sources field on
// AIExternalSearchSettings is ignored — credentials alone now drive gating.
func (s aiExternalSettingsShim) EnabledExternalSources(ctx context.Context) ([]ai_external.SourceID, error) {
	sources := []ai_external.SourceID{ai_external.SourcePubMed}

	s2Key, err := s.librarySvc.GetAppSetting("s2_api_key")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(s2Key) != "" {
		sources = append(sources, ai_external.SourceSemanticScholar)
	}
	return sources, nil
}

type librarySvcImporterShim struct {
	*service.LibraryService
}
