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
)

type AIModelConfig struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Provider         AIProvider `json:"provider"`
	APIKey           string     `json:"api_key"`
	BaseURL          string     `json:"base_url"`
	Model            string     `json:"model"`
	OpenAILegacyMode bool       `json:"openai_legacy_mode"`
}

type AISceneModelSelection struct {
	DefaultModelID string `json:"default_model_id"`
	QAModelID      string `json:"qa_model_id"`
	FigureModelID  string `json:"figure_model_id"`
	TagModelID     string `json:"tag_model_id"`
	GroupModelID   string `json:"group_model_id"`
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
}

type AIReadRequest struct {
	PaperID  int64                `json:"paper_id"`
	Action   AIAction             `json:"action"`
	Question string               `json:"question"`
	History  []AIConversationTurn `json:"history,omitempty"`
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

func DefaultAISettings() AISettings {
	defaultModel := AIModelConfig{
		ID:               "default-openai",
		Name:             "OpenAI Default",
		Provider:         AIProviderOpenAI,
		BaseURL:          "https://api.openai.com",
		Model:            "gpt-4.1-mini",
		OpenAILegacyMode: false,
	}

	return AISettings{
		Provider:         defaultModel.Provider,
		BaseURL:          defaultModel.BaseURL,
		Model:            defaultModel.Model,
		OpenAILegacyMode: defaultModel.OpenAILegacyMode,
		Models:           []AIModelConfig{defaultModel},
		SceneModels: AISceneModelSelection{
			DefaultModelID: defaultModel.ID,
			QAModelID:      defaultModel.ID,
			FigureModelID:  defaultModel.ID,
			TagModelID:     defaultModel.ID,
			GroupModelID:   defaultModel.ID,
		},
		Temperature:     0.2,
		MaxOutputTokens: 1200,
		MaxFigures:      0,
		SystemPrompt:    "你是一名帮助用户阅读科研论文的 AI 助手。回答时必须优先引用文献原文、摘要、笔记和提取图片中的证据；证据不足时要明确说明不确定。",
		QAPrompt:        "请结合全文和图片，围绕用户问题给出精炼但信息密度高的回答，优先解释结论、证据和局限。",
		FigurePrompt:    "请重点解读这篇文献里的图片：说明关键图分别展示了什么、支持了什么结论、有哪些实验设计与可能局限。",
		TagPrompt:       "请为这篇文献建议 3 到 8 个标签，优先复用已有标签；如果现有标签不够，再补充必要的新标签。",
		GroupPrompt:     "请判断这篇文献最适合放入哪个分组，优先复用已有分组；如果没有合适分组，再给出一个新的分组名称。",
	}
}
