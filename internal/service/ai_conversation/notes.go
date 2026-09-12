package ai_conversation

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
)

func (s *Service) AppendAnswerToPaperNote(conversationID, messageID, paperID int64, language string) (bool, error) {
	if conversationID <= 0 || messageID <= 0 || paperID <= 0 {
		return false, apperr.New(apperr.CodeInvalidArgument, "invalid source or target")
	}
	message, question, err := s.repo.GetNoteSource(conversationID, messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, apperr.New(apperr.CodeNotFound, "answer not found in this conversation")
	}
	if err != nil {
		return false, err
	}
	if message.Role != "assistant" || strings.TrimSpace(message.Content) == "" {
		return false, apperr.New(apperr.CodeInvalidArgument, "only completed answers can be saved")
	}
	pinned, err := s.repo.ListPinnedPapers(conversationID)
	if err != nil {
		return false, err
	}
	allowed := false
	for _, p := range pinned {
		if p.PaperID == paperID {
			allowed = true
			break
		}
	}
	if !allowed {
		return false, apperr.New(apperr.CodeInvalidArgument, "target paper must be pinned to this conversation")
	}
	marker := fmt.Sprintf("/ai?conversation=%d&message=%d)", conversationID, messageID)
	heading, questionLabel, sourceLabel, notice := "AI 助手", "问题", "来源", "以下为 AI 整理内容，请结合原文核对。"
	if language == "en" {
		heading, questionLabel, sourceLabel, notice = "AI Assistant", "Question", "Source", "AI-generated notes; verify against the original paper."
	}
	block := fmt.Sprintf("## %s · #%d\n\n> %s\n\n%s: %s\n\n%s: [#%d](/ai?conversation=%d&message=%d) · %s · %s / %s\n\n%s",
		heading, messageID, notice, questionLabel, question, sourceLabel, conversationID, conversationID, messageID, message.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"), message.Provider, message.Model, message.Content)
	saved, err := s.papers.AppendAINote(paperID, marker, block)
	if errors.Is(err, sql.ErrNoRows) {
		return false, apperr.New(apperr.CodeNotFound, "paper not found")
	}
	return saved, err
}
