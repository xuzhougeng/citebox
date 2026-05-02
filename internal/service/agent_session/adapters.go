package agent_session

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/service/agent_session/commands"
	"github.com/xuzhougeng/citebox/internal/service/ai_conversation"
)

type aiConversationAdapter struct{ s *ai_conversation.Service }

// NewAIConversationAdapter returns a FreeTextHandler that delegates free-text
// turns to the existing ai_conversation.Service via SendForSurface.
func NewAIConversationAdapter(s *ai_conversation.Service) FreeTextHandler {
	return &aiConversationAdapter{s: s}
}

func (a *aiConversationAdapter) SendForSurface(ctx context.Context, in commands.FreeTextInput,
	onPlaceholder func(string) error) (commands.FreeTextResult, error) {
	res, err := a.s.SendForSurface(ctx, ai_conversation.SurfaceMessageInput{
		ConversationID: in.ConversationID,
		Text:           in.Text,
		Surface:        in.Surface,
	}, onPlaceholder)
	if err != nil {
		return commands.FreeTextResult{}, err
	}
	return commands.FreeTextResult{
		UserMessageID:      res.UserMessageID,
		AssistantMessageID: res.AssistantMessageID,
		AnswerText:         res.AnswerText,
		PlaceholderText:    res.PlaceholderText,
	}, nil
}
