package app

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/service"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

// librarySvcImporterShim adapts *service.LibraryService to
// research.LibraryImporter so that the basket can import papers into the local
// library without creating an import cycle.
type librarySvcImporterShim struct {
	svc *service.LibraryService
}

func (s librarySvcImporterShim) ImportPaperFromS2WithID(ctx context.Context, p research.Paper) (int64, error) {
	return s.svc.ImportPaperFromS2WithID(ctx, p)
}
