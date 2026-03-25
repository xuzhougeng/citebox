package service

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func buildAIPrompts(
	settings model.AISettings,
	paper *model.Paper,
	groups []model.Group,
	tags []model.Tag,
	action model.AIAction,
	displayQuestion string,
	promptQuestion string,
	history []model.AIConversationTurn,
	figureSummaries []string,
	includedFigures int,
	activeRolePrompts []model.AIRolePrompt,
	structuredOutput bool,
) (string, string) {
	groupName := "未分组"
	if strings.TrimSpace(paper.GroupName) != "" {
		groupName = paper.GroupName
	}

	tagNames := make([]string, 0, len(paper.Tags))
	for _, tag := range paper.Tags {
		tagNames = append(tagNames, tag.Name)
	}

	fullText := strings.TrimSpace(paper.PDFText)
	if fullText == "" {
		fullText = "未提取到正文内容。"
	}

	abstractText := strings.TrimSpace(paper.AbstractText)
	if abstractText == "" {
		abstractText = "无"
	}
	notesText := buildPaperNotesContext(paper)

	figureSection := "未提取到图片。"
	if len(figureSummaries) > 0 {
		figureSection = strings.Join(figureSummaries, "\n")
	}

	existingTagNames := "当前库中还没有标签。"
	if len(tags) > 0 {
		names := make([]string, 0, len(tags))
		for _, tag := range tags {
			names = append(names, tag.Name)
		}
		existingTagNames = strings.Join(names, "，")
	}

	existingGroupNames := "当前库中还没有分组。"
	if len(groups) > 0 {
		names := make([]string, 0, len(groups))
		for _, group := range groups {
			if description := strings.TrimSpace(group.Description); description != "" {
				names = append(names, fmt.Sprintf("%s（%s）", group.Name, description))
			} else {
				names = append(names, group.Name)
			}
		}
		existingGroupNames = strings.Join(names, "；")
	}

	conversationSection := ""
	if action == model.AIActionPaperQA {
		conversationSection = buildConversationSection(history)
	}

	rolePromptNames := buildAIRolePromptNames(activeRolePrompts)
	outputRequirements := strings.TrimSpace(aiOutputRequirements(action, structuredOutput))
	scopeDescription := actionScopeDescription(action)
	if strings.TrimSpace(displayQuestion) == "" {
		displayQuestion = promptQuestion
	}

	userPrompt := fmt.Sprintf(`任务类型: %s

场景范围:
%s

论文信息:
- 标题: %s
- 原始文件名: %s
- 当前分组: %s
- 当前标签: %s
- 摘要: %s
- 备注: %s
- 本次附带图片数: %d

图片列表:
%s

现有标签库:
%s

现有分组库:
%s

用户问题:
%s

原始输入:
%s

角色调用:
%s

历史对话:
%s

场景指令:
%s

全文:
%s

输出要求:
%s`,
		action,
		scopeDescription,
		paper.Title,
		paper.OriginalFilename,
		groupName,
		joinOrFallback(tagNames, "无"),
		abstractText,
		notesText,
		includedFigures,
		figureSection,
		existingTagNames,
		existingGroupNames,
		promptQuestion,
		displayQuestion,
		rolePromptNames,
		conversationSection,
		actionPromptFor(settings, action),
		fullText,
		outputRequirements,
	)

	systemPrompt := settings.SystemPrompt
	roleSystemPrompt := buildAIRolePromptSystemSection(activeRolePrompts)
	if roleSystemPrompt != "" {
		systemPrompt = strings.TrimSpace(systemPrompt + "\n\n" + roleSystemPrompt)
	}

	return systemPrompt, userPrompt
}

func buildPaperNotesContext(paper *model.Paper) string {
	managementNotes := strings.TrimSpace(paper.NotesText)
	paperNotes := strings.TrimSpace(paper.PaperNotesText)

	switch {
	case managementNotes == "" && paperNotes == "":
		return "无"
	case managementNotes == "":
		return "文献笔记:\n" + paperNotes
	case paperNotes == "":
		return "管理笔记:\n" + managementNotes
	default:
		return "管理笔记:\n" + managementNotes + "\n\n文献笔记:\n" + paperNotes
	}
}

