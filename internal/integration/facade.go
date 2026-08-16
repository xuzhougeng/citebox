package integration

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/buildinfo"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

// MCP 工具名
const (
	ToolGetCapabilities = "citebox_get_capabilities"
	ToolSearchLibrary   = "citebox_search_library"
	ToolGetPaperContext = "citebox_get_paper_context"
	ToolSearchPaperText = "citebox_search_paper_text"
	ToolGetEntity       = "citebox_get_entity"
	ToolExportAsset     = "citebox_export_asset"
	ToolListChanges     = "citebox_list_changes"
)

const (
	defaultSearchLimit     = 20
	maxSearchLimit         = 100
	searchBatchSize        = 50
	defaultFigureLimit     = 20
	defaultAnnotationLimit = 50
	defaultTextSearchLimit = 12
	defaultContextChars    = 1200
	defaultChangesLimit    = 100
	maxChangesLimit        = 500
	snippetRunes           = 200
)

// Facade 面向外部工具的只读研究上下文门面，组合图书馆服务、仓库、资产暂存区和集成设置
type Facade struct {
	library  *service.LibraryService
	repo     *repository.LibraryRepository
	assets   *AssetStore
	settings *Service
}

// NewFacade 创建研究上下文门面
func NewFacade(library *service.LibraryService, repo *repository.LibraryRepository, assets *AssetStore, settings *Service) *Facade {
	return &Facade{library: library, repo: repo, assets: assets, settings: settings}
}

// ToolNames 返回 MCP 适配层暴露的工具名列表
func ToolNames() []string {
	return []string{
		ToolGetCapabilities,
		ToolSearchLibrary,
		ToolGetPaperContext,
		ToolSearchPaperText,
		ToolGetEntity,
		ToolExportAsset,
		ToolListChanges,
	}
}

// GetCapabilities 返回集成能力描述
func (f *Facade) GetCapabilities() map[string]any {
	return map[string]any{
		"citebox_version":         buildinfo.CurrentVersion(),
		"research_context_schema": ResearchContextSchema,
		"transfer_package_schema": service.FigureTransferSchemaName,
		"entity_types":            []string{EntityTypePaper, EntityTypeFigure, EntityTypeNote, EntityTypeAnnotation},
		"scopes":                  ReadScopes(),
		"max_page_size":           maxSearchLimit,
		"max_changes_limit":       maxChangesLimit,
		"tools":                   ToolNames(),
	}
}

// ========== search_library ==========

// searchOffsets 是 search_library 游标中各实体的消费偏移（按原始行数推进）
type searchOffsets struct {
	Paper      int `json:"paper"`
	Figure     int `json:"figure"`
	NotePaper  int `json:"note_paper"`
	NoteFigure int `json:"note_figure"`
	Annotation int `json:"annotation"`
}

type searchCursor struct {
	Offsets searchOffsets `json:"offsets"`
}

type searchHit struct {
	item      SearchLibraryItem
	updatedAt time.Time
}

// searchKindState 跟踪单个检索源的分页偏移和抓取函数
type searchKindState struct {
	name   string
	offset int
	fetch  func(offset, limit int) ([]searchHit, int, error)
}

