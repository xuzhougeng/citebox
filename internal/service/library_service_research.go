package service

import (
	"context"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

// ImportPaperFromS2 creates a metadata-only paper record from a Semantic Scholar
// Paper. If a paper with the same DOI already exists, returns the existing
// record (no duplicate, no error).
func (s *LibraryService) ImportPaperFromS2(ctx context.Context, p research.Paper) (*model.Paper, error) {
	if dup, err := s.findDuplicateByDOI(strings.TrimSpace(p.ExternalIDs.DOI)); err != nil {
		return nil, err
	} else if dup != nil {
		return dup.Paper, nil
	}

	authors := make([]string, 0, len(p.Authors))
	for _, a := range p.Authors {
		if a.Name != "" {
			authors = append(authors, a.Name)
		}
	}
	authorsText := strings.Join(authors, ", ")

	publishedAt := ""
	if p.Year > 0 {
		publishedAt = strconvItoa(p.Year)
	}

	input := repository.PaperUpsertInput{
		Title:            p.Title,
		DOI:              p.ExternalIDs.DOI,
		AuthorsText:      authorsText,
		Journal:          p.Venue,
		PublishedAt:      publishedAt,
		AbstractText:     p.Abstract,
		OriginalFilename: "",
		StoredPDFName:    "",
		ExtractionStatus: "manual_pending",
	}

	paper, err := s.repo.CreatePaper(input)
	if err != nil {
		return nil, err
	}
	s.decoratePaper(paper)
	return paper, nil
}

// strconvItoa converts a non-negative integer to its decimal string representation.
// It is kept here as a local helper to avoid importing strconv solely for this use.
func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + strconvItoa(-n)
	}
	rev := []byte{}
	for n > 0 {
		rev = append(rev, byte('0'+n%10))
		n /= 10
	}
	digits := make([]byte, len(rev))
	for i, b := range rev {
		digits[len(rev)-1-i] = b
	}
	return string(digits)
}
