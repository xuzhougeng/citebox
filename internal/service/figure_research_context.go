package service

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/xuzhougeng/citebox/internal/model"
)

const (
	FigureResearchContextSchemaName    = "citebox.figure-research-context/v1"
	FigureResearchContextSchemaVersion = 1
	figureTransferContextName          = "research-context.json"
	figureTransferHandoffName          = "handoff.md"
	defaultTransferExcerptLimit        = 6
	defaultTransferExcerptContext      = 400
	missingHandoffValue                = "（缺失）"
)

var officialFigureLabelPattern = regexp.MustCompile(`(?i)^((?:extended\s+data\s+|supplementary\s+|supplemental\s+)?fig(?:ure)?\.?\s*\d+[a-z]?|图\s*\d+[a-z]?)`)

type FigureResearchContext struct {
	Schema       string                     `json:"schema"`
	Version      int                        `json:"version"`
	Figure       FigureResearchFigure       `json:"figure"`
	Paper        FigureResearchPaper        `json:"paper"`
	Notes        FigureResearchNotes        `json:"notes"`
	Excerpts     []FigureResearchExcerpt    `json:"excerpts"`
	Completeness FigureResearchCompleteness `json:"completeness"`
	Replication  FigureResearchReplication  `json:"replication"`
}

type FigureResearchFigure struct {
	FigureID           int64                   `json:"figure_id"`
	SourceID           string                  `json:"source_id"`
	DisplayLabel       string                  `json:"display_label"`
	OfficialLabel      string                  `json:"official_label"`
	Caption            string                  `json:"caption"`
	PageNumber         *int                    `json:"page_number"`
	FigureIndex        int                     `json:"figure_index"`
	Kind               string                  `json:"kind"`
	Extraction         string                  `json:"extraction"`
	ParentFigureID     *int64                  `json:"parent_figure_id"`
	ParentSourceID     *string                 `json:"parent_source_id"`
	ParentDisplayLabel string                  `json:"parent_display_label,omitempty"`
	SubfigureLabel     string                  `json:"subfigure_label,omitempty"`
	Subfigures         []FigureResearchSubitem `json:"subfigures"`
}

type FigureResearchSubitem struct {
	FigureID       int64  `json:"figure_id"`
	SourceID       string `json:"source_id"`
	DisplayLabel   string `json:"display_label"`
	SubfigureLabel string `json:"subfigure_label"`
	Caption        string `json:"caption"`
}

type FigureResearchPaper struct {
	ID          int64    `json:"id"`
	SourceID    string   `json:"source_id"`
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	Year        *int     `json:"year"`
	PublishedAt string   `json:"published_at,omitempty"`
	Journal     *string  `json:"journal"`
	DOI         *string  `json:"doi"`
	URL         *string  `json:"url"`
	Abstract    string   `json:"abstract"`
}

type FigureResearchNotes struct {
	FigureNotes *string `json:"figure_notes"`
	PaperNotes  *string `json:"paper_notes"`
}

type FigureResearchExcerpt struct {
	Snippet     string `json:"snippet"`
	Page        *int   `json:"page"`
	MatchReason string `json:"match_reason"`
	SourceID    string `json:"source_id"`
}

type FigureResearchCompleteness struct {
	Title       bool `json:"title"`
	Authors     bool `json:"authors"`
	Year        bool `json:"year"`
	Journal     bool `json:"journal"`
	DOI         bool `json:"doi"`
	URL         bool `json:"url"`
	Abstract    bool `json:"abstract"`
	Caption     bool `json:"caption"`
	PageTexts   bool `json:"page_texts"`
	Excerpts    bool `json:"excerpts"`
	FigureNotes bool `json:"figure_notes"`
	PaperNotes  bool `json:"paper_notes"`
}

type FigureResearchReplication struct {
	HasSourceData bool   `json:"has_source_data"`
	HasCode       bool   `json:"has_code"`
	Mode          string `json:"mode"`
	Summary       string `json:"summary"`
}

