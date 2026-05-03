package repository

import (
	"database/sql"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/repository/schema"
	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable fk: %v", err)
	}
	if err := schema.NewManager(db).Initialize(); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedConversationAndTurn(t *testing.T, db *sql.DB) (convID, turnID int64) {
	t.Helper()
	res, err := db.Exec(`INSERT INTO ai_conversations (title) VALUES (?)`, "test")
	if err != nil {
		t.Fatalf("seed conv: %v", err)
	}
	convID, _ = res.LastInsertId()

	msg, err := db.Exec(`INSERT INTO ai_messages (conversation_id, role, content) VALUES (?, 'user', 'hi')`, convID)
	if err != nil {
		t.Fatalf("seed msg: %v", err)
	}
	msgID, _ := msg.LastInsertId()

	tr, err := db.Exec(`INSERT INTO ai_turn_runs (conversation_id, user_message_id) VALUES (?, ?)`, convID, msgID)
	if err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	turnID, _ = tr.LastInsertId()
	return convID, turnID
}

func TestAIGeneratedImageRepository_RoundTrip(t *testing.T) {
	db := newTestDB(t)
	convID, turnID := seedConversationAndTurn(t, db)

	repo := NewAIGeneratedImageRepository(db)
	id, err := repo.Insert(AIGeneratedImage{
		TurnRunID:        turnID,
		ConversationID:   convID,
		FilePath:         "data/ai_generated/1/abc.png",
		Prompt:           "a graphical abstract of CRISPR",
		Model:            "gpt-image-2",
		Size:             "1024x1024",
		Quality:          "high",
		SourcePaperIDs:   []int64{1, 2},
		SourceFigureIDs:  []int64{},
		CostEstimateUSD:  sql.NullFloat64{Float64: 0.19, Valid: true},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected non-zero id, got %d", id)
	}

	got, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.FilePath != "data/ai_generated/1/abc.png" || got.Model != "gpt-image-2" {
		t.Fatalf("unexpected row: %+v", got)
	}
	if len(got.SourcePaperIDs) != 2 || got.SourcePaperIDs[0] != 1 {
		t.Fatalf("source_paper_ids round-trip wrong: %+v", got.SourcePaperIDs)
	}
	if got.CreatedAt.IsZero() || time.Since(got.CreatedAt) > time.Minute {
		t.Fatalf("created_at not populated: %v", got.CreatedAt)
	}
}

func TestAIGeneratedImageRepository_CascadeOnTurnDelete(t *testing.T) {
	db := newTestDB(t)
	convID, turnID := seedConversationAndTurn(t, db)

	repo := NewAIGeneratedImageRepository(db)
	id, err := repo.Insert(AIGeneratedImage{
		TurnRunID: turnID, ConversationID: convID,
		FilePath: "x.png", Prompt: "p", Model: "gpt-image-2",
		Size: "1024x1024", Quality: "high",
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM ai_turn_runs WHERE id = ?`, turnID); err != nil {
		t.Fatalf("delete turn: %v", err)
	}

	if _, err := repo.GetByID(id); err != ErrAIGeneratedImageNotFound {
		t.Fatalf("expected ErrAIGeneratedImageNotFound after cascade, got %v", err)
	}
}
