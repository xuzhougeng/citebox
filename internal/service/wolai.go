package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	wolaiapi "github.com/xuzhougeng/citebox/internal/wolai"
)

const (
	wolaiSettingsKey      = "wolai_settings"
	wolaiTextBlockMaxSize = 1600
)

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

	pageID, err := createWolaiNotePage(client, settings.ParentBlockID, "Test Page")
	if err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "创建 Wolai 测试页面失败", err)
	}

	pageBlock, err := client.GetBlock(pageID)
	if err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "读取 Wolai 测试页面信息失败", err)
	}

	spaceID := extractWolaiSpaceID(pageBlock)
	if spaceID == "" {
		return model.WolaiSaveNoteResponse{}, apperr.New(apperr.CodeUnavailable, "无法从 Wolai 页面响应中解析 space ID")
	}
	pageURL := extractWolaiBlockURL(pageBlock)

	if _, err := client.CreateBlocks(pageID, []map[string]any{{
		"type":           "text",
		"content":        "Test works",
		"text_alignment": "left",
	}}); err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "写入 Wolai 测试文本失败", err)
	}

	imageBlocks, err := client.CreateBlocks(pageID, []map[string]any{{
		"type": "image",
	}})
	if err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "创建 Wolai 测试图片块失败", err)
	}
	if len(imageBlocks) == 0 || strings.TrimSpace(imageBlocks[0].ID) == "" {
		return model.WolaiSaveNoteResponse{}, apperr.New(apperr.CodeUnavailable, "Wolai 测试图片块创建成功但未返回块 ID")
	}

	imageBytes, err := buildWolaiTestImage()
	if err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeInternal, "生成 Wolai 测试图片失败", err)
	}

	session, err := client.CreateUploadSession(wolaiapi.UploadSessionRequest{
		SpaceID:  spaceID,
		FileSize: int64(len(imageBytes)),
		BlockID:  strings.TrimSpace(imageBlocks[0].ID),
		Type:     "image",
		FileName: "wolai-test.png",
		OSSPath:  "static",
	})
	if err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "创建 Wolai 测试图片上传会话失败", err)
	}

	if err := client.UploadFile(*session, "wolai-test.png", "image/png", bytes.NewReader(imageBytes)); err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "上传 Wolai 测试图片失败", err)
	}
	if err := client.UpdateBlockFile(strings.TrimSpace(imageBlocks[0].ID), strings.TrimSpace(session.FileID)); err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "回填 Wolai 测试图片失败", err)
	}

	return model.WolaiSaveNoteResponse{
		Success:        true,
		Message:        "Wolai 测试页面已创建，并写入测试文本与纯色图片",
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

	pageID, err := createWolaiNotePage(client, settings.ParentBlockID, buildPaperNoteWolaiPageTitle(paper))
	if err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "创建 Wolai 文献笔记页面失败", err)
	}

	if _, err := client.CreateBlocks(pageID, buildPaperNoteWolaiBlocks(paper, content)); err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "写入 Wolai 文献笔记内容失败", err)
	}
	pageURL := lookupWolaiBlockURL(client, pageID)

	return model.WolaiSaveNoteResponse{
		Success:        true,
		Message:        "文献笔记已保存到 Wolai",
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

	pageID, err := createWolaiNotePage(client, settings.ParentBlockID, buildFigureNoteWolaiPageTitle(figure))
	if err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "创建 Wolai 图片笔记页面失败", err)
	}

	if _, err := client.CreateBlocks(pageID, buildFigureNoteWolaiBlocks(figure, content)); err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "写入 Wolai 图片笔记内容失败", err)
	}
	pageURL := lookupWolaiBlockURL(client, pageID)

	return model.WolaiSaveNoteResponse{
		Success:        true,
		Message:        "图片笔记已保存到 Wolai",
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

func createWolaiNotePage(client wolaiClient, parentID, title string) (string, error) {
	created, err := client.CreateBlocks(parentID, []map[string]any{{
		"type":    "page",
		"content": title,
	}})
	if err != nil {
		return "", err
	}
	if len(created) == 0 || strings.TrimSpace(created[0].ID) == "" {
		return "", fmt.Errorf("wolai create blocks response missing page id")
	}
	return strings.TrimSpace(created[0].ID), nil
}

func buildWolaiTestImage() ([]byte, error) {
	const size = 96

	img := image.NewRGBA(image.Rect(0, 0, size, size))
	fill := color.RGBA{R: 34, G: 139, B: 230, A: 255}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, fill)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildPaperNoteWolaiPageTitle(paper *model.Paper) string {
	return fmt.Sprintf("文献笔记｜%s", buildWolaiNoteSubject(paper.Title, paper.OriginalFilename, "未命名文献"))
}

func buildPaperNoteWolaiBlocks(paper *model.Paper, notesText string) []map[string]any {
	sections := []string{
		strings.Join([]string{
			"导出时间：" + time.Now().Format("2006-01-02 15:04:05"),
			"原始文件：" + firstNonEmpty(strings.TrimSpace(paper.OriginalFilename), "未记录"),
			"当前分组：" + firstNonEmpty(strings.TrimSpace(paper.GroupName), "未分组"),
			"文献标签：" + firstNonEmpty(joinTagNames(paper.Tags), "无标签"),
		}, "\n"),
	}

	if abstract := strings.TrimSpace(paper.AbstractText); abstract != "" {
		sections = append(sections, "摘要：\n"+abstract)
	}
	sections = append(sections, "文献笔记：\n"+strings.TrimSpace(notesText))

	return buildWolaiTextBlocks(sections)
}

func buildFigureNoteWolaiPageTitle(figure *model.FigureListItem) string {
	return fmt.Sprintf("图片笔记｜%s", buildWolaiNoteSubject(figure.PaperTitle, figure.Filename, "未命名图片"))
}

func buildFigureNoteWolaiBlocks(figure *model.FigureListItem, notesText string) []map[string]any {
	sections := []string{
		strings.Join([]string{
			"导出时间：" + time.Now().Format("2006-01-02 15:04:05"),
			"来源文献：" + firstNonEmpty(strings.TrimSpace(figure.PaperTitle), "未记录"),
			"图片定位：" + buildFigureLocation(figure),
			"来源分组：" + firstNonEmpty(strings.TrimSpace(figure.GroupName), "未分组"),
			"图片标签：" + firstNonEmpty(joinTagNames(figure.Tags), "无标签"),
		}, "\n"),
	}

	if caption := strings.TrimSpace(figure.Caption); caption != "" {
		sections = append(sections, "图片说明：\n"+caption)
	}
	sections = append(sections, "图片笔记：\n"+strings.TrimSpace(notesText))

	return buildWolaiTextBlocks(sections)
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

func extractWolaiSpaceID(block map[string]any) string {
	return findWolaiSpaceID(block)
}

func findWolaiSpaceID(node any) string {
	switch value := node.(type) {
	case map[string]any:
		if spaceID := strings.TrimSpace(stringValue(value["space_id"])); spaceID != "" {
			return spaceID
		}
		if spaceID := strings.TrimSpace(stringValue(value["spaceId"])); spaceID != "" {
			return spaceID
		}
		if nested, ok := value["space"].(map[string]any); ok {
			if spaceID := strings.TrimSpace(stringValue(nested["id"])); spaceID != "" {
				return spaceID
			}
		}
		for _, child := range value {
			if spaceID := findWolaiSpaceID(child); spaceID != "" {
				return spaceID
			}
		}
	case []any:
		for _, child := range value {
			if spaceID := findWolaiSpaceID(child); spaceID != "" {
				return spaceID
			}
		}
	}
	return ""
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
