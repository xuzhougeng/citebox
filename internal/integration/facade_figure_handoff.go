package integration

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// GetFigureHandoff 打包单张图（或子图）的研究交接信封：出处、限额文献材料与资产指针。
// 不生成生物学问题，不导出 R，也不写入外部图库。
func (f *Facade) GetFigureHandoff(p GetFigureHandoffParams) (*Envelope, error) {
	figureID, err := resolveFigureHandoffID(p)
	if err != nil {
		return nil, err
	}

	figure, err := f.library.GetFigure(figureID)
	if err != nil {
		return nil, err
	}
	if figure == nil {
		return nil, apperr.New(apperr.CodeNotFound, "figure not found")
	}
	decorateFigureHandoffLabels(figure)

	paper, err := f.library.GetPaper(figure.PaperID)
	if err != nil {
		return nil, err
	}

	pageTexts, err := f.repo.GetPaperPDFPageTexts(paper.ID)
	if err != nil {
		return nil, err
	}

	excerpts := collectFigureHandoffExcerpts(paper, *figure, pageTexts)
	notesText := strings.TrimSpace(figure.NotesText)
	abstract := strings.TrimSpace(paper.AbstractText)

	var parent *model.Figure
	var subfigures []model.Figure
	for i := range paper.Figures {
		item := paper.Figures[i]
		if figure.ParentFigureID != nil && item.ID == *figure.ParentFigureID {
			copy := item
			parent = &copy
		}
		if item.ParentFigureID != nil && *item.ParentFigureID == figure.ID {
			subfigures = append(subfigures, item)
		}
	}

	figureData := map[string]any{
		"figure_id":     figure.ID,
		"source_id":     FigureSourceID(figure.ID),
		"display_label": figure.DisplayLabel,
		"caption":       figure.Caption,
		"page_number":   figure.PageNumber,
		"figure_index":  figure.FigureIndex,
		"kind":          figureHandoffKind(*figure),
		"extraction":    figure.Source,
	}
	if figure.SubfigureLabel != "" {
		figureData["subfigure_label"] = figure.SubfigureLabel
	}
	if figure.ParentFigureID != nil {
		figureData["parent_figure_id"] = *figure.ParentFigureID
		figureData["parent_source_id"] = FigureSourceID(*figure.ParentFigureID)
		figureData["parent_display_label"] = figure.ParentDisplayLabel
	}
	if len(subfigures) > 0 {
		listed := make([]map[string]any, 0, len(subfigures))
		for _, sub := range subfigures {
			listed = append(listed, map[string]any{
				"figure_id":       sub.ID,
				"source_id":       FigureSourceID(sub.ID),
				"display_label":   sub.DisplayLabel,
				"subfigure_label": sub.SubfigureLabel,
				"caption":         sub.Caption,
			})
		}
		figureData["subfigures"] = listed
	}

	paperData := map[string]any{
		"id":           paper.ID,
		"source_id":    PaperSourceID(paper.ID),
		"title":        paper.Title,
		"authors_text": paper.AuthorsText,
		"journal":      paper.Journal,
		"published_at": paper.PublishedAt,
		"doi":          paper.DOI,
		"abstract":     paper.AbstractText,
	}

	var notes any
	if notesText != "" {
		notes = map[string]any{
			"figure_id":     figure.ID,
			"source_id":     FigureNoteSourceID(figure.ID),
			"display_label": figure.DisplayLabel,
			"notes_text":    notesText,
		}
	}

	data := map[string]any{
		"figure":   figureData,
		"paper":    paperData,
		"notes":    notes,
		"excerpts": excerpts,
		"completeness": map[string]any{
			"abstract":   abstract != "",
			"page_texts": pageTexts != nil,
			"excerpts":   len(excerpts) > 0,
			"notes":      notesText != "",
		},
	}

	relations := []any{
		map[string]any{"type": EntityTypePaper, "source_id": PaperSourceID(paper.ID)},
	}
	if parent != nil {
		relations = append(relations, map[string]any{"type": EntityTypeFigure, "source_id": FigureSourceID(parent.ID)})
	}
	for _, sub := range subfigures {
		relations = append(relations, map[string]any{"type": EntityTypeFigure, "source_id": FigureSourceID(sub.ID)})
	}

	assets := []any{
		map[string]any{"kind": AssetKindFigureImage, "figure_id": figure.ID, "source_id": FigureSourceID(figure.ID)},
		map[string]any{"kind": AssetKindFigureTransferPackage, "figure_id": figure.ID, "source_id": FigureSourceID(figure.ID)},
	}

	envelope := NewEnvelope(SourceRef{Kind: EntityTypeFigure, ID: figure.ID}, figure.UpdatedAt, data)
	envelope.Relations = relations
	envelope.Assets = assets
	return envelope, nil
}

func resolveFigureHandoffID(p GetFigureHandoffParams) (int64, error) {
	var fromSource int64
	if trimmed := strings.TrimSpace(p.SourceID); trimmed != "" {
		ref, err := ParseSourceID(trimmed)
		if err != nil {
			return 0, err
		}
		if ref.Kind != EntityTypeFigure {
			return 0, apperr.New(apperr.CodeInvalidArgument, "source_id 必须指向 figure")
		}
		fromSource = ref.ID
	}
	if p.FigureID <= 0 && fromSource == 0 {
		return 0, apperr.New(apperr.CodeInvalidArgument, "figure_id 或 source_id 不能为空")
	}
	if p.FigureID > 0 && fromSource > 0 && p.FigureID != fromSource {
		return 0, apperr.New(apperr.CodeInvalidArgument, "figure_id 与 source_id 不一致")
	}
	if p.FigureID > 0 {
		return p.FigureID, nil
	}
	return fromSource, nil
}