// SearchLibrary 跨实体检索图书馆。按 paper→figure→note→annotation 的固定顺序合并结果；
// 游标记录各实体已消费的原始行数，UpdatedAfter 按批次后置过滤（可能返回短页，但游标保持一致）
func (f *Facade) SearchLibrary(p SearchLibraryParams) (*SearchLibraryResult, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	offsets := searchOffsets{}
	if strings.TrimSpace(p.Cursor) != "" {
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(p.Cursor))
		if err != nil {
			return nil, apperr.New(apperr.CodeInvalidArgument, "游标格式无效")
		}
		var cursor searchCursor
		if err := json.Unmarshal(raw, &cursor); err != nil {
			return nil, apperr.New(apperr.CodeInvalidArgument, "游标格式无效")
		}
		offsets = cursor.Offsets
	}

	states, err := f.searchKindStates(p, offsets)
	if err != nil {
		return nil, err
	}

	items := []SearchLibraryItem{}
	remaining := limit
	more := false
	for i := range states {
		st := &states[i]
		for remaining > 0 {
			raw, total, err := st.fetch(st.offset, searchBatchSize)
			if err != nil {
				return nil, err
			}
			if len(raw) == 0 {
				break
			}
			for _, hit := range raw {
				st.offset++
				if p.UpdatedAfter != nil && !hit.updatedAt.After(*p.UpdatedAfter) {
					continue
				}
				items = append(items, hit.item)
				remaining--
				if remaining == 0 {
					break
				}
			}
			if remaining == 0 {
				if st.offset < total {
					more = true
				}
				break
			}
			if st.offset >= total {
				break
			}
		}
		if remaining == 0 {
			// 当前类型已占满配额，探测后续类型是否还有数据
			for j := i + 1; j < len(states) && !more; j++ {
				raw, _, err := states[j].fetch(states[j].offset, 1)
				if err != nil {
					return nil, err
				}
				if len(raw) > 0 {
					more = true
				}
			}
			break
		}
	}

	result := &SearchLibraryResult{Items: items}
	if more {
		result.NextCursor = encodeSearchOffsets(searchOffsetsFromStates(states))
	}
	return result, nil
}

// searchKindStates 按固定顺序构建请求的检索源；note 类型展开为文献笔记和图片笔记两个源
func (f *Facade) searchKindStates(p SearchLibraryParams, offsets searchOffsets) ([]searchKindState, error) {
	wanted := map[string]bool{}
	if len(p.EntityTypes) == 0 {
		for _, t := range []string{EntityTypePaper, EntityTypeFigure, EntityTypeNote, EntityTypeAnnotation} {
			wanted[t] = true
		}
	} else {
		for _, t := range p.EntityTypes {
			t = strings.TrimSpace(t)
			switch t {
			case EntityTypePaper, EntityTypeFigure, EntityTypeNote, EntityTypeAnnotation:
				wanted[t] = true
			default:
				return nil, apperr.New(apperr.CodeInvalidArgument, "未知的实体类型: "+t)
			}
		}
	}

	states := []searchKindState{}
	if wanted[EntityTypePaper] {
		states = append(states, searchKindState{name: EntityTypePaper, offset: offsets.Paper, fetch: f.makePaperSearch(p, false)})
	}
	if wanted[EntityTypeFigure] {
		states = append(states, searchKindState{name: EntityTypeFigure, offset: offsets.Figure, fetch: f.makeFigureSearch(p, false)})
	}
	if wanted[EntityTypeNote] {
		states = append(states,
			searchKindState{name: "note_paper", offset: offsets.NotePaper, fetch: f.makePaperSearch(p, true)},
			searchKindState{name: "note_figure", offset: offsets.NoteFigure, fetch: f.makeFigureSearch(p, true)},
		)
	}
	if wanted[EntityTypeAnnotation] {
		states = append(states, searchKindState{name: EntityTypeAnnotation, offset: offsets.Annotation, fetch: f.makeAnnotationSearch(p)})
	}
	return states, nil
}

func searchOffsetsFromStates(states []searchKindState) searchOffsets {
	offsets := searchOffsets{}
	for _, st := range states {
		switch st.name {
		case EntityTypePaper:
			offsets.Paper = st.offset
		case EntityTypeFigure:
			offsets.Figure = st.offset
		case "note_paper":
			offsets.NotePaper = st.offset
		case "note_figure":
			offsets.NoteFigure = st.offset
		case EntityTypeAnnotation:
			offsets.Annotation = st.offset
		}
	}
	return offsets
}

