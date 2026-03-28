package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

const (
	oaLookupTimeout   = 20 * time.Second
	oaDownloadTimeout = 2 * time.Minute
)

var (
	unpaywallAPIBaseURL = "https://api.unpaywall.org/v2/"
	europePMCSearchURL  = "https://www.ebi.ac.uk/europepmc/webservices/rest/search"
	pmcIDConvURL        = "https://pmc.ncbi.nlm.nih.gov/tools/idconv/api/v1/articles/"
)

var doiPattern = regexp.MustCompile(`(?i)^10\.\d{4,9}/\S+$`)

type oaPDFCandidate struct {
	Provider string
	Title    string
	URL      string
}

type remotePDFDownload struct {
	Filename      string
	ContentType   string
	ContentLength int64
	Body          io.ReadCloser
}

type cancelOnCloseReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelOnCloseReadCloser) Close() error {
	err := r.ReadCloser.Close()
	if r.cancel != nil {
		r.cancel()
	}
	return err
}

type unpaywallResponse struct {
	DOI            string                `json:"doi"`
	Title          string                `json:"title"`
	BestOALocation *unpaywallOALocation  `json:"best_oa_location"`
	OALocations    []unpaywallOALocation `json:"oa_locations"`
}

type unpaywallOALocation struct {
	URL       string `json:"url"`
	URLForPDF string `json:"url_for_pdf"`
}

type europePMCSearchResponse struct {
	ResultList struct {
		Result []europePMCResult `json:"result"`
	} `json:"resultList"`
}

type europePMCResult struct {
	DOI                  string `json:"doi"`
	Title                string `json:"title"`
	AuthorString         string `json:"authorString"`
	JournalTitle         string `json:"journalTitle"`
	FirstPublicationDate string `json:"firstPublicationDate"`
	PubYear              string `json:"pubYear"`
	AbstractText         string `json:"abstractText"`
	PMCID                string `json:"pmcid"`
	IsOpenAccess         string `json:"isOpenAccess"`
	HasPDF               string `json:"hasPDF"`
	FullTextURLList      struct {
		FullTextURL []europePMCFullTextURL `json:"fullTextUrl"`
	} `json:"fullTextUrlList"`
}

type europePMCFullTextURL struct {
	URL              string `json:"url"`
	DocumentStyle    string `json:"documentStyle"`
	AvailabilityCode string `json:"availabilityCode"`
}

type pmcIDConvResponse struct {
	Records []pmcIDConvRecord `json:"records"`
}

type pmcIDConvRecord struct {
	DOI   string `json:"doi"`
	PMCID string `json:"pmcid"`
}

func (s *LibraryService) ImportPaperByDOI(ctx context.Context, params ImportPaperByDOIParams) (*model.Paper, error) {
	doi, err := normalizeDOIInput(params.DOI)
	if err != nil {
		return nil, err
	}
	if doi == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "请先输入 DOI")
	}

	if duplicate, err := s.findDuplicateByDOI(doi); err != nil {
		return nil, err
	} else if duplicate != nil {
		return nil, duplicate
	}

	metadata, metadataErr := s.lookupPaperDOIMetadata(ctx, doi)
	if metadataErr != nil {
		s.logger.Warn("doi metadata lookup failed", "doi", doi, "error", metadataErr)
	}

	candidates, titleHint, err := s.resolveOpenAccessPDFCandidates(ctx, doi)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, apperr.New(apperr.CodeNotFound, "未找到可合法下载的 Open Access PDF")
	}

	var lastDownloadErr error
	for _, candidate := range candidates {
		download, err := s.downloadOpenAccessPDF(ctx, doi, candidate)
		if err != nil {
			lastDownloadErr = apperr.New(apperr.CodeUnavailable, "找到了开放获取记录，但 PDF 下载失败")
			continue
		}

		uploadParams := UploadPaperParams{
			Title:          firstNonEmpty(strings.TrimSpace(params.Title), strings.TrimSpace(metadata.Title), strings.TrimSpace(titleHint), strings.TrimSpace(candidate.Title)),
			DOI:            doi,
			AuthorsText:    metadata.AuthorsText,
			Journal:        metadata.Journal,
			PublishedAt:    metadata.PublishedAt,
			AbstractText:   metadata.AbstractText,
			GroupID:        params.GroupID,
			Tags:           params.Tags,
			ExtractionMode: params.ExtractionMode,
		}
		paper, uploadErr := s.uploadPaperFromReader(download.Body, paperUploadSource{
			Filename:     download.Filename,
			ContentType:  download.ContentType,
			DeclaredSize: download.ContentLength,
			DOI:          doi,
			TitleHint:    firstNonEmpty(strings.TrimSpace(candidate.Title), strings.TrimSpace(metadata.Title), strings.TrimSpace(titleHint)),
		}, uploadParams)
		_ = download.Body.Close()
		if uploadErr != nil {
			if apperr.IsCode(uploadErr, apperr.CodeUnsupportedMedia) {
				lastDownloadErr = apperr.New(apperr.CodeUnavailable, "找到了开放获取记录，但 PDF 下载失败")
				continue
			}
			return nil, uploadErr
		}
		return paper, nil
	}

	if lastDownloadErr != nil {
		return nil, lastDownloadErr
	}
	return nil, apperr.New(apperr.CodeNotFound, "未找到可合法下载的 Open Access PDF")
}

