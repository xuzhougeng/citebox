package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/agent_session/commands"
)

// libraryDOIAdapter bridges *LibraryService into commands.DOIImporter so the
// agent_session DOI command can normalize and import a paper-by-DOI without
// importing the service package directly.
type libraryDOIAdapter struct {
	s *LibraryService
}

// NewDOIImportAdapter wraps a *LibraryService for use as commands.DOIImporter.
func NewDOIImportAdapter(s *LibraryService) commands.DOIImporter {
	return &libraryDOIAdapter{s: s}
}

func (a *libraryDOIAdapter) NormalizeDOI(input string) (string, error) {
	return a.s.NormalizeDOI(input)
}

func (a *libraryDOIAdapter) ImportPaperByDOI(ctx context.Context, p commands.ImportPaperByDOIParams) (*model.Paper, error) {
	return a.s.ImportPaperByDOI(ctx, ImportPaperByDOIParams{DOI: p.DOI})
}

// figureLookupAdapter wraps a *repository.FigureRepository so the agent_session
// FigureCommand and InterpretCommand can resolve figure metadata without
// importing the repository package directly.
type figureLookupAdapter struct {
	repo *repository.FigureRepository
}

// NewFigureLookupAdapter wraps a *FigureRepository for use as both
// commands.FigureLookup and commands.FigureOwnerLookup. Both interfaces have
// identical signatures so a single adapter satisfies both.
func NewFigureLookupAdapter(repo *repository.FigureRepository) *figureLookupAdapter {
	return &figureLookupAdapter{repo: repo}
}

func (a *figureLookupAdapter) GetFigure(id int64) (*model.FigureListItem, error) {
	return a.repo.GetFigure(id)
}

// randomFigureAdapter wraps *LibraryService so the agent_session RandomCommand
// can pick + render a random figure. It piggybacks on the same temp-file
// helper the daily-recommendation ticker uses, so the resulting preview path
// lives under the WeChat bridge state dir and gets cleaned by the same
// rotation policy. The returned cleanup is intentionally not exposed (the
// commands.RandomFigurePicker contract does not surface it); the file is
// short-lived and the caller owns delivery.
type randomFigureAdapter struct {
	s *LibraryService
}

// NewRandomFigureAdapter wraps a *LibraryService for use as
// commands.RandomFigurePicker.
func NewRandomFigureAdapter(s *LibraryService) commands.RandomFigurePicker {
	return &randomFigureAdapter{s: s}
}

func (a *randomFigureAdapter) PickRandomFigure(ctx context.Context) (int64, string, string, error) {
	figure, path, _, err := a.s.pickWeixinDailyRecommendationFigure(ctx)
	if err != nil {
		return 0, "", "", err
	}
	caption := buildRandomFigureCaption(figure)
	return figure.ID, path, caption, nil
}

func buildRandomFigureCaption(figure model.FigureListItem) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("已挑选图片 #%d", figure.ID))
	if title := strings.TrimSpace(figure.PaperTitle); title != "" {
		lines = append(lines, "文献："+title)
	}
	if label := formatFigureDisplayLabel(figure.FigureIndex, figure.SubfigureLabel); label != "" {
		lines = append(lines, "图片："+label)
	}
	if caption := clipRunes(strings.TrimSpace(figure.Caption), 160); caption != "" {
		lines = append(lines, "说明："+caption)
	}
	return strings.Join(lines, "\n")
}
