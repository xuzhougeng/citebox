package agent_session

import (
	"context"
	"log/slog"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/agent_session/commands"
)

// FreeTextHandler is the subset of ai_conversation.Service the agent_session
// needs for free-text turns. Defined narrowly so tests can fake it.
type FreeTextHandler interface {
	SendForSurface(ctx context.Context, in commands.FreeTextInput,
		onPlaceholder func(string) error) (commands.FreeTextResult, error)
}

type Service struct {
	repo     *repository.AIConversationRepository
	cmds     *commands.Registry
	freeText FreeTextHandler
	state    commands.SurfaceStateMutator
	logger   *slog.Logger
}

func New(repo *repository.AIConversationRepository, cmds *commands.Registry,
	freeText FreeTextHandler, state commands.SurfaceStateMutator, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default().With("component", "agent_session")
	}
	return &Service{repo: repo, cmds: cmds, freeText: freeText, state: state, logger: logger}
}

// Handle is the single entry point for both surfaces.
func (s *Service) Handle(ctx context.Context, req AgentRequest) (*AgentResponse, error) {
	convID, err := s.resolveConversation(req.Conversation, req.Surface)
	if err != nil {
		return nil, err
	}

	if looksLikeDOI(req.Input.Text) {
		cmdResp, err := s.cmds.Dispatch(ctx, ":doi", req.Input.Text, commands.RuntimeCtx{
			ConversationID: convID,
			Surface:        string(req.Surface),
			UserID:         req.UserID,
			SurfaceCtx:     toCommandsSurfaceContext(req.SurfaceContext),
			State:          s.state,
			Repo:           s.repo,
		})
		if err != nil {
			return nil, err
		}
		return wrapCmdResp(cmdResp), nil
	}

	if cmd, arg, ok := parseSlash(req.Input.Text); ok {
		cmdResp, err := s.cmds.Dispatch(ctx, cmd, arg, commands.RuntimeCtx{
			ConversationID: convID,
			Surface:        string(req.Surface),
			UserID:         req.UserID,
			SurfaceCtx:     toCommandsSurfaceContext(req.SurfaceContext),
			State:          s.state,
			Repo:           s.repo,
		})
		if err != nil {
			return nil, err
		}
		return wrapCmdResp(cmdResp), nil
	}

	res, err := s.freeText.SendForSurface(ctx, commands.FreeTextInput{
		ConversationID: convID,
		Text:           req.Input.Text,
		Surface:        string(req.Surface),
		SurfaceCtx:     toCommandsSurfaceContext(req.SurfaceContext),
	}, nil)
	if err != nil {
		return nil, err
	}
	return wrapCmdResp(commands.FreeTextResultToAgentResponse(convID, res)), nil
}

func (s *Service) resolveConversation(ref ConversationRef, surface Surface) (int64, error) {
	switch ref.Kind {
	case KindMainWeChat:
		return s.repo.FindOrCreateByKind("main_wechat", "wechat")
	case KindDefaultWeb:
		return s.repo.FindOrCreateByKind("default_web", "web")
	case KindAdHoc:
		if ref.ID <= 0 {
			return 0, apperr.New(apperr.CodeInvalidArgument, "ad_hoc conversation requires id")
		}
		return ref.ID, nil
	default:
		return 0, apperr.New(apperr.CodeInvalidArgument, "unknown conversation kind")
	}
}

func toCommandsSurfaceContext(ctx SurfaceContext) commands.SurfaceContext {
	return commands.SurfaceContext{
		CurrentPaperID:        ctx.CurrentPaperID,
		CurrentPaperTitle:     ctx.CurrentPaperTitle,
		CurrentFigureID:       ctx.CurrentFigureID,
		RecentSearchPaperIDs:  ctx.RecentSearchPaperIDs,
		RecentSearchFigureIDs: ctx.RecentSearchFigureIDs,
	}
}

func wrapCmdResp(r *commands.AgentResponse) *AgentResponse {
	if r == nil {
		return nil
	}
	out := &AgentResponse{
		ConversationID:     r.ConversationID,
		UserMessageID:      r.UserMessageID,
		AssistantMessageID: r.AssistantMessageID,
		UsedShortcut:       r.UsedShortcut,
	}
	for _, c := range r.Chunks {
		out.Chunks = append(out.Chunks, OutboundChunk{
			Kind:          ChunkKind(c.Kind),
			Text:          c.Text,
			ImagePath:     c.ImagePath,
			IsPlaceholder: c.IsPlaceholder,
		})
	}
	return out
}
