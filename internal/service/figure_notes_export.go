package service

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

const maxFigureNotesExportCount = 1000
const maxFigureNotesExportBytes int64 = 500 << 20

// ExportFigureNotes creates a complete archive before HTTP headers are sent.
// The caller owns the returned file and must close and remove it.
func (s *LibraryService) ExportFigureNotes(ctx context.Context, filter model.FigureFilter, language string) (*os.File, error) {
	filter.Page, filter.PageSize = 1, 200
	figures := []model.FigureListItem{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, total, err := s.repo.ListFigures(filter)
		if err != nil {
			return nil, err
		}
		if total > maxFigureNotesExportCount || len(figures)+len(page) > maxFigureNotesExportCount {
			return nil, apperr.New(apperr.CodeInvalidArgument, "Export supports up to 1000 figures; narrow the filters")
		}
		figures = append(figures, page...)
		if len(page) < filter.PageSize || len(figures) >= total {
			break
		}
		filter.Page++
	}
	if len(figures) == 0 {
		return nil, apperr.New(apperr.CodeNotFound, "No figures match the export filters")
	}
	root, err := filepath.EvalSymlinks(s.config.FiguresDir())
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "citebox-figure-notes-*.zip")
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			file.Close()
			os.Remove(file.Name())
		}
	}()
	archive := zip.NewWriter(file)
	defer archive.Close()
	label := func(en, zh string) string {
		if language == "en" {
			return en
		}
		return zh
	}
	var index strings.Builder
	fmt.Fprintf(&index, "# %s\n\n%s\n\n", label("Figure notes", "图片笔记"), label("Exported from CiteBox. Images and captions retain their source attribution.", "导出自 CiteBox。图片和图注保留原文献来源。"))
	var used int64
	seen := map[int64]bool{}
	for _, figure := range figures {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if seen[figure.ID] {
			return nil, apperr.New(apperr.CodeFailedPrecondition, "Figures changed during export; retry")
		}
		seen[figure.ID] = true
		paper, err := s.repo.GetPaperDetail(figure.PaperID)
		if err != nil {
			return nil, err
		}
		if paper == nil {
			return nil, apperr.New(apperr.CodeNotFound, "Source paper no longer exists")
		}
		// Only top-level physical images are listed, matching the library.
		imagePath, err := filepath.EvalSymlinks(filepath.Join(root, filepath.Base(figure.Filename)))
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeFailedPrecondition, fmt.Sprintf("Cannot read image for figure %d", figure.ID), err)
		}
		relative, err := filepath.Rel(root, imagePath)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return nil, apperr.New(apperr.CodeFailedPrecondition, "Image path is outside the figure library")
		}
		source, err := os.Open(imagePath)
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeFailedPrecondition, fmt.Sprintf("Cannot read image for figure %d", figure.ID), err)
		}
		stat, err := source.Stat()
		if err != nil {
			source.Close()
			return nil, err
		}
		if !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > maxFigureNotesExportBytes-used {
			source.Close()
			return nil, apperr.New(apperr.CodeInvalidArgument, "Export images must be nonempty regular files, up to 500 MiB total")
		}
		ext := strings.ToLower(filepath.Ext(figure.Filename))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".pdf", ".tif", ".tiff", ".bmp":
		default:
			ext = ".bin"
		}
		imageName := fmt.Sprintf("images/figure-%d%s", figure.ID, ext)
		dest, err := archive.Create(imageName)
		if err != nil {
			source.Close()
			return nil, err
		}
		n, copyErr := io.Copy(dest, &exportContextReader{ctx: ctx, reader: io.LimitReader(source, maxFigureNotesExportBytes-used+1)})
		source.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		used += n
		if used > maxFigureNotesExportBytes {
			return nil, apperr.New(apperr.CodeInvalidArgument, "Export exceeds 500 MiB; narrow the filters")
		}
		noteName := fmt.Sprintf("figures/figure-%d.md", figure.ID)
		title := firstNonEmpty(figure.DisplayLabel, fmt.Sprintf("Fig %d", figure.FigureIndex))
		fmt.Fprintf(&index, "- [%s — %s](%s)\n", exportMarkdownText(paper.Title), exportMarkdownText(title), noteName)
		var note strings.Builder
		fmt.Fprintf(&note, "# %s — %s\n\n", exportMarkdownText(paper.Title), exportMarkdownText(title))
		metadata := [][2]string{{label("Source paper", "来源文献"), paper.Title}, {"DOI", paper.DOI}, {label("Authors", "作者"), paper.AuthorsText}, {label("Journal", "期刊"), paper.Journal}, {label("Published", "发表日期"), paper.PublishedAt}, {label("Group", "分组"), figure.GroupName}, {label("Page", "页码"), fmt.Sprint(figure.PageNumber)}, {"CiteBox IDs", fmt.Sprintf("paper=%d, figure=%d", paper.ID, figure.ID)}}
		tags := []string{}
		for _, tag := range figure.Tags {
			tags = append(tags, tag.Name)
		}
		metadata = append(metadata, [2]string{label("Figure tags", "图片标签"), strings.Join(tags, ", ")})
		for _, pair := range metadata {
			if pair[1] != "" {
				fmt.Fprintf(&note, "- %s: %s\n", pair[0], exportMarkdownText(pair[1]))
			}
		}
		fmt.Fprintf(&note, "\n![%s](../%s)\n\n## %s\n\n%s\n\n## %s\n\n%s\n", exportMarkdownText(title), imageName, label("Caption", "图注"), exportMarkdownText(figure.Caption), label("Notes", "笔记"), figure.NotesText)
		used += int64(note.Len())
		if used > maxFigureNotesExportBytes {
			return nil, apperr.New(apperr.CodeInvalidArgument, "Export exceeds 500 MiB; narrow the filters")
		}
		entry, err := archive.Create(noteName)
		if err != nil {
			return nil, err
		}
		if _, err = io.WriteString(entry, note.String()); err != nil {
			return nil, err
		}
	}
	entry, err := archive.Create("README.md")
	if err != nil {
		return nil, err
	}
	if _, err = io.WriteString(entry, index.String()); err != nil {
		return nil, err
	}
	if err = archive.Close(); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	success = true
	return file, nil
}

type exportContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *exportContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
func exportMarkdownText(value string) string {
	return strings.NewReplacer("\\", "\\\\", "&", "&amp;", "<", "&lt;", ">", "&gt;", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`", "#", "\\#", "\r", " ", "\n", " ").Replace(value)
}
