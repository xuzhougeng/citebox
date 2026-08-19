package service

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/buildinfo"
	"github.com/xuzhougeng/citebox/internal/model"
)

const (
	FigureTransferSchemaName    = "figure-transfer-package.v2"
	FigureTransferSchemaVersion = 2
	figureTransferManifestName  = "manifest.json"
	maxTransferManifestSize     = 1 << 20
	maxTransferContextSize      = 1 << 20
	maxTransferHandoffSize      = 1 << 20
	maxTransferImageSize        = 20 << 20
	maxTransferTitleRunes       = 2000
	maxTransferAuthorRunes      = 300
	maxTransferCaptionRunes     = 20000
)

// figureTransferMediaTypes maps the media types accepted by Figure Transfer
// Package consumers to the package-internal file extension they require.
var figureTransferMediaTypes = map[string]string{
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
	"image/webp":      ".webp",
	"image/svg+xml":   ".svg",
	"application/pdf": ".pdf",
}

type FigureTransferManifest struct {
	Schema     string                 `json:"schema"`
	Version    int                    `json:"version"`
	Producer   FigureTransferProducer `json:"producer"`
	ExportedAt string                 `json:"exportedAt"`
	Entry      string                 `json:"entry"`
	Source     FigureTransferSource   `json:"source"`
	Figure     FigureTransferFigure   `json:"figure"`
	Files      []FigureTransferFile   `json:"files"`
}

type FigureTransferProducer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type FigureTransferSource struct {
	SourceID         string                `json:"sourceId"`
	FigureID         int64                 `json:"figureId"`
	ParentFigureID   *int64                `json:"parentFigureId"`
	FigureLabel      string                `json:"figureLabel"`
	OfficialLabel    string                `json:"officialLabel"`
	SubfigureLabels  []string              `json:"subfigureLabels"`
	Caption          string                `json:"caption"`
	Page             *int                  `json:"page"`
	Paper            FigureTransferPaper   `json:"paper"`
	License          FigureTransferLicense `json:"license"`
	ExtractionMethod string                `json:"extractionMethod"`
}

type FigureTransferPaper struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	Year        *int     `json:"year"`
	PublishedAt string   `json:"publishedAt,omitempty"`
	Journal     *string  `json:"journal"`
	DOI         *string  `json:"doi"`
	URL         *string  `json:"url"`
}

type FigureTransferLicense struct {
	Scope string  `json:"scope"`
	Text  *string `json:"text"`
}

type FigureTransferFigure struct {
	File      string `json:"file"`
	MediaType string `json:"mediaType"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
	Kind      string `json:"kind"`
	Number    int    `json:"number"`
}

type FigureTransferFile struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	MediaType string `json:"mediaType"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
}

type FigureTransferPackage struct {
	Filename string
	Data     []byte
	Manifest FigureTransferManifest
}

