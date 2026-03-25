package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func (s *AIService) PlanWeixinSearch(ctx context.Context, query, forcedTarget string) (*weixinSearchPlan, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "缺少检索内容")
	}

	runtimeSettings, err := s.resolveWeixinSearchRuntimeSettings(400)
	if err != nil {
		return nil, err
	}
	systemPrompt := buildWeixinSearchSystemPrompt(runtimeSettings.SystemPrompt)

	targetInstruction := "auto"
	switch normalizeWeixinSearchTarget(forcedTarget) {
	case weixinSearchTargetPaper:
		targetInstruction = "paper"
	case weixinSearchTargetFigure:
		targetInstruction = "figure"
	}

	userPrompt := fmt.Sprintf(`请把下面这条微信 IM 检索请求改写成 JSON：

输出要求：
1. 只返回 JSON 对象，不要加 Markdown 代码块。
2. JSON 必须包含 target、keywords_zh、keywords_en 三个字段。
3. target 只能是 "paper" 或 "figure"。
4. keywords_zh 和 keywords_en 都必须是字符串数组。两组加起来总共给出 4 到 6 个检索短语，优先接近 5 个。
5. 必须同时包含中文检索词和英文检索词；如果用户只用了一种语言，也要补出另一种语言对应的检索词。
6. keywords_zh 只放中文短语；keywords_en 只放英文短语。不要把中英混在同一个数组里。
7. 如果用户用中文描述英文图型或术语，请补出常用英文检索词，例如 "火山图" -> "volcano plot"；如果用户用英文描述，也要补出自然的中文检索词。
8. 如果用户是想找图片，target 设为 "figure"；如果更像想找论文主题、方法、综述或某类文献，target 设为 "paper"。
9. 不要塞整句，不要重复，不要输出无意义的超泛词。

限制：
- 当前强制目标：%s
- 用户原始输入：%s`,
		targetInstruction,
		query,
	)

	rawText, err := s.callTextProvider(ctx, runtimeSettings, systemPrompt, userPrompt, nil)
	if err != nil {
		return nil, err
	}

	plan, err := parseWeixinSearchPlan(rawText)
	if err != nil {
		return nil, err
	}

	if forced := normalizeWeixinSearchTarget(forcedTarget); forced != "" {
		plan.Target = forced
	}
	fallbackPlan := heuristicWeixinSearchPlan(query, forcedTarget)
	plan.KeywordsZH = mergeWeixinSearchKeywords(plan.KeywordsZH, fallbackPlan.KeywordsZH)
	plan.KeywordsEN = mergeWeixinSearchKeywords(plan.KeywordsEN, fallbackPlan.KeywordsEN)
	if len(plan.KeywordsZH) == 0 || len(plan.KeywordsEN) == 0 {
		return nil, apperr.New(apperr.CodeUnavailable, "IM 检索规划未同时返回有效的中英文关键词")
	}
	plan.Keywords = mergeWeixinSearchKeywords(plan.KeywordsZH, plan.KeywordsEN)

	return plan, nil
}

func (s *AIService) PlanWeixinCommand(ctx context.Context, text string, context weixinIntentContext) (*weixinCommandPlan, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "缺少 IM 文本")
	}

	runtimeSettings, err := s.resolveWeixinSearchRuntimeSettings(300)
	if err != nil {
		return nil, err
	}
	systemPrompt := buildWeixinSearchSystemPrompt(runtimeSettings.SystemPrompt)

	contextSummary := []string{
		fmt.Sprintf("- current_paper_id: %d", context.CurrentPaperID),
		fmt.Sprintf("- current_paper_title: %s", firstNonEmpty(context.CurrentPaperTitle, "无")),
		fmt.Sprintf("- current_figure_id: %d", context.CurrentFigureID),
		fmt.Sprintf("- search_paper_count: %d", context.SearchPaperCount),
		fmt.Sprintf("- search_figure_count: %d", context.SearchFigureCount),
	}

	userPrompt := fmt.Sprintf(`请把下面这条微信 IM 普通文本改写成最合适的 slash 命令 JSON：

输出要求：
1. 只返回 JSON 对象，不要加 Markdown 代码块。
2. JSON 必须包含 command 和 arg 两个字段。
3. command 只能从以下列表中选择：
   "/help", "/status", "/reset", "/recent", "/figures", "/search", "/search-papers", "/search-figures", "/paper", "/figure", "/ask", "/note", "/interpret"
4. arg 必须是字符串；如果命令不需要参数，就返回空字符串 ""。
5. 如果用户是在找文献或图片，用 "/search"、"/search-papers" 或 "/search-figures"。
6. 如果用户是在选择最近一次检索结果里的某篇文献，例如“第一篇文献”“第三篇论文”“第 2 个结果”，并且 search_paper_count > 0，就返回 "/paper"，arg 填对应阿拉伯数字序号，如 "3"。
7. 如果用户是在选择最近一次图片检索结果或当前文献图片列表里的某张图，例如“第二张图”“看看第三幅图”，并且 search_figure_count > 0 或 current_paper_id > 0，就返回 "/figure"，arg 填对应阿拉伯数字序号，如 "2"。
8. 如果当前已有文献上下文，而用户是在继续追问当前文献内容，用 "/ask"。
9. 如果当前已有图片上下文，而用户是在要求解释当前图片，用 "/interpret"。
10. 如果用户是在追加笔记，用 "/note"。
11. 识别不准时优先选择最保守、最合理的命令；完全无法判断时返回 "/help"。

当前上下文：
%s

用户原始文本：
%s`,
		strings.Join(contextSummary, "\n"),
		text,
	)

	rawText, err := s.callTextProvider(ctx, runtimeSettings, systemPrompt, userPrompt, nil)
	if err != nil {
		return nil, err
	}

	return parseWeixinCommandPlan(rawText)
}