func buildFigureResearchContext(paper *model.Paper, figure *model.Figure, pageTexts []string) FigureResearchContext {
	displayLabel := strings.TrimSpace(figure.DisplayLabel)
	if displayLabel == "" {
		displayLabel = formatFigureDisplayLabel(figure.FigureIndex, figure.SubfigureLabel)
	}
	officialLabel := officialFigureLabel(figure.Caption, displayLabel)
	authors := figureTransferAuthors(paper.AuthorsText)
	year := figureTransferPublicationYear(paper.PublishedAt)
	journal := optionalTransferString(paper.Journal)
	doi := optionalTransferString(paper.DOI)
	url := optionalTransferString(figureTransferDOIURL(paper.DOI))
	abstract := strings.TrimSpace(paper.AbstractText)
	caption := strings.TrimSpace(figure.Caption)
	figureNotes := optionalTransferString(figure.NotesText)
	paperNotes := optionalTransferString(firstNonEmpty(paper.PaperNotesText, paper.NotesText))
	var page *int
	if figure.PageNumber > 0 {
		value := figure.PageNumber
		page = &value
	}

	kind := "figure"
	var parentID *int64
	var parentSourceID *string
	parentDisplay := ""
	if figure.ParentFigureID != nil {
		kind = "subfigure"
		parentID = figure.ParentFigureID
		sourceID := figureTransferSourceID(*figure.ParentFigureID)
		parentSourceID = &sourceID
		parentDisplay = strings.TrimSpace(figure.ParentDisplayLabel)
		if parentDisplay == "" {
			parentDisplay = formatFigureDisplayLabel(figure.FigureIndex, "")
		}
	}

	subfigures := make([]FigureResearchSubitem, 0)
	for _, candidate := range paper.Figures {
		if candidate.ParentFigureID == nil || *candidate.ParentFigureID != figure.ID {
			continue
		}
		subfigures = append(subfigures, FigureResearchSubitem{
			FigureID:       candidate.ID,
			SourceID:       figureTransferSourceID(candidate.ID),
			DisplayLabel:   formatFigureDisplayLabel(candidate.FigureIndex, candidate.SubfigureLabel),
			SubfigureLabel: strings.TrimSpace(candidate.SubfigureLabel),
			Caption:        strings.TrimSpace(candidate.Caption),
		})
	}

	excerpts := collectFigureTransferExcerpts(paper, *figure, displayLabel, officialLabel, parentDisplay, pageTexts)

	return FigureResearchContext{
		Schema:  FigureResearchContextSchemaName,
		Version: FigureResearchContextSchemaVersion,
		Figure: FigureResearchFigure{
			FigureID:           figure.ID,
			SourceID:           figureTransferSourceID(figure.ID),
			DisplayLabel:       displayLabel,
			OfficialLabel:      officialLabel,
			Caption:            caption,
			PageNumber:         page,
			FigureIndex:        figure.FigureIndex,
			Kind:               kind,
			Extraction:         firstNonEmpty(figure.Source, "unknown"),
			ParentFigureID:     parentID,
			ParentSourceID:     parentSourceID,
			ParentDisplayLabel: parentDisplay,
			SubfigureLabel:     strings.TrimSpace(figure.SubfigureLabel),
			Subfigures:         subfigures,
		},
		Paper: FigureResearchPaper{
			ID:          paper.ID,
			SourceID:    paperTransferSourceID(paper.ID),
			Title:       strings.TrimSpace(paper.Title),
			Authors:     authors,
			Year:        year,
			PublishedAt: strings.TrimSpace(paper.PublishedAt),
			Journal:     journal,
			DOI:         doi,
			URL:         url,
			Abstract:    abstract,
		},
		Notes: FigureResearchNotes{
			FigureNotes: figureNotes,
			PaperNotes:  paperNotes,
		},
		Excerpts: excerpts,
		Completeness: FigureResearchCompleteness{
			Title:       strings.TrimSpace(paper.Title) != "",
			Authors:     len(authors) > 0,
			Year:        year != nil,
			Journal:     journal != nil,
			DOI:         doi != nil,
			URL:         url != nil,
			Abstract:    abstract != "",
			Caption:     caption != "",
			PageTexts:   pageTexts != nil,
			Excerpts:    len(excerpts) > 0,
			FigureNotes: figureNotes != nil,
			PaperNotes:  paperNotes != nil,
		},
		Replication: FigureResearchReplication{
			HasSourceData: false,
			HasCode:       false,
			Mode:          "visual_reference_only",
			Summary:       "This package does not include source data or plotting code. It can be used as a visual reference, not as a faithful scientific remake.",
		},
	}
}

