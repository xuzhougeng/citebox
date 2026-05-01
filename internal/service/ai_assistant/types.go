package ai_assistant

import "github.com/xuzhougeng/citebox/internal/service/research"

const (
	IntentAuto           = ""
	IntentLibrarySearch  = "library_search"
	IntentExternalSearch = "external_search"
	IntentPaperRead      = "paper_read"
	IntentFigureLookup   = "figure_lookup"
	IntentChat           = "chat"
)

type RequestContext struct {
	Source   string  `json:"source,omitempty"`
	PaperID  int64   `json:"paper_id,omitempty"`
	PaperIDs []int64 `json:"paper_ids,omitempty"`
	FigureID int64   `json:"figure_id,omitempty"`
}

type RouteInput struct {
	Content    string         `json:"content"`
	IntentHint string         `json:"intent_hint,omitempty"`
	Context    RequestContext `json:"context,omitempty"`
}

type ToolInput struct {
	Query      string         `json:"query"`
	Context    RequestContext `json:"context,omitempty"`
	Limit      int            `json:"limit,omitempty"`
	IntentHint string         `json:"intent_hint,omitempty"`
}

type RouteDecision struct {
	Intent     string `json:"intent"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason,omitempty"`
}

type ProcessStage struct {
	Label      string `json:"label"`
	Count      int    `json:"count,omitempty"`
	Unit       string `json:"unit,omitempty"`
	Status     string `json:"status,omitempty"`
	Detail     string `json:"detail,omitempty"`
	DurationMS int    `json:"duration_ms,omitempty"`
}

type ProcessSummary struct {
	Intent string         `json:"intent"`
	Stages []ProcessStage `json:"stages"`
	Note   string         `json:"note,omitempty"`
}

type ResultCard struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type PaperHitCard struct {
	PaperID        int64             `json:"paper_id"`
	Title          string            `json:"title"`
	DOI            string            `json:"doi,omitempty"`
	Year           string            `json:"year,omitempty"`
	Reason         string            `json:"reason"`
	HighlightTerms []string          `json:"highlight_terms,omitempty"`
	Snippets       []PaperHitSnippet `json:"snippets"`
}

type PaperHitSnippet struct {
	CitationIndex int    `json:"citation_index"`
	Location      string `json:"location"`
	Text          string `json:"text"`
}

type ToolCallSummary struct {
	ToolName          string `json:"tool_name"`
	InputJSON         string `json:"input_json,omitempty"`
	OutputSummaryJSON string `json:"output_summary_json,omitempty"`
	Status            string `json:"status,omitempty"`
	DurationMS        int    `json:"duration_ms,omitempty"`
	Error             string `json:"error,omitempty"`
}

type Citation struct {
	I          int              `json:"i"`
	PaperID    int64            `json:"paper_id,omitempty"`
	ExternalID string           `json:"external_id,omitempty"`
	S2PaperID  string           `json:"s2_paper_id,omitempty"`
	Title      string           `json:"title,omitempty"`
	Source     string           `json:"source,omitempty"`
	Snippet    research.Snippet `json:"snippet"`
	Score      float64          `json:"score,omitempty"`
}

type ToolResult struct {
	Process       ProcessSummary    `json:"process"`
	Cards         []ResultCard      `json:"cards,omitempty"`
	Citations     []Citation        `json:"citations,omitempty"`
	AnswerContext string            `json:"answer_context,omitempty"`
	ToolCalls     []ToolCallSummary `json:"tool_calls,omitempty"`
}

func WithAnswerGenerationStage(summary ProcessSummary, status string) ProcessSummary {
	stage := ProcessStage{
		Label:  "生成回答",
		Status: status,
		Detail: "根据工具结果生成最终回答",
	}
	out := summary
	out.Stages = append([]ProcessStage(nil), summary.Stages...)
	for i, existing := range out.Stages {
		if existing.Label == stage.Label {
			out.Stages[i] = stage
			return out
		}
	}
	out.Stages = append(out.Stages, stage)
	return out
}