func (s *LibraryService) resolveOpenAccessPDFCandidates(ctx context.Context, doi string) ([]oaPDFCandidate, string, error) {
	allCandidates := make([]oaPDFCandidate, 0, 8)
	seen := map[string]struct{}{}
	titleHint := ""

	appendCandidates := func(candidates []oaPDFCandidate) {
		for _, candidate := range candidates {
			targetURL := strings.TrimSpace(candidate.URL)
			if targetURL == "" {
				continue
			}
			if _, ok := seen[targetURL]; ok {
				continue
			}
			seen[targetURL] = struct{}{}
			allCandidates = append(allCandidates, candidate)
			if titleHint == "" {
				titleHint = strings.TrimSpace(candidate.Title)
			}
		}
	}

	if candidates, err := s.lookupUnpaywallCandidates(ctx, doi); err == nil {
		appendCandidates(candidates)
	} else {
		s.logger.Warn("unpaywall lookup failed", "doi", doi, "error", err)
	}

	europePMCCandidates, europePMCTitle, europePMCErr := s.lookupEuropePMCCandidates(ctx, doi)
	if europePMCErr == nil {
		appendCandidates(europePMCCandidates)
		if titleHint == "" {
			titleHint = europePMCTitle
		}
	} else {
		s.logger.Warn("europe pmc lookup failed", "doi", doi, "error", europePMCErr)
	}

	pmcCandidates, pmcTitle, pmcErr := s.lookupPMCIDCandidates(ctx, doi)
	if pmcErr == nil {
		appendCandidates(pmcCandidates)
		if titleHint == "" {
			titleHint = pmcTitle
		}
	} else {
		s.logger.Warn("pmc id converter lookup failed", "doi", doi, "error", pmcErr)
	}

	if len(allCandidates) == 0 {
		if europePMCErr != nil && pmcErr != nil && strings.TrimSpace(s.config.OAContactEmail) != "" {
			return nil, "", apperr.New(apperr.CodeUnavailable, "Open Access 检索服务暂不可用，请稍后重试")
		}
		return nil, titleHint, nil
	}

	return allCandidates, titleHint, nil
}

func (s *LibraryService) lookupUnpaywallCandidates(ctx context.Context, doi string) ([]oaPDFCandidate, error) {
	email := strings.TrimSpace(s.config.OAContactEmail)
	if email == "" {
		return nil, nil
	}

	targetURL := unpaywallAPIBaseURL + url.PathEscape(doi) + "?email=" + url.QueryEscape(email)
	var payload unpaywallResponse
	if err := s.getJSON(ctx, targetURL, &payload); err != nil {
		var statusErr *httpStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}

	candidates := make([]oaPDFCandidate, 0, 4)
	appendLocation := func(location *unpaywallOALocation) {
		if location == nil {
			return
		}
		for _, candidateURL := range []string{location.URLForPDF, location.URL} {
			candidateURL = strings.TrimSpace(candidateURL)
			if candidateURL == "" {
				continue
			}
			candidates = append(candidates, oaPDFCandidate{
				Provider: "unpaywall",
				Title:    strings.TrimSpace(payload.Title),
				URL:      candidateURL,
			})
		}
	}

	appendLocation(payload.BestOALocation)
	for i := range payload.OALocations {
		appendLocation(&payload.OALocations[i])
	}

	return candidates, nil
}

