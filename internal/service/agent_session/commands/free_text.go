package commands

type FreeTextInput struct {
	ConversationID int64
	Text           string
	Surface        string
	SurfaceCtx     SurfaceContext
}

type FreeTextResult struct {
	UserMessageID      int64
	AssistantMessageID int64
	AnswerText         string
	PlaceholderText    string
}

func FreeTextResultToAgentResponse(convID int64, r FreeTextResult) *AgentResponse {
	out := &AgentResponse{
		ConversationID:     convID,
		UserMessageID:      r.UserMessageID,
		AssistantMessageID: r.AssistantMessageID,
		UsedShortcut:       false,
	}
	if r.PlaceholderText != "" {
		out.Chunks = append(out.Chunks, OutboundChunk{Kind: "text", Text: r.PlaceholderText, IsPlaceholder: true})
	}
	if r.AnswerText != "" {
		out.Chunks = append(out.Chunks, OutboundChunk{Kind: "text", Text: r.AnswerText})
	}
	return out
}
