package service

import (
	"encoding/json"
	"fmt"
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
	CreateBlocks(parentID string, blocks any) error
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

	if err := client.CreateBlocks(settings.ParentBlockID, buildPaperNoteWolaiBlocks(paper, content)); err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "保存文献笔记到 Wolai 失败", err)
	}

	return model.WolaiSaveNoteResponse{
		Success:       true,
		Message:       "文献笔记已保存到 Wolai",
		TargetBlockID: settings.ParentBlockID,
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

	if err := client.CreateBlocks(settings.ParentBlockID, buildFigureNoteWolaiBlocks(figure, content)); err != nil {
		return model.WolaiSaveNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "保存图片笔记到 Wolai 失败", err)
	}

	return model.WolaiSaveNoteResponse{
		Success:       true,
		Message:       "图片笔记已保存到 Wolai",
		TargetBlockID: settings.ParentBlockID,
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

func buildPaperNoteWolaiBlocks(paper *model.Paper, notesText string) []map[string]any {
	sections := []string{
		fmt.Sprintf("文献笔记｜%s", strings.TrimSpace(firstNonEmpty(paper.Title, paper.OriginalFilename, "未命名文献"))),
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

func buildFigureNoteWolaiBlocks(figure *model.FigureListItem, notesText string) []map[string]any {
	sections := []string{
		fmt.Sprintf("图片笔记｜%s", strings.TrimSpace(firstNonEmpty(figure.PaperTitle, figure.Filename, "未命名图片"))),
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