func resolveAIRolePrompts(question string, available []model.AIRolePrompt) (string, []model.AIRolePrompt) {
	if strings.TrimSpace(question) == "" || len(available) == 0 {
		return strings.TrimSpace(question), nil
	}

	type candidate struct {
		rolePrompt model.AIRolePrompt
		token      string
	}

	candidates := make([]candidate, 0, len(available))
	for _, item := range available {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		candidates = append(candidates, candidate{
			rolePrompt: item,
			token:      "@" + name,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return len([]rune(candidates[i].rolePrompt.Name)) > len([]rune(candidates[j].rolePrompt.Name))
	})

	cleanedQuestion := question
	activeRolePrompts := make([]model.AIRolePrompt, 0, len(candidates))
	seenNames := make(map[string]struct{}, len(candidates))
	for _, item := range candidates {
		if !strings.Contains(cleanedQuestion, item.token) {
			continue
		}
		cleanedQuestion = strings.ReplaceAll(cleanedQuestion, item.token, " ")
		lookupKey := strings.ToLower(strings.TrimSpace(item.rolePrompt.Name))
		if _, exists := seenNames[lookupKey]; exists {
			continue
		}
		seenNames[lookupKey] = struct{}{}
		activeRolePrompts = append(activeRolePrompts, item.rolePrompt)
	}

	return strings.Join(strings.Fields(cleanedQuestion), " "), activeRolePrompts
}

func buildAIRolePromptNames(rolePrompts []model.AIRolePrompt) string {
	if len(rolePrompts) == 0 {
		return "未调用角色 Prompt。"
	}

	names := make([]string, 0, len(rolePrompts))
	for _, item := range rolePrompts {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		names = append(names, "@"+name)
	}
	if len(names) == 0 {
		return "未调用角色 Prompt。"
	}
	return strings.Join(names, "，")
}

func buildAIRolePromptSystemSection(rolePrompts []model.AIRolePrompt) string {
	if len(rolePrompts) == 0 {
		return ""
	}

	sections := make([]string, 0, len(rolePrompts))
	for _, item := range rolePrompts {
		name := strings.TrimSpace(item.Name)
		prompt := strings.TrimSpace(item.Prompt)
		if name == "" || prompt == "" {
			continue
		}
		sections = append(sections, fmt.Sprintf("角色：%s\n%s", name, prompt))
	}
	if len(sections) == 0 {
		return ""
	}

	return "以下是当前用户通过 @ 调用的角色 Prompt，请在本次回答中一并遵守：\n\n" + strings.Join(sections, "\n\n")
}

func buildAITranslatePrompts(settings model.AISettings, sourceLanguage, targetLanguage, text string) (string, string) {
	userPrompt := fmt.Sprintf(
		`任务类型: translate

翻译方向:
- 原文语言: %s
- 目标语言: %s

场景指令:
%s

输出要求:
1. 只返回译文正文，不要附加解释、注释、标题、前缀或代码块。
2. 保留原文中的换行、列表层级、数字、单位、缩写和专有名词。
3. 如果原文已经是目标语言，也请只做必要润色后输出正文。

待翻译文本:
%s`,
		sourceLanguage,
		targetLanguage,
		settings.TranslatePrompt,
		text,
	)
	return settings.SystemPrompt, userPrompt
}

func resolveTranslationDirection(config model.AITranslationConfig, text string) (string, string) {
	primaryLanguage := strings.TrimSpace(config.PrimaryLanguage)
	targetLanguage := strings.TrimSpace(config.TargetLanguage)
	if primaryLanguage == "" {
		primaryLanguage = model.DefaultAISettings().Translation.PrimaryLanguage
	}
	if targetLanguage == "" {
		targetLanguage = model.DefaultAISettings().Translation.TargetLanguage
	}
	if translationTextMatchesPrimaryLanguage(primaryLanguage, text) {
		return primaryLanguage, targetLanguage
	}
	return "其他语言", primaryLanguage
}

func translationTextMatchesPrimaryLanguage(primaryLanguage, text string) bool {
	normalizedPrimary := normalizeTranslationLanguageKey(primaryLanguage)
	detectedLanguage := detectTranslationLanguageKey(text)
	if normalizedPrimary == "" || detectedLanguage == "" {
		return false
	}
	return normalizedPrimary == detectedLanguage
}

func normalizeTranslationLanguageKey(language string) string {
	normalized := strings.ToLower(strings.TrimSpace(language))
	switch normalized {
	case "zh", "zh-cn", "zh-hans", "zh-hant", "chinese", "mandarin", "中文", "汉语", "简体中文", "繁體中文", "繁体中文":
		return "han"
	case "ja", "jp", "japanese", "日语", "日文", "日本語", "日本语":
		return "japanese"
	case "ko", "korean", "韩语", "韓語", "한국어":
		return "hangul"
	case "en", "english", "英文", "英语":
		return "latin"
	case "fr", "french", "法语", "法文":
		return "latin"
	case "de", "german", "德语", "德文":
		return "latin"
	case "es", "spanish", "西班牙语", "西班牙文":
		return "latin"
	case "pt", "portuguese", "葡萄牙语", "葡萄牙文":
		return "latin"
	case "it", "italian", "意大利语", "意大利文":
		return "latin"
	case "ru", "russian", "俄语", "俄文":
		return "cyrillic"
	case "ar", "arabic", "阿拉伯语", "阿拉伯文":
		return "arabic"
	default:
		return ""
	}
}

func detectTranslationLanguageKey(text string) string {
	type scriptCounts struct {
		japanese int
		hangul   int
		han      int
		latin    int
		cyrillic int
		arabic   int
	}

	var counts scriptCounts
	for _, r := range text {
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			counts.japanese += 2
		case unicode.In(r, unicode.Hangul):
			counts.hangul += 2
		case unicode.In(r, unicode.Han):
			counts.han++
		case unicode.In(r, unicode.Cyrillic):
			counts.cyrillic++
		case unicode.In(r, unicode.Arabic):
			counts.arabic++
		case unicode.In(r, unicode.Latin):
			if unicode.IsLetter(r) {
				counts.latin++
			}
		}
	}

	switch {
	case counts.japanese > 0:
		return "japanese"
	case counts.hangul > 0:
		return "hangul"
	case counts.han > 0:
		return "han"
	case counts.cyrillic > 0:
		return "cyrillic"
	case counts.arabic > 0:
		return "arabic"
	case counts.latin > 0:
		return "latin"
	default:
		return ""
	}
}

