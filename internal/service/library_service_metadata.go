package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var (
	crossrefWorksAPIBaseURL = "https://api.crossref.org/works/"
	metadataTagPattern      = regexp.MustCompile(`<[^>]+>`)
)

type paperDOIMetadata struct {
	Title        string
	AbstractText string
	AuthorsText  string
	Journal      string
	PublishedAt  string
}

type crossrefWorkResponse struct {
	Message crossrefWorkMessage `json:"message"`
}

type crossrefWorkMessage struct {
	Title           []string          `json:"title"`
	Abstract        string            `json:"abstract"`
	Author          []crossrefAuthor  `json:"author"`
	ContainerTitle  []string          `json:"container-title"`
	PublishedPrint  crossrefDateParts `json:"published-print"`
	PublishedOnline crossrefDateParts `json:"published-online"`
	Published       crossrefDateParts `json:"published"`
	Issued          crossrefDateParts `json:"issued"`
}

type crossrefAuthor struct {
	Given  string `json:"given"`
	Family string `json:"family"`
	Name   string `json:"name"`
}

type crossrefDateParts struct {
	DateParts [][]int `json:"date-parts"`
}

func (m paperDOIMetadata) hasValues() bool {
	return strings.TrimSpace(m.Title) != "" ||
		strings.TrimSpace(m.AbstractText) != "" ||
		strings.TrimSpace(m.AuthorsText) != "" ||
		strings.TrimSpace(m.Journal) != "" ||
		strings.TrimSpace(m.PublishedAt) != ""
}

func mergePaperDOIMetadata(primary, fallback paperDOIMetadata) paperDOIMetadata {
	return paperDOIMetadata{
		Title:        firstNonEmpty(strings.TrimSpace(primary.Title), strings.TrimSpace(fallback.Title)),
		AbstractText: firstNonEmpty(strings.TrimSpace(primary.AbstractText), strings.TrimSpace(fallback.AbstractText)),
		AuthorsText:  firstNonEmpty(strings.TrimSpace(primary.AuthorsText), strings.TrimSpace(fallback.AuthorsText)),
		Journal:      firstNonEmpty(strings.TrimSpace(primary.Journal), strings.TrimSpace(fallback.Journal)),
		PublishedAt:  firstNonEmpty(strings.TrimSpace(primary.PublishedAt), strings.TrimSpace(fallback.PublishedAt)),
	}
}

func (s *LibraryService) lookupPaperDOIMetadata(ctx context.Context, doi string) (paperDOIMetadata, error) {
	metadata := paperDOIMetadata{}
	failures := make([]string, 0, 2)

	if crossrefMetadata, err := s.lookupCrossrefMetadata(ctx, doi); err == nil {
		metadata = mergePaperDOIMetadata(metadata, crossrefMetadata)
	} else {
		failures = append(failures, "crossref: "+err.Error())
	}

	if europePMCMetadata, err := s.lookupEuropePMCMetadata(ctx, doi); err == nil {
		metadata = mergePaperDOIMetadata(metadata, europePMCMetadata)
	} else {
		failures = append(failures, "europe pmc: "+err.Error())
	}

	if metadata.hasValues() || len(failures) == 0 {
		return metadata, nil
	}

	return metadata, errors.New(strings.Join(failures, "; "))
}

func (s *LibraryService) lookupCrossrefMetadata(ctx context.Context, doi string) (paperDOIMetadata, error) {
	var payload crossrefWorkResponse
	if err := s.getJSON(ctx, crossrefWorksAPIBaseURL+url.PathEscape(doi), &payload); err != nil {
		var statusErr *httpStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			return paperDOIMetadata{}, nil
		}
		return paperDOIMetadata{}, err
	}

	message := payload.Message
	return paperDOIMetadata{
		Title:        normalizeMetadataText(firstCrossrefString(message.Title)),
		AbstractText: cleanMetadataAbstract(message.Abstract),
		AuthorsText:  normalizeMetadataText(joinCrossrefAuthors(message.Author)),
		Journal:      normalizeMetadataText(firstCrossrefString(message.ContainerTitle)),
		PublishedAt: firstNonEmpty(
			formatCrossrefDateParts(message.PublishedPrint),
			formatCrossrefDateParts(message.PublishedOnline),
			formatCrossrefDateParts(message.Published),
			formatCrossrefDateParts(message.Issued),
		),
	}, nil
}

func (s *LibraryService) lookupEuropePMCMetadata(ctx context.Context, doi string) (paperDOIMetadata, error) {
	query := url.Values{}
	query.Set("query", "DOI:"+doi)
	query.Set("format", "json")
	query.Set("pageSize", "1")
	query.Set("resultType", "core")

	var payload europePMCSearchResponse
	if err := s.getJSON(ctx, europePMCSearchURL+"?"+query.Encode(), &payload); err != nil {
		return paperDOIMetadata{}, err
	}
	if len(payload.ResultList.Result) == 0 {
		return paperDOIMetadata{}, nil
	}

	result := payload.ResultList.Result[0]
	return paperDOIMetadata{
		Title:        normalizeMetadataText(result.Title),
		AbstractText: normalizeMetadataText(result.AbstractText),
		AuthorsText:  normalizeMetadataText(result.AuthorString),
		Journal:      normalizeMetadataText(result.JournalTitle),
		PublishedAt: firstNonEmpty(
			normalizeMetadataText(result.FirstPublicationDate),
			normalizeMetadataText(result.PubYear),
		),
	}, nil
}

func firstCrossrefString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func joinCrossrefAuthors(authors []crossrefAuthor) string {
	names := make([]string, 0, len(authors))
	for _, author := range authors {
		name := strings.TrimSpace(author.Name)
		if name == "" {
			name = strings.TrimSpace(strings.TrimSpace(author.Given) + " " + strings.TrimSpace(author.Family))
		}
		if name == "" {
			name = firstNonEmpty(strings.TrimSpace(author.Family), strings.TrimSpace(author.Given))
		}
		name = normalizeMetadataText(name)
		if name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

func formatCrossrefDateParts(value crossrefDateParts) string {
	if len(value.DateParts) == 0 || len(value.DateParts[0]) == 0 {
		return ""
	}

	parts := value.DateParts[0]
	switch len(parts) {
	case 1:
		return fmt.Sprintf("%04d", parts[0])
	case 2:
		return fmt.Sprintf("%04d-%02d", parts[0], parts[1])
	default:
		return fmt.Sprintf("%04d-%02d-%02d", parts[0], parts[1], parts[2])
	}
}

func cleanMetadataAbstract(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	replacer := strings.NewReplacer(
		"</jats:p>", "\n\n",
		"<jats:p>", "",
		"</p>", "\n\n",
		"<p>", "",
		"</jats:title>", "\n",
		"<jats:title>", "",
		"</title>", "\n",
		"<title>", "",
	)
	raw = replacer.Replace(raw)
	raw = metadataTagPattern.ReplaceAllString(raw, "")
	return normalizeMetadataText(raw)
}

func normalizeMetadataText(raw string) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	if raw == "" {
		return ""
	}

	raw = strings.ReplaceAll(raw, "\u00a0", " ")
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")

	lines := strings.Split(raw, "\n")
	normalizedLines := make([]string, 0, len(lines))
	lastBlank := false
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if len(normalizedLines) > 0 && !lastBlank {
				normalizedLines = append(normalizedLines, "")
			}
			lastBlank = true
			continue
		}
		normalizedLines = append(normalizedLines, line)
		lastBlank = false
	}

	return strings.TrimSpace(strings.Join(normalizedLines, "\n"))
}
