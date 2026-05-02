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

// Semantic Scholar enable derives from the s2_api_key app setting (single
// switch in the research settings panel). Legacy "semantic_scholar" entries
// in Sources are ignored at the gate so old data stays valid.
func (s aiExternalSettingsShim) EnabledExternalSources(ctx context.Context) ([]ai_external.SourceID, error) {
	settings, err := s.librarySvc.GetAIExternalSearchSettings()
	if err != nil {
		return nil, err
	}

	seen := make(map[ai_external.SourceID]bool, 2)
	sources := make([]ai_external.SourceID, 0, 2)
	for _, raw := range settings.Sources {
		sid := ai_external.SourceID(raw)
		if sid == ai_external.SourceSemanticScholar {
			continue
		}
		if seen[sid] {
			continue
		}
		seen[sid] = true
		sources = append(sources, sid)
	}

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