func normalizeTranslationOutput(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
		if newline := strings.Index(trimmed, "\n"); newline >= 0 {
			trimmed = trimmed[newline+1:]
		}
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "```"))
	}
	return strings.TrimSpace(trimmed)
}

func aiOutputRequirements(action model.AIAction, structuredOutput bool) string {
	if structuredOutput {
		if action == model.AIActionPaperQA {
			return `1. 只返回 JSON 对象，不要使用 Markdown 代码块。
2. JSON 必须包含 answer、suggested_tags、suggested_group 三个字段。
3. 如果当前任务不需要标签建议或分组建议，请分别返回空数组 [] 和空字符串 ""。
4. answer 中请直接给出结论、依据和必要的限制说明。
5. answer 支持使用 Markdown；如果需要插入论文图片，只能使用系统提供的图片引用，格式为 ![图片说明](figure://<figure_id>)。
6. 不要伪造本地文件路径、文件名或外部图片 URL。`
		}
		if action == model.AIActionTagSuggestion {
			return `1. 只返回 JSON 对象，不要使用 Markdown 代码块。
2. JSON 必须包含 suggested_tags 字段（字符串数组）。
3. answer 字段只填一句话概括，不需要展开分析。
4. 不要长篇解释，只给出标签列表。`
		}
		return `1. 只返回 JSON 对象，不要使用 Markdown 代码块。
2. JSON 必须包含 answer、suggested_tags、suggested_group 三个字段。
3. 如果当前任务不需要标签建议或分组建议，请分别返回空数组 [] 和空字符串 ""。
4. answer 中请直接给出结论、依据和必要的限制说明。`
	}

	if action == model.AIActionFigureInterpretation {
		return `1. 直接输出自然语言正文，不要返回 JSON、代码块或额外元数据。
2. 优先围绕当前图片说明图像内容、支撑结论、与全文主线的关系以及局限。
3. 尽量分成短段落，保证可以逐段流式阅读。`
	}

	return `1. 直接输出自然语言正文，不要返回 JSON、代码块或额外元数据。`
}