func (s *LibraryService) ExportFigureTransferPackage(id int64) (*FigureTransferPackage, error) {
	figureRef, err := s.repo.GetFigure(id)
	if err != nil {
		return nil, err
	}
	if figureRef == nil {
		return nil, apperr.New(apperr.CodeNotFound, "figure not found")
	}

	paper, err := s.repo.GetPaperDetail(figureRef.PaperID)
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	figure := findFigureByID(paper.Figures, id)
	if figure == nil {
		return nil, apperr.New(apperr.CodeNotFound, "figure not found")
	}

	imageData, mediaType, err := loadFigureImageData(s.config.FiguresDir(), paper.Figures, *figure)
	if err != nil {
		return nil, err
	}
	if len(imageData) == 0 {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "figure image is empty")
	}

	extension, ok := figureTransferMediaTypes[normalizeTransferMediaType(mediaType)]
	if !ok {
		return nil, apperr.New(apperr.CodeFailedPrecondition, fmt.Sprintf("figure media type %q is not supported by Figure Transfer Package v2", mediaType))
	}

	pageTexts, err := s.repo.GetPaperPDFPageTexts(paper.ID)
	if err != nil {
		return nil, err
	}

	exportedAt := time.Now().UTC().Truncate(time.Second)
	imageFilename := "figure" + extension
	contextDoc := buildFigureResearchContext(paper, figure, pageTexts)
	contextData, err := marshalTransferJSON(contextDoc)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "生成 research-context.json 失败", err)
	}
	handoffData := []byte(renderFigureHandoffMarkdown(contextDoc, imageFilename))
	files := []FigureTransferFile{
		newFigureTransferFile(imageFilename, "figure", normalizeTransferMediaType(mediaType), imageData),
		newFigureTransferFile(figureTransferContextName, "research_context", "application/json", contextData),
		newFigureTransferFile(figureTransferHandoffName, "handoff", "text/markdown", handoffData),
	}
	manifest := newFigureTransferManifest(paper, figure, contextDoc, imageFilename, normalizeTransferMediaType(mediaType), int64(len(imageData)), files[0].SHA256, exportedAt, files)

	payload := map[string][]byte{
		imageFilename:             imageData,
		figureTransferContextName: contextData,
		figureTransferHandoffName: handoffData,
	}
	data, err := writeFigureTransferPackage(manifest, payload, exportedAt)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "生成 Figure Transfer Package 失败", err)
	}
	if _, err := ValidateFigureTransferPackage(data); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "生成的 Figure Transfer Package 校验失败", err)
	}

	return &FigureTransferPackage{
		Filename: fmt.Sprintf("citebox-figure-%d-transfer-package.zip", id),
		Data:     data,
		Manifest: manifest,
	}, nil
}

func newFigureTransferManifest(
	paper *model.Paper,
	figure *model.Figure,
	contextDoc FigureResearchContext,
	imageFilename string,
	mediaType string,
	byteSize int64,
	imageSHA256 string,
	exportedAt time.Time,
	files []FigureTransferFile,
) FigureTransferManifest {
	subfigureLabels := []string{}
	if figure.ParentFigureID != nil {
		if label := strings.TrimSpace(figure.SubfigureLabel); label != "" {
			subfigureLabels = append(subfigureLabels, label)
		}
	} else {
		for _, candidate := range paper.Figures {
			if candidate.ParentFigureID == nil || *candidate.ParentFigureID != figure.ID {
				continue
			}
			if label := strings.TrimSpace(candidate.SubfigureLabel); label != "" {
				subfigureLabels = append(subfigureLabels, label)
			}
		}
	}

	var page *int
	if figure.PageNumber > 0 {
		value := figure.PageNumber
		page = &value
	}

	return FigureTransferManifest{
		Schema:  FigureTransferSchemaName,
		Version: FigureTransferSchemaVersion,
		Producer: FigureTransferProducer{
			Name:    "CiteBox",
			Version: buildinfo.CurrentVersion(),
		},
		ExportedAt: exportedAt.Format(time.RFC3339),
		Entry:      figureTransferHandoffName,
		Source: FigureTransferSource{
			SourceID:        figureTransferSourceID(figure.ID),
			FigureID:        figure.ID,
			ParentFigureID:  figure.ParentFigureID,
			FigureLabel:     contextDoc.Figure.DisplayLabel,
			OfficialLabel:   contextDoc.Figure.OfficialLabel,
			SubfigureLabels: subfigureLabels,
			Caption:         truncateTransferText(strings.TrimSpace(figure.Caption), maxTransferCaptionRunes),
			Page:            page,
			Paper: FigureTransferPaper{
				ID:          paper.ID,
				Title:       truncateTransferText(strings.TrimSpace(paper.Title), maxTransferTitleRunes),
				Authors:     figureTransferAuthors(paper.AuthorsText),
				Year:        figureTransferPublicationYear(paper.PublishedAt),
				PublishedAt: strings.TrimSpace(paper.PublishedAt),
				Journal:     optionalTransferString(paper.Journal),
				DOI:         optionalTransferString(paper.DOI),
				URL:         optionalTransferString(figureTransferDOIURL(paper.DOI)),
			},
			License: FigureTransferLicense{
				Scope: "unknown",
				Text:  nil,
			},
			ExtractionMethod: firstNonEmpty(figure.Source, "unknown"),
		},
		Figure: FigureTransferFigure{
			File:      imageFilename,
			MediaType: mediaType,
			Bytes:     byteSize,
			SHA256:    imageSHA256,
			Kind:      contextDoc.Figure.Kind,
			Number:    figure.FigureIndex,
		},
		Files: files,
	}
}

