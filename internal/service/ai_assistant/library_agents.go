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

type LLMExternalSearchPlanner struct {
	settings AISettingsProvider
	caller   NonStreamCaller
}

func NewLLMExternalSearchPlanner(settings AISettingsProvider, caller NonStreamCaller) *LLMExternalSearchPlanner {
	return &LLMExternalSearchPlanner{settings: settings, caller: caller}
}

func (p *LLMExternalSearchPlanner) PlanExternalSearch(ctx context.Context, query string) (ExternalSearchPlan, error) {
	if p == nil || p.settings == nil || p.caller == nil {
		return ExternalSearchPlan{}, fmt.Errorf("external search planner not configured")
	}
	settings, err := p.settings.GetSettings()
	if err != nil {
		return ExternalSearchPlan{}, err
	}
	out, _, err := p.caller.CallProviderGeneric(ctx, assistantMasterSettings(*settings), externalPlannerSystemPrompt, externalPlannerUserPrompt(query))
	if err != nil {
		return ExternalSearchPlan{}, err
	}
	var plan ExternalSearchPlan
	if err := decodeFirstJSONObject(out, &plan); err != nil {
		return ExternalSearchPlan{}, err
	}
	plan.SearchQuery = strings.Join(strings.Fields(plan.SearchQuery), " ")
	plan.SearchQueries = sanitizeExternalQueries(append([]string{plan.SearchQuery}, plan.SearchQueries...))
	if len(plan.SearchQueries) > 0 {
		plan.SearchQuery = plan.SearchQueries[0]
	}
	plan.Rationale = strings.TrimSpace(plan.Rationale)
	if len(plan.SearchQueries) == 0 {
		return ExternalSearchPlan{}, fmt.Errorf("empty external search query")
	}
	return plan, nil
}

type LLMExternalPaperClassifier struct {
	settings AISettingsProvider
	caller   NonStreamCaller
}

func NewLLMExternalPaperClassifier(settings AISettingsProvider, caller NonStreamCaller) *LLMExternalPaperClassifier {
	return &LLMExternalPaperClassifier{settings: settings, caller: caller}
}

func (c *LLMExternalPaperClassifier) ClassifyExternalPaper(ctx context.Context, in ExternalPaperClassificationInput) (ExternalPaperClassificationResult, error) {
	if c == nil || c.settings == nil || c.caller == nil {
		return ExternalPaperClassificationResult{}, fmt.Errorf("external paper classifier not configured")
	}
	settings, err := c.settings.GetSettings()
	if err != nil {
		return ExternalPaperClassificationResult{}, err
	}
	out, _, err := c.caller.CallProviderGeneric(ctx, assistantSubagentSettings(*settings), externalClassifierSystemPrompt, externalClassifierUserPrompt(in))
	if err != nil {
		return ExternalPaperClassificationResult{}, err
	}
	var res ExternalPaperClassificationResult
	if err := decodeFirstJSONObject(out, &res); err != nil {
		return ExternalPaperClassificationResult{}, err
	}
	return sanitizeExternalClassification(res), nil
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

const externalPlannerSystemPrompt = `你是 CiteBox AI 助手的 Master Agent。你的任务是把用户的外部调研或出处查找请求改写成适合 Semantic Scholar 的英文检索式。
只输出 JSON，不要输出 Markdown，不要解释思考过程。
JSON 格式：
{"search_query":"primary concise English academic query","search_queries":["query 1","query 2"],"rationale":"一句话说明检索式如何覆盖用户需求"}
规则：
- 用户可能使用任意语言提问；理解其真实学术需求，并改写为简短英文关键词检索式。
- 保留用户明确给出的技术名词、实验类型、测序类型、物种、疾病和缩写，例如 ChIP-seq、ATAC-seq、single-cell RNA-seq。
- 用户要求找出处、引用或证据时，检索核心断言本身，不要保留 source、citation、reference、出处、引用等操作词。
- 不要加入 article、paper、literature、data、search、find、about、external 等泛词。
- 生成 2 到 4 个互补查询：第一个高精度，后续用于召回同义表达；不要把所有限定词塞进同一个长查询。
- 每个查询优先输出 3 到 8 个关键词；必要时加入一个能提高召回的同义技术短语。`

func libraryPlannerUserPrompt(query string) string {
	return "用户检索请求：\n" + strings.TrimSpace(query)
}

func externalPlannerUserPrompt(query string) string {
	return "用户外部检索请求：\n" + strings.TrimSpace(query)
}

const libraryClassifierSystemPrompt = `你是 CiteBox 的 Sub-Agent，负责逐篇判断全文扫描候选是否真正符合用户需求。
只输出 JSON，不要输出 Markdown。
JSON 格式：
{"relevant":true,"reason":"一句话给出命中理由"}
判断规则：
- 只有候选文献确实使用、包含、分析或明确比较了用户要求的数据/方法，才 relevant=true。
- 只是泛泛提到背景词、引用无关术语、或者只出现“数据/文章”等泛词，必须 relevant=false。
- reason 必须引用候选片段中的具体证据词。`

const externalClassifierSystemPrompt = `你是 CiteBox 的 Sub-Agent，负责判断外部检索候选是否能作为用户原句或问题的出处。
只输出 JSON，不要输出 Markdown。
JSON 格式：
{"relevant":true,"reason":"一句话说明为什么保留或排除","annotations":[{"claim":"用户原句中被支持的子断言","evidence":"候选文本中对应的原文句子","verdict":"supported|partial|unsupported","rationale":"一句话解释对应关系"}]}
判断规则：
- 必须把用户原句拆成可以核查的子断言，再看候选标题、TLDR、摘要中是否有对应原文。
- relevant=true 只用于候选能支持核心断言，或能支持一个对用户问题非常关键的子断言。
- 如果只有主题词相似，但没有能对应原句的证据句，必须 relevant=false。
- evidence 必须是候选文本里的原句或紧密片段；不要编造正文中不存在的句子。
- annotations 最多 4 条，优先标注 supported 和 partial 的对应关系。`

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

func externalClassifierUserPrompt(in ExternalPaperClassificationInput) string {
	var b strings.Builder
	b.WriteString("用户原句或问题：\n")
	b.WriteString(strings.TrimSpace(in.Query))
	b.WriteString("\n\nMaster 检索式：")
	b.WriteString(strings.Join(in.SearchQueries, " | "))
	if strings.TrimSpace(in.MatchedQuery) != "" {
		b.WriteString("\n候选命中的检索式：")
		b.WriteString(strings.TrimSpace(in.MatchedQuery))
	}
	b.WriteString("\n\n候选文献：\n标题：")
	b.WriteString(in.Paper.Title)
	if in.Paper.ExternalIDs.DOI != "" {
		b.WriteString("\nDOI：")
		b.WriteString(in.Paper.ExternalIDs.DOI)
	}
	if in.Paper.Year > 0 {
		fmt.Fprintf(&b, "\n年份：%d", in.Paper.Year)
	}
	if strings.TrimSpace(in.Paper.Venue) != "" {
		b.WriteString("\n来源：")
		b.WriteString(strings.TrimSpace(in.Paper.Venue))
	}
	b.WriteString("\n\n候选证据文本：\n")
	b.WriteString(trimRunes(normalizeEvidenceWhitespace(in.EvidenceText), 2200))
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