func (s *LibraryService) lookupEuropePMCCandidates(ctx context.Context, doi string) ([]oaPDFCandidate, string, error) {
	query := url.Values{}
	query.Set("query", "DOI:"+doi)
	query.Set("format", "json")
	query.Set("pageSize", "5")
	query.Set("resultType", "core")

	var payload europePMCSearchResponse
	if err := s.getJSON(ctx, europePMCSearchURL+"?"+query.Encode(), &payload); err != nil {
		return nil, "", err
	}

	candidates := make([]oaPDFCandidate, 0, 6)
	titleHint := ""
	for _, result := range payload.ResultList.Result {
		if titleHint == "" {
			titleHint = strings.TrimSpace(result.Title)
		}
		for _, fullTextURL := range result.FullTextURLList.FullTextURL {
			availability := strings.TrimSpace(strings.ToUpper(fullTextURL.AvailabilityCode))
			style := strings.TrimSpace(strings.ToLower(fullTextURL.DocumentStyle))
			if availability != "" && availability != "OA" {
				continue
			}
			if style == "pdf" || strings.Contains(strings.ToLower(fullTextURL.URL), "/pdf") {
				candidates = append(candidates, oaPDFCandidate{
					Provider: "europe_pmc",
					Title:    strings.TrimSpace(result.Title),
					URL:      strings.TrimSpace(fullTextURL.URL),
				})
			}
		}
		if result.PMCID != "" && (strings.EqualFold(result.IsOpenAccess, "Y") || strings.EqualFold(result.HasPDF, "Y")) {
			candidates = append(candidates, oaPDFCandidate{
				Provider: "pmc",
				Title:    strings.TrimSpace(result.Title),
				URL:      pmcPDFURL(result.PMCID),
			})
		}
	}
	return candidates, titleHint, nil
}

func (s *LibraryService) lookupPMCIDCandidates(ctx context.Context, doi string) ([]oaPDFCandidate, string, error) {
	query := url.Values{}
	query.Set("ids", doi)
	query.Set("format", "json")
	if email := strings.TrimSpace(s.config.OAContactEmail); email != "" {
		query.Set("email", email)
	}
	query.Set("tool", "CiteBox")

	var payload pmcIDConvResponse
	if err := s.getJSON(ctx, pmcIDConvURL+"?"+query.Encode(), &payload); err != nil {
		return nil, "", err
	}

	candidates := make([]oaPDFCandidate, 0, 2)
	for _, record := range payload.Records {
		if strings.TrimSpace(record.PMCID) == "" {
			continue
		}
		candidates = append(candidates, oaPDFCandidate{
			Provider: "pmc",
			Title:    "",
			URL:      pmcPDFURL(record.PMCID),
		})
	}
	return candidates, "", nil
}

func pmcPDFURL(pmcid string) string {
	pmcid = strings.TrimSpace(pmcid)
	if pmcid == "" {
		return ""
	}
	return "https://pmc.ncbi.nlm.nih.gov/articles/" + url.PathEscape(pmcid) + "/pdf"
}

func (s *LibraryService) downloadOpenAccessPDF(ctx context.Context, doi string, candidate oaPDFCandidate) (*remotePDFDownload, error) {
	targetURL, err := validateRemoteDownloadURL(candidate.URL)
	if err != nil {
		return nil, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, oaDownloadTimeout)
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		cancel()
		return nil, apperr.Wrap(apperr.CodeInternal, "创建 PDF 下载请求失败", err)
	}
	request.Header.Set("Accept", "application/pdf, application/octet-stream;q=0.9, */*;q=0.1")
	request.Header.Set("User-Agent", oaRequestUserAgent(s.config.OAContactEmail))

	response, err := s.httpClient.Do(request)
	if err != nil {
		cancel()
		return nil, mapOARequestError(err, "Open Access PDF 下载失败")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_ = response.Body.Close()
		cancel()
		if response.StatusCode == http.StatusNotFound {
			return nil, apperr.New(apperr.CodeNotFound, "未找到可合法下载的 Open Access PDF")
		}
		return nil, apperr.New(apperr.CodeUnavailable, "找到了开放获取记录，但 PDF 下载失败")
	}

	if response.ContentLength > s.config.MaxUploadSize && response.ContentLength > 0 {
		_ = response.Body.Close()
		cancel()
		return nil, apperr.New(apperr.CodeResourceExhausted, fmt.Sprintf("PDF 大小超过限制 %s", humanFileSize(s.config.MaxUploadSize)))
	}

	filename := filenameFromContentDisposition(response.Header.Get("Content-Disposition"))
	if filename == "" {
		filename = filenameFromURL(response.Request.URL.String())
	}
	if filename == "" {
		filename = sanitizeDOIForFilename(doi)
	}

	return &remotePDFDownload{
		Filename:      filename,
		ContentType:   response.Header.Get("Content-Type"),
		ContentLength: response.ContentLength,
		Body: &cancelOnCloseReadCloser{
			ReadCloser: response.Body,
			cancel:     cancel,
		},
	}, nil
}

