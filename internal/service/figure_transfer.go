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
	FigureTransferSchemaName    = "citebox.figure-transfer-package"
	FigureTransferSchemaVersion = "1.0"
	figureTransferManifestName  = "manifest.json"
	maxTransferManifestSize     = 1 << 20
	maxTransferImageSize        = 512 << 20
)

type FigureTransferManifest struct {
	Schema FigureTransferSchema `json:"schema"`
	Source FigureTransferSource `json:"source"`
	Figure FigureTransferFigure `json:"figure"`
	Paper  FigureTransferPaper  `json:"paper"`
	Image  FigureTransferImage  `json:"image"`
	Rights FigureTransferRights `json:"rights"`
	Export FigureTransferExport `json:"export"`
}

type FigureTransferSchema struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type FigureTransferSource struct {
	System           string `json:"system"`
	ID               string `json:"id"`
	ExtractionMethod string `json:"extraction_method"`
	URL              string `json:"url"`
}

type FigureTransferFigure struct {
	ID             int64   `json:"id"`
	ParentID       *int64  `json:"parent_id"`
	ParentSourceID *string `json:"parent_source_id"`
	Kind           string  `json:"kind"`
	Number         int     `json:"number"`
	DisplayLabel   string  `json:"display_label"`
	SubfigureLabel string  `json:"subfigure_label"`
	Caption        string  `json:"caption"`
	PageNumber     int     `json:"page_number"`
}

type FigureTransferPaper struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Authors     string `json:"authors"`
	Year        *int   `json:"year"`
	PublishedAt string `json:"published_at"`
	Journal     string `json:"journal"`
	DOI         string `json:"doi"`
	URL         string `json:"url"`
}

type FigureTransferImage struct {
	Filename  string `json:"filename"`
	MediaType string `json:"media_type"`
	ByteSize  int64  `json:"byte_size"`
	SHA256    string `json:"sha256"`
}

type FigureTransferRights struct {
	License   string `json:"license"`
	Statement string `json:"statement"`
}

type FigureTransferExport struct {
	ExportedAt     string `json:"exported_at"`
	CiteBoxVersion string `json:"citebox_version"`
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

	exportedAt := time.Now().UTC().Truncate(time.Second)
	imageFilename := "figure" + figureTransferImageExtension(mediaType)
	imageHash := sha256.Sum256(imageData)
	manifest := newFigureTransferManifest(paper, figure, imageFilename, mediaType, int64(len(imageData)), hex.EncodeToString(imageHash[:]), exportedAt)

	data, err := writeFigureTransferPackage(manifest, imageData, exportedAt)
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
	imageFilename string,
	mediaType string,
	byteSize int64,
	imageSHA256 string,
	exportedAt time.Time,
) FigureTransferManifest {
	sourceURL := figureTransferDOIURL(paper.DOI)
	kind := "figure"
	var parentSourceID *string
	if figure.ParentFigureID != nil {
		kind = "subfigure"
		value := figureTransferSourceID(*figure.ParentFigureID)
		parentSourceID = &value
	}

	return FigureTransferManifest{
		Schema: FigureTransferSchema{
			Name:    FigureTransferSchemaName,
			Version: FigureTransferSchemaVersion,
		},
		Source: FigureTransferSource{
			System:           "citebox",
			ID:               figureTransferSourceID(figure.ID),
			ExtractionMethod: firstNonEmpty(figure.Source, "unknown"),
			URL:              sourceURL,
		},
		Figure: FigureTransferFigure{
			ID:             figure.ID,
			ParentID:       figure.ParentFigureID,
			ParentSourceID: parentSourceID,
			Kind:           kind,
			Number:         figure.FigureIndex,
			DisplayLabel:   formatFigureDisplayLabel(figure.FigureIndex, figure.SubfigureLabel),
			SubfigureLabel: strings.TrimSpace(figure.SubfigureLabel),
			Caption:        strings.TrimSpace(figure.Caption),
			PageNumber:     figure.PageNumber,
		},
		Paper: FigureTransferPaper{
			ID:          paper.ID,
			Title:       strings.TrimSpace(paper.Title),
			Authors:     strings.TrimSpace(paper.AuthorsText),
			Year:        figureTransferPublicationYear(paper.PublishedAt),
			PublishedAt: strings.TrimSpace(paper.PublishedAt),
			Journal:     strings.TrimSpace(paper.Journal),
			DOI:         strings.TrimSpace(paper.DOI),
			URL:         sourceURL,
		},
		Image: FigureTransferImage{
			Filename:  imageFilename,
			MediaType: mediaType,
			ByteSize:  byteSize,
			SHA256:    imageSHA256,
		},
		Rights: FigureTransferRights{
			License:   "unknown",
			Statement: "unknown",
		},
		Export: FigureTransferExport{
			ExportedAt:     exportedAt.Format(time.RFC3339),
			CiteBoxVersion: buildinfo.CurrentVersion(),
		},
	}
}

