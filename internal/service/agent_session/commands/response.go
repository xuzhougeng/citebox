package commands

type AgentResponse struct {
	ConversationID     int64
	UserMessageID      int64
	AssistantMessageID int64
	Chunks             []OutboundChunk
	UsedShortcut       bool
}

type OutboundChunk struct {
	Kind          string // "text" | "image"
	Text          string
	ImagePath     string
	IsPlaceholder bool
}

func resultToAgentResponse(convID int64, r *Result) *AgentResponse {
	out := &AgentResponse{
		ConversationID:     convID,
		UserMessageID:      r.UserMessageID,
		AssistantMessageID: r.AssistantMsgID,
		UsedShortcut:       r.UsedShortcut,
	}
	for _, t := range r.ChunksText {
		out.Chunks = append(out.Chunks, OutboundChunk{Kind: "text", Text: t})
	}
	if r.ImagePath != "" {
		out.Chunks = append(out.Chunks, OutboundChunk{Kind: "image", ImagePath: r.ImagePath})
	}
	return out
}
