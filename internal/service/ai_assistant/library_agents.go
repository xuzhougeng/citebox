package ai_assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xuzhougeng/citebox/internal/model"
)

type AISettingsProvider interface {
	GetSettings() (*model.AISettings, error)
}

type NonStreamCaller interface {
	CallProviderGeneric(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string) (string, string, error)
}

type LLMLibrarySearchPlanner struct {
	settings AISettingsProvider
	caller   NonStreamCaller
}

func NewLLMLibrarySearchPlanner(settings AISettingsProvider, caller NonStreamCaller) *LLMLibrarySearchPlanner {
	return &LLMLibrarySearchPlanner{settings: settings, caller: caller}
}

func (p *LLMLibrarySearchPlanner) PlanLibrarySearch(ctx context.Context, query string) (LibrarySearchPlan, error) {
	if p == nil || p.settings == nil || p.caller == nil {
		return LibrarySearchPlan{}, fmt.Errorf("library search planner not configured")
	}
	settings, err := p.settings.GetSettings()
	if err != nil {
		return LibrarySearchPlan{}, err
	}
	out, _, err := p.caller.CallProviderGeneric(ctx, assistantMasterSettings(*settings), libraryPlannerSystemPrompt, libraryPlannerUserPrompt(query))
	if err != nil {
		return LibrarySearchPlan{}, err
	}
	var plan LibrarySearchPlan
	if err := decodeFirstJSONObject(out, &plan); err != nil {
		return LibrarySearchPlan{}, err
	}
	plan.SearchTerms = sanitizeEvidenceTerms(plan.SearchTerms)
	if len(plan.SearchTerms) == 0 {
		plan.SearchTerms = EvidenceSearchTerms(query)
	}
	return plan, nil
}

type LLMLibraryPaperClassifier struct {
	settings AISettingsProvider
	caller   NonStreamCaller
}

func NewLLMLibraryPaperClassifier(settings AISettingsProvider, caller NonStreamCaller) *LLMLibraryPaperClassifier {
	return &LLMLibraryPaperClassifier{settings: settings, caller: caller}
}

func (c *LLMLibraryPaperClassifier) ClassifyLibraryPaper(ctx context.Context, in LibraryPaperClassificationInput) (LibraryPaperClassificationResult, error) {
	if c == nil || c.settings == nil || c.caller == nil {
		return LibraryPaperClassificationResult{}, fmt.Errorf("library paper classifier not configured")
	}
	settings, err := c.settings.GetSettings()
	if err != nil {
		return LibraryPaperClassificationResult{}, err
	}
	out, _, err := c.caller.CallProviderGeneric(ctx, assistantSubagentSettings(*settings), libraryClassifierSystemPrompt, libraryClassifierUserPrompt(in))
	if err != nil {
		return LibraryPaperClassificationResult{}, err
	}
	var res LibraryPaperClassificationResult
	if err := decodeFirstJSONObject(out, &res); err != nil {
		return LibraryPaperClassificationResult{}, err
	}
	return res, nil
}

const libraryPlannerSystemPrompt = `你是 CiteBox AI 助手的 Master Agent。你的任务是把用户的文献库检索请求改写成可执行的全文扫描计划。
只输出 JSON，不要输出 Markdown，不要解释思考过程。
JSON 格式：
{"search_terms":["term1","term2"],"rationale":"一句话说明为什么这些词能覆盖用户需求"}
规则：
- 保留用户明确给出的技术名词、测序类型、实验类型和缩写，例如 ChIP-seq、ATAC-seq、single-cell RNA-seq。
- 可以补充少量同义写法，例如 ChIP seq、chromatin immunoprecipitation sequencing。
- 不要加入“数据、文章、论文、文献、相关、查找、paper、data”等泛词。
- 不要把窄问题扩大成泛化问题；用户问 ChIP-seq 时，不要改成所有 sequencing。`

func libraryPlannerUserPrompt(query string) string {
	return "用户检索请求：\n" + strings.TrimSpace(query)
}

const libraryClassifierSystemPrompt = `你是 CiteBox 的 Sub-Agent，负责逐篇判断全文扫描候选是否真正符合用户需求。
只输出 JSON，不要输出 Markdown。
JSON 格式：
{"relevant":true,"reason":"一句话给出命中理由"}
判断规则：
- 只有候选文献确实使用、包含、分析或明确比较了用户要求的数据/方法，才 relevant=true。
- 只是泛泛提到背景词、引用无关术语、或者只出现“数据/文章”等泛词，必须 relevant=false。
- reason 必须引用候选片段中的具体证据词。`

func libraryClassifierUserPrompt(in LibraryPaperClassificationInput) string {
	var b strings.Builder
	b.WriteString("用户需求：\n")
	b.WriteString(strings.TrimSpace(in.Query))
	b.WriteString("\n\nMaster 检索词：")
	b.WriteString(strings.Join(in.Terms, " | "))
	if in.Plan.Rationale != "" {
		b.WriteString("\nMaster 规划说明：")
		b.WriteString(in.Plan.Rationale)
	}
	b.WriteString("\n\n候选文献：\n标题：")
	b.WriteString(in.Paper.Title)
	if in.Paper.DOI != "" {
		b.WriteString("\nDOI：")
		b.WriteString(in.Paper.DOI)
	}
	if strings.TrimSpace(in.Paper.AbstractText) != "" {
		b.WriteString("\n摘要：")
		b.WriteString(trimRunes(normalizeEvidenceWhitespace(in.Paper.AbstractText), 1400))
	}
	b.WriteString("\n\n全文命中片段：\n")
	maxSnippets := in.MaxSnippets
	if maxSnippets <= 0 || maxSnippets > 5 {
		maxSnippets = 3
	}
	for i, match := range in.Matches {
		if i >= maxSnippets {
			break
		}
		fmt.Fprintf(&b, "[%d] %s: %s\n", i+1, match.Location, trimRunes(match.Snippet.Text, 1200))
	}
	return b.String()
}

func decodeFirstJSONObject(raw string, dest any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("empty JSON response")
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return fmt.Errorf("JSON object not found")
	}
	return json.Unmarshal([]byte(raw[start:end+1]), dest)
}

func assistantMasterSettings(settings model.AISettings) model.AISettings {
	return assistantSettingsWithSceneModel(settings, firstNonEmpty(
		settings.SceneModels.AssistantMasterModelID,
		settings.SceneModels.QAModelID,
		settings.SceneModels.DefaultModelID,
	))
}

func assistantSubagentSettings(settings model.AISettings) model.AISettings {
	return assistantSettingsWithSceneModel(settings, firstNonEmpty(
		settings.SceneModels.AssistantSubagentModelID,
		settings.SceneModels.IMIntentModelID,
		settings.SceneModels.AssistantMasterModelID,
		settings.SceneModels.DefaultModelID,
	))
}

func assistantSettingsWithSceneModel(settings model.AISettings, modelID string) model.AISettings {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return settings
	}
	for _, item := range settings.Models {
		if item.ID == modelID {
			settings.Provider = item.Provider
			settings.APIKey = item.APIKey
			settings.BaseURL = item.BaseURL
			settings.Model = item.Model
			settings.MaxOutputTokens = item.MaxOutputTokens
			settings.OpenAILegacyMode = item.OpenAILegacyMode
			settings.OmitTemperature = item.OmitTemperature
			settings.ThinkingEnabled = item.ThinkingEnabled
			settings.ReasoningEffort = item.ReasoningEffort
			return settings
		}
	}
	return settings
}

func trimRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}