func (s *AIService) ReviewWeixinPaperSearch(ctx context.Context, query string, keywords []string, candidates []model.Paper) (*weixinSearchReview, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "缺少检索内容")
	}
	if len(candidates) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "缺少文献候选")
	}

	runtimeSettings, err := s.resolveWeixinSearchRuntimeSettings(700)
	if err != nil {
		return nil, err
	}

	systemPrompt := buildWeixinSearchSystemPrompt(runtimeSettings.SystemPrompt)
	userPrompt := buildWeixinPaperSearchReviewPrompt(query, keywords, candidates)
	rawText, err := s.callTextProvider(ctx, runtimeSettings, systemPrompt, userPrompt, nil)
	if err != nil {
		return nil, err
	}

	return parseWeixinSearchReview(rawText, collectPaperCandidateIDs(candidates))
}

func (s *AIService) ReviewWeixinFigureSearch(ctx context.Context, query string, keywords []string, candidates []model.FigureListItem) (*weixinSearchReview, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "缺少检索内容")
	}
	if len(candidates) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "缺少图片候选")
	}

	runtimeSettings, err := s.resolveWeixinSearchRuntimeSettings(700)
	if err != nil {
		return nil, err
	}

	images, imageIDs := s.loadWeixinFigureReviewImages(candidates)
	systemPrompt := buildWeixinSearchSystemPrompt(runtimeSettings.SystemPrompt)
	userPrompt := buildWeixinFigureSearchReviewPrompt(query, keywords, candidates, imageIDs)
	rawText, err := s.callTextProvider(ctx, runtimeSettings, systemPrompt, userPrompt, images)
	if err != nil {
		return nil, err
	}

	return parseWeixinSearchReview(rawText, collectFigureCandidateIDs(candidates))
}

func (s *AIService) resolveWeixinSearchRuntimeSettings(maxOutputTokens int) (model.AISettings, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return model.AISettings{}, err
	}

	modelID := firstNonEmpty(settings.SceneModels.IMIntentModelID, settings.SceneModels.DefaultModelID)
	modelConfig, err := resolveModelByID(settings.Models, modelID)
	if err != nil {
		return model.AISettings{}, err
	}
	if strings.TrimSpace(modelConfig.APIKey) == "" {
		return model.AISettings{}, apperr.New(apperr.CodeFailedPrecondition, "请先在 AI 页面为 IM 意图识别场景配置可用模型和 API Key")
	}

	runtimeSettings := *settings
	runtimeSettings.Provider = modelConfig.Provider
	runtimeSettings.APIKey = modelConfig.APIKey
	runtimeSettings.BaseURL = modelConfig.BaseURL
	runtimeSettings.Model = modelConfig.Model
	runtimeSettings.MaxOutputTokens = minInt(modelConfig.MaxOutputTokens, maxOutputTokens)
	if runtimeSettings.MaxOutputTokens <= 0 {
		runtimeSettings.MaxOutputTokens = maxOutputTokens
	}
	runtimeSettings.OpenAILegacyMode = modelConfig.OpenAILegacyMode
	runtimeSettings.Temperature = 0.1

	return runtimeSettings, nil
}

