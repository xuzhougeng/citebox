package ai_conversation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

// AISettingsProvider exposes the parts of *service.AIService we depend on.
// It is satisfied by *service.AIService.
type AISettingsProvider interface {
	GetSettings() (*model.AISettings, error)
}

// StreamCaller is the minimal LLM streaming primitive we need.
// Satisfied by service.AIService.CallProviderStreamGeneric (added in Task 1.6).
type StreamCaller interface {
	CallProviderStreamGeneric(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string,
		images []model.AIImageInput, onDelta func(string) error) (string, string, error)
}

// Service is the conversation lifecycle manager.
type Service struct {
	repo     *repository.AIConversationRepository
	papers   *repository.PaperRepository
	settings AISettingsProvider
	caller   StreamCaller
	logger   *slog.Logger
}

// New builds the service. All deps required.
func New(repo *repository.AIConversationRepository, papers *repository.PaperRepository,
	settings AISettingsProvider, caller StreamCaller, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default().With("component", "ai_conversation")
	}
	return &Service{repo: repo, papers: papers, settings: settings, caller: caller, logger: logger}
}

// CreateDraft creates an empty conversation row. Used by handler when client
// posts to /messages with conversation_id=0.
func (s *Service) CreateDraft() (int64, error) {
	return s.repo.CreateConversation()
}

// GetConversation returns the meta + recent messages for the active pane.
func (s *Service) GetConversation(id int64) (Conversation, error) {
	row, err := s.repo.GetConversation(id)
	if err != nil {
		return Conversation{}, mapRepoErr(err)
	}
	pinned, err := s.repo.ListPinnedPapers(id)
	if err != nil {
		return Conversation{}, err
	}
	msgs, err := s.repo.ListMessages(id, 0, 200)
	if err != nil {
		return Conversation{}, err
	}
	return Conversation{
		ID:             row.ID,
		Title:          row.Title,
		TitleLocked:    row.TitleLocked,
		StrictEvidence: row.StrictEvidence,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		PinnedPapers:   toPinnedPapers(pinned),
		RecentMessages: toMessages(msgs),
	}, nil
}

// ListConversations is a passthrough.
func (s *Service) ListConversations(q string, limit, offset int) ([]Conversation, error) {
	rows, err := s.repo.ListConversations(q, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]Conversation, 0, len(rows))
	for _, r := range rows {
		// Sidebar list: include pinned summary but skip messages.
		pinned, _ := s.repo.ListPinnedPapers(r.ID)
		out = append(out, Conversation{
			ID:             r.ID,
			Title:          r.Title,
			TitleLocked:    r.TitleLocked,
			StrictEvidence: r.StrictEvidence,
			CreatedAt:      r.CreatedAt,
			UpdatedAt:      r.UpdatedAt,
			PinnedPapers:   toPinnedPapers(pinned),
		})
	}
	return out, nil
}

// ListMessages returns messages with id > afterID, oldest first, up to limit.
// Used by the GET /messages endpoint for "load older history" pagination.
func (s *Service) ListMessages(id int64, afterID int64, limit int) ([]Message, error) {
	rows, err := s.repo.ListMessages(id, afterID, limit)
	if err != nil {
		return nil, err
	}
	return toMessages(rows), nil
}

// UpdateTitle replaces the title; lock=true marks it user-edited.
func (s *Service) UpdateTitle(id int64, title string, lock bool) error {
	t := strings.TrimSpace(title)
	if t == "" {
		return apperr.New(apperr.CodeInvalidArgument, "标题不能为空")
	}
	if len([]rune(t)) > 50 {
		t = string([]rune(t)[:50])
	}
	if err := s.repo.UpdateTitle(id, t, lock); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// UpdateStrictEvidence flips the per-conversation flag.
func (s *Service) UpdateStrictEvidence(id int64, on bool) error {
	if err := s.repo.UpdateStrictEvidence(id, on); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// DeleteConversation hard-deletes (cascade).
func (s *Service) DeleteConversation(id int64) error {
	if err := s.repo.DeleteConversation(id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// PinPaper attaches a paper subject to the configured limit.
func (s *Service) PinPaper(conversationID, paperID int64) error {
	settings, err := s.settings.GetSettings()
	if err != nil {
		return err
	}
	limit := settings.PinPapersLimit
	if limit <= 0 {
		limit = 5
	}
	count, err := s.repo.CountPinnedPapers(conversationID)
	if err != nil {
		return err
	}
	if count >= limit {
		// idempotent re-pin should not 422
		pinned, _ := s.repo.ListPinnedPapers(conversationID)
		for _, p := range pinned {
			if p.PaperID == paperID {
				return nil
			}
		}
		return apperr.New(apperr.CodeFailedPrecondition,
			fmt.Sprintf("最多可 pin %d 篇文献", limit))
	}
	return s.repo.PinPaper(conversationID, paperID)
}

// UnpinPaper detaches.
func (s *Service) UnpinPaper(conversationID, paperID int64) error {
	return s.repo.UnpinPaper(conversationID, paperID)
}

func mapRepoErr(err error) error {
	if errors.Is(err, repository.ErrAIConversationNotFound) {
		return apperr.New(apperr.CodeNotFound, "会话不存在")
	}
	return err
}

func toPinnedPapers(rows []repository.AIPinnedPaper) []PinnedPaper {
	out := make([]PinnedPaper, 0, len(rows))
	for _, r := range rows {
		out = append(out, PinnedPaper{PaperID: r.PaperID, Title: r.Title, DOI: r.DOI, PinnedAt: r.PinnedAt})
	}
	return out
}

func toMessages(rows []repository.AIMessage) []Message {
	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, Message{
			ID: r.ID, Role: r.Role, Content: r.Content,
			Provider: r.Provider, Model: r.Model, Mode: r.Mode,
			IncludedFigures: r.IncludedFigures, CitationsJSON: r.CitationsJSON,
			CreatedAt: r.CreatedAt,
		})
	}
	return out
}