func actionPromptFor(settings model.AISettings, action model.AIAction) string {
	switch action {
	case model.AIActionFigureInterpretation:
		return settings.FigurePrompt
	case model.AIActionTagSuggestion:
		return settings.TagPrompt
	case model.AIActionGroupSuggestion:
		return settings.GroupPrompt
	case model.AIActionTranslate:
		return settings.TranslatePrompt
	default:
		return settings.QAPrompt
	}
}

func actionScopeDescription(action model.AIAction) string {
	switch action {
	case model.AIActionFigureInterpretation:
		return "只针对当前选中的这张图片进行解读；论文全文仅作为补充上下文。"
	case model.AIActionTagSuggestion:
		return "只针对当前选中的这张图片生成图片标签；不是整篇文献的文献标签。"
	case model.AIActionGroupSuggestion:
		return "针对当前整篇文献进行分组判断。"
	default:
		return "针对当前整篇文献回答用户问题。"
	}
}

func normalizeAIAction(action model.AIAction) model.AIAction {
	switch action {
	case model.AIActionFigureInterpretation, model.AIActionTagSuggestion, model.AIActionGroupSuggestion, model.AIActionPaperQA, model.AIActionTranslate:
		return action
	default:
		return model.AIActionPaperQA
	}
}

func tagScopeForAIAction(action model.AIAction) model.TagScope {
	switch action {
	case model.AIActionFigureInterpretation, model.AIActionTagSuggestion:
		return model.TagScopeFigure
	default:
		return model.TagScopePaper
	}
}

func defaultAIQuestion(action model.AIAction) string {
	switch action {
	case model.AIActionFigureInterpretation:
		return "当前任务只针对当前选中的这张图片。"
	case model.AIActionTagSuggestion:
		return "当前任务只针对当前选中的这张图片。"
	case model.AIActionGroupSuggestion:
		return "请判断这篇文献最适合放到哪个分组。"
	default:
		return "请概括这篇文献的核心问题、方法、主要结论和证据。"
	}
}

func normalizeConversationHistory(action model.AIAction, input []model.AIConversationTurn) ([]model.AIConversationTurn, error) {
	if action != model.AIActionPaperQA {
		return nil, nil
	}

	history := make([]model.AIConversationTurn, 0, len(input))
	for _, turn := range input {
		question := strings.TrimSpace(turn.Question)
		answer := strings.TrimSpace(turn.Answer)
		if question == "" || answer == "" {
			continue
		}
		history = append(history, model.AIConversationTurn{
			Question: question,
			Answer:   answer,
		})
	}
	if len(history) > 4 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "自由提问最多支持 5 轮对话")
	}
	return history, nil
}

func buildConversationSection(history []model.AIConversationTurn) string {
	if len(history) == 0 {
		return "这是当前会话的第一轮提问。"
	}

	lines := make([]string, 0, len(history)*2)
	for index, turn := range history {
		lines = append(lines, fmt.Sprintf("第 %d 轮用户: %s", index+1, turn.Question))
		lines = append(lines, fmt.Sprintf("第 %d 轮助手: %s", index+1, turn.Answer))
	}
	return strings.Join(lines, "\n")
}

func selectFiguresForAI(paper *model.Paper, action model.AIAction, figureID int64, maxFigures int) ([]model.Figure, error) {
	if (action == model.AIActionFigureInterpretation || action == model.AIActionTagSuggestion) && figureID > 0 {
		for _, figure := range paper.Figures {
			if figure.ID == figureID {
				return []model.Figure{figure}, nil
			}
		}
		return nil, apperr.New(apperr.CodeInvalidArgument, "指定图片不存在于当前文献")
	}

	figures := topLevelFigures(paper.Figures)
	if maxFigures > 0 && len(figures) > maxFigures {
		figures = figures[:maxFigures]
	}
	return figures, nil
}
