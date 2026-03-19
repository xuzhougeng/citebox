package ai

import (
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
)

// BuildPrompts 构建 AI 提示词
func BuildPrompts(
	settings model.AISettings,
	paper *model.Paper,
	groups []model.Group,
	tags []model.Tag,
	action model.AIAction,
	question string,
	history []model.AIConversationTurn,
	figureSummaries []string,
	includedFigures int,
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

	outputRequirements := strings.TrimSpace(outputRequirementsForAction(action, structuredOutput))
	scopeDescription := actionScopeDescription(action)

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
		question,
		conversationSection,
		actionPromptFor(settings, action),
		fullText,
		outputRequirements,
	)

	return settings.SystemPrompt, userPrompt
}

// buildPaperNotesContext 构建文献备注上下文
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

// outputRequirementsForAction 返回指定动作的输出要求
func outputRequirementsForAction(action model.AIAction, structuredOutput bool) string {
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

// actionPromptFor 返回指定动作的提示词
func actionPromptFor(settings model.AISettings, action model.AIAction) string {
	switch action {
	case model.AIActionFigureInterpretation:
		return settings.FigurePrompt
	case model.AIActionTagSuggestion:
		return settings.TagPrompt
	case model.AIActionGroupSuggestion:
		return settings.GroupPrompt
	default:
		return settings.QAPrompt
	}
}

// actionScopeDescription 返回动作范围描述
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

// DefaultQuestion 返回指定动作的默认问题
func DefaultQuestion(action model.AIAction) string {
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

// buildConversationSection 构建对话历史部分
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

// joinOrFallback 连接字符串数组或使用默认值
func joinOrFallback(items []string, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	return strings.Join(items, "，")
}
