package ai_conversation

import (
	"encoding/json"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
	"github.com/xuzhougeng/citebox/internal/service/research"
	"strings"
	"testing"
)

func TestSavedAnswerRetainsCitationAfterSourceChangesAndConversationDeletion(t *testing.T) {
	svc, repo, _ := newServiceForTest(t)
	conv, _ := svc.CreateDraft()
	p := mustInsertPaperForTest(t, repo, "Study", "10.1/snapshot")
	if err := svc.PinPaper(conv, p); err != nil {
		t.Fatal(err)
	}
	page := 3
	cites, _ := json.Marshal([]ai_assistant.Citation{{I: 1, PaperID: p, Title: "Original title", Page: &page, SourceURL: "/viewer?kind=pdf&page=3", SourceRevision: "sha256:old", Snippet: research.Snippet{Text: "Original negative result.", Origin: "original"}}})
	repo.AIConversation.AddMessage(conv, "user", "Does it support the claim?", repository.AIMessageMeta{})
	message, _ := repo.AIConversation.AddMessage(conv, "assistant", "The claim was not supported [1].", repository.AIMessageMeta{CitationsJSON: string(cites)})
	if _, err := repo.DB().Exec(`UPDATE papers SET pdf_text='New text', title='New title' WHERE id=?`, p); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AppendAnswerToPaperNote(conv, message, p, "en"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteConversation(conv); err != nil {
		t.Fatal(err)
	}
	paper, _ := repo.Paper.GetPaperDetail(p)
	for _, want := range []string{"Original title", "Original negative result.", "sha256:old", "Page 3", "[1](/viewer?kind=pdf&page=3)"} {
		if !strings.Contains(paper.PaperNotesText, want) {
			t.Fatalf("missing %s: %s", want, paper.PaperNotesText)
		}
	}
}

func TestCitationSnapshotPreservesCodeLinksAndRejectsUnsafeURLs(t *testing.T) {
	raw := `[{"i":1,"title":"Study","source_url":"javascript:alert(1)","snippet":{"text":"<script>example</script>"}}]`
	answer := "Finding [1]. Code `[1]`. Existing [1](https://example.org).\n```\n[1]\n```"
	result, err := answerWithCitationSnapshots(answer, raw, "en")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result, answer) || strings.Contains(result, "javascript:") || strings.Contains(result, "<script>") {
		t.Fatal(result)
	}
	if !strings.Contains(result, "Page unknown") {
		t.Fatal("unknown page not disclosed")
	}
}

func TestStoppedAnswerCannotBeSavedAsCompleteNote(t *testing.T) {
	svc, repo, _ := newServiceForTest(t)
	conv, _ := svc.CreateDraft()
	p := mustInsertPaperForTest(t, repo, "Study", "10.1/stopped")
	svc.PinPaper(conv, p)
	message, _ := repo.AIConversation.AddMessage(conv, "assistant", "Partial result", repository.AIMessageMeta{Mode: "stopped"})
	if _, err := svc.AppendAnswerToPaperNote(conv, message, p, "en"); err == nil {
		t.Fatal("stopped answer accepted")
	}
}