func encodeSearchOffsets(offsets searchOffsets) string {
	raw, err := json.Marshal(searchCursor{Offsets: offsets})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// skipHits 丢弃偏移落在当前页内的前 skip 条结果，保证跨页游标不重不漏
func skipHits(hits []searchHit, skip int) []searchHit {
	if skip >= len(hits) {
		return nil
	}
	return hits[skip:]
}

// makePaperSearch 构造文献检索源；notesOnly 为 true 时仅检索有文献笔记的文献（作为 note 结果）
func (f *Facade) makePaperSearch(p SearchLibraryParams, notesOnly bool) func(offset, limit int) ([]searchHit, int, error) {
	return func(offset, limit int) ([]searchHit, int, error) {
		if limit <= 0 {
			limit = 1
		}
		filter := model.PaperFilter{
			Keyword:       strings.TrimSpace(p.Query),
			GroupID:       p.GroupID,
			HasPaperNotes: notesOnly,
			Page:          offset/limit + 1,
			PageSize:      limit,
		}
		if len(p.Tags) > 0 {
			// 目前仅应用第一个标签作为过滤条件，多标签交集暂不支持
			tagID := p.Tags[0]
			filter.TagID = &tagID
		}
		resp, err := f.library.ListPapers(filter)
		if err != nil {
			return nil, 0, err
		}
		hits := make([]searchHit, 0, len(resp.Papers))
		for _, paper := range resp.Papers {
			if notesOnly {
				hits = append(hits, searchHit{
					updatedAt: paper.UpdatedAt,
					item: SearchLibraryItem{
						SourceID:   PaperNoteSourceID(paper.ID),
						EntityType: EntityTypeNote,
						Revision:   RevisionOf(paper.UpdatedAt),
						Title:      paper.Title,
						Snippet:    truncateRunes(paper.PaperNotesText, snippetRunes),
						Data: map[string]any{
							"paper_id": paper.ID,
							"text":     paper.PaperNotesText,
						},
					},
				})
				continue
			}
			hits = append(hits, searchHit{
				updatedAt: paper.UpdatedAt,
				item: SearchLibraryItem{
					SourceID:   PaperSourceID(paper.ID),
					EntityType: EntityTypePaper,
					Revision:   RevisionOf(paper.UpdatedAt),
					Title:      paper.Title,
					Snippet:    truncateRunes(paper.AbstractText, snippetRunes),
					Data: map[string]any{
						"id":           paper.ID,
						"title":        paper.Title,
						"authors_text": paper.AuthorsText,
						"journal":      paper.Journal,
						"published_at": paper.PublishedAt,
						"doi":          paper.DOI,
						"figure_count": paper.FigureCount,
					},
				},
			})
		}
		return skipHits(hits, offset%limit), resp.Total, nil
	}
}

// makeFigureSearch 构造图片检索源；notesOnly 为 true 时仅检索有笔记的图片（作为 note 结果）
func (f *Facade) makeFigureSearch(p SearchLibraryParams, notesOnly bool) func(offset, limit int) ([]searchHit, int, error) {
	return func(offset, limit int) ([]searchHit, int, error) {
		if limit <= 0 {
			limit = 1
		}
		filter := model.FigureFilter{
			Keyword:  strings.TrimSpace(p.Query),
			GroupID:  p.GroupID,
			HasNotes: notesOnly,
			Page:     offset/limit + 1,
			PageSize: limit,
		}
		if len(p.Tags) > 0 {
			// 与文献检索一致，仅应用第一个标签
			tagID := p.Tags[0]
			filter.TagID = &tagID
		}
		resp, err := f.library.ListFigures(filter)
		if err != nil {
			return nil, 0, err
		}
		hits := make([]searchHit, 0, len(resp.Figures))
		for _, figure := range resp.Figures {
			if notesOnly {
				hits = append(hits, searchHit{
					updatedAt: figure.UpdatedAt,
					item: SearchLibraryItem{
						SourceID:   FigureNoteSourceID(figure.ID),
						EntityType: EntityTypeNote,
						Revision:   RevisionOf(figure.UpdatedAt),
						Title:      figure.PaperTitle,
						Snippet:    truncateRunes(figure.NotesText, snippetRunes),
						Data: map[string]any{
							"figure_id": figure.ID,
							"paper_id":  figure.PaperID,
							"text":      figure.NotesText,
						},
					},
				})
				continue
			}
			hits = append(hits, searchHit{
				updatedAt: figure.UpdatedAt,
				item: SearchLibraryItem{
					SourceID:   FigureSourceID(figure.ID),
					EntityType: EntityTypeFigure,
					Revision:   RevisionOf(figure.UpdatedAt),
					Title:      figure.PaperTitle,
					Snippet:    truncateRunes(figure.Caption, snippetRunes),
					Data: map[string]any{
						"id":            figure.ID,
						"paper_id":      figure.PaperID,
						"paper_title":   figure.PaperTitle,
						"caption":       figure.Caption,
						"page_number":   figure.PageNumber,
						"display_label": figure.DisplayLabel,
					},
				},
			})
		}
		return skipHits(hits, offset%limit), resp.Total, nil
	}
}

// makeAnnotationSearch 构造 PDF 标注检索源（不支持分组和标签过滤）
func (f *Facade) makeAnnotationSearch(p SearchLibraryParams) func(offset, limit int) ([]searchHit, int, error) {
	return func(offset, limit int) ([]searchHit, int, error) {
		if limit <= 0 {
			limit = 1
		}
		resp, err := f.library.ListPDFAnnotationsGlobal(service.ListPDFAnnotationsParams{
			Query:    strings.TrimSpace(p.Query),
			Page:     offset/limit + 1,
			PageSize: limit,
		})
		if err != nil {
			return nil, 0, err
		}
		hits := make([]searchHit, 0, len(resp.Annotations))
		for _, annotation := range resp.Annotations {
			hits = append(hits, searchHit{
				updatedAt: annotation.UpdatedAt,
				item: SearchLibraryItem{
					SourceID:   AnnotationSourceID(annotation.ID),
					EntityType: EntityTypeAnnotation,
					Revision:   RevisionOf(annotation.UpdatedAt),
					Title:      annotation.PaperTitle,
					Snippet:    truncateRunes(annotation.QuoteText, snippetRunes),
					Data: map[string]any{
						"id":          annotation.ID,
						"paper_id":    annotation.PaperID,
						"paper_title": annotation.PaperTitle,
						"type":        annotation.Type,
						"quote_text":  annotation.QuoteText,
						"note_text":   annotation.NoteText,
						"page_start":  annotation.PageStart,
						"page_end":    annotation.PageEnd,
						"color":       annotation.Color,
					},
				},
			})
		}
		return skipHits(hits, offset%limit), resp.Pagination.Total, nil
	}
}

// ========== get_paper_context ==========

// GetPaperContext 打包单篇文献的研究上下文信封
func (f *Facade) GetPaperContext(p GetPaperContextParams) (*Envelope, error) {
	if p.PaperID <= 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "paper_id 无效")
	}
	includeAll := len(p.Include) == 0
	include := map[string]bool{}
	for _, name := range p.Include {
		include[strings.TrimSpace(name)] = true
	}
	want := func(name string) bool { return includeAll || include[name] }

	figureLimit := p.FigureLimit
	if figureLimit <= 0 {
		figureLimit = defaultFigureLimit
	}
	annotationLimit := p.AnnotationLimit
	if annotationLimit <= 0 {
		annotationLimit = defaultAnnotationLimit
	}

	paper, err := f.library.GetPaper(p.PaperID)
	if err != nil {
		return nil, err
	}

	// 顶层图片（子图嵌套在其中），按 figure_limit 截断
	figures := make([]model.Figure, 0, len(paper.Figures))
	for _, figure := range paper.Figures {
		if figure.ParentFigureID != nil {
			continue
		}
		if len(figures) >= figureLimit {
			break
		}
		figures = append(figures, figure)
	}

	var annotations []model.PDFAnnotation
	if want(IncludeAnnotations) {
		annotations, err = f.library.ListPDFAnnotations(p.PaperID)
		if err != nil {
			return nil, err
		}
		if len(annotations) > annotationLimit {
			annotations = annotations[:annotationLimit]
		}
	}

	data := map[string]any{}
	if want(IncludeMetadata) {
		data["metadata"] = map[string]any{
			"id":                paper.ID,
			"title":             paper.Title,
			"authors_text":      paper.AuthorsText,
			"journal":           paper.Journal,
			"published_at":      paper.PublishedAt,
			"doi":               paper.DOI,
			"original_filename": paper.OriginalFilename,
			"extraction_status": paper.ExtractionStatus,
			"figure_count":      paper.FigureCount,
			"created_at":        paper.CreatedAt.UTC().Format(time.RFC3339),
			"updated_at":        paper.UpdatedAt.UTC().Format(time.RFC3339),
		}
	}
	if want(IncludeAbstract) {
		data["abstract"] = paper.AbstractText
	}
	if want(IncludePaperNotes) {
		data["paper_notes"] = paper.PaperNotesText
	}
	if want(IncludeFigureNotes) {
		// 图片笔记不受 figure_limit 限制，覆盖所有含笔记的图片（含子图）
		figureNotes := []map[string]any{}
		for _, figure := range paper.Figures {
			if strings.TrimSpace(figure.NotesText) == "" {
				continue
			}
			figureNotes = append(figureNotes, map[string]any{
				"figure_id":     figure.ID,
				"source_id":     FigureSourceID(figure.ID),
				"display_label": figure.DisplayLabel,
				"notes_text":    figure.NotesText,
			})
		}
		data["figure_notes"] = figureNotes
	}
	if want(IncludeFigures) {
		figureData := make([]map[string]any, 0, len(figures))
		for _, figure := range figures {
			entry := map[string]any{
				"figure_id":     figure.ID,
				"source_id":     FigureSourceID(figure.ID),
				"caption":       figure.Caption,
				"display_label": figure.DisplayLabel,
				"page_number":   figure.PageNumber,
			}
			if len(figure.Subfigures) > 0 {
				subfigures := make([]map[string]any, 0, len(figure.Subfigures))
				for _, sub := range figure.Subfigures {
					subfigures = append(subfigures, map[string]any{
						"figure_id":       sub.ID,
						"source_id":       FigureSourceID(sub.ID),
						"caption":         sub.Caption,
						"display_label":   sub.DisplayLabel,
						"subfigure_label": sub.SubfigureLabel,
					})
				}
				entry["subfigures"] = subfigures
			}
			figureData = append(figureData, entry)
		}
		data["figures"] = figureData
	}
	if want(IncludeAnnotations) {
		data["annotations"] = annotations
	}
	if want(IncludeTags) {
		data["tags"] = paper.Tags
	}
	if want(IncludeGroup) {
		group := map[string]any{}
		if paper.GroupID != nil {
			group["group_id"] = *paper.GroupID
			group["group_name"] = paper.GroupName
		}
		data["group"] = group
	}

	relations := []any{}
	if want(IncludeFigures) {
		for _, figure := range figures {
			relations = append(relations, map[string]any{"type": EntityTypeFigure, "source_id": FigureSourceID(figure.ID)})
		}
	}
	if want(IncludeAnnotations) {
		for _, annotation := range annotations {
			relations = append(relations, map[string]any{"type": EntityTypeAnnotation, "source_id": AnnotationSourceID(annotation.ID)})
		}
	}
	if want(IncludeTags) {
		for _, tag := range paper.Tags {
			relations = append(relations, map[string]any{"type": "tag", "id": tag.ID, "name": tag.Name})
		}
	}
	if want(IncludeGroup) && paper.GroupID != nil {
		relations = append(relations, map[string]any{"type": "group", "id": *paper.GroupID, "name": paper.GroupName})
	}

	// 资产描述符不直接给下载地址，外部工具通过 export_asset 按需换取
	assetDescriptors := []any{}
	if want(IncludeFigures) {
		for _, figure := range figures {
			assetDescriptors = append(assetDescriptors, map[string]any{
				"kind":      AssetKindFigureImage,
				"figure_id": figure.ID,
				"source_id": FigureSourceID(figure.ID),
			})
		}
	}

	envelope := NewEnvelope(SourceRef{Kind: EntityTypePaper, ID: paper.ID}, paper.UpdatedAt, data)
	if len(relations) > 0 {
		envelope.Relations = relations
	}
	if len(assetDescriptors) > 0 {
		envelope.Assets = assetDescriptors
	}
	return envelope, nil
}

