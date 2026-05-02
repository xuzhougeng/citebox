package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
)

// AppendPaperNote appends a timestamped entry to the paper-level notes
// (PaperNotesText) of the given paper. Format mirrors appendWeixinNote so that
// new and legacy entries render identically; the surface label is generic so
// the helper works for any AI surface (web chat, WeChat, etc.).
func (s *LibraryService) AppendPaperNote(paperID int64, text string) error {
	paper, err := s.GetPaper(paperID)
	if err != nil {
		return err
	}
	if paper == nil {
		return apperr.New(apperr.CodeNotFound, "paper not found")
	}
	next := appendNoteEntry(paper.PaperNotesText, text)
	_, err = s.UpdatePaper(paperID, UpdatePaperParams{
		Title:          paper.Title,
		AbstractText:   paper.AbstractText,
		NotesText:      paper.NotesText,
		PaperNotesText: next,
		GroupID:        paper.GroupID,
		Tags:           tagNamesFromPaper(paper.Tags),
	})
	return err
}

// AppendFigureNote appends a timestamped entry to the per-figure NotesText.
func (s *LibraryService) AppendFigureNote(figureID int64, text string) error {
	figureRef, err := s.repo.GetFigure(figureID)
	if err != nil {
		return err
	}
	if figureRef == nil {
		return apperr.New(apperr.CodeNotFound, "figure not found")
	}
	paper, err := s.GetPaper(figureRef.PaperID)
	if err != nil {
		return err
	}
	if paper == nil {
		return apperr.New(apperr.CodeNotFound, "paper not found")
	}
	figure := findFigureByID(paper.Figures, figureID)
	if figure == nil {
		return apperr.New(apperr.CodeNotFound, "figure not found")
	}
	next := appendNoteEntry(figure.NotesText, text)
	_, err = s.UpdateFigure(figureID, UpdateFigureParams{NotesText: &next})
	return err
}

// appendNoteEntry mirrors appendWeixinNote but uses a generic "AI 助手" label so
// the helper can be shared across surfaces. Format: existing notes joined with
// the new entry by a blank line; the entry header is `[AI 助手 YYYY-MM-DD HH:MM]`.
func appendNoteEntry(existing, incoming string) string {
	entry := fmt.Sprintf("[AI 助手 %s]\n%s",
		time.Now().Format("2006-01-02 15:04"),
		strings.TrimSpace(incoming))
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return entry
	}
	return existing + "\n\n" + entry
}