func buildWeixinSearchSystemPrompt(systemPrompt string) string {
	systemPrompt = strings.TrimSpace(systemPrompt)
	if systemPrompt == "" {
		systemPrompt = model.DefaultAISettings().SystemPrompt
	}
	return systemPrompt + "\n\n你负责微信 IM 检索规划与候选复核。只输出检索计划或候选评估结果，不回答论文内容，不解释提示词，不寒暄。"
}

func buildWeixinPaperSearchReviewPrompt(query string, keywords []string, candidates []model.Paper) string {
	lines := []string{
		"请从下面的文献候选里选出最符合用户自然语言检索意图的 1 到 3 条。",
		"",
		"输出要求：",
		"1. 只返回 JSON 对象，不要加 Markdown 代码块。",
		`2. JSON 必须包含 summary 和 selected_ids 两个字段。`,
		`3. selected_ids 只能从候选里的 id 选择，数量必须是 1 到 3，并按匹配度从高到低排序。`,
		"4. summary 用 1 到 2 句简要说明你为什么这样判断，并给出整体把握度；不要复述整段候选文本。",
		"5. 优先判断主题、任务、方法和文献类型是否真正匹配，不要因为共用几个单词就误判。",
		"",
		fmt.Sprintf("用户原始请求：%s", query),
		fmt.Sprintf("展开关键词：%s", strings.Join(keywords, " | ")),
		"",
		"候选文献：",
	}

	for _, candidate := range candidates {
		tagNames := make([]string, 0, len(candidate.Tags))
		for _, tag := range candidate.Tags {
			if name := strings.TrimSpace(tag.Name); name != "" {
				tagNames = append(tagNames, name)
			}
		}
		abstractText := clipRunes(firstNonEmpty(candidate.AbstractText, candidate.PaperNotesText, candidate.NotesText), 220)
		lines = append(lines, fmt.Sprintf(
			"- id=%d | title=%s | tags=%s | figures=%d | status=%s | summary=%s",
			candidate.ID,
			clipRunes(candidate.Title, 120),
			joinOrFallback(tagNames, "无"),
			candidate.FigureCount,
			firstNonEmpty(candidate.ExtractionStatus, "unknown"),
			firstNonEmpty(abstractText, "无"),
		))
	}

	return strings.Join(lines, "\n")
}

func buildWeixinFigureSearchReviewPrompt(query string, keywords []string, candidates []model.FigureListItem, imageIDs []int64) string {
	lines := []string{
		"请从下面的图片候选里选出最符合用户自然语言检索意图的 1 到 3 条。",
		"",
		"输出要求：",
		"1. 只返回 JSON 对象，不要加 Markdown 代码块。",
		`2. JSON 必须包含 summary 和 selected_ids 两个字段。`,
		`3. selected_ids 只能从候选里的 id 选择，数量必须是 1 到 3，并按匹配度从高到低排序。`,
		"4. summary 用 1 到 2 句概括判断依据和整体把握度。",
		"5. 优先判断图本身的类型、图注语义和所属文献主题是否真正匹配，不要只看共享单词。",
		"",
		fmt.Sprintf("用户原始请求：%s", query),
		fmt.Sprintf("展开关键词：%s", strings.Join(keywords, " | ")),
	}

	if len(imageIDs) > 0 {
		imageRefs := make([]string, 0, len(imageIDs))
		for _, id := range imageIDs {
			imageRefs = append(imageRefs, strconv.FormatInt(id, 10))
		}
		lines = append(lines, fmt.Sprintf("附加缩略图顺序对应的候选 ID：%s", strings.Join(imageRefs, ", ")))
	} else {
		lines = append(lines, "本次没有附加缩略图，只能依据文本候选判断。")
	}

	lines = append(lines, "", "候选图片：")
	for _, candidate := range candidates {
		tagNames := make([]string, 0, len(candidate.Tags))
		for _, tag := range candidate.Tags {
			if name := strings.TrimSpace(tag.Name); name != "" {
				tagNames = append(tagNames, name)
			}
		}
		lines = append(lines, fmt.Sprintf(
			"- id=%d | paper=%s | label=%s | page=%d | tags=%s | caption=%s | notes=%s",
			candidate.ID,
			clipRunes(candidate.PaperTitle, 90),
			firstNonEmpty(candidate.DisplayLabel, fmt.Sprintf("图 %d", candidate.FigureIndex)),
			candidate.PageNumber,
			joinOrFallback(tagNames, "无"),
			firstNonEmpty(clipRunes(candidate.Caption, 180), "无"),
			firstNonEmpty(clipRunes(candidate.NotesText, 100), "无"),
		))
	}

	return strings.Join(lines, "\n")
}