func figureHandoffKind(figure model.FigureListItem) string {
	if figure.ParentFigureID != nil {
		return "subfigure"
	}
	return "figure"
}

func decorateFigureHandoffLabels(figure *model.FigureListItem) {
	if strings.TrimSpace(figure.DisplayLabel) == "" {
		figure.DisplayLabel = formatHandoffDisplayLabel(figure.FigureIndex, figure.SubfigureLabel)
	}
	if figure.ParentFigureID != nil && strings.TrimSpace(figure.ParentDisplayLabel) == "" {
		figure.ParentDisplayLabel = formatHandoffDisplayLabel(figure.FigureIndex, "")
	}
}

func formatHandoffDisplayLabel(figureIndex int, subfigureLabel string) string {
	if figureIndex <= 0 {
		return ""
	}
	label := strings.TrimSpace(subfigureLabel)
	if label == "" {
		return fmt.Sprintf("Fig %d", figureIndex)
	}
	return fmt.Sprintf("Fig %d%s", figureIndex, strings.ToLower(label))
}

func collectFigureHandoffExcerpts(paper *model.Paper, figure model.FigureListItem, pageTexts []string) []map[string]any {
	queries := figureHandoffQueries(figure)
	if len(queries) == 0 {
		return []map[string]any{}
	}

	excerpts := make([]map[string]any, 0, defaultHandoffExcerptLimit)
	seen := map[string]struct{}{}
	appendMatch := func(page *int, snippet, query string) bool {
		snippet = strings.TrimSpace(snippet)
		if snippet == "" {
			return false
		}
		pageKey := "none"
		if page != nil {
			pageKey = fmt.Sprintf("%d", *page)
		}
		key := pageKey + "\n" + strings.ToLower(snippet)
		if _, ok := seen[key]; ok {
			return false
		}
		seen[key] = struct{}{}
		entry := map[string]any{
			"snippet":      snippet,
			"match_reason": query,
			"source_id":    PaperSourceID(paper.ID),
		}
		if page != nil {
			entry["page"] = *page
		} else {
			entry["page"] = nil
		}
		excerpts = append(excerpts, entry)
		return len(excerpts) >= defaultHandoffExcerptLimit
	}

	if pageTexts == nil {
		for _, query := range queries {
			for _, snippet := range findBoundedTextMatches(paper.PDFText, query, defaultHandoffContextChars) {
				if appendMatch(nil, snippet, query) {
					return excerpts
				}
			}
		}
		return excerpts
	}

	for _, query := range queries {
		for i, pageText := range pageTexts {
			page := i + 1
			for _, snippet := range findBoundedTextMatches(pageText, query, defaultHandoffContextChars) {
				if appendMatch(&page, snippet, query) {
					return excerpts
				}
			}
		}
	}
	return excerpts
}

func figureHandoffQueries(figure model.FigureListItem) []string {
	queries := make([]string, 0, 8)
	add := func(raw string) {
		query := compactQuery(raw)
		if query == "" {
			return
		}
		for _, existing := range queries {
			if strings.EqualFold(existing, query) {
				return
			}
		}
		queries = append(queries, query)
	}

	add(figure.DisplayLabel)
	add(figure.ParentDisplayLabel)
	if figure.FigureIndex > 0 {
		n := figure.FigureIndex
		add(fmt.Sprintf("Fig %d", n))
		add(fmt.Sprintf("Fig. %d", n))
		add(fmt.Sprintf("Figure %d", n))
		add(fmt.Sprintf("图 %d", n))
		if sub := strings.TrimSpace(figure.SubfigureLabel); sub != "" {
			sub = strings.ToLower(sub)
			add(fmt.Sprintf("Fig %d%s", n, sub))
			add(fmt.Sprintf("Figure %d%s", n, sub))
			add(fmt.Sprintf("Fig %d%s", n, strings.ToUpper(sub)))
		}
	}
	add(shortCaptionQuery(figure.Caption))
	return queries
}

func compactQuery(raw string) string {
	query := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if query == "" {
		return ""
	}
	if utf8.RuneCountInString(query) < 2 {
		return ""
	}
	return query
}

func shortCaptionQuery(caption string) string {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return ""
	}
	fields := strings.Fields(caption)
	if len(fields) == 0 {
		return ""
	}
	if len(fields) > 6 {
		fields = fields[:6]
	}
	query := strings.Join(fields, " ")
	count := utf8.RuneCountInString(query)
	if count < 8 || count > 48 {
		return ""
	}
	return query
}

func findBoundedTextMatches(text, query string, contextChars int) []string {
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	if lowerQuery == "" {
		return nil
	}
	var snippets []string
	for offset := 0; offset < len(lowerText); {
		idx := indexBoundedIgnoreCase(lowerText, lowerQuery, offset)
		if idx < 0 {
			break
		}
		snippets = append(snippets, contextWindow(text, idx, contextChars))
		offset = idx + len(lowerQuery)
	}
	return snippets
}

func indexBoundedIgnoreCase(lowerText, lowerQuery string, offset int) int {
	for offset <= len(lowerText) {
		rel := strings.Index(lowerText[offset:], lowerQuery)
		if rel < 0 {
			return -1
		}
		start := offset + rel
		end := start + len(lowerQuery)
		if hasTrailingDigit(lowerText, end) {
			offset = start + 1
			continue
		}
		return start
	}
	return -1
}

func hasTrailingDigit(text string, byteIndex int) bool {
	if byteIndex < 0 || byteIndex >= len(text) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text[byteIndex:])
	return unicode.IsDigit(r)
}
