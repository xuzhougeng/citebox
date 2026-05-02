package commands

import (
	"context"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/repository"
)

type SurfaceContext struct {
	CurrentPaperID        int64
	CurrentPaperTitle     string
	CurrentFigureID       int64
	RecentSearchPaperIDs  []int64
	RecentSearchFigureIDs []int64
}

type RuntimeCtx struct {
	ConversationID int64
	Surface        string
	UserID         string
	SurfaceCtx     SurfaceContext
	State          SurfaceStateMutator
	Repo           *repository.AIConversationRepository
}

type SurfaceStateMutator interface {
	Reset(surface string) error
	SetCurrentPaper(surface string, paperID int64, title string) error
	SetCurrentFigure(surface string, figureID int64) error
	SetSearchResults(surface string, paperIDs, figureIDs []int64) error
	Get(surface string) (SurfaceContext, error)
}

type Result struct {
	ChunksText     []string
	ImagePath      string
	UsedShortcut   bool
	UserMessageID  int64
	AssistantMsgID int64
}

type Command interface {
	Name() string
	Execute(ctx context.Context, arg string, rt RuntimeCtx) (*Result, error)
}

type Registry struct {
	by map[string]Command
}

func NewRegistry(cmds ...Command) *Registry {
	r := &Registry{by: map[string]Command{}}
	for _, c := range cmds {
		r.by[strings.ToLower(c.Name())] = c
	}
	return r
}

func (r *Registry) Dispatch(ctx context.Context, name, arg string, rt RuntimeCtx) (*AgentResponse, error) {
	c, ok := r.by[strings.ToLower(name)]
	if !ok {
		return nil, apperr.New(apperr.CodeInvalidArgument, "unknown command: "+name)
	}
	res, err := c.Execute(ctx, arg, rt)
	if err != nil {
		return nil, err
	}
	return resultToAgentResponse(rt.ConversationID, res), nil
}