func officialFigureLabel(caption, fallback string) string {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return strings.TrimSpace(fallback)
	}
	match := officialFigureLabelPattern.FindString(caption)
	if match == "" {
		return strings.TrimSpace(fallback)
	}
	return strings.Join(strings.Fields(match), " ")
}

func paperTransferSourceID(paperID int64) string {
	return fmt.Sprintf("citebox:paper:%d", paperID)
}

func collectFigureTransferExcerpts(paper *model.Paper, figure model.Figure, displayLabel, officialLabel, parentDisplay string, pageTexts []string) []FigureResearchExcerpt {
	queries := figureTransferExcerptQueries(figure, displayLabel, officialLabel, parentDisplay)
	if len(queries) == 0 {
		return []FigureResearchExcerpt{}
	}

	excerpts := make([]FigureResearchExcerpt, 0, defaultTransferExcerptLimit)
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
		excerpts = append(excerpts, FigureResearchExcerpt{
			Snippet:     snippet,
			Page:        page,
			MatchReason: query,
			SourceID:    paperTransferSourceID(paper.ID),
		})
		return len(excerpts) >= defaultTransferExcerptLimit
	}

	if pageTexts == nil {
		for _, query := range queries {
			for _, snippet := range findBoundedTextMatches(paper.PDFText, query, defaultTransferExcerptContext) {
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
			for _, snippet := range findBoundedTextMatches(pageText, query, defaultTransferExcerptContext) {
				if appendMatch(&page, snippet, query) {
					return excerpts
				}
			}
		}
	}
	return excerpts
}