// ========== search_paper_text ==========

// SearchPaperText 在指定文献的 PDF 全文中做大小写不敏感的子串检索
func (f *Facade) SearchPaperText(p SearchPaperTextParams) (*SearchPaperTextResult, error) {
	query := strings.TrimSpace(p.Query)
	if query == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "检索关键词不能为空")
	}
	if len(p.PaperIDs) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "paper_ids 不能为空")
	}
	limit := p.Limit
	if limit <= 0 {
		limit = defaultTextSearchLimit
	}
	contextChars := p.ContextChars
	if contextChars <= 0 {
		contextChars = defaultContextChars
	}

	matches := []PaperTextMatch{}
	for _, paperID := range p.PaperIDs {
		if len(matches) >= limit {
			break
		}
		if paperID <= 0 {
			return nil, apperr.New(apperr.CodeInvalidArgument, "paper_id 无效")
		}
		pageTexts, err := f.repo.GetPaperPDFPageTexts(paperID)
		if err != nil {
			return nil, err
		}
		paper, err := f.library.GetPaper(paperID)
		if err != nil {
			return nil, err
		}
		revision := RevisionOf(paper.UpdatedAt)
		if pageTexts == nil {
			// 未存储逐页文本时退化为整篇 PDFText，页码为 null
			for _, snippet := range findTextMatches(paper.PDFText, query, contextChars) {
				matches = append(matches, PaperTextMatch{
					PaperID:  paperID,
					Page:     nil,
					Snippet:  snippet,
					SourceID: PaperSourceID(paperID),
					Revision: revision,
				})
				if len(matches) >= limit {
					break
				}
			}
			continue
		}
	outer:
		for i, pageText := range pageTexts {
			page := i + 1
			for _, snippet := range findTextMatches(pageText, query, contextChars) {
				matches = append(matches, PaperTextMatch{
					PaperID:  paperID,
					Page:     &page,
					Snippet:  snippet,
					SourceID: PaperSourceID(paperID),
					Revision: revision,
				})
				if len(matches) >= limit {
					break outer
				}
			}
		}
	}
	return &SearchPaperTextResult{Matches: matches}, nil
}

