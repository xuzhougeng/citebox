package agent_session

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"

	"github.com/xuzhougeng/citebox/internal/repository"
)

// legacyWeixinContext mirrors the JSON shape of the old `im_context.json`
// produced by the WeChat IM bridge before the agent_session cutover. Only the
// fields that survive the migration are decoded; any other keys (notably
// `updated_at`) are ignored.
type legacyWeixinContext struct {
	CurrentPaperID  int64        `json:"current_paper_id"`
	CurrentFigureID int64        `json:"current_figure_id"`
	QAHistory       []legacyTurn `json:"qa_history"`
	SearchPaperIDs  []int64      `json:"search_paper_ids"`
	SearchFigureIDs []int64      `json:"search_figure_ids"`
}

// legacyTurn covers two on-disk shapes the bridge has used historically:
//   - `{"role":"user","content":"..."}` — raw role/content rows
//   - `{"question":"...","answer":"..."}` — model.AIConversationTurn rows
//
// MigrateLegacyWeixinContext flattens both into a pair of user/assistant
// messages so subsequent runs of the WeChat surface see the same history that
// the legacy bridge accumulated.
type legacyTurn struct {
	Role     string `json:"role,omitempty"`
	Content  string `json:"content,omitempty"`
	Question string `json:"question,omitempty"`
	Answer   string `json:"answer,omitempty"`
}

// MigrateLegacyWeixinContext is a one-shot best-effort import of the legacy
// `im_context.json` file into the new agent_session-owned conversation row and
// surface state. It is idempotent: once the legacy file has been renamed to
// `*.migrated.bak` (or never existed), calling it again is a no-op.
//
// The function intentionally swallows the rename error after a successful
// migration: if the file still exists on the next boot the migration will
// simply replay (FindOrCreateByKind keeps the same conversation, and the
// duplicate AddMessage rows are still ordered by id so the bounded-window
// query keeps the latest). Returning the rename error would mask the more
// important fact that the data was already moved.
func MigrateLegacyWeixinContext(path string, repo *repository.AIConversationRepository,
	state *SurfaceStateStore, logger *slog.Logger) error {
	if repo == nil {
		return errors.New("agent_session: nil ai conversation repo")
	}
	if state == nil {
		return errors.New("agent_session: nil surface state store")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var ctx legacyWeixinContext
	if err := json.Unmarshal(raw, &ctx); err != nil {
		return err
	}

	cid, err := repo.FindOrCreateByKind("main_wechat", "wechat")
	if err != nil {
		return err
	}

	for _, t := range ctx.QAHistory {
		role := t.Role
		content := t.Content

		// AIConversationTurn shape: split into a user (question) + assistant
		// (answer) pair. Only the populated half(s) are inserted.
		if role == "" && (t.Question != "" || t.Answer != "") {
			if t.Question != "" {
				if _, err := repo.AddMessage(cid, "user", t.Question, repository.AIMessageMeta{Mode: "migrated"}); err != nil {
					return err
				}
			}
			if t.Answer != "" {
				if _, err := repo.AddMessage(cid, "assistant", t.Answer, repository.AIMessageMeta{Mode: "migrated"}); err != nil {
					return err
				}
			}
			continue
		}

		if role != "user" && role != "assistant" {
			role = "assistant"
		}
		if content == "" {
			continue
		}
		if _, err := repo.AddMessage(cid, role, content, repository.AIMessageMeta{Mode: "migrated"}); err != nil {
			return err
		}
	}

	if ctx.CurrentPaperID > 0 {
		_ = state.SetCurrentPaper("wechat", ctx.CurrentPaperID, "")
	}
	if ctx.CurrentFigureID > 0 {
		_ = state.SetCurrentFigure("wechat", ctx.CurrentFigureID)
	}
	if len(ctx.SearchPaperIDs) > 0 || len(ctx.SearchFigureIDs) > 0 {
		_ = state.SetSearchResults("wechat", ctx.SearchPaperIDs, ctx.SearchFigureIDs)
	}

	if err := os.Rename(path, path+".migrated.bak"); err != nil {
		if logger != nil {
			logger.Warn("rename legacy weixin context failed", "error", err, "path", path)
		}
	}
	return nil
}
