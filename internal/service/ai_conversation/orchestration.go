package ai_conversation

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
)

func (s *Service) emitStreamEvent(in SendMessageInput, event StreamEvent) error {
	if in.OnEvent == nil {
		return nil
	}
	if err := in.OnEvent(event); err != nil {
		s.logger.Warn("ai_conversation: stream event callback failed", "type", event.Type, "error", err)
		return err
	}
	return nil
}

func (s *Service) persistRunArtifacts(conversationID, userMsgID, assistantMsgID int64, mode string, out ai_assistant.RunOutput) {
	runID, err := s.repo.CreateTurnRun(repository.AITurnRun{
		ConversationID:     conversationID,
		UserMessageID:      userMsgID,
		AssistantMessageID: sql.NullInt64{Int64: assistantMsgID, Valid: true},
		Intent:             out.Intent,
		IntentHint:         out.IntentHint,
		ProcessSummaryJSON: mustJSON(out.Process),
		Status:             modeOrCompleted(mode),
	})
	if err != nil {
		s.logger.Warn("ai_conversation: persist turn run failed", "error", err)
		return
	}
	persistToolCalls(s.repo, runID, out.ToolCalls, s.logger)
	persistResultCards(s.repo, runID, out.Cards, s.logger)
}

func marshalAssistantCitations(citations []ai_assistant.Citation) string {
	if len(citations) == 0 {
		return ""
	}
	return mustJSON(citations)
}

func mustJSON(v any) string {
	buf, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(buf)
}

func persistToolCalls(repo *repository.AIConversationRepository, runID int64, calls []ai_assistant.ToolCallSummary, logger *slog.Logger) {
	for _, call := range calls {
		if _, err := repo.AddToolCall(repository.AIToolCall{
			TurnRunID:         runID,
			ToolName:          call.ToolName,
			InputJSON:         call.InputJSON,
			OutputSummaryJSON: call.OutputSummaryJSON,
			Status:            toolCallStatusOrCompleted(call.Status),
			DurationMS:        call.DurationMS,
			Error:             call.Error,
		}); err != nil {
			logger.Warn("ai_conversation: persist tool call failed", "tool", call.ToolName, "error", err)
		}
	}
}

func persistResultCards(repo *repository.AIConversationRepository, runID int64, cards []ai_assistant.ResultCard, logger *slog.Logger) {
	for i, card := range cards {
		payloadJSON := mustJSON(card.Payload)
		if payloadJSON == "" {
			payloadJSON = "{}"
		}
		if _, err := repo.AddResultCard(repository.AIResultCard{
			TurnRunID:   runID,
			CardType:    card.Type,
			SortOrder:   i,
			PayloadJSON: payloadJSON,
		}); err != nil {
			logger.Warn("ai_conversation: persist result card failed", "type", card.Type, "error", err)
		}
	}
}

func modeOrCompleted(mode string) string {
	mode = strings.TrimSpace(mode)
	switch mode {
	case "running", "completed", "stopped", "failed":
		return mode
	default:
		return "completed"
	}
}

func toolCallStatusOrCompleted(status string) string {
	status = strings.TrimSpace(status)
	switch status {
	case "running", "completed", "skipped", "failed":
		return status
	default:
		return "completed"
	}
}

func requestContextEmpty(ctx ai_assistant.RequestContext) bool {
	return strings.TrimSpace(ctx.Source) == "" &&
		ctx.PaperID == 0 &&
		ctx.FigureID == 0 &&
		len(ctx.PaperIDs) == 0 &&
		len(ctx.FigureIDs) == 0 &&
		len(ctx.Excerpts) == 0
}

func toResultCards(rows []repository.AIResultCard) []ResultCard {
	out := make([]ResultCard, 0, len(rows))
	for _, r := range rows {
		out = append(out, ResultCard{
			ID:          r.ID,
			TurnRunID:   r.TurnRunID,
			CardType:    r.CardType,
			SortOrder:   r.SortOrder,
			PayloadJSON: r.PayloadJSON,
		})
	}
	return out
}