func writeFigureTransferPackage(manifest FigureTransferManifest, payload map[string][]byte, modifiedAt time.Time) ([]byte, error) {
	manifestData, err := marshalTransferJSON(manifest)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	if err := writeFigureTransferEntry(writer, figureTransferManifestName, manifestData, modifiedAt); err != nil {
		return nil, err
	}
	for _, name := range []string{figureTransferContextName, figureTransferHandoffName, manifest.Figure.File} {
		data, ok := payload[name]
		if !ok {
			return nil, fmt.Errorf("missing package payload %q", name)
		}
		if err := writeFigureTransferEntry(writer, name, data, modifiedAt); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeFigureTransferEntry(writer *zip.Writer, name string, data []byte, modifiedAt time.Time) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetModTime(modifiedAt)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = entry.Write(data)
	return err
}

func ValidateFigureTransferPackage(data []byte) (*FigureTransferManifest, error) {
	if len(data) == 0 {
		return nil, invalidFigureTransferPackage("package is empty")
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, invalidFigureTransferPackage("invalid ZIP: %v", err)
	}
	if len(reader.File) != 4 {
		return nil, invalidFigureTransferPackage("expected exactly four files")
	}

	entries := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		if !validFigureTransferPackageFileName(file.Name) || file.FileInfo().IsDir() {
			return nil, invalidFigureTransferPackage("invalid ZIP entry %q", file.Name)
		}
		if _, exists := entries[file.Name]; exists {
			return nil, invalidFigureTransferPackage("duplicate ZIP entry %q", file.Name)
		}
		entries[file.Name] = file
	}

	manifestFile := entries[figureTransferManifestName]
	if manifestFile == nil {
		return nil, invalidFigureTransferPackage("missing %s", figureTransferManifestName)
	}
	manifestData, err := readFigureTransferEntry(manifestFile, maxTransferManifestSize)
	if err != nil {
		return nil, invalidFigureTransferPackage("read manifest: %v", err)
	}

	var manifest FigureTransferManifest
	decoder := json.NewDecoder(bytes.NewReader(manifestData))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, invalidFigureTransferPackage("invalid manifest: %v", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, invalidFigureTransferPackage("invalid manifest: %v", err)
	}
	if err := validateFigureTransferManifest(manifest); err != nil {
		return nil, err
	}

	contextFile := entries[figureTransferContextName]
	if contextFile == nil {
		return nil, invalidFigureTransferPackage("missing %s", figureTransferContextName)
	}
	contextData, err := readFigureTransferEntry(contextFile, maxTransferContextSize)
	if err != nil {
		return nil, invalidFigureTransferPackage("read research context: %v", err)
	}
	var contextDoc FigureResearchContext
	contextDecoder := json.NewDecoder(bytes.NewReader(contextData))
	contextDecoder.DisallowUnknownFields()
	if err := contextDecoder.Decode(&contextDoc); err != nil {
		return nil, invalidFigureTransferPackage("invalid research context: %v", err)
	}
	if err := ensureJSONEOF(contextDecoder); err != nil {
		return nil, invalidFigureTransferPackage("invalid research context: %v", err)
	}
	if contextDoc.Schema != FigureResearchContextSchemaName || contextDoc.Version != FigureResearchContextSchemaVersion {
		return nil, invalidFigureTransferPackage("unsupported research context schema")
	}

	handoffFile := entries[figureTransferHandoffName]
	if handoffFile == nil {
		return nil, invalidFigureTransferPackage("missing %s", figureTransferHandoffName)
	}
	handoffData, err := readFigureTransferEntry(handoffFile, maxTransferHandoffSize)
	if err != nil {
		return nil, invalidFigureTransferPackage("read handoff: %v", err)
	}
	if !bytes.Contains(handoffData, []byte("### 原始图注")) {
		return nil, invalidFigureTransferPackage("handoff is missing the original caption section")
	}

	imageFile := entries[manifest.Figure.File]
	if imageFile == nil {
		return nil, invalidFigureTransferPackage("missing image %q", manifest.Figure.File)
	}
	imageData, err := readFigureTransferEntry(imageFile, maxTransferImageSize)
	if err != nil {
		return nil, invalidFigureTransferPackage("read image: %v", err)
	}
	if int64(len(imageData)) != manifest.Figure.Bytes {
		return nil, invalidFigureTransferPackage("image bytes mismatch")
	}
	if sha256Hex(imageData) != manifest.Figure.SHA256 {
		return nil, invalidFigureTransferPackage("image sha256 mismatch")
	}

	payload := map[string][]byte{
		manifest.Figure.File:      imageData,
		figureTransferContextName: contextData,
		figureTransferHandoffName: handoffData,
	}
	if err := validateFigureTransferFiles(manifest, payload); err != nil {
		return nil, err
	}

	return &manifest, nil
}

