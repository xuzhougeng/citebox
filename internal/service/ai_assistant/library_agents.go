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

func (p *LLMExternalSearchPlanner) PlanExternalSearch(ctx context.Context, query string, goalHint ExternalSearchGoal) (ExternalSearchPlan, error) {
	if p == nil || p.settings == nil || p.caller == nil {
		return ExternalSearchPlan{}, fmt.Errorf("external search planner not configured")
	}
	settings, err := p.settings.GetSettings()
	if err != nil {
		return ExternalSearchPlan{}, err
	}
	out, _, err := p.caller.CallProviderGeneric(ctx, assistantMasterSettings(*settings), externalPlannerSystemPrompt, externalPlannerUserPrompt(query, goalHint))
	if err != nil {
		return ExternalSearchPlan{}, err
	}
	var plan ExternalSearchPlan
	if err := decodeFirstJSONObject(out, &plan); err != nil {
		return ExternalSearchPlan{}, err
	}
	rawGoal := strings.TrimSpace(string(plan.SearchGoal))
	plan = sanitizeExternalPlan(plan)
	if !isKnownExternalSearchGoal(rawGoal) {
		plan.SearchGoal = fallbackExternalSearchGoal(query)
	}
	if len(plan.SearchQueries) == 0 && len(plan.QueriesBySource) == 0 {
		plan.SearchQueries = ExternalSearchQueries(query)
		plan.SearchQuery = firstExternalQuery(plan.SearchQueries)
	}
	if len(plan.SearchQueries) == 0 && len(plan.QueriesBySource) == 0 {
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

const externalPlannerSystemPrompt = `你是 CiteBox AI 助手的 Master Agent。你的任务是把用户的外部调研或出处查找请求改写成适合已启用外部来源的英文检索式。
只输出 JSON，不要输出 Markdown，不要解释思考过程。
JSON 格式：
{"search_goal":"discovery|evidence","must_match":["term 1","term 2"],"soft_preferences":["term 1","term 2"],"target_year":2024,"queries_by_source":{"pubmed":["query 1","query 2"],"semantic_scholar":["query 1","query 2"]},"rationale":"一句话说明检索式如何覆盖用户需求"}
规则：
- 用户可能使用任意语言提问；理解其真实学术需求，并改写为简短英文关键词检索式。
- 保留用户明确给出的技术名词、实验类型、测序类型、物种、疾病和缩写，例如 ChIP-seq、ATAC-seq、single-cell RNA-seq。
- "search_goal" 只能是 "discovery" 或 "evidence"。主题摸排、综述、最新进展、有哪些方向等用 discovery；核查具体断言、找出处、找编号对应文献等用 evidence。
- "must_match" 只放用户需求里真正不能放松的硬约束，例如核心实体、疾病、干预、方法、明确限定的人群/物种。去重，控制在 2 到 6 项。
- "soft_preferences" 放有助于召回或排序的同义表达、补充表述、上下文限定或扩展词。查询扩展词不能自动升级为 "must_match" 硬约束。
- "target_year" 只在用户明确要求某一年或近年窗口的锚点年份时填写，否则填 0。
- 用户要求找出处、引用或证据时，检索核心断言本身，不要保留 source、citation、reference、出处、引用等操作词。
- 不要加入 article、paper、literature、data、search、find、about、external 等泛词。
- 只为已启用的来源生成查询；当前支持来源键为 pubmed 和 semantic_scholar。
- 为每个已启用来源生成 2 到 4 个互补短英文查询：第一个高精度，后续用于召回同义表达；不要把所有限定词塞进同一个长查询。
- pubmed 查询应偏生物医学和 MeSH 风格，可使用疾病、干预、实验体系、物种、基因/蛋白名和常见 MeSH 词组，不要使用 PubMed 不支持的复杂语法。
- semantic_scholar 查询应更宽泛，面向跨学科论文标题、摘要和关键词检索，覆盖方法、核心断言和同义学术表达。
- 每个查询优先输出 3 到 8 个关键词；必要时加入一个能提高召回的同义技术短语。`

func libraryPlannerUserPrompt(query string) string {
	return "用户检索请求：\n" + strings.TrimSpace(query)
}

func externalPlannerUserPrompt(query string, goalHint ExternalSearchGoal) string {
	var b strings.Builder
	b.WriteString("用户外部检索请求：\n")
	b.WriteString(strings.TrimSpace(query))
	if isKnownExternalSearchGoal(string(goalHint)) {
		b.WriteString("\n\nexplicit_search_goal_hint: ")
		b.WriteString(string(normalizeExternalSearchGoal(string(goalHint))))
	}
	return b.String()
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
{"tier":"strong_match|weak_match|needs_review|drop","reason":"一句话说明为什么保留或排除","matched_constraints":["命中的 must_match 约束"],"matched_preferences":["命中的 soft_preferences"],"article_role":"primary_study|review|meta_analysis|guideline|case_report|dataset|method|commentary|other","annotations":[{"claim":"用户原句中被支持的子断言","evidence":"候选文本中对应的原文句子","verdict":"supported|partial|unsupported","rationale":"一句话解释对应关系"}]}
判断规则：
- 必须把用户原句拆成可以核查的子断言，再看候选标题、TLDR、摘要中是否有对应原文。
- 先根据 search_goal 判断分层规则。
- 当 search_goal=discovery 时：strong_match 用于真正命中 must_match 且明显值得保留的候选；weak_match 用于主题相关、命中部分偏好、可作为拓展阅读的候选；needs_review 用于信息不足、只看标题难判断、或年份/角色存在歧义的候选；drop 用于明显不相关或违背 must_match 的候选。
- 当 search_goal=evidence 时：strong_match 只用于能直接支持核心断言或关键子断言的候选；weak_match 用于只支持背景、旁证、部分子断言或只给出间接线索的候选；needs_review 用于证据关系不清、年份版本可疑、或需要正文才能确认的候选；drop 用于只有主题词相似但没有对应证据句、或明显不能支持断言的候选。
- matched_constraints 只列真正被候选文本满足的 must_match 项；matched_preferences 只列真正命中的 soft_preferences 项；两者都不要编造。
- article_role 选择最贴近的文献角色；拿不准就填 other。
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
	if in.SearchGoal != "" {
		b.WriteString("\n\nsearch_goal: ")
		b.WriteString(strings.TrimSpace(string(in.SearchGoal)))
	}
	if len(in.MustMatch) > 0 {
		b.WriteString("\nmust_match: ")
		b.WriteString(strings.Join(in.MustMatch, " | "))
	}
	if len(in.SoftPreferences) > 0 {
		b.WriteString("\nsoft_preferences: ")
		b.WriteString(strings.Join(in.SoftPreferences, " | "))
	}
	if strings.TrimSpace(in.YearLabel) != "" {
		b.WriteString("\nyear_label: ")
		b.WriteString(strings.TrimSpace(in.YearLabel))
	}
	if in.OnlineYear > 0 {
		fmt.Fprintf(&b, "\nonline_year: %d", in.OnlineYear)
	}
	if in.IssueYear > 0 {
		fmt.Fprintf(&b, "\nissue_year: %d", in.IssueYear)
	}
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