// findTextMatches 返回所有大小写不敏感命中的上下文片段
func findTextMatches(text, query string, contextChars int) []string {
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	if lowerQuery == "" {
		return nil
	}
	var snippets []string
	for offset := 0; offset < len(lowerText); {
		idx := strings.Index(lowerText[offset:], lowerQuery)
		if idx < 0 {
			break
		}
		start := offset + idx
		snippets = append(snippets, contextWindow(text, start, contextChars))
		offset = start + len(lowerQuery)
	}
	return snippets
}

// contextWindow 以字节位置为中心截取 rune 安全的上下文窗口
func contextWindow(text string, byteStart, contextChars int) string {
	if contextChars <= 0 {
		contextChars = defaultContextChars
	}
	if byteStart > len(text) {
		byteStart = len(text)
	}
	runes := []rune(text)
	center := len([]rune(text[:byteStart]))
	from := center - contextChars/2
	if from < 0 {
		from = 0
	}
	to := from + contextChars
	if to > len(runes) {
		to = len(runes)
	}
	return strings.TrimSpace(string(runes[from:to]))
}

// ========== get_entity ==========

// GetEntity 按 source_id 获取单个实体的信封
func (f *Facade) GetEntity(sourceID string) (*Envelope, error) {
	ref, err := ParseSourceID(sourceID)
	if err != nil {
		return nil, err
	}
	switch ref.Kind {
	case EntityTypePaper:
		paper, err := f.library.GetPaper(ref.ID)
		if err != nil {
			return nil, err
		}
		return NewEnvelope(ref, paper.UpdatedAt, paper), nil
	case EntityTypeFigure:
		figure, err := f.library.GetFigure(ref.ID)
		if err != nil {
			return nil, err
		}
		if figure == nil {
			return nil, apperr.New(apperr.CodeNotFound, "figure not found")
		}
		return NewEnvelope(ref, figure.UpdatedAt, figure), nil
	case EntityTypeAnnotation:
		annotation, err := f.repo.GetPDFAnnotation(ref.ID)
		if err != nil {
			return nil, err
		}
		return NewEnvelope(ref, annotation.UpdatedAt, annotation), nil
	case EntityTypeNote:
		if ref.NoteName != "main" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "笔记名称无效")
		}
		if ref.NoteParent == EntityTypeFigure {
			figure, err := f.library.GetFigure(ref.ID)
			if err != nil {
				return nil, err
			}
			if figure == nil {
				return nil, apperr.New(apperr.CodeNotFound, "figure not found")
			}
			return NewEnvelope(ref, figure.UpdatedAt, map[string]any{"text": figure.NotesText}), nil
		}
		paper, err := f.library.GetPaper(ref.ID)
		if err != nil {
			return nil, err
		}
		return NewEnvelope(ref, paper.UpdatedAt, map[string]any{"text": paper.PaperNotesText}), nil
	default:
		return nil, apperr.New(apperr.CodeInvalidArgument, "source_id 格式无效")
	}
}