func figureTransferExcerptQueries(figure model.Figure, displayLabel, officialLabel, parentDisplay string) []string {
	queries := make([]string, 0, 10)
	add := func(raw string) {
		query := compactTransferQuery(raw)
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

	add(displayLabel)
	add(officialLabel)
	add(parentDisplay)
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

func compactTransferQuery(raw string) string {
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

func contextWindow(text string, byteIndex, contextChars int) string {
	if contextChars <= 0 {
		return ""
	}
	runes := []rune(text)
	startByte := 0
	startRune := 0
	for startRune < len(runes) {
		width := utf8.RuneLen(runes[startRune])
		if startByte+width > byteIndex {
			break
		}
		startByte += width
		startRune++
	}
	from := startRune - contextChars/2
	if from < 0 {
		from = 0
	}
	to := from + contextChars
	if to > len(runes) {
		to = len(runes)
		from = to - contextChars
		if from < 0 {
			from = 0
		}
	}
	return strings.TrimSpace(string(runes[from:to]))
}

func renderFigureHandoffMarkdown(ctx FigureResearchContext, imageName string) string {
	var b strings.Builder
	b.WriteString("# Figure Transfer Package\n\n")
	b.WriteString("## 图\n\n")
	alt := firstNonEmpty(ctx.Figure.OfficialLabel, ctx.Figure.DisplayLabel, "figure")
	fmt.Fprintf(&b, "![%s](%s)\n\n", escapeMarkdownAlt(alt), imageName)
	b.WriteString("### 原始图注\n\n")
	b.WriteString(handoffText(ctx.Figure.Caption))
	b.WriteString("\n\n## 文献出处\n\n")
	fmt.Fprintf(&b, "- 题目：%s\n", handoffInline(ctx.Paper.Title))
	fmt.Fprintf(&b, "- 作者：%s\n", handoffList(ctx.Paper.Authors))
	fmt.Fprintf(&b, "- 期刊：%s\n", handoffPointer(ctx.Paper.Journal))
	fmt.Fprintf(&b, "- 年份：%s\n", handoffYear(ctx.Paper.Year))
	fmt.Fprintf(&b, "- DOI：%s\n", handoffPointer(ctx.Paper.DOI))
	fmt.Fprintf(&b, "- URL：%s\n\n", handoffPointer(ctx.Paper.URL))
	b.WriteString("## 摘要\n\n")
	b.WriteString(handoffText(ctx.Paper.Abstract))
	b.WriteString("\n\n## 图身份\n\n")
	fmt.Fprintf(&b, "- CiteBox 标签：%s\n", handoffInline(ctx.Figure.DisplayLabel))
	fmt.Fprintf(&b, "- 论文图号：%s\n", handoffInline(ctx.Figure.OfficialLabel))
	fmt.Fprintf(&b, "- 页码：%s\n", handoffPage(ctx.Figure.PageNumber))
	fmt.Fprintf(&b, "- 类型：%s\n", handoffInline(ctx.Figure.Kind))
	fmt.Fprintf(&b, "- 提取方式：%s\n\n", handoffInline(ctx.Figure.Extraction))
	b.WriteString("## 相关正文摘录\n\n")
	if len(ctx.Excerpts) == 0 {
		b.WriteString("（无匹配摘录）\n")
	} else {
		for i, excerpt := range ctx.Excerpts {
			fmt.Fprintf(&b, "### 摘录 %d\n\n", i+1)
			fmt.Fprintf(&b, "- 页码：%s\n", handoffPage(excerpt.Page))
			fmt.Fprintf(&b, "- 匹配原因：%s\n\n", handoffInline(excerpt.MatchReason))
			b.WriteString(handoffText(excerpt.Snippet))
			b.WriteString("\n\n")
		}
	}
	b.WriteString("## 笔记\n\n")
	fmt.Fprintf(&b, "- 图片笔记：%s\n", handoffPointer(ctx.Notes.FigureNotes))
	fmt.Fprintf(&b, "- 文献笔记：%s\n\n", handoffPointer(ctx.Notes.PaperNotes))
	b.WriteString("## 完整性\n\n")
	fmt.Fprintf(&b, "- 题目：%s\n", handoffFlag(ctx.Completeness.Title))
	fmt.Fprintf(&b, "- 作者：%s\n", handoffFlag(ctx.Completeness.Authors))
	fmt.Fprintf(&b, "- 年份：%s\n", handoffFlag(ctx.Completeness.Year))
	fmt.Fprintf(&b, "- 期刊：%s\n", handoffFlag(ctx.Completeness.Journal))
	fmt.Fprintf(&b, "- DOI：%s\n", handoffFlag(ctx.Completeness.DOI))
	fmt.Fprintf(&b, "- 摘要：%s\n", handoffFlag(ctx.Completeness.Abstract))
	fmt.Fprintf(&b, "- 图注：%s\n", handoffFlag(ctx.Completeness.Caption))
	fmt.Fprintf(&b, "- 逐页文本：%s\n", handoffFlag(ctx.Completeness.PageTexts))
	fmt.Fprintf(&b, "- 正文摘录：%s\n", handoffFlag(ctx.Completeness.Excerpts))
	fmt.Fprintf(&b, "- 图片笔记：%s\n\n", handoffFlag(ctx.Completeness.FigureNotes))
	b.WriteString("## 复刻边界\n\n")
	b.WriteString("本包不含原始绘图数据或代码。它可作为 visual reference，不能忠实复现原图数据或分析结果。\n")
	return b.String()
}

func handoffText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return missingHandoffValue
	}
	return value
}

func handoffInline(value string) string {
	return handoffText(value)
}

func handoffPointer(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return missingHandoffValue
	}
	return strings.TrimSpace(*value)
}

func handoffYear(value *int) string {
	if value == nil {
		return missingHandoffValue
	}
	return fmt.Sprintf("%d", *value)
}

func handoffPage(value *int) string {
	if value == nil {
		return missingHandoffValue
	}
	return fmt.Sprintf("%d", *value)
}

func handoffList(values []string) string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return missingHandoffValue
	}
	return strings.Join(cleaned, "; ")
}

func handoffFlag(ok bool) string {
	if ok {
		return "完整"
	}
	return "缺失"
}

func escapeMarkdownAlt(value string) string {
	value = strings.ReplaceAll(value, "[", "(")
	value = strings.ReplaceAll(value, "]", ")")
	return value
}