func writeFigureTransferPackage(manifest FigureTransferManifest, imageData []byte, modifiedAt time.Time) ([]byte, error) {
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	manifestData = append(manifestData, '\n')

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	if err := writeFigureTransferEntry(writer, figureTransferManifestName, manifestData, modifiedAt); err != nil {
		return nil, err
	}
	if err := writeFigureTransferEntry(writer, manifest.Image.Filename, imageData, modifiedAt); err != nil {
		return nil, err
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
	if len(reader.File) != 2 {
		return nil, invalidFigureTransferPackage("expected exactly two files")
	}

	entries := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		if !validFigureTransferEntryName(file.Name) || file.FileInfo().IsDir() {
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

	imageFile := entries[manifest.Image.Filename]
	if imageFile == nil {
		return nil, invalidFigureTransferPackage("missing image %q", manifest.Image.Filename)
	}
	imageData, err := readFigureTransferEntry(imageFile, maxTransferImageSize)
	if err != nil {
		return nil, invalidFigureTransferPackage("read image: %v", err)
	}
	if int64(len(imageData)) != manifest.Image.ByteSize {
		return nil, invalidFigureTransferPackage("image byte_size mismatch")
	}
	imageHash := sha256.Sum256(imageData)
	if hex.EncodeToString(imageHash[:]) != manifest.Image.SHA256 {
		return nil, invalidFigureTransferPackage("image sha256 mismatch")
	}

	return &manifest, nil
}

func validateFigureTransferManifest(manifest FigureTransferManifest) error {
	if manifest.Schema.Name != FigureTransferSchemaName || manifest.Schema.Version != FigureTransferSchemaVersion {
		return invalidFigureTransferPackage("unsupported schema %q version %q", manifest.Schema.Name, manifest.Schema.Version)
	}
	if manifest.Source.System != "citebox" || manifest.Figure.ID <= 0 || manifest.Paper.ID <= 0 {
		return invalidFigureTransferPackage("invalid source, figure, or paper identifier")
	}
	if manifest.Source.ID != figureTransferSourceID(manifest.Figure.ID) {
		return invalidFigureTransferPackage("source id does not match figure id")
	}
	if strings.TrimSpace(manifest.Source.ExtractionMethod) == "" {
		return invalidFigureTransferPackage("missing extraction method")
	}

	switch manifest.Figure.Kind {
	case "figure":
		if manifest.Figure.ParentID != nil || manifest.Figure.ParentSourceID != nil || manifest.Figure.SubfigureLabel != "" {
			return invalidFigureTransferPackage("main figure contains subfigure identifiers")
		}
	case "subfigure":
		if manifest.Figure.ParentID == nil || *manifest.Figure.ParentID <= 0 || manifest.Figure.ParentSourceID == nil {
			return invalidFigureTransferPackage("subfigure is missing parent identifiers")
		}
		if *manifest.Figure.ParentSourceID != figureTransferSourceID(*manifest.Figure.ParentID) {
			return invalidFigureTransferPackage("parent source id does not match parent id")
		}
		if strings.TrimSpace(manifest.Figure.SubfigureLabel) == "" {
			return invalidFigureTransferPackage("subfigure is missing its label")
		}
	default:
		return invalidFigureTransferPackage("invalid figure kind %q", manifest.Figure.Kind)
	}

	if !validFigureTransferImageName(manifest.Image.Filename) {
		return invalidFigureTransferPackage("invalid image filename %q", manifest.Image.Filename)
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(manifest.Image.MediaType)), "image/") {
		return invalidFigureTransferPackage("invalid image media_type %q", manifest.Image.MediaType)
	}
	if manifest.Image.ByteSize <= 0 {
		return invalidFigureTransferPackage("invalid image byte_size")
	}
	decodedHash, err := hex.DecodeString(manifest.Image.SHA256)
	if err != nil || len(decodedHash) != sha256.Size || manifest.Image.SHA256 != strings.ToLower(manifest.Image.SHA256) {
		return invalidFigureTransferPackage("invalid image sha256")
	}
	if strings.TrimSpace(manifest.Rights.License) == "" || strings.TrimSpace(manifest.Rights.Statement) == "" {
		return invalidFigureTransferPackage("missing rights information")
	}
	if _, err := time.Parse(time.RFC3339, manifest.Export.ExportedAt); err != nil {
		return invalidFigureTransferPackage("invalid exported_at")
	}
	if strings.TrimSpace(manifest.Export.CiteBoxVersion) == "" {
		return invalidFigureTransferPackage("missing CiteBox version")
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

func figureTransferImageExtension(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(mediaType, ";", 2)[0])) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/tiff":
		return ".tiff"
	case "image/bmp":
		return ".bmp"
	case "image/svg+xml":
		return ".svg"
	case "image/avif":
		return ".avif"
	default:
		return ".img"
	}
}

func invalidFigureTransferPackage(format string, args ...interface{}) error {
	return apperr.New(apperr.CodeInvalidArgument, "invalid Figure Transfer Package: "+fmt.Sprintf(format, args...))
}
