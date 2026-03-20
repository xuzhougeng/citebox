package model

type AIProvider string

const (
	AIProviderOpenAI    AIProvider = "openai"
	AIProviderAnthropic AIProvider = "anthropic"
	AIProviderGemini    AIProvider = "gemini"
)

type AIAction string

const (
	AIActionPaperQA              AIAction = "paper_qa"
	AIActionFigureInterpretation AIAction = "figure_interpretation"
	AIActionTagSuggestion        AIAction = "tag_suggestion"
	AIActionGroupSuggestion      AIAction = "group_suggestion"
	AIActionTranslate            AIAction = "translate"
)

type AIModelConfig struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Provider         AIProvider `json:"provider"`
	APIKey           string     `json:"api_key"`
	BaseURL          string     `json:"base_url"`
	Model            string     `json:"model"`
	MaxOutputTokens  int        `json:"max_output_tokens"`
	OpenAILegacyMode bool       `json:"openai_legacy_mode"`
}

type AISceneModelSelection struct {
	DefaultModelID   string `json:"default_model_id"`
	QAModelID        string `json:"qa_model_id"`
	FigureModelID    string `json:"figure_model_id"`
	TagModelID       string `json:"tag_model_id"`
	GroupModelID     string `json:"group_model_id"`
	TranslateModelID string `json:"translate_model_id"`
}

type AITranslationConfig struct {
	PrimaryLanguage string `json:"primary_language"`
	TargetLanguage  string `json:"target_language"`
}

type AIRolePrompt struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type AIRolePromptCollection struct {
	RolePrompts []AIRolePrompt `json:"role_prompts"`
}

type AIModelSettingsUpdate struct {
	Models      []AIModelConfig       `json:"models"`
	SceneModels AISceneModelSelection `json:"scene_models"`
	Temperature float64               `json:"temperature"`
	MaxFigures  int                   `json:"max_figures"`
	Translation AITranslationConfig   `json:"translation"`
}

type AIPromptSettingsUpdate struct {
	SystemPrompt    string `json:"system_prompt"`
	QAPrompt        string `json:"qa_prompt"`
	FigurePrompt    string `json:"figure_prompt"`
	TagPrompt       string `json:"tag_prompt"`
	GroupPrompt     string `json:"group_prompt"`
	TranslatePrompt string `json:"translate_prompt"`
}

type AISettings struct {
	Provider         AIProvider            `json:"provider"`
	APIKey           string                `json:"api_key"`
	BaseURL          string                `json:"base_url"`
	Model            string                `json:"model"`
	OpenAILegacyMode bool                  `json:"openai_legacy_mode"`
	Models           []AIModelConfig       `json:"models"`
	SceneModels      AISceneModelSelection `json:"scene_models"`
	Temperature      float64               `json:"temperature"`
	MaxOutputTokens  int                   `json:"max_output_tokens"`
	MaxFigures       int                   `json:"max_figures"`
	SystemPrompt     string                `json:"system_prompt"`
	QAPrompt         string                `json:"qa_prompt"`
	FigurePrompt     string                `json:"figure_prompt"`
	TagPrompt        string                `json:"tag_prompt"`
	GroupPrompt      string                `json:"group_prompt"`
	TranslatePrompt  string                `json:"translate_prompt"`
	Translation      AITranslationConfig   `json:"translation"`
	RolePrompts      []AIRolePrompt        `json:"role_prompts"`
}

type AIReadRequest struct {
	PaperID  int64                `json:"paper_id"`
	FigureID int64                `json:"figure_id,omitempty"`
	Action   AIAction             `json:"action"`
	Question string               `json:"question"`
	History  []AIConversationTurn `json:"history,omitempty"`
}

type AIReadExportRequest struct {
	PaperID   int64  `json:"paper_id"`
	Answer    string `json:"answer,omitempty"`
	Content   string `json:"content,omitempty"`
	Scope     string `json:"scope,omitempty"`
	TurnIndex int    `json:"turn_index,omitempty"`
}