func normalizeDOIInput(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	raw = strings.Trim(raw, "<>")
	raw = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(raw, "."), ","), ";")
	raw = strings.TrimSpace(raw)

	if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" && strings.HasSuffix(strings.ToLower(parsed.Host), "doi.org") {
		raw = strings.Trim(parsed.Path, "/")
	}

	raw = strings.TrimSpace(raw)
	if len(raw) >= 4 && strings.EqualFold(raw[:4], "doi:") {
		raw = strings.TrimSpace(raw[4:])
	}
	raw = strings.TrimLeft(raw, "/")
	raw = strings.ToLower(strings.TrimSpace(raw))
	if !doiPattern.MatchString(raw) {
		return "", apperr.New(apperr.CodeInvalidArgument, "DOI 格式无效")
	}
	return raw, nil
}

func validateRemoteDownloadURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", apperr.New(apperr.CodeUnavailable, "找到了开放获取记录，但 PDF 下载失败")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", apperr.New(apperr.CodeUnavailable, "找到了开放获取记录，但 PDF 下载失败")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", apperr.New(apperr.CodeUnavailable, "找到了开放获取记录，但 PDF 下载失败")
	}
	if host := strings.ToLower(parsed.Hostname()); host == "" || host == "localhost" {
		return "", apperr.New(apperr.CodeUnavailable, "找到了开放获取记录，但 PDF 下载失败")
	}
	return parsed.String(), nil
}

type httpStatusError struct {
	StatusCode int
}

func (e *httpStatusError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("unexpected status: %d", e.StatusCode)
}

func (s *LibraryService) getJSON(ctx context.Context, targetURL string, out interface{}) error {
	requestCtx, cancel := context.WithTimeout(ctx, oaLookupTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "创建 Open Access 查询请求失败", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", oaRequestUserAgent(s.config.OAContactEmail))

	response, err := s.httpClient.Do(request)
	if err != nil {
		return mapOARequestError(err, "Open Access 检索失败")
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &httpStatusError{StatusCode: response.StatusCode}
	}

	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return apperr.Wrap(apperr.CodeUnavailable, "Open Access 检索失败", err)
	}
	return nil
}

func mapOARequestError(err error, message string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return apperr.New(apperr.CodeDeadlineExceeded, message)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return apperr.New(apperr.CodeDeadlineExceeded, message)
	}
	return apperr.Wrap(apperr.CodeUnavailable, message, err)
}

func oaRequestUserAgent(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return "CiteBox/1.0"
	}
	return "CiteBox/1.0 (" + email + ")"
}

func filenameFromContentDisposition(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}

	if _, params, err := mime.ParseMediaType(header); err == nil {
		if filename := strings.TrimSpace(params["filename*"]); filename != "" {
			if decoded, decodeErr := url.QueryUnescape(strings.TrimPrefix(filename, "UTF-8''")); decodeErr == nil && strings.TrimSpace(decoded) != "" {
				return normalizePaperOriginalFilename(decoded, "")
			}
		}
		if filename := strings.TrimSpace(params["filename"]); filename != "" {
			return normalizePaperOriginalFilename(filename, "")
		}
	}
	return ""
}

func filenameFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	base := strings.TrimSpace(path.Base(parsed.Path))
	if base == "" || base == "." || base == "/" {
		return ""
	}
	return normalizePaperOriginalFilename(base, "")
}
