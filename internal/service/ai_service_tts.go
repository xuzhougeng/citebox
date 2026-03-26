package service

import (
	"context"
	"regexp"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

var (
	markdownImagePattern        = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]*)\)`)
	markdownLinkPattern         = regexp.MustCompile(`\[(.*?)\]\(([^)]*)\)`)
	markdownCodeFencePattern    = regexp.MustCompile("(?s)```(?:[a-zA-Z0-9_-]+)?\\s*(.*?)```")
	markdownInlineCodePattern   = regexp.MustCompile("`([^`]*)`")
	markdownHeadingPattern      = regexp.MustCompile(`(?m)^[\t ]*#{1,6}\s*`)
	markdownBulletListPattern   = regexp.MustCompile(`(?m)^[\t ]*[-*+]\s+`)
	markdownOrderedListPattern  = regexp.MustCompile(`(?m)^[\t ]*\d+\.\s+`)
	markdownFigureURLPattern    = regexp.MustCompile(`figure://[^\s)]+`)
	markdownPageRefParenPattern = regexp.MustCompile(`\((\s*[，,;；]?\s*)+\)`)
	markdownSpacePattern        = regexp.MustCompile(`[ \t]{2,}`)
	markdownBlankLinePattern    = regexp.MustCompile(`\n{3,}`)
)

var markdownTTSReplacer = strings.NewReplacer(
	"**", "",
	"__", "",
	"~~", "",
	"*", "",
	"_", "",
)

func (s *AIService) RewriteTextForTTS(ctx context.Context, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", apperr.New(apperr.CodeInvalidArgument, "缺少需要优化的 TTS 文本")
	}

	settings, err := s.GetSettings()
	if err != nil {
		return "", err
	}

	modelConfig, err := resolveModelForAction(*settings, model.AIActionTTSRewrite)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(modelConfig.APIKey) == "" {
		return "", apperr.New(apperr.CodeFailedPrecondition, "请先在 AI 页面为 TTS 优化场景配置可用模型和 API Key")
	}

	systemPrompt, userPrompt := buildAITTSPrompts(*settings, text)

	runtimeSettings := *settings
	runtimeSettings.Provider = modelConfig.Provider
	runtimeSettings.APIKey = modelConfig.APIKey
	runtimeSettings.BaseURL = modelConfig.BaseURL
	runtimeSettings.Model = modelConfig.Model
	runtimeSettings.MaxOutputTokens = modelConfig.MaxOutputTokens
	runtimeSettings.OpenAILegacyMode = modelConfig.OpenAILegacyMode
	runtimeSettings.Temperature = 0.1

	rawText, err := s.callTextProvider(ctx, runtimeSettings, systemPrompt, userPrompt, nil)
	if err != nil {
		return "", err
	}

	rewritten := normalizeTTSReadbackText(rawText)
	if rewritten == "" {
		return "", apperr.New(apperr.CodeUnavailable, "TTS 优化结果为空")
	}
	return rewritten, nil
}

func normalizeTTSReadbackText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	if matches := markdownCodeFencePattern.FindStringSubmatch(text); len(matches) == 2 {
		text = matches[1]
	}
	text = strings.Trim(text, " \t\r\n`\"'")
	text = strings.Trim(text, "“”")
	text = sanitizeMarkdownForTTS(text)
	return strings.TrimSpace(text)
}

func sanitizeMarkdownForTTS(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	text = markdownCodeFencePattern.ReplaceAllString(text, "$1")
	text = markdownImagePattern.ReplaceAllStringFunc(text, func(match string) string {
		submatches := markdownImagePattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return "相关配图"
		}
		alt := strings.TrimSpace(submatches[1])
		if alt != "" {
			return alt
		}
		return "相关配图"
	})
	text = markdownLinkPattern.ReplaceAllString(text, "$1")
	text = markdownInlineCodePattern.ReplaceAllString(text, "$1")
	text = markdownFigureURLPattern.ReplaceAllString(text, "")
	text = markdownTTSReplacer.Replace(text)
	text = markdownHeadingPattern.ReplaceAllString(text, "")
	text = markdownBulletListPattern.ReplaceAllString(text, "")
	text = markdownOrderedListPattern.ReplaceAllString(text, "")
	text = markdownPageRefParenPattern.ReplaceAllString(text, "")
	text = markdownSpacePattern.ReplaceAllString(text, " ")
	text = markdownBlankLinePattern.ReplaceAllString(text, "\n\n")

	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimSpace(line)
	}
	text = strings.Join(lines, "\n")
	text = markdownBlankLinePattern.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
