package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// DOIImporter is the narrow surface DOICommand needs from the library service.
//
// service.LibraryService cannot satisfy this directly because its
// ImportPaperByDOI takes service.ImportPaperByDOIParams. The wiring layer
// (Task 14) is expected to provide a small adapter, e.g.:
//
//	type libraryDOIAdapter struct{ s *service.LibraryService }
//	func (a *libraryDOIAdapter) NormalizeDOI(in string) (string, error) {
//	    return a.s.NormalizeDOI(in)
//	}
//	func (a *libraryDOIAdapter) ImportPaperByDOI(ctx context.Context, p commands.ImportPaperByDOIParams) (*model.Paper, error) {
//	    return a.s.ImportPaperByDOI(ctx, service.ImportPaperByDOIParams{DOI: p.DOI})
//	}
//
// This keeps the commands subpackage decoupled from the service package.
type DOIImporter interface {
	NormalizeDOI(input string) (string, error)
	ImportPaperByDOI(ctx context.Context, params ImportPaperByDOIParams) (*model.Paper, error)
}

// ImportPaperByDOIParams is a narrow shape that mirrors the subset of
// service.ImportPaperByDOIParams the auto-import path uses.
type ImportPaperByDOIParams struct {
	DOI string
}

// DuplicateDetector is satisfied by *service.DuplicatePaperError via its
// DuplicatePaper() accessor; lets DOICommand recognize a duplicate without
// importing the service package.
type DuplicateDetector interface {
	DuplicatePaper() *model.Paper
}

// DOICommand is the agent command invoked by the bare-DOI auto-import path.
// Its name (":doi") is intentionally not user-typeable; the dispatcher routes
// to it directly when the input text looks like a DOI.
type DOICommand struct {
	Importer DOIImporter
}

// Name returns the internal command name. The leading ":" prevents collision
// with any user-typed slash command.
func (*DOICommand) Name() string { return ":doi" }

func (c *DOICommand) Execute(ctx context.Context, raw string, rt RuntimeCtx) (*Result, error) {
	if rt.State == nil {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "缺少 surface state")
	}
	if c.Importer == nil {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "DOI 导入服务未配置")
	}
	doi, err := c.Importer.NormalizeDOI(strings.TrimSpace(raw))
	if err != nil {
		return nil, apperr.New(apperr.CodeInvalidArgument, "DOI 格式无效")
	}
	if doi == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "DOI 格式无效")
	}
	paper, err := c.Importer.ImportPaperByDOI(ctx, ImportPaperByDOIParams{DOI: doi})
	if err != nil {
		var dup DuplicateDetector
		if errors.As(err, &dup) {
			existing := dup.DuplicatePaper()
			if existing != nil {
				_ = rt.State.SetCurrentPaper(rt.Surface, existing.ID, existing.Title)
				return &Result{
					UsedShortcut: true,
					ChunksText: []string{fmt.Sprintf(
						"该 DOI 已在文献库中，已切换到现有文献：\n#%d %s",
						existing.ID, strings.TrimSpace(existing.Title),
					)},
				}, nil
			}
		}
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "DOI 导入未返回文献")
	}
	_ = rt.State.SetCurrentPaper(rt.Surface, paper.ID, paper.Title)
	return &Result{
		UsedShortcut: true,
		ChunksText: []string{fmt.Sprintf(
			"已通过 DOI 导入：\n#%d %s",
			paper.ID, strings.TrimSpace(paper.Title),
		)},
	}, nil
}