// ========== export_asset ==========

// ExportAsset 导出资产到暂存区，返回带过期时间的下载描述
func (f *Facade) ExportAsset(kind string, id int64) (*AssetExport, error) {
	if id <= 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "资产 ID 无效")
	}
	var data []byte
	var mediaType, filename string
	switch strings.TrimSpace(kind) {
	case AssetKindFigureImage:
		imageData, imageMIME, imageFilename, err := f.library.GetFigureImage(id)
		if err != nil {
			return nil, err
		}
		data, mediaType, filename = imageData, imageMIME, imageFilename
	case AssetKindFigureTransferPackage:
		pkg, err := f.library.ExportFigureTransferPackage(id)
		if err != nil {
			return nil, err
		}
		data, mediaType, filename = pkg.Data, "application/zip", pkg.Filename
	default:
		return nil, apperr.New(apperr.CodeInvalidArgument, "未知的资产类型")
	}

	sum := sha256.Sum256(data)
	assetID, expiresAt := f.assets.Put(data, mediaType, filename, 0)
	return &AssetExport{
		URL:       fmt.Sprintf("%s/assets/%s", f.settings.BaseURL(), assetID),
		ByteSize:  len(data),
		MediaType: mediaType,
		SHA256:    hex.EncodeToString(sum[:]),
		ExpiresAt: expiresAt.UTC(),
	}, nil
}