type AITranslateRequest struct {
	Text string `json:"text"`
}

type AIConversationTurn struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type AIReadResponse struct {
	Success         bool       `json:"success"`
	Provider        AIProvider `json:"provider"`
	Model           string     `json:"model"`
	Mode            string     `json:"mode"`
	Action          AIAction   `json:"action"`
	PaperID         int64      `json:"paper_id"`
	Question        string     `json:"question"`
	Answer          string     `json:"answer"`
	SuggestedTags   []string   `json:"suggested_tags"`
	SuggestedGroup  string     `json:"suggested_group"`
	IncludedFigures int        `json:"included_figures"`
}

type AIModelCheckResponse struct {
	Success  bool       `json:"success"`
	Provider AIProvider `json:"provider"`
	Model    string     `json:"model"`
	Mode     string     `json:"mode"`
	Message  string     `json:"message"`
}

type AIReadStreamEvent struct {
	Type   string          `json:"type"`
	Delta  string          `json:"delta,omitempty"`
	Result *AIReadResponse `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
	Code   string          `json:"code,omitempty"`
}

type AITranslateResponse struct {
	Success        bool       `json:"success"`
	Provider       AIProvider `json:"provider"`
	Model          string     `json:"model"`
	Mode           string     `json:"mode"`
	SourceLanguage string     `json:"source_language"`
	TargetLanguage string     `json:"target_language"`
	Translation    string     `json:"translation"`
}

func DefaultAISettings() AISettings {
	defaultModel := AIModelConfig{
		ID:               "default-openai",
		Name:             "OpenAI Default",
		Provider:         AIProviderOpenAI,
		BaseURL:          "https://api.openai.com",
		Model:            "gpt-4.1-mini",
		MaxOutputTokens:  1200,
		OpenAILegacyMode: false,
	}

	return AISettings{
		Provider:         defaultModel.Provider,
		BaseURL:          defaultModel.BaseURL,
		Model:            defaultModel.Model,
		OpenAILegacyMode: defaultModel.OpenAILegacyMode,
		Models:           []AIModelConfig{defaultModel},
		SceneModels: AISceneModelSelection{
			DefaultModelID:   defaultModel.ID,
			QAModelID:        defaultModel.ID,
			FigureModelID:    defaultModel.ID,
			TagModelID:       defaultModel.ID,
			GroupModelID:     defaultModel.ID,
			TranslateModelID: defaultModel.ID,
		},
		Temperature:     0.2,
		MaxOutputTokens: 1200,
		MaxFigures:      0,
		SystemPrompt:    "你是一名帮助用户阅读科研论文的 AI 助手。回答时必须优先引用文献原文、摘要、笔记和提取图片中的证据；证据不足时要明确说明不确定。",
		QAPrompt:        "请结合全文和图片，围绕用户问题给出精炼但信息密度高的回答，优先解释结论、证据和局限；在有帮助时可直接插入系统提供的图片引用。",
		FigurePrompt:    "说明这张图片展示了什么、关键实验设计或对照关系是什么、它支持了什么结论，以及有哪些可能的局限。",
		TagPrompt:       "给出 3 到 8 个适合检索和归档的图片标签，优先复用已有图片标签；避免过泛词，只有在现有标签不够时再补充必要的新标签。",
		GroupPrompt:     "请根据整篇文献的主题、方法和用途，判断它最适合放入哪个分组，优先复用已有分组；如果没有合适分组，再给出一个新的分组名称。",
		TranslatePrompt: "你只负责翻译，不做解释、不补充背景、不总结。必须保持原文含义、语气、术语和格式；如果原文里有专有名词、缩写、数字或单位，优先准确保留。",
		Translation: AITranslationConfig{
			PrimaryLanguage: "中文",
			TargetLanguage:  "英文",
		},
		RolePrompts: []AIRolePrompt{},
	}
}
