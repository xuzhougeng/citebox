package ai_conversation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
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

type orchestrator interface {
	Run(ctx context.Context, in ai_assistant.RunInput) (ai_assistant.RunOutput, error)
}

// Service is the conversation lifecycle manager.
type Service struct {
	repo          *repository.AIConversationRepository
	papers        *repository.PaperRepository
	settings      AISettingsProvider
	caller        StreamCaller
	searcher      ExternalEvidenceSearcher
	orchestrator  orchestrator
	titleCaller   NonStreamCaller
	summaryCaller NonStreamCaller
	logger        *slog.Logger
}

// New builds the service. All deps required.
func New(repo *repository.AIConversationRepository, papers *repository.PaperRepository,
	settings AISettingsProvider, caller StreamCaller, searcher ExternalEvidenceSearcher,
	logger *slog.Logger, orchestrator orchestrator) *Service {
	if logger == nil {
		logger = slog.Default().With("component", "ai_conversation")
	}
	s := &Service{repo: repo, papers: papers, settings: settings, caller: caller, searcher: searcher, orchestrator: orchestrator, logger: logger}
	if tc, ok := caller.(NonStreamCaller); ok {
		s.titleCaller = tc
		s.summaryCaller = tc
	}
	return s
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
	turnRuns, err := s.repo.ListTurnRuns(id)
	if err != nil {
		return Conversation{}, err
	}
	runs := make([]TurnRun, 0, len(turnRuns))
	for _, run := range turnRuns {
		cards, err := s.repo.ListResultCards(run.ID)
		if err != nil {
			return Conversation{}, err
		}
		assistantMessageID := int64(0)
		if run.AssistantMessageID.Valid {
			assistantMessageID = run.AssistantMessageID.Int64
		}
		runs = append(runs, TurnRun{
			ID:                 run.ID,
			UserMessageID:      run.UserMessageID,
			AssistantMessageID: assistantMessageID,
			Intent:             run.Intent,
			IntentHint:         run.IntentHint,
			ProcessSummaryJSON: run.ProcessSummaryJSON,
			Status:             run.Status,
			Cards:              toResultCards(cards),
		})
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
		TurnRuns:       runs,
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

// TruncateLastTurn removes the latest user turn so a modified question can be
// sent again without the stale answer staying in context.
func (s *Service) TruncateLastTurn(id int64) error {
	if id <= 0 {
		return apperr.New(apperr.CodeInvalidArgument, "conversation_id 无效")
	}
	if _, err := s.repo.GetConversation(id); err != nil {
		return mapRepoErr(err)
	}
	if err := s.repo.TruncateLastTurn(id); err != nil {
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

// SendMessage runs one turn: optional auto-pin → INSERT user message → assemble
// prompt → stream LLM → INSERT assistant message → bump updated_at. The onDelta
// callback receives streamed chunks; the caller is responsible for forwarding
// them to the HTTP client. On context cancellation any partial text already
// streamed is persisted as an assistant message with mode="stopped".
func (s *Service) SendMessage(ctx context.Context, in SendMessageInput, onDelta func(string) error) (SendMessageResult, error) {
	if in.ConversationID <= 0 {
		return SendMessageResult{}, apperr.New(apperr.CodeInvalidArgument, "conversation_id 无效")
	}
	if strings.TrimSpace(in.Content) == "" {
		return SendMessageResult{}, apperr.New(apperr.CodeInvalidArgument, "消息内容为空")
	}

	conv, err := s.repo.GetConversation(in.ConversationID)
	if err != nil {
		return SendMessageResult{}, mapRepoErr(err)
	}

	// Auto-pin (β/γ flow). Pin-limit may reject here.
	pinIDs := in.PaperIDs
	if len(pinIDs) == 0 && in.PaperID > 0 {
		pinIDs = []int64{in.PaperID}
	}
	for _, pid := range pinIDs {
		if pid <= 0 {
			continue
		}
		if err := s.PinPaper(in.ConversationID, pid); err != nil {
			return SendMessageResult{}, err
		}
	}

	// Persist user message immediately so it survives provider failures.
	userMsgID, err := s.repo.AddMessage(in.ConversationID, "user", in.Content, repository.AIMessageMeta{})
	if err != nil {
		return SendMessageResult{}, err
	}

	settings, err := s.settings.GetSettings()
	if err != nil {
		return SendMessageResult{}, err
	}
	masterSettings := assistantMasterSettings(*settings)
	subagentSettings := assistantSubagentSettings(*settings)
	pinned, err := s.repo.ListPinnedPapers(in.ConversationID)
	if err != nil {
		return SendMessageResult{}, err
	}
	history, err := s.repo.ListMessages(in.ConversationID, conv.SummaryThroughMessageID.Int64, 1000)
	if err != nil {
		return SendMessageResult{}, err
	}
	// Drop the just-inserted user msg from history (we're about to append it explicitly).
	if len(history) > 0 {
		history = history[:len(history)-1]
	}

	s.maybeSummarize(ctx, &conv, &history, subagentSettings)

	// Re-read StrictEvidence from DB so a PATCH between turns is honoured
	// immediately. We only refresh that flag to avoid clobbering the summary
	// state that maybeSummarize just wrote into conv.
	if fresh, freshErr := s.repo.GetConversation(in.ConversationID); freshErr == nil {
		conv.StrictEvidence = fresh.StrictEvidence
	}

	asm, err := s.assembleForTurn(conv, pinned, history, in.Content, masterSettings)
	if err != nil {
		return SendMessageResult{}, err
	}

	var citations []Citation
	var citationsJSON string
	var runOut ai_assistant.RunOutput
	var runUsed bool
	legacyEvidenceRequested := conv.StrictEvidence || in.IncludeExternalEvidence
	explicitAssistantRequest := strings.TrimSpace(in.IntentHint) != "" ||
		!requestContextEmpty(in.Context) ||
		len(in.Sources) > 0
	if s.orchestrator != nil && (!legacyEvidenceRequested || explicitAssistantRequest) {
		out, orchErr := s.orchestrator.Run(ctx, ai_assistant.RunInput{
			Content:    in.Content,
			IntentHint: in.IntentHint,
			Sources:    in.Sources,
			Context:    in.Context,
		})
		if orchErr != nil {
			s.logger.Warn("ai_conversation: orchestrator failed", "error", orchErr)
		} else {
			runOut = out
			runUsed = true
			if strings.TrimSpace(out.AnswerContext) != "" {
				suffix := "用户问题：\n" + in.Content
				if strings.HasSuffix(asm.userPrompt, suffix) {
					asm.userPrompt = strings.TrimSuffix(asm.userPrompt, suffix) + out.AnswerContext
				} else {
					asm.userPrompt += "\n\n" + out.AnswerContext
				}
			}
			citationsJSON = marshalAssistantCitations(out.Citations)
			streamProcess := ai_assistant.WithAnswerGenerationStage(out.Process, "running")
			if err := s.emitStreamEvent(in, StreamEvent{Type: "process", Data: streamProcess}); err != nil {
				return SendMessageResult{}, err
			}
			if err := s.emitStreamEvent(in, StreamEvent{Type: "cards", Data: out.Cards}); err != nil {
				return SendMessageResult{}, err
			}
			if err := s.emitStreamEvent(in, StreamEvent{Type: "citations", Data: out.Citations}); err != nil {
				return SendMessageResult{}, err
			}
		}
	}
	if !runUsed && legacyEvidenceRequested {
		enrichedUser, cites, evErr := injectEvidence(ctx, s.papers, s.searcher, in.Content, pinned, EvidenceOptions{
			IncludeExternal: in.IncludeExternalEvidence,
			DisableLocal:    !conv.StrictEvidence,
		})
		if evErr != nil {
			s.logger.Warn("ai_conversation: evidence search failed", "error", evErr)
			// Surface a single warning line through the stream so the UI can toast.
			_ = onDelta("\n\n_(证据检索失败，本次按普通模式作答)_\n\n")
		} else {
			// Replace the trailing "用户问题：\n<userText>" portion of asm.userPrompt
			// with the enriched evidence block (which already ends with the same).
			suffix := "用户问题：\n" + in.Content
			if strings.HasSuffix(asm.userPrompt, suffix) {
				asm.userPrompt = strings.TrimSuffix(asm.userPrompt, suffix) + enrichedUser
			} else {
				// Defensive: if the suffix shape changed, just append the evidence.
				asm.userPrompt = asm.userPrompt + "\n\n" + enrichedUser
			}
			citations = cites
			citationsJSON = MarshalCitations(citations)
		}
	}

	rawText, mode, err := s.caller.CallProviderStreamGeneric(ctx, masterSettings, asm.systemPrompt, asm.userPrompt, asm.images, onDelta)
	if err != nil {
		// User-cancelled stream: persist whatever was already streamed with mode="stopped".
		if errors.Is(err, context.Canceled) && rawText != "" {
			if runUsed {
				runOut.Process = ai_assistant.WithAnswerGenerationStage(runOut.Process, "stopped")
			}
			asstID, persistErr := s.repo.AddMessage(in.ConversationID, "assistant", rawText, repository.AIMessageMeta{
				Provider:      string(masterSettings.Provider),
				Model:         masterSettings.Model,
				Mode:          "stopped",
				CitationsJSON: citationsJSON,
			})
			if persistErr != nil {
				s.logger.Warn("ai_conversation: persist stopped message failed", "error", persistErr)
			} else if runUsed {
				s.persistRunArtifacts(in.ConversationID, userMsgID, asstID, "stopped", runOut)
			}
			_ = s.repo.TouchConversation(in.ConversationID)
		}
		if !errors.Is(err, context.Canceled) && runUsed {
			if fallbackText := fallbackToolAnswer(in.Content, runOut, err); fallbackText != "" {
				runOut.Process = ai_assistant.WithAnswerGenerationStage(runOut.Process, "failed")
				if eventErr := s.emitStreamEvent(in, StreamEvent{Type: "process", Data: runOut.Process}); eventErr != nil {
					return SendMessageResult{}, eventErr
				}
				if deltaErr := onDelta(fallbackText); deltaErr != nil {
					return SendMessageResult{}, deltaErr
				}
				asstID, persistErr := s.repo.AddMessage(in.ConversationID, "assistant", fallbackText, repository.AIMessageMeta{
					Provider:      string(masterSettings.Provider),
					Model:         masterSettings.Model,
					Mode:          "tool_fallback",
					CitationsJSON: citationsJSON,
				})
				if persistErr != nil {
					return SendMessageResult{}, persistErr
				}
				_ = s.repo.TouchConversation(in.ConversationID)
				s.persistRunArtifacts(in.ConversationID, userMsgID, asstID, "completed", runOut)
				return SendMessageResult{
					ConversationID:   in.ConversationID,
					UserMessage:      Message{ID: userMsgID, Role: "user", Content: in.Content},
					AssistantMessage: Message{ID: asstID, Role: "assistant", Content: fallbackText, Provider: string(masterSettings.Provider), Model: masterSettings.Model, Mode: "tool_fallback"},
				}, nil
			}
		}
		return SendMessageResult{}, err
	}

	if runUsed {
		runOut.Process = ai_assistant.WithAnswerGenerationStage(runOut.Process, "completed")
		if err := s.emitStreamEvent(in, StreamEvent{Type: "process", Data: runOut.Process}); err != nil {
			return SendMessageResult{}, err
		}
	}

	asstID, err := s.repo.AddMessage(in.ConversationID, "assistant", rawText, repository.AIMessageMeta{
		Provider:      string(masterSettings.Provider),
		Model:         masterSettings.Model,
		Mode:          mode,
		CitationsJSON: citationsJSON,
	})
	if err != nil {
		return SendMessageResult{}, err
	}
	_ = s.repo.TouchConversation(in.ConversationID)
	if runUsed {
		s.persistRunArtifacts(in.ConversationID, userMsgID, asstID, mode, runOut)
	}

	if conv.Title == "" && !conv.TitleLocked && s.titleCaller != nil {
		go func(convID int64, settings model.AISettings, userText, asstText string) {
			bg, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			title := generateTitle(bg, s.titleCaller, settings, userText, asstText)
			if title == "" {
				return
			}
			if err := s.repo.UpdateTitle(convID, title, false); err != nil {
				s.logger.Warn("ai_conversation: title persist failed", "error", err)
			}
		}(in.ConversationID, subagentSettings, in.Content, rawText)
	}

	res := SendMessageResult{
		ConversationID:   in.ConversationID,
		UserMessage:      Message{ID: userMsgID, Role: "user", Content: in.Content},
		AssistantMessage: Message{ID: asstID, Role: "assistant", Content: rawText, Provider: string(masterSettings.Provider), Model: masterSettings.Model, Mode: mode},
	}
	return res, nil
}

func fallbackToolAnswer(userText string, out ai_assistant.RunOutput, providerErr error) string {
	contextText := strings.TrimSpace(out.AnswerContext)
	if contextText == "" {
		return ""
	}
	if idx := strings.Index(contextText, "工具结果："); idx >= 0 {
		contextText = strings.TrimSpace(contextText[idx+len("工具结果："):])
	}
	if idx := strings.LastIndex(contextText, "\n\n用户问题："); idx >= 0 {
		contextText = strings.TrimSpace(contextText[:idx])
	}
	if contextText == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("模型生成最终回答失败，先返回已经完成的工具结果。\n\n")
	if providerErr != nil {
		b.WriteString("失败原因：")
		b.WriteString(providerErr.Error())
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(userText) != "" {
		b.WriteString("用户问题：")
		b.WriteString(strings.TrimSpace(userText))
		b.WriteString("\n\n")
	}
	b.WriteString(contextText)
	return b.String()
}

// maybeSummarize compresses old history when the projected prompt estimate
// exceeds the budget. Mutates conv.SummaryText / SummaryThroughMessageID and
// strips summarized rows from history. Up to 3 attempts; falls back silently
// (truncation in assembleForTurn) on summarizer failure.
func (s *Service) maybeSummarize(ctx context.Context, conv *repository.AIConversation,
	history *[]repository.AIMessage, settings model.AISettings) {
	if s.summaryCaller == nil {
		return
	}
	for attempt := 0; attempt < 3; attempt++ {
		total := estimateTokens(conv.SummaryText)
		for _, m := range *history {
			total += estimateTokens(m.Content)
		}
		if total <= settings.ContextBudgetTokens {
			return
		}
		newSummary, through, err := summarize(ctx, s.summaryCaller, settings, conv.SummaryText, *history)
		if err != nil {
			s.logger.Warn("ai_conversation: summarize failed; falling back to truncation", "error", err)
			return
		}
		if through == 0 {
			return
		}
		if err := s.repo.UpdateSummary(conv.ID, newSummary, through); err != nil {
			s.logger.Warn("ai_conversation: persist summary failed", "error", err)
			return
		}
		conv.SummaryText = newSummary
		conv.SummaryThroughMessageID.Int64 = through
		conv.SummaryThroughMessageID.Valid = true
		filtered := (*history)[:0]
		for _, m := range *history {
			if m.ID > through {
				filtered = append(filtered, m)
			}
		}
		*history = filtered
	}
}

func mapRepoErr(err error) error {
	if errors.Is(err, repository.ErrAIConversationNotFound) {
		return apperr.New(apperr.CodeNotFound, "会话不存在")
	}
	if errors.Is(err, repository.ErrAIConversationNoUserMessage) {
		return apperr.New(apperr.CodeFailedPrecondition, "没有可重新发送的上一轮对话")
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