func validateFigureTransferManifest(manifest FigureTransferManifest) error {
	if manifest.Schema != FigureTransferSchemaName || manifest.Version != FigureTransferSchemaVersion {
		return invalidFigureTransferPackage("unsupported schema %q version %d", manifest.Schema, manifest.Version)
	}
	if strings.TrimSpace(manifest.Producer.Name) == "" || strings.TrimSpace(manifest.Producer.Version) == "" {
		return invalidFigureTransferPackage("missing producer information")
	}
	if _, err := time.Parse(time.RFC3339, manifest.ExportedAt); err != nil {
		return invalidFigureTransferPackage("invalid exportedAt")
	}
	if manifest.Entry != figureTransferHandoffName {
		return invalidFigureTransferPackage("invalid entry file")
	}
	if manifest.Source.FigureID <= 0 || manifest.Source.Paper.ID <= 0 {
		return invalidFigureTransferPackage("invalid figure or paper identifier")
	}
	if manifest.Source.SourceID != figureTransferSourceID(manifest.Source.FigureID) {
		return invalidFigureTransferPackage("source id does not match figure id")
	}
	if strings.TrimSpace(manifest.Source.ExtractionMethod) == "" {
		return invalidFigureTransferPackage("missing extraction method")
	}

	switch manifest.Figure.Kind {
	case "figure":
		if manifest.Source.ParentFigureID != nil {
			return invalidFigureTransferPackage("main figure contains a parent identifier")
		}
	case "subfigure":
		if manifest.Source.ParentFigureID == nil || *manifest.Source.ParentFigureID <= 0 {
			return invalidFigureTransferPackage("subfigure is missing a valid parent identifier")
		}
		if len(manifest.Source.SubfigureLabels) == 0 {
			return invalidFigureTransferPackage("subfigure is missing its label")
		}
	default:
		return invalidFigureTransferPackage("invalid figure kind %q", manifest.Figure.Kind)
	}

	if manifest.Source.Page != nil && *manifest.Source.Page <= 0 {
		return invalidFigureTransferPackage("invalid page number")
	}
	if strings.TrimSpace(manifest.Source.License.Scope) == "" {
		return invalidFigureTransferPackage("missing license scope")
	}
	if !validFigureTransferImageName(manifest.Figure.File) {
		return invalidFigureTransferPackage("invalid image filename %q", manifest.Figure.File)
	}
	extension, ok := figureTransferMediaTypes[strings.ToLower(strings.TrimSpace(manifest.Figure.MediaType))]
	if !ok {
		return invalidFigureTransferPackage("unsupported image media type %q", manifest.Figure.MediaType)
	}
	if path.Ext(manifest.Figure.File) != extension {
		return invalidFigureTransferPackage("image filename does not match media type")
	}
	if manifest.Figure.Bytes <= 0 || manifest.Figure.Bytes > maxTransferImageSize {
		return invalidFigureTransferPackage("invalid image bytes")
	}
	decodedHash, err := hex.DecodeString(manifest.Figure.SHA256)
	if err != nil || len(decodedHash) != sha256.Size || manifest.Figure.SHA256 != strings.ToLower(manifest.Figure.SHA256) {
		return invalidFigureTransferPackage("invalid image sha256")
	}
	return nil
}

