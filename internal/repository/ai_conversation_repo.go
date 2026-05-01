package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrAIConversationNotFound is returned when a conversation id has no row.
var ErrAIConversationNotFound = errors.New("ai_conversation: not found")

// ErrAIConversationNoUserMessage is returned when a conversation has no user
// turn that can be edited or resent.
var ErrAIConversationNoUserMessage = errors.New("ai_conversation: no user message")

// AIConversation is one persisted chat session.
type AIConversation struct {
	ID                      int64
	Title                   string
	TitleLocked             bool
	StrictEvidence          bool
	SummaryText             string
	SummaryThroughMessageID sql.NullInt64
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// AIMessage is one persisted message (user or assistant).
type AIMessage struct {
	ID              int64
	ConversationID  int64
	Role            string // "user" | "assistant"
	Content         string
	Provider        string
	Model           string
	Mode            string
	IncludedFigures int
	CitationsJSON   string
	CreatedAt       time.Time
}

// AIMessageMeta carries assistant-specific fields when persisting a message.
type AIMessageMeta struct {
	Provider        string
	Model           string
	Mode            string
	IncludedFigures int
	CitationsJSON   string
}

// AIPinnedPaper joins ai_conversation_papers and papers for sidebar / pin chips.
type AIPinnedPaper struct {
	PaperID  int64
	Title    string
	DOI      string
	PinnedAt time.Time
}

type AITurnRun struct {
	ID                 int64
	ConversationID     int64
	UserMessageID      int64
	AssistantMessageID sql.NullInt64
	Intent             string
	IntentHint         string
	ProcessSummaryJSON string
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type AIToolCall struct {
	ID                int64
	TurnRunID         int64
	ToolName          string
	InputJSON         string
	OutputSummaryJSON string
	Status            string
	DurationMS        int
	Error             string
	CreatedAt         time.Time
}

type AIResultCard struct {
	ID          int64
	TurnRunID   int64
	CardType    string
	SortOrder   int
	PayloadJSON string
	CreatedAt   time.Time
}

// AIConversationRepository owns all three new tables.
type AIConversationRepository struct {
	db *sql.DB
}

// NewAIConversationRepository wires the repo around an open db handle.
func NewAIConversationRepository(db *sql.DB) *AIConversationRepository {
	return &AIConversationRepository{db: db}
}

// CreateConversation inserts a blank row and returns its id.
func (r *AIConversationRepository) CreateConversation() (int64, error) {
	res, err := r.db.Exec(`INSERT INTO ai_conversations DEFAULT VALUES`)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetConversation returns a single conversation row.
func (r *AIConversationRepository) GetConversation(id int64) (AIConversation, error) {
	row := r.db.QueryRow(`
		SELECT id, title, title_locked, strict_evidence,
		       summary_text, summary_through_message_id,
		       created_at, updated_at
		FROM ai_conversations WHERE id = ?
	`, id)
	var c AIConversation
	var titleLocked, strict int
	if err := row.Scan(&c.ID, &c.Title, &titleLocked, &strict,
		&c.SummaryText, &c.SummaryThroughMessageID,
		&c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AIConversation{}, ErrAIConversationNotFound
		}
		return AIConversation{}, err
	}
	c.TitleLocked = titleLocked != 0
	c.StrictEvidence = strict != 0
	return c, nil
}

// ListConversations returns conversations matching the optional query string,
// sorted by updated_at DESC, with offset / limit pagination.
func (r *AIConversationRepository) ListConversations(q string, limit, offset int) ([]AIConversation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q = strings.TrimSpace(q)

	var rows *sql.Rows
	var err error
	if q == "" {
		rows, err = r.db.Query(`
			SELECT id, title, title_locked, strict_evidence,
			       summary_text, summary_through_message_id,
			       created_at, updated_at
			FROM ai_conversations
			ORDER BY updated_at DESC, id DESC
			LIMIT ? OFFSET ?
		`, limit, offset)
	} else {
		// Match title OR any message body OR any pinned paper title.
		like := "%" + strings.ToLower(q) + "%"
		rows, err = r.db.Query(`
			SELECT DISTINCT c.id, c.title, c.title_locked, c.strict_evidence,
			                c.summary_text, c.summary_through_message_id,
			                c.created_at, c.updated_at
			FROM ai_conversations c
			LEFT JOIN ai_messages m ON m.conversation_id = c.id
			LEFT JOIN ai_conversation_papers cp ON cp.conversation_id = c.id
			LEFT JOIN papers p ON p.id = cp.paper_id
			WHERE LOWER(c.title) LIKE ?
			   OR LOWER(m.content) LIKE ?
			   OR LOWER(p.title) LIKE ?
			ORDER BY c.updated_at DESC, c.id DESC
			LIMIT ? OFFSET ?
		`, like, like, like, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]AIConversation, 0)
	for rows.Next() {
		var c AIConversation
		var titleLocked, strict int
		if err := rows.Scan(&c.ID, &c.Title, &titleLocked, &strict,
			&c.SummaryText, &c.SummaryThroughMessageID,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.TitleLocked = titleLocked != 0
		c.StrictEvidence = strict != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// TouchConversation bumps updated_at to now (millisecond precision so ordering
// within the same second is stable).
func (r *AIConversationRepository) TouchConversation(id int64) error {
	_, err := r.db.Exec(`UPDATE ai_conversations SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now') WHERE id = ?`, id)
	return err
}

// UpdateTitle sets title (and optionally locks it so the auto-titler skips).
func (r *AIConversationRepository) UpdateTitle(id int64, title string, lock bool) error {
	lockVal := 0
	if lock {
		lockVal = 1
	}
	_, err := r.db.Exec(
		`UPDATE ai_conversations SET title = ?, title_locked = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		title, lockVal, id)
	return err
}

// UpdateStrictEvidence flips the strict_evidence boolean.
func (r *AIConversationRepository) UpdateStrictEvidence(id int64, on bool) error {
	v := 0
	if on {
		v = 1
	}
	_, err := r.db.Exec(
		`UPDATE ai_conversations SET strict_evidence = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		v, id)
	return err
}

// UpdateSummary writes the new summary text and the message id it includes.
func (r *AIConversationRepository) UpdateSummary(id int64, summary string, throughMessageID int64) error {
	_, err := r.db.Exec(
		`UPDATE ai_conversations
		 SET summary_text = ?, summary_through_message_id = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		summary, throughMessageID, id)
	return err
}

// DeleteConversation hard-deletes the conversation; cascade removes messages and pins.
func (r *AIConversationRepository) DeleteConversation(id int64) error {
	_, err := r.db.Exec(`DELETE FROM ai_conversations WHERE id = ?`, id)
	return err
}

// TruncateLastTurn removes the most recent user message and every message after
// it in the same conversation. Turn-run artifacts are removed through FK
// cascades from ai_turn_runs.user_message_id.
func (r *AIConversationRepository) TruncateLastTurn(conversationID int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	var userMessageID int64
	err = tx.QueryRow(`
		SELECT id
		FROM ai_messages
		WHERE conversation_id = ? AND role = 'user'
		ORDER BY id DESC
		LIMIT 1
	`, conversationID).Scan(&userMessageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAIConversationNoUserMessage
		}
		return err
	}

	if _, err = tx.Exec(`
		DELETE FROM ai_messages
		WHERE conversation_id = ? AND id >= ?
	`, conversationID, userMessageID); err != nil {
		return err
	}
	if _, err = tx.Exec(`
		UPDATE ai_conversations
		SET
			summary_text = CASE
				WHEN summary_through_message_id IS NOT NULL AND summary_through_message_id >= ? THEN ''
				ELSE summary_text
			END,
			summary_through_message_id = CASE
				WHEN summary_through_message_id IS NOT NULL AND summary_through_message_id >= ? THEN NULL
				ELSE summary_through_message_id
			END,
			updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now')
		WHERE id = ?
	`, userMessageID, userMessageID, conversationID); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

// AddMessage inserts a message; returns its id. Implemented in Task 1.3.
func (r *AIConversationRepository) AddMessage(conversationID int64, role, content string, meta AIMessageMeta) (int64, error) {
	res, err := r.db.Exec(`
		INSERT INTO ai_messages (conversation_id, role, content, provider, model, mode, included_figures, citations_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, conversationID, role, content, nullableString(meta.Provider), nullableString(meta.Model),
		nullableString(meta.Mode), meta.IncludedFigures, nullableString(meta.CitationsJSON))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListMessages returns messages with id > afterID, oldest first, up to limit.
func (r *AIConversationRepository) ListMessages(conversationID int64, afterID int64, limit int) ([]AIMessage, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := r.db.Query(`
		SELECT id, conversation_id, role, content,
		       COALESCE(provider, ''), COALESCE(model, ''), COALESCE(mode, ''),
		       COALESCE(included_figures, 0), COALESCE(citations_json, ''), created_at
		FROM ai_messages
		WHERE conversation_id = ? AND id > ?
		ORDER BY id ASC
		LIMIT ?
	`, conversationID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AIMessage, 0)
	for rows.Next() {
		var m AIMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
			&m.Provider, &m.Model, &m.Mode, &m.IncludedFigures, &m.CitationsJSON, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *AIConversationRepository) CreateTurnRun(run AITurnRun) (int64, error) {
	status := strings.TrimSpace(run.Status)
	if status == "" {
		status = "completed"
	}
	if err := r.ensureMessageInConversation(run.UserMessageID, run.ConversationID); err != nil {
		return 0, fmt.Errorf("user_message_id: %w", err)
	}
	if run.AssistantMessageID.Valid {
		if err := r.ensureMessageInConversation(run.AssistantMessageID.Int64, run.ConversationID); err != nil {
			return 0, fmt.Errorf("assistant_message_id: %w", err)
		}
	}
	res, err := r.db.Exec(`
		INSERT INTO ai_turn_runs (
			conversation_id, user_message_id, assistant_message_id,
			intent, intent_hint, process_summary_json, status
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, run.ConversationID, run.UserMessageID, nullableInt64(run.AssistantMessageID),
		run.Intent, run.IntentHint, run.ProcessSummaryJSON, status)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *AIConversationRepository) UpdateTurnRunAssistant(runID, assistantMessageID int64, status string) error {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "completed"
	}
	var conversationID int64
	if err := r.db.QueryRow(`SELECT conversation_id FROM ai_turn_runs WHERE id = ?`, runID).Scan(&conversationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("turn run not found")
		}
		return err
	}
	if err := r.ensureMessageInConversation(assistantMessageID, conversationID); err != nil {
		return fmt.Errorf("assistant_message_id: %w", err)
	}
	_, err := r.db.Exec(`
		UPDATE ai_turn_runs
		SET assistant_message_id = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, assistantMessageID, status, runID)
	return err
}

func (r *AIConversationRepository) AddToolCall(call AIToolCall) (int64, error) {
	if call.DurationMS < 0 {
		return 0, fmt.Errorf("duration_ms must be non-negative")
	}
	status := strings.TrimSpace(call.Status)
	if status == "" {
		status = "completed"
	}
	res, err := r.db.Exec(`
		INSERT INTO ai_tool_calls (
			turn_run_id, tool_name, input_json, output_summary_json,
			status, duration_ms, error
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, call.TurnRunID, call.ToolName, call.InputJSON, call.OutputSummaryJSON,
		status, call.DurationMS, call.Error)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *AIConversationRepository) AddResultCard(card AIResultCard) (int64, error) {
	payload := card.PayloadJSON
	if payload == "" {
		payload = "{}"
	}
	if !json.Valid([]byte(payload)) {
		return 0, fmt.Errorf("payload_json must be valid JSON")
	}
	res, err := r.db.Exec(`
		INSERT INTO ai_result_cards (turn_run_id, card_type, sort_order, payload_json)
		VALUES (?, ?, ?, ?)
	`, card.TurnRunID, card.CardType, card.SortOrder, payload)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *AIConversationRepository) ListTurnRuns(conversationID int64) ([]AITurnRun, error) {
	rows, err := r.db.Query(`
		SELECT id, conversation_id, user_message_id, assistant_message_id,
		       intent, intent_hint, process_summary_json, status, created_at, updated_at
		FROM ai_turn_runs
		WHERE conversation_id = ?
		ORDER BY id ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AITurnRun, 0)
	for rows.Next() {
		var run AITurnRun
		if err := rows.Scan(&run.ID, &run.ConversationID, &run.UserMessageID,
			&run.AssistantMessageID, &run.Intent, &run.IntentHint,
			&run.ProcessSummaryJSON, &run.Status, &run.CreatedAt, &run.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *AIConversationRepository) ListToolCalls(turnRunID int64) ([]AIToolCall, error) {
	rows, err := r.db.Query(`
		SELECT id, turn_run_id, tool_name, input_json, output_summary_json,
		       status, duration_ms, error, created_at
		FROM ai_tool_calls
		WHERE turn_run_id = ?
		ORDER BY id ASC
	`, turnRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AIToolCall, 0)
	for rows.Next() {
		var call AIToolCall
		if err := rows.Scan(&call.ID, &call.TurnRunID, &call.ToolName, &call.InputJSON,
			&call.OutputSummaryJSON, &call.Status, &call.DurationMS, &call.Error, &call.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, call)
	}
	return out, rows.Err()
}

func (r *AIConversationRepository) ListResultCards(turnRunID int64) ([]AIResultCard, error) {
	rows, err := r.db.Query(`
		SELECT id, turn_run_id, card_type, sort_order, payload_json, created_at
		FROM ai_result_cards
		WHERE turn_run_id = ?
		ORDER BY sort_order ASC, id ASC
	`, turnRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AIResultCard, 0)
	for rows.Next() {
		var card AIResultCard
		if err := rows.Scan(&card.ID, &card.TurnRunID, &card.CardType, &card.SortOrder, &card.PayloadJSON, &card.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, card)
	}
	return out, rows.Err()
}

func (r *AIConversationRepository) ensureMessageInConversation(messageID, conversationID int64) error {
	row := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM ai_messages
		WHERE id = ? AND conversation_id = ?
	`, messageID, conversationID)
	var count int
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("message does not belong to conversation")
	}
	return nil
}

// PinPaper attaches a paper to a conversation. Idempotent (PRIMARY KEY).
func (r *AIConversationRepository) PinPaper(conversationID, paperID int64) error {
	_, err := r.db.Exec(`
		INSERT INTO ai_conversation_papers (conversation_id, paper_id)
		VALUES (?, ?)
		ON CONFLICT(conversation_id, paper_id) DO NOTHING
	`, conversationID, paperID)
	if err == nil {
		_ = r.TouchConversation(conversationID)
	}
	return err
}

// UnpinPaper removes the pin. Not-found is silent.
func (r *AIConversationRepository) UnpinPaper(conversationID, paperID int64) error {
	_, err := r.db.Exec(
		`DELETE FROM ai_conversation_papers WHERE conversation_id = ? AND paper_id = ?`,
		conversationID, paperID)
	if err == nil {
		_ = r.TouchConversation(conversationID)
	}
	return err
}

// CountPinnedPapers returns the current pin count (for limit checks).
func (r *AIConversationRepository) CountPinnedPapers(conversationID int64) (int, error) {
	row := r.db.QueryRow(`SELECT COUNT(*) FROM ai_conversation_papers WHERE conversation_id = ?`, conversationID)
	var n int
	err := row.Scan(&n)
	return n, err
}

// ListPinnedPapers returns pinned papers (with paper.title / doi) ordered by pin time.
func (r *AIConversationRepository) ListPinnedPapers(conversationID int64) ([]AIPinnedPaper, error) {
	rows, err := r.db.Query(`
		SELECT p.id, p.title, COALESCE(p.doi, ''), cp.pinned_at
		FROM ai_conversation_papers cp
		JOIN papers p ON p.id = cp.paper_id
		WHERE cp.conversation_id = ?
		ORDER BY cp.pinned_at ASC, p.id ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AIPinnedPaper, 0)
	for rows.Next() {
		var p AIPinnedPaper
		if err := rows.Scan(&p.PaperID, &p.Title, &p.DOI, &p.PinnedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt64(v sql.NullInt64) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Int64
}
