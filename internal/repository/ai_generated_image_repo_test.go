package repository

import (
	"database/sql"
	"testing"
	"time"
)

// seedConversationAndTurn creates a minimal conversation + turn_run row via
// the AIConversation sub-repository and returns their IDs.
func seedConversationAndTurn(t *testing.T, libRepo *LibraryRepository) (convID, turnID int64) {
	t.Helper()
	convRepo := libRepo.AIConversation

	var err error
	convID, err = convRepo.CreateConversation()
	if err != nil {
		t.Fatalf("seed conv: %v", err)
	}

	msgID, err := convRepo.AddMessage(convID, "user", "hi", AIMessageMeta{})
	if err != nil {
		t.Fatalf("seed msg: %v", err)
	}

	turnID, err = convRepo.CreateTurnRun(AITurnRun{
		ConversationID: convID,
		UserMessageID:  msgID,
		Intent:         "generate_image",
		Status:         "completed",
	})
	if err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	return convID, turnID
}

func TestAIGeneratedImageRepository_RoundTrip(t *testing.T) {
	libRepo := newTestRepository(t)
	convID, turnID := seedConversationAndTurn(t, libRepo)

	repo := libRepo.AIGeneratedImage
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
	libRepo := newTestRepository(t)
	convID, turnID := seedConversationAndTurn(t, libRepo)

	repo := libRepo.AIGeneratedImage
	id, err := repo.Insert(AIGeneratedImage{
		TurnRunID: turnID, ConversationID: convID,
		FilePath: "x.png", Prompt: "p", Model: "gpt-image-2",
		Size: "1024x1024", Quality: "high",
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if _, err := libRepo.DB().Exec(`DELETE FROM ai_turn_runs WHERE id = ?`, turnID); err != nil {
		t.Fatalf("delete turn: %v", err)
	}

	if _, err := repo.GetByID(id); err != ErrAIGeneratedImageNotFound {
		t.Fatalf("expected ErrAIGeneratedImageNotFound after cascade, got %v", err)
	}
}