// ========== list_changes ==========

// changeWatermark 是单类实体的增量水位线；各表 ID 空间独立，不能跨表复用
type changeWatermark struct {
	T  time.Time `json:"t"`
	ID int64     `json:"id"`
}

type changesCursor struct {
	Paper      changeWatermark `json:"paper"`
	Figure     changeWatermark `json:"figure"`
	Annotation changeWatermark `json:"annotation"`
}

type changeRow struct {
	sourceID  string
	updatedAt time.Time
	id        int64
}

// ListChanges 按 (updated_at, id) 水位线增量同步变更，固定顺序 paper→figure→annotation。
// 笔记变更承载在父 paper/figure 行上，不单独跟踪。next_cursor 始终返回（即使当前没有
// 更多变更），客户端应保存最近一次游标用于后续轮询，避免重复拉取。
func (f *Facade) ListChanges(p ListChangesParams) (*ListChangesResult, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = defaultChangesLimit
	}
	if limit > maxChangesLimit {
		limit = maxChangesLimit
	}

	cursor := changesCursor{}
	if strings.TrimSpace(p.Cursor) != "" {
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(p.Cursor))
		if err != nil {
			return nil, apperr.New(apperr.CodeInvalidArgument, "游标格式无效")
		}
		if err := json.Unmarshal(raw, &cursor); err != nil {
			return nil, apperr.New(apperr.CodeInvalidArgument, "游标格式无效")
		}
	}

	wanted := map[string]bool{
		EntityTypePaper:      true,
		EntityTypeFigure:     true,
		EntityTypeAnnotation: true,
	}
	if len(p.EntityTypes) > 0 {
		wanted = map[string]bool{}
		for _, t := range p.EntityTypes {
			t = strings.TrimSpace(t)
			switch t {
			case EntityTypePaper, EntityTypeFigure, EntityTypeAnnotation:
				wanted[t] = true
			case EntityTypeNote:
				// note 变更通过父 paper/figure 行体现
			default:
				return nil, apperr.New(apperr.CodeInvalidArgument, "未知的实体类型: "+t)
			}
		}
	}

	changes := []ChangeItem{}
	more := false
	if wanted[EntityTypePaper] {
		typeMore, err := consumeTypeChanges(&cursor.Paper, limit-len(changes), f.fetchChangedPapers, &changes)
		if err != nil {
			return nil, err
		}
		more = more || typeMore
	}
	if wanted[EntityTypeFigure] {
		typeMore, err := consumeTypeChanges(&cursor.Figure, limit-len(changes), f.fetchChangedFigures, &changes)
		if err != nil {
			return nil, err
		}
		more = more || typeMore
	}
	if wanted[EntityTypeAnnotation] {
		typeMore, err := consumeTypeChanges(&cursor.Annotation, limit-len(changes), f.fetchChangedAnnotations, &changes)
		if err != nil {
			return nil, err
		}
		more = more || typeMore
	}
	_ = more

	return &ListChangesResult{Changes: changes, NextCursor: encodeChangesCursor(cursor)}, nil
}