func (s *AIService) loadWeixinFigureReviewImages(candidates []model.FigureListItem) ([]aiImageInput, []int64) {
	images := make([]aiImageInput, 0, len(candidates))
	imageIDs := make([]int64, 0, len(candidates))

	for _, candidate := range candidates {
		filename := filepath.Base(strings.TrimSpace(candidate.Filename))
		if filename == "" {
			continue
		}

		path := filepath.Join(s.config.FiguresDir(), filename)
		data, err := os.ReadFile(path)
		if err != nil {
			s.logger.Warn("load weixin figure review image failed", "figure_id", candidate.ID, "path", path, "error", err)
			continue
		}

		mimeType := http.DetectContentType(data)
		compressedData, compressedMIMEType, err := compressAIImage(data, mimeType)
		if err != nil {
			s.logger.Warn("compress weixin figure review image failed", "figure_id", candidate.ID, "error", err)
			continue
		}

		images = append(images, aiImageInput{
			MIMEType: compressedMIMEType,
			Data:     base64.StdEncoding.EncodeToString(compressedData),
		})
		imageIDs = append(imageIDs, candidate.ID)
	}

	return images, imageIDs
}

func collectPaperCandidateIDs(candidates []model.Paper) []int64 {
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	return ids
}

func collectFigureCandidateIDs(candidates []model.FigureListItem) []int64 {
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	return ids
}

func parseWeixinSearchPlan(raw string) (*weixinSearchPlan, error) {
	payload, err := decodeWeixinSearchJSON(raw)
	if err != nil {
		return nil, err
	}

	plan := &weixinSearchPlan{
		Target: normalizeWeixinSearchTarget(firstString(payload["target"], payload["type"], payload["mode"])),
		KeywordsZH: normalizeWeixinSearchKeywordsForLanguage(toStringSlice(
			payload["keywords_zh"],
			payload["zh_keywords"],
			payload["keywordsZh"],
			filterWeixinSearchKeywordsByLanguage(toStringSlice(payload["keywords"], payload["keyword"], payload["queries"]), "zh"),
		), "zh"),
		KeywordsEN: normalizeWeixinSearchKeywordsForLanguage(toStringSlice(
			payload["keywords_en"],
			payload["en_keywords"],
			payload["keywordsEn"],
			filterWeixinSearchKeywordsByLanguage(toStringSlice(payload["keywords"], payload["keyword"], payload["queries"]), "en"),
		), "en"),
	}
	if plan.Target == "" {
		return nil, apperr.New(apperr.CodeUnavailable, "IM 检索规划缺少 target")
	}
	plan.Keywords = mergeWeixinSearchKeywords(plan.KeywordsZH, plan.KeywordsEN)
	if len(plan.KeywordsZH) == 0 || len(plan.KeywordsEN) == 0 {
		return nil, apperr.New(apperr.CodeUnavailable, "IM 检索规划缺少中英文关键词")
	}
	return plan, nil
}

func parseWeixinCommandPlan(raw string) (*weixinCommandPlan, error) {
	payload, err := decodeWeixinSearchJSON(raw)
	if err != nil {
		return nil, err
	}

	command := normalizeWeixinPlainTextCommand(firstString(payload["command"], payload["action"], payload["intent"]))
	if command == "" {
		return nil, apperr.New(apperr.CodeUnavailable, "IM 命令规划缺少有效 command")
	}

	return &weixinCommandPlan{
		Command: command,
		Arg:     strings.TrimSpace(firstString(payload["arg"], payload["query"], payload["text"])),
	}, nil
}

