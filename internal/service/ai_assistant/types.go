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
	Content    string
	IntentHint string
	Context    RequestContext
}

type RouteDecision struct {
	Intent     string
	Confidence string
	Reason     string
}

type ProcessStage struct {
	Label      string `json:"label"`
	Count      int    `json:"count,omitempty"`
	Unit       string `json:"unit,omitempty"`
	Status     string `json:"status,omitempty"`
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

type ToolCallSummary struct {
	ToolName          string
	InputJSON         string
	OutputSummaryJSON string
	Status            string
	DurationMS        int
	Error             string
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
	Process       ProcessSummary
	Cards         []ResultCard
	Citations     []Citation
	AnswerContext string
	ToolCalls     []ToolCallSummary
}
