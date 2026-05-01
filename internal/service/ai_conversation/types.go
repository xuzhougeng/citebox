// Package ai_conversation implements stateful AI chat conversations:
// CRUD + pin papers + send-message orchestration with sliding-window context
// management. Strict-evidence and summarization live in sibling files.
package ai_conversation

import (
	"time"

	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
)

// Conversation is the read view returned by GetConversation.
type Conversation struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	TitleLocked    bool          `json:"title_locked"`
	StrictEvidence bool          `json:"strict_evidence"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	PinnedPapers   []PinnedPaper `json:"pinned_papers"`
	RecentMessages []Message     `json:"recent_messages"`
	TurnRuns       []TurnRun     `json:"turn_runs,omitempty"`
}

// PinnedPaper mirrors AIPinnedPaper from the repo layer.
type PinnedPaper struct {
	PaperID  int64     `json:"paper_id"`
	Title    string    `json:"title"`
	DOI      string    `json:"doi,omitempty"`
	PinnedAt time.Time `json:"pinned_at"`
}

// Message is a single user/assistant message.
type Message struct {
	ID              int64     `json:"id"`
	Role            string    `json:"role"`
	Content         string    `json:"content"`
	Provider        string    `json:"provider,omitempty"`
	Model           string    `json:"model,omitempty"`
	Mode            string    `json:"mode,omitempty"`
	IncludedFigures int       `json:"included_figures,omitempty"`
	CitationsJSON   string    `json:"citations_json,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type TurnRun struct {
	ID                 int64        `json:"id"`
	UserMessageID      int64        `json:"user_message_id"`
	AssistantMessageID int64        `json:"assistant_message_id,omitempty"`
	Intent             string       `json:"intent"`
	IntentHint         string       `json:"intent_hint,omitempty"`
	ProcessSummaryJSON string       `json:"process_summary_json,omitempty"`
	Status             string       `json:"status"`
	Cards              []ResultCard `json:"cards,omitempty"`
}

type ResultCard struct {
	ID          int64  `json:"id"`
	TurnRunID   int64  `json:"turn_run_id"`
	CardType    string `json:"card_type"`
	SortOrder   int    `json:"sort_order"`
	PayloadJSON string `json:"payload_json"`
}

type StreamEvent struct {
	Type string
	Data any
}

// SendMessageInput is the body for POST .../messages.
type SendMessageInput struct {
	ConversationID          int64 // 0 means "create new"
	Content                 string
	PaperID                 int64   // optional auto-pin (single)
	PaperIDs                []int64 // optional auto-pin (multi); takes precedence over PaperID when non-empty
	IncludeExternalEvidence bool
	IntentHint              string
	Sources                 []string
	Context                 ai_assistant.RequestContext
	OnEvent                 func(StreamEvent) error
}

// SendMessageResult is the metadata returned to the handler when the stream is done.
type SendMessageResult struct {
	ConversationID   int64
	UserMessage      Message
	AssistantMessage Message
	GeneratedTitle   string // present only when title was just auto-generated
}