func parseWeixinSearchReview(raw string, allowedIDs []int64) (*weixinSearchReview, error) {
	payload, err := decodeWeixinSearchJSON(raw)
	if err != nil {
		return nil, err
	}

	allowed := make(map[int64]struct{}, len(allowedIDs))
	for _, id := range allowedIDs {
		if id > 0 {
			allowed[id] = struct{}{}
		}
	}

	selectedIDs := []int64{}
	for _, text := range toStringSlice(payload["selected_ids"], payload["ids"], payload["selected"]) {
		id, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := allowed[id]; !ok {
			continue
		}
		if containsInt64(selectedIDs, id) {
			continue
		}
		selectedIDs = append(selectedIDs, id)
		if len(selectedIDs) >= weixinSearchResultLimit {
			break
		}
	}

	if len(selectedIDs) == 0 {
		return nil, apperr.New(apperr.CodeUnavailable, "IM 检索复核未返回有效候选")
	}

	return &weixinSearchReview{
		Summary:     clipRunes(firstNonEmpty(firstString(payload["summary"], payload["reason"], payload["analysis"])), 180),
		SelectedIDs: selectedIDs,
	}, nil
}

func decodeWeixinSearchJSON(raw string) (map[string]interface{}, error) {
	trimmed := strings.TrimSpace(raw)
	candidates := []string{
		trimmed,
		trimCodeFence(trimmed),
		trimJSONObject(trimmed),
	}

	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(candidate), &payload); err == nil {
			return payload, nil
		}
	}

	return nil, apperr.New(apperr.CodeUnavailable, "AI 未返回可解析的 JSON")
}

func normalizeWeixinSearchTarget(target string) string {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "paper", "papers", "article", "articles", "literature", "文献", "论文", "paper_search":
		return weixinSearchTargetPaper
	case "figure", "figures", "image", "images", "plot", "plots", "图片", "图", "配图", "figure_search":
		return weixinSearchTargetFigure
	default:
		return ""
	}
}

func normalizeWeixinSearchKeywords(keywords []string) []string {
	normalized := make([]string, 0, minInt(len(keywords), weixinSearchKeywordLimit))
	seen := map[string]struct{}{}

	for _, keyword := range keywords {
		for _, part := range splitWeixinSearchKeyword(keyword) {
			part = strings.Trim(part, " \t\r\n'\"`[](){}<>")
			part = strings.Join(strings.Fields(part), " ")
			if part == "" {
				continue
			}
			lookupKey := strings.ToLower(part)
			if _, exists := seen[lookupKey]; exists {
				continue
			}
			seen[lookupKey] = struct{}{}
			normalized = append(normalized, part)
			if len(normalized) >= weixinSearchKeywordLimit {
				return normalized
			}
		}
	}

	return normalized
}

func normalizeWeixinSearchKeywordsForLanguage(keywords []string, language string) []string {
	filtered := filterWeixinSearchKeywordsByLanguage(keywords, language)
	normalized := normalizeWeixinSearchKeywords(filtered)
	if len(normalized) > weixinSearchKeywordPerLanguageLimit {
		return append([]string(nil), normalized[:weixinSearchKeywordPerLanguageLimit]...)
	}
	return normalized
}

func filterWeixinSearchKeywordsByLanguage(keywords []string, language string) []string {
	filtered := make([]string, 0, len(keywords))
	want := strings.ToLower(strings.TrimSpace(language))
	for _, keyword := range keywords {
		for _, part := range splitWeixinSearchKeyword(keyword) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			switch want {
			case "zh":
				if detectTranslationLanguageKey(part) == "han" {
					filtered = append(filtered, part)
				}
			case "en":
				if detectTranslationLanguageKey(part) == "latin" {
					filtered = append(filtered, part)
				}
			default:
				filtered = append(filtered, part)
			}
		}
	}
	return filtered
}

func mergeWeixinSearchKeywords(keywordGroups ...[]string) []string {
	merged := make([]string, 0, weixinSearchKeywordLimit)
	seen := map[string]struct{}{}

	for _, keywords := range keywordGroups {
		for _, keyword := range keywords {
			for _, part := range splitWeixinSearchKeyword(keyword) {
				part = strings.Trim(part, " \t\r\n'\"`[](){}<>")
				part = strings.Join(strings.Fields(part), " ")
				if part == "" {
					continue
				}
				lookupKey := strings.ToLower(part)
				if _, exists := seen[lookupKey]; exists {
					continue
				}
				seen[lookupKey] = struct{}{}
				merged = append(merged, part)
				if len(merged) >= weixinSearchKeywordLimit {
					return merged
				}
			}
		}
	}

	return merged
}

func splitWeixinSearchKeyword(keyword string) []string {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil
	}

	replacer := strings.NewReplacer("\n", ",", "；", ",", ";", ",", "，", ",", "、", ",", "|", ",")
	parts := strings.Split(replacer.Replace(keyword), ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return []string{keyword}
	}
	return result
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
