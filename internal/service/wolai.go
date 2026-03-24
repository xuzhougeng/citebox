package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	wolaiapi "github.com/xuzhougeng/citebox/internal/wolai"
)

const (
	wolaiSettingsKey           = "wolai_settings"
	wolaiTextBlockMaxSize      = 1600
	wolaiNoteImageTODOText     = "暂不支持保存到 Wolai，等待后续完成。"
	wolaiTestPageImageTODOText = "TODO：Wolai 图片导出尚未实现，等待后续完成。"
)

var wolaiMarkdownImagePattern = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)\)`)
var wolaiMarkdownHeadingPattern = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
var wolaiMarkdownOrderedListPattern = regexp.MustCompile(`^\s*\d+\.\s+(.+)$`)
var wolaiMarkdownBulletListPattern = regexp.MustCompile(`^\s*[-+*]\s+(.+)$`)
var wolaiMarkdownBlockquotePattern = regexp.MustCompile(`^\s*>\s?(.*)$`)
var wolaiMarkdownFencePattern = regexp.MustCompile("^```\\s*([^`]*)$")

// TODO: Wire note image upload through these methods once Wolai image export is implemented.
type wolaiClient interface {
	GetBlock(id string) (map[string]any, error)
	CreateBlocks(parentID string, blocks any) ([]wolaiapi.CreatedBlock, error)
	CreateUploadSession(input wolaiapi.UploadSessionRequest) (*wolaiapi.UploadSession, error)
	UploadFile(session wolaiapi.UploadSession, filename, contentType string, file io.Reader) error
	UpdateBlockFile(blockID, fileID string) error
}

func defaultWolaiClientFactory(settings model.WolaiSettings) (wolaiClient, error) {
	return wolaiapi.NewClient(wolaiapi.Config{
		Token:   settings.Token,
		BaseURL: settings.BaseURL,
		Timeout: 15 * time.Second,
	})
}

func (s *LibraryService) GetWolaiSettings() (*model.WolaiSettings, error) {
	raw, err := s.repo.GetAppSetting(wolaiSettingsKey)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取 Wolai 配置失败", err)
	}

	settings := model.WolaiSettings{
		BaseURL: wolaiapi.DefaultBaseURL,
	}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "解析 Wolai 配置失败", err)
		}
	}

	normalized := normalizeWolaiSettings(settings)
	return &normalized, nil
}

func (s *LibraryService) UpdateWolaiSettings(input model.WolaiSettings) (*model.WolaiSettings, error) {
	settings := normalizeWolaiSettings(input)

	payload, err := json.Marshal(settings)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化 Wolai 配置失败", err)
	}
	if err := s.repo.UpsertAppSetting(wolaiSettingsKey, string(payload)); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "保存 Wolai 配置失败", err)
	}

	return &settings, nil
}

func (s *LibraryService) TestWolaiSettings(input model.WolaiSettings) (model.WolaiTestResult, error) {
	settings := normalizeWolaiSettings(input)
	if err := validateWolaiSettings(settings); err != nil {
		return model.WolaiTestResult{}, err
	}

	client, err := s.newWolaiClient(settings)
	if err != nil {
		return model.WolaiTestResult{}, err
	}

	if _, err := client.GetBlock(settings.ParentBlockID); err != nil {
		return model.WolaiTestResult{}, apperr.Wrap(apperr.CodeUnavailable, "Wolai token 测试失败", err)
	}

	return model.WolaiTestResult{
		Success: true,
		Message: "Wolai token 可用，已验证目标块访问权限",
	}, nil
}

func (s *LibraryService) InsertWolaiTestPage(input model.WolaiSettings) (model.WolaiSaveNoteResponse, error) {
	settings := normalizeWolaiSettings(input)
	if err := validateWolaiSettings(settings); err != nil {
		return model.WolaiSaveNoteResponse{}, err
	}

	client, err := s.newWolaiClient(settings)
	if err != nil {
		return model.WolaiSaveNoteResponse{}, err
	}

	pageBlock, err := createWolaiNotePage(client, settings.ParentBlockID, "Test Page")
	if err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "创建 Wolai 测试页面失败", err)
	}
	pageID := pageBlock.ID
	pageURL := strings.TrimSpace(pageBlock.URL)
	if pageURL == "" {
		pageURL = lookupWolaiBlockURL(client, pageID)
	}

	if _, err := client.CreateBlocks(pageID, []map[string]any{{
		"type":           "text",
		"content":        "Test works",
		"text_alignment": "left",
	}}); err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "写入 Wolai 测试文本失败", err)
	}

	if _, err := client.CreateBlocks(pageID, []map[string]any{{
		"type":           "text",
		"content":        wolaiTestPageImageTODOText,
		"text_alignment": "left",
	}}); err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "写入 Wolai 测试页面图片 TODO 失败", err)
	}

	return model.WolaiSaveNoteResponse{
		Success:        true,
		Message:        "Wolai 测试页面已创建，并写入测试文本与图片导出 TODO",
		TargetBlockID:  pageID,
		TargetBlockURL: pageURL,
	}, nil
}

func (s *LibraryService) SavePaperNoteToWolai(paperID int64, notesText string) (model.WolaiSaveNoteResponse, error) {
	paper, err := s.repo.GetPaperDetail(paperID)
	if err != nil {
		return model.WolaiSaveNoteResponse{}, err
	}
	if paper == nil {
		return model.WolaiSaveNoteResponse{}, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	content := strings.TrimSpace(firstNonEmpty(notesText, paper.PaperNotesText))
	if content == "" {
		return model.WolaiSaveNoteResponse{}, apperr.New(apperr.CodeInvalidArgument, "文献笔记为空，无法保存到 Wolai")
	}

	settings, client, err := s.prepareWolaiClient()
	if err != nil {
		return model.WolaiSaveNoteResponse{}, err
	}

	pageBlock, err := createWolaiNotePage(client, settings.ParentBlockID, buildPaperNoteWolaiPageTitle(paper))
	if err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "创建 Wolai 文献笔记页面失败", err)
	}
	pageID := pageBlock.ID

	blocks, hasPendingImages := buildPaperNoteWolaiBlocks(paper, content)
	if _, err := client.CreateBlocks(pageID, blocks); err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "写入 Wolai 文献笔记内容失败", err)
	}
	pageURL := strings.TrimSpace(pageBlock.URL)
	if pageURL == "" {
		pageURL = lookupWolaiBlockURL(client, pageID)
	}

	message := "文献笔记已保存到 Wolai"
	if hasPendingImages {
		message = "文献笔记已保存到 Wolai，笔记内图片已标记 TODO，等待后续完成"
	}

	return model.WolaiSaveNoteResponse{
		Success:        true,
		Message:        message,
		TargetBlockID:  pageID,
		TargetBlockURL: pageURL,
	}, nil
}

func (s *LibraryService) SaveFigureNoteToWolai(figureID int64, notesText string) (model.WolaiSaveNoteResponse, error) {
	figure, err := s.repo.GetFigure(figureID)
	if err != nil {
		return model.WolaiSaveNoteResponse{}, err
	}
	if figure == nil {
		return model.WolaiSaveNoteResponse{}, apperr.New(apperr.CodeNotFound, "figure not found")
	}

	content := strings.TrimSpace(firstNonEmpty(notesText, figure.NotesText))
	if content == "" {
		return model.WolaiSaveNoteResponse{}, apperr.New(apperr.CodeInvalidArgument, "图片笔记为空，无法保存到 Wolai")
	}

	settings, client, err := s.prepareWolaiClient()
	if err != nil {
		return model.WolaiSaveNoteResponse{}, err
	}

	pageBlock, err := createWolaiNotePage(client, settings.ParentBlockID, buildFigureNoteWolaiPageTitle(figure))
	if err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "创建 Wolai 图片笔记页面失败", err)
	}
	pageID := pageBlock.ID

	blocks, hasPendingImages := buildFigureNoteWolaiBlocks(figure, content)
	if _, err := client.CreateBlocks(pageID, blocks); err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "写入 Wolai 图片笔记内容失败", err)
	}
	pageURL := strings.TrimSpace(pageBlock.URL)
	if pageURL == "" {
		pageURL = lookupWolaiBlockURL(client, pageID)
	}

	message := "图片笔记已保存到 Wolai"
	if hasPendingImages {
		message = "图片笔记已保存到 Wolai，笔记内图片已标记 TODO，等待后续完成"
	}

	return model.WolaiSaveNoteResponse{
		Success:        true,
		Message:        message,
		TargetBlockID:  pageID,
		TargetBlockURL: pageURL,
	}, nil
}

func (s *LibraryService) prepareWolaiClient() (model.WolaiSettings, wolaiClient, error) {
	settings, err := s.GetWolaiSettings()
	if err != nil {
		return model.WolaiSettings{}, nil, err
	}
	if err := validateWolaiSettings(*settings); err != nil {
		return model.WolaiSettings{}, nil, err
	}

	client, err := s.newWolaiClient(*settings)
	if err != nil {
		return model.WolaiSettings{}, nil, err
	}
	return *settings, client, nil
}

func (s *LibraryService) newWolaiClient(settings model.WolaiSettings) (wolaiClient, error) {
	if s.wolaiClientFactory == nil {
		return nil, apperr.New(apperr.CodeInternal, "Wolai client factory 未配置")
	}

	client, err := s.wolaiClientFactory(settings)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInvalidArgument, "创建 Wolai 客户端失败", err)
	}
	return client, nil
}

func normalizeWolaiSettings(input model.WolaiSettings) model.WolaiSettings {
	settings := model.WolaiSettings{
		Token:         strings.TrimSpace(input.Token),
		ParentBlockID: strings.TrimSpace(input.ParentBlockID),
		BaseURL:       strings.TrimRight(strings.TrimSpace(input.BaseURL), "/"),
	}
	if settings.BaseURL == "" {
		settings.BaseURL = wolaiapi.DefaultBaseURL
	}
	return settings
}

func validateWolaiSettings(settings model.WolaiSettings) error {
	if strings.TrimSpace(settings.Token) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "请先填写 Wolai token")
	}
	if strings.TrimSpace(settings.ParentBlockID) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "请先填写 Wolai 目标块 ID")
	}
	return nil
}

func createWolaiNotePage(client wolaiClient, parentID, title string) (wolaiapi.CreatedBlock, error) {
	created, err := client.CreateBlocks(parentID, []map[string]any{{
		"type":    "page",
		"content": title,
	}})
	if err != nil {
		return wolaiapi.CreatedBlock{}, err
	}
	if len(created) == 0 || strings.TrimSpace(created[0].ID) == "" {
		return wolaiapi.CreatedBlock{}, fmt.Errorf("wolai create blocks response missing page id")
	}
	created[0].ID = strings.TrimSpace(created[0].ID)
	created[0].URL = strings.TrimSpace(created[0].URL)
	return created[0], nil
}

func buildPaperNoteWolaiPageTitle(paper *model.Paper) string {
	return fmt.Sprintf("文献笔记｜%s", buildWolaiNoteSubject(paper.Title, paper.OriginalFilename, "未命名文献"))
}

func buildPaperNoteWolaiBlocks(paper *model.Paper, notesText string) ([]map[string]any, bool) {
	notesText, hasPendingImages := rewriteWolaiMarkdownImages(notesText)
	blocks := []map[string]any{
		buildWolaiHeadingBlock("导出信息", 2),
	}
	blocks = append(blocks, buildWolaiTextBlocks([]string{
		strings.Join([]string{
			"导出时间：" + time.Now().Format("2006-01-02 15:04:05"),
			"原始文件：" + firstNonEmpty(strings.TrimSpace(paper.OriginalFilename), "未记录"),
			"当前分组：" + firstNonEmpty(strings.TrimSpace(paper.GroupName), "未分组"),
			"文献标签：" + firstNonEmpty(joinTagNames(paper.Tags), "无标签"),
		}, "\n"),
	})...)

	if abstract := strings.TrimSpace(paper.AbstractText); abstract != "" {
		blocks = append(blocks, buildWolaiHeadingBlock("摘要", 2))
		blocks = append(blocks, buildWolaiMarkdownBlocks(abstract)...)
	}

	blocks = append(blocks, buildWolaiHeadingBlock("文献笔记", 2))
	blocks = append(blocks, buildWolaiMarkdownBlocks(strings.TrimSpace(notesText))...)

	return blocks, hasPendingImages
}

func buildFigureNoteWolaiPageTitle(figure *model.FigureListItem) string {
	return fmt.Sprintf("图片笔记｜%s", buildWolaiNoteSubject(figure.PaperTitle, figure.Filename, "未命名图片"))
}

func buildFigureNoteWolaiBlocks(figure *model.FigureListItem, notesText string) ([]map[string]any, bool) {
	notesText, hasPendingImages := rewriteWolaiMarkdownImages(notesText)
	blocks := []map[string]any{
		buildWolaiHeadingBlock("导出信息", 2),
	}
	blocks = append(blocks, buildWolaiTextBlocks([]string{
		strings.Join([]string{
			"导出时间：" + time.Now().Format("2006-01-02 15:04:05"),
			"来源文献：" + firstNonEmpty(strings.TrimSpace(figure.PaperTitle), "未记录"),
			"图片定位：" + buildFigureLocation(figure),
			"来源分组：" + firstNonEmpty(strings.TrimSpace(figure.GroupName), "未分组"),
			"图片标签：" + firstNonEmpty(joinTagNames(figure.Tags), "无标签"),
		}, "\n"),
	})...)

	if caption := strings.TrimSpace(figure.Caption); caption != "" {
		blocks = append(blocks, buildWolaiHeadingBlock("图片说明", 2))
		blocks = append(blocks, buildWolaiMarkdownBlocks(caption)...)
	}

	blocks = append(blocks, buildWolaiHeadingBlock("图片笔记", 2))
	blocks = append(blocks, buildWolaiMarkdownBlocks(strings.TrimSpace(notesText))...)

	return blocks, hasPendingImages
}

func rewriteWolaiMarkdownImages(notesText string) (string, bool) {
	normalized := strings.TrimSpace(notesText)
	if normalized == "" {
		return "", false
	}

	replaced := false
	rewritten := wolaiMarkdownImagePattern.ReplaceAllStringFunc(normalized, func(match string) string {
		parts := wolaiMarkdownImagePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}

		replaced = true
		label := normalizeWolaiInlineTitle(parts[1])
		if label == "" {
			label = "未命名图片"
		}
		return fmt.Sprintf("【TODO：图片“%s”%s】", label, wolaiNoteImageTODOText)
	})

	return strings.TrimSpace(rewritten), replaced
}

func buildWolaiTextBlocks(sections []string) []map[string]any {
	blocks := make([]map[string]any, 0, len(sections))
	for _, section := range sections {
		for _, chunk := range splitWolaiText(section, wolaiTextBlockMaxSize) {
			blocks = append(blocks, map[string]any{
				"type":           "text",
				"content":        chunk,
				"text_alignment": "left",
			})
		}
	}
	return blocks
}

func buildWolaiHeadingBlock(content string, level int) map[string]any {
	level = maxWolaiHeadingLevel(level)
	return map[string]any{
		"type":    "heading",
		"content": strings.TrimSpace(content),
		"level":   level,
	}
}

func buildWolaiMarkdownBlocks(content string) []map[string]any {
	normalized := strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	if normalized == "" {
		return nil
	}

	lines := strings.Split(normalized, "\n")
	blocks := make([]map[string]any, 0, len(lines))

	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			i++
			continue
		}

		if match := wolaiMarkdownFencePattern.FindStringSubmatch(trimmed); len(match) == 2 {
			language := strings.TrimSpace(match[1])
			i++
			codeLines := make([]string, 0, 8)
			for i < len(lines) && !wolaiMarkdownFencePattern.MatchString(strings.TrimSpace(lines[i])) {
				codeLines = append(codeLines, strings.TrimRight(lines[i], "\r"))
				i++
			}
			if i < len(lines) {
				i++
			}
			blocks = append(blocks, buildWolaiCodeBlocks(strings.Join(codeLines, "\n"), language)...)
			continue
		}

		if isWolaiMathFence(trimmed) {
			i++
			equationLines := make([]string, 0, 4)
			for i < len(lines) && !isWolaiMathFence(strings.TrimSpace(lines[i])) {
				equationLines = append(equationLines, strings.TrimSpace(lines[i]))
				i++
			}
			if i < len(lines) {
				i++
			}
			equation := strings.TrimSpace(strings.Join(equationLines, "\n"))
			if equation != "" {
				blocks = append(blocks, map[string]any{
					"type":    "block_equation",
					"content": equation,
				})
			}
			continue
		}

		if isWolaiMarkdownDivider(trimmed) {
			blocks = append(blocks, map[string]any{"type": "divider"})
			i++
			continue
		}

		if match := wolaiMarkdownHeadingPattern.FindStringSubmatch(trimmed); len(match) == 3 {
			blocks = append(blocks, buildWolaiHeadingBlock(match[2], len(match[1])))
			i++
			continue
		}

		if wolaiMarkdownBlockquotePattern.MatchString(trimmed) {
			quoteLines := make([]string, 0, 4)
			for i < len(lines) {
				current := strings.TrimSpace(lines[i])
				match := wolaiMarkdownBlockquotePattern.FindStringSubmatch(current)
				if len(match) != 2 {
					break
				}
				quoteLines = append(quoteLines, strings.TrimSpace(match[1]))
				i++
			}
			blocks = append(blocks, buildWolaiQuoteBlocks(strings.Join(quoteLines, "\n"))...)
			continue
		}

		if wolaiMarkdownOrderedListPattern.MatchString(trimmed) {
			for i < len(lines) {
				match := wolaiMarkdownOrderedListPattern.FindStringSubmatch(strings.TrimSpace(lines[i]))
				if len(match) != 2 {
					break
				}
				blocks = append(blocks, map[string]any{
					"type":    "enum_list",
					"content": strings.TrimSpace(match[1]),
				})
				i++
			}
			continue
		}

		if wolaiMarkdownBulletListPattern.MatchString(trimmed) {
			for i < len(lines) {
				match := wolaiMarkdownBulletListPattern.FindStringSubmatch(strings.TrimSpace(lines[i]))
				if len(match) != 2 {
					break
				}
				blocks = append(blocks, map[string]any{
					"type":    "bull_list",
					"content": strings.TrimSpace(match[1]),
				})
				i++
			}
			continue
		}

		paragraphLines := make([]string, 0, 4)
		for i < len(lines) {
			current := strings.TrimSpace(lines[i])
			if current == "" {
				break
			}
			if isWolaiMarkdownSpecialLine(current) {
				break
			}
			paragraphLines = append(paragraphLines, strings.TrimRight(lines[i], "\r"))
			i++
		}
		blocks = append(blocks, buildWolaiTextBlocks([]string{strings.Join(paragraphLines, "\n")})...)
	}

	if len(blocks) == 0 {
		return buildWolaiTextBlocks([]string{normalized})
	}
	return blocks
}

func buildWolaiQuoteBlocks(content string) []map[string]any {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	chunks := splitWolaiText(content, wolaiTextBlockMaxSize)
	blocks := make([]map[string]any, 0, len(chunks))
	for _, chunk := range chunks {
		blocks = append(blocks, map[string]any{
			"type":    "quote",
			"content": chunk,
		})
	}
	return blocks
}

func buildWolaiCodeBlocks(content, language string) []map[string]any {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	language = strings.TrimSpace(language)
	if language == "" {
		language = "plain text"
	}

	chunks := splitWolaiCodeContent(content, wolaiTextBlockMaxSize)
	blocks := make([]map[string]any, 0, len(chunks))
	for _, chunk := range chunks {
		blocks = append(blocks, map[string]any{
			"type":     "code",
			"content":  chunk,
			"language": language,
		})
	}
	return blocks
}

func isWolaiMarkdownSpecialLine(line string) bool {
	return wolaiMarkdownFencePattern.MatchString(line) ||
		isWolaiMathFence(line) ||
		isWolaiMarkdownDivider(line) ||
		wolaiMarkdownHeadingPattern.MatchString(line) ||
		wolaiMarkdownBlockquotePattern.MatchString(line) ||
		wolaiMarkdownOrderedListPattern.MatchString(line) ||
		wolaiMarkdownBulletListPattern.MatchString(line)
}

func isWolaiMathFence(line string) bool {
	return strings.TrimSpace(line) == "$$"
}

func isWolaiMarkdownDivider(line string) bool {
	compact := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
	if len(compact) < 3 {
		return false
	}

	first := compact[0]
	if first != '-' && first != '*' && first != '_' {
		return false
	}

	for i := 1; i < len(compact); i++ {
		if compact[i] != first {
			return false
		}
	}
	return true
}

func maxWolaiHeadingLevel(level int) int {
	switch {
	case level <= 1:
		return 1
	case level >= 3:
		return 3
	default:
		return level
	}
}

func splitWolaiText(content string, maxRunes int) []string {
	normalized := strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	if normalized == "" {
		return nil
	}

	if maxRunes <= 0 || runeCount(normalized) <= maxRunes {
		return []string{normalized}
	}

	paragraphs := strings.Split(normalized, "\n\n")
	chunks := make([]string, 0, len(paragraphs))
	current := ""

	flush := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			chunks = append(chunks, value)
		}
	}

	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		if runeCount(paragraph) > maxRunes {
			if current != "" {
				flush(current)
				current = ""
			}
			for _, part := range splitWolaiLongParagraph(paragraph, maxRunes) {
				flush(part)
			}
			continue
		}

		candidate := paragraph
		if current != "" {
			candidate = current + "\n\n" + paragraph
		}
		if runeCount(candidate) > maxRunes {
			flush(current)
			current = paragraph
			continue
		}
		current = candidate
	}

	flush(current)
	if len(chunks) == 0 {
		return []string{normalized}
	}
	return chunks
}

func splitWolaiLongParagraph(paragraph string, maxRunes int) []string {
	runes := []rune(strings.TrimSpace(paragraph))
	if len(runes) == 0 || maxRunes <= 0 {
		return nil
	}

	parts := make([]string, 0, (len(runes)/maxRunes)+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		parts = append(parts, strings.TrimSpace(string(runes[start:end])))
	}
	return parts
}

func splitWolaiCodeContent(content string, maxRunes int) []string {
	normalized := strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	if normalized == "" {
		return nil
	}
	if maxRunes <= 0 || runeCount(normalized) <= maxRunes {
		return []string{normalized}
	}

	lines := strings.Split(normalized, "\n")
	parts := make([]string, 0, len(lines))
	current := ""

	flush := func() {
		if strings.TrimSpace(current) == "" {
			current = ""
			return
		}
		parts = append(parts, strings.TrimRight(current, "\n"))
		current = ""
	}

	for _, line := range lines {
		candidate := line
		if current != "" {
			candidate = current + "\n" + line
		}
		if runeCount(candidate) > maxRunes && current != "" {
			flush()
			candidate = line
		}
		if runeCount(candidate) > maxRunes {
			for _, chunk := range splitWolaiLongParagraph(line, maxRunes) {
				if chunk != "" {
					parts = append(parts, chunk)
				}
			}
			continue
		}
		current = candidate
	}

	flush()
	if len(parts) == 0 {
		return []string{normalized}
	}
	return parts
}

func runeCount(value string) int {
	return len([]rune(value))
}

func joinTagNames(tags []model.Tag) string {
	if len(tags) == 0 {
		return ""
	}

	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

func buildFigureLocation(figure *model.FigureListItem) string {
	label := strings.TrimSpace(firstNonEmpty(figure.DisplayLabel, fmt.Sprintf("Fig %d", figure.FigureIndex)))
	page := "-"
	if figure.PageNumber > 0 {
		page = fmt.Sprintf("第 %d 页", figure.PageNumber)
	}
	if parent := strings.TrimSpace(figure.ParentDisplayLabel); parent != "" {
		return fmt.Sprintf("%s · %s · 来自 %s", page, label, parent)
	}
	return fmt.Sprintf("%s · %s", page, label)
}

func buildWolaiNoteSubject(primary, fallback, defaultValue string) string {
	if normalized := normalizeWolaiInlineTitle(primary); normalized != "" {
		return normalized
	}

	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return defaultValue
	}

	if ext := filepath.Ext(fallback); ext != "" {
		fallback = strings.TrimSuffix(fallback, ext)
	}
	fallback = strings.NewReplacer("-", " ", "_", " ").Replace(fallback)
	if normalized := normalizeWolaiInlineTitle(fallback); normalized != "" {
		return normalized
	}
	return defaultValue
}

func normalizeWolaiInlineTitle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func lookupWolaiBlockURL(client wolaiClient, blockID string) string {
	block, err := client.GetBlock(blockID)
	if err != nil {
		return ""
	}
	return extractWolaiBlockURL(block)
}

func extractWolaiBlockURL(block map[string]any) string {
	return findWolaiBlockURL(block)
}

func findWolaiBlockURL(node any) string {
	switch value := node.(type) {
	case map[string]any:
		for _, key := range []string{"url", "link", "href", "page_url", "pageUrl"} {
			if link := normalizeWolaiLink(stringValue(value[key])); link != "" {
				return link
			}
		}
		for _, child := range value {
			if link := findWolaiBlockURL(child); link != "" {
				return link
			}
		}
	case []any:
		for _, child := range value {
			if link := findWolaiBlockURL(child); link != "" {
				return link
			}
		}
	}
	return ""
}

func normalizeWolaiLink(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return value
}
