package commands

import (
	"context"
	"database/sql"
)

// ClearCommand drops all conversation history visible to the next free-text turn
// by advancing the conversation's clear barrier past the most recent message.
type ClearCommand struct{}

func (*ClearCommand) Name() string { return "/clear" }

func (*ClearCommand) Execute(_ context.Context, _ string, rt RuntimeCtx) (*Result, error) {
	var maxID sql.NullInt64
	if err := rt.Repo.DB().QueryRow(
		`SELECT MAX(id) FROM ai_messages WHERE conversation_id = ?`, rt.ConversationID,
	).Scan(&maxID); err != nil {
		return nil, err
	}
	barrier := int64(0)
	if maxID.Valid {
		barrier = maxID.Int64
	}
	if err := rt.Repo.SetClearBarrier(rt.ConversationID, barrier); err != nil {
		return nil, err
	}
	return &Result{
		UsedShortcut: true,
		ChunksText:   []string{"已清空对话上下文。后续提问将从这里重新开始。"},
	}, nil
}