func validateFigureTransferFiles(manifest FigureTransferManifest, payload map[string][]byte) error {
	if len(manifest.Files) != 3 {
		return invalidFigureTransferPackage("manifest files must list figure, research context, and handoff")
	}
	seen := map[string]struct{}{}
	for _, file := range manifest.Files {
		if _, exists := seen[file.Name]; exists {
			return invalidFigureTransferPackage("duplicate manifest file %q", file.Name)
		}
		seen[file.Name] = struct{}{}
		data, ok := payload[file.Name]
		if !ok {
			return invalidFigureTransferPackage("missing listed file %q", file.Name)
		}
		if int64(len(data)) != file.Bytes {
			return invalidFigureTransferPackage("file %q bytes mismatch", file.Name)
		}
		if sha256Hex(data) != file.SHA256 {
			return invalidFigureTransferPackage("file %q sha256 mismatch", file.Name)
		}
	}
	if _, ok := seen[manifest.Figure.File]; !ok {
		return invalidFigureTransferPackage("files list is missing the figure")
	}
	if _, ok := seen[figureTransferContextName]; !ok {
		return invalidFigureTransferPackage("files list is missing research-context.json")
	}
	if _, ok := seen[figureTransferHandoffName]; !ok {
		return invalidFigureTransferPackage("files list is missing handoff.md")
	}
	return nil
}

func readFigureTransferEntry(file *zip.File, maxSize int64) ([]byte, error) {
	if file.UncompressedSize64 > uint64(maxSize) {
		return nil, fmt.Errorf("entry exceeds %d bytes", maxSize)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, maxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("entry exceeds %d bytes", maxSize)
	}
	return data, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func validFigureTransferEntryName(name string) bool {
	return name != "" && name == path.Clean(name) && name == path.Base(name) && !path.IsAbs(name) && !strings.Contains(name, "\\")
}

func validFigureTransferImageName(name string) bool {
	return validFigureTransferEntryName(name) && strings.HasPrefix(name, "figure.") && len(strings.TrimPrefix(name, "figure.")) > 0
}

func validFigureTransferPackageFileName(name string) bool {
	return name == figureTransferManifestName || name == figureTransferContextName || name == figureTransferHandoffName || validFigureTransferImageName(name)
}

func figureTransferSourceID(figureID int64) string {
	return "citebox:figure:" + strconv.FormatInt(figureID, 10)
}

func figureTransferDOIURL(doi string) string {
	normalized, err := normalizeDOIInput(doi)
	if err != nil || normalized == "" {
		return ""
	}
	return (&url.URL{Scheme: "https", Host: "doi.org", Path: "/" + normalized}).String()
}

func figureTransferPublicationYear(publishedAt string) *int {
	value := strings.TrimSpace(publishedAt)
	if len(value) < 4 {
		return nil
	}
	year, err := strconv.Atoi(value[:4])
	if err != nil || year < 1000 || year > 9999 {
		return nil
	}
	return &year
}

func figureTransferAuthors(authorsText string) []string {
	authors := []string{}
	for _, part := range strings.Split(authorsText, ",") {
		if name := strings.TrimSpace(part); name != "" {
			authors = append(authors, truncateTransferText(name, maxTransferAuthorRunes))
		}
	}
	return authors
}

func optionalTransferString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeTransferMediaType(mediaType string) string {
	return strings.ToLower(strings.TrimSpace(strings.SplitN(mediaType, ";", 2)[0]))
}

func truncateTransferText(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func marshalTransferJSON(value interface{}) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func newFigureTransferFile(name, role, mediaType string, data []byte) FigureTransferFile {
	return FigureTransferFile{
		Name:      name,
		Role:      role,
		MediaType: mediaType,
		Bytes:     int64(len(data)),
		SHA256:    sha256Hex(data),
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func invalidFigureTransferPackage(format string, args ...interface{}) error {
	return apperr.New(apperr.CodeInvalidArgument, "invalid Figure Transfer Package: "+fmt.Sprintf(format, args...))
}
