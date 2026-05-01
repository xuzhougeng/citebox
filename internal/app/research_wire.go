package app

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/service"
	"github.com/xuzhougeng/citebox/internal/service/ai_external"
)

type aiExternalSettingsShim struct {
	librarySvc *service.LibraryService
}

func (s aiExternalSettingsShim) EnabledExternalSources(ctx context.Context) ([]ai_external.SourceID, error) {
	settings, err := s.librarySvc.GetAIExternalSearchSettings()
	if err != nil {
		return nil, err
	}

	sources := make([]ai_external.SourceID, 0, len(settings.Sources))
	for _, source := range settings.Sources {
		sources = append(sources, ai_external.SourceID(source))
	}
	return sources, nil
}

type librarySvcImporterShim struct {
	*service.LibraryService
}
