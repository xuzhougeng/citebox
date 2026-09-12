package ai_conversation

import (
	"github.com/xuzhougeng/citebox/internal/repository"
	"strings"
	"testing"
)

func TestAppendAnswerToSelectedPaperPreservesNotesAndDeduplicates(t *testing.T) {
	svc, repo, _ := newServiceForTest(t)
	c, _ := repo.AIConversation.CreateConversation()
	p1 := mustInsertPaperForTest(t, repo, "First", "10.1/first")
	p2 := mustInsertPaperForTest(t, repo, "Second", "10.1/second")
	for _, p := range []int64{p1, p2} {
		if err := svc.PinPaper(c, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.DB().Exec(`UPDATE papers SET notes_text='Management',paper_notes_text='Human note' WHERE id=?`, p2); err != nil {
		t.Fatal(err)
	}
	repo.AIConversation.AddMessage(c, "user", "What are the limitations?", repository.AIMessageMeta{})
	m, _ := repo.AIConversation.AddMessage(c, "assistant", "Sample size is limited.", repository.AIMessageMeta{Provider: "test", Model: "model"})
	for i := 0; i < 2; i++ {
		saved, err := svc.AppendAnswerToPaperNote(c, m, p2, "en")
		if err != nil || saved != (i == 0) {
			t.Fatalf("attempt %d: saved=%v err=%v", i, saved, err)
		}
	}
	p, _ := repo.Paper.GetPaperDetail(p2)
	if p.NotesText != "Management" || !strings.HasPrefix(p.PaperNotesText, "Human note\n\n") {
		t.Fatalf("existing notes lost: %+v", p)
	}
	if strings.Count(p.PaperNotesText, "Sample size is limited.") != 1 || !strings.Contains(p.PaperNotesText, "What are the limitations?") || !strings.Contains(p.PaperNotesText, "AI-generated") {
		t.Fatal(p.PaperNotesText)
	}
	other, _ := repo.Paper.GetPaperDetail(p1)
	if other.PaperNotesText != "" {
		t.Fatal("unselected paper changed")
	}
}
func TestAppendAnswerRejectsWrongSourceAndUnpinnedTarget(t *testing.T) {
	svc, repo, _ := newServiceForTest(t)
	c, _ := repo.AIConversation.CreateConversation()
	other, _ := repo.AIConversation.CreateConversation()
	p := mustInsertPaperForTest(t, repo, "Paper", "10.1/source")
	user, _ := repo.AIConversation.AddMessage(c, "user", "Question", repository.AIMessageMeta{})
	answer, _ := repo.AIConversation.AddMessage(c, "assistant", "Answer", repository.AIMessageMeta{})
	for _, pair := range [][2]int64{{c, user}, {other, answer}, {c, answer}} {
		if _, err := svc.AppendAnswerToPaperNote(pair[0], pair[1], p, "en"); err == nil {
			t.Fatalf("invalid save accepted: %v", pair)
		}
	}
}