// consumeTypeChanges 拉取单类实体的变更并推进水位线；预算为 0 时只探测是否还有变更
func consumeTypeChanges(wm *changeWatermark, budget int, fetch func(since time.Time, afterID int64, limit int) ([]changeRow, error), changes *[]ChangeItem) (bool, error) {
	if budget <= 0 {
		rows, err := fetch(wm.T, wm.ID, 1)
		if err != nil {
			return false, err
		}
		return len(rows) > 0, nil
	}
	rows, err := fetch(wm.T, wm.ID, budget+1)
	if err != nil {
		return false, err
	}
	hasMore := false
	if len(rows) > budget {
		hasMore = true
		rows = rows[:budget]
	}
	for _, row := range rows {
		*changes = append(*changes, ChangeItem{
			Operation: "updated",
			SourceID:  row.sourceID,
			Revision:  RevisionOf(row.updatedAt),
		})
		*wm = changeWatermark{T: row.updatedAt, ID: row.id}
	}
	return hasMore, nil
}

func (f *Facade) fetchChangedPapers(since time.Time, afterID int64, limit int) ([]changeRow, error) {
	rows, err := f.repo.ListPapersChangedSince(since, afterID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]changeRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, changeRow{sourceID: PaperSourceID(row.ID), updatedAt: row.UpdatedAt, id: row.ID})
	}
	return out, nil
}

func (f *Facade) fetchChangedFigures(since time.Time, afterID int64, limit int) ([]changeRow, error) {
	rows, err := f.repo.ListFiguresChangedSince(since, afterID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]changeRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, changeRow{sourceID: FigureSourceID(row.ID), updatedAt: row.UpdatedAt, id: row.ID})
	}
	return out, nil
}

func (f *Facade) fetchChangedAnnotations(since time.Time, afterID int64, limit int) ([]changeRow, error) {
	rows, err := f.repo.ListPDFAnnotationsChangedSince(since, afterID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]changeRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, changeRow{sourceID: AnnotationSourceID(row.ID), updatedAt: row.UpdatedAt, id: row.ID})
	}
	return out, nil
}

func encodeChangesCursor(cursor changesCursor) string {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// truncateRunes 按 rune 截断文本
func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
