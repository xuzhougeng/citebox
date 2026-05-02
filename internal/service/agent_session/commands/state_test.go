package commands

import (
	"context"
	"testing"

	"github.com/xuzhougeng/citebox/internal/repository"
)

type fakeState struct {
	paperID int64
	title   string
	figID   int64
	papers  []int64
	figs    []int64
	reset   bool
}

func (f *fakeState) Reset(string) error {
	f.reset = true
	f.paperID = 0
	f.title = ""
	f.figID = 0
	f.papers = nil
	f.figs = nil
	return nil
}
func (f *fakeState) SetCurrentPaper(_ string, id int64, t string) error {
	f.paperID = id
	f.title = t
	return nil
}
func (f *fakeState) SetCurrentFigure(_ string, id int64) error { f.figID = id; return nil }
func (f *fakeState) SetSearchResults(_ string, p, fg []int64) error {
	f.papers = p
	f.figs = fg
	return nil
}
func (f *fakeState) Get(string) (SurfaceContext, error) {
	return SurfaceContext{
		CurrentPaperID: f.paperID, CurrentPaperTitle: f.title, CurrentFigureID: f.figID,
		RecentSearchPaperIDs: f.papers, RecentSearchFigureIDs: f.figs,
	}, nil
}

func TestClearSetsBarrier(t *testing.T) {
	repo := repository.NewTestAIConversationRepo(t)
	cid, _ := repo.FindOrCreateByKind("main_wechat", "wechat")
	m, _ := repo.AddMessage(cid, "user", "hello", repository.AIMessageMeta{})
	cmd := &ClearCommand{}
	res, err := cmd.Execute(context.Background(), "", RuntimeCtx{
		ConversationID: cid, Repo: repo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.UsedShortcut {
		t.Error("expected UsedShortcut=true")
	}
	after, _ := repo.ListMessagesAfterBarrier(cid, 10)
	if len(after) != 0 {
		t.Fatalf("after /clear, no msg should be visible; got %d (latest msg id %d)", len(after), m)
	}
}

func TestResetClearsSurfaceState(t *testing.T) {
	fs := &fakeState{paperID: 7, title: "X"}
	cmd := &ResetCommand{}
	_, err := cmd.Execute(context.Background(), "", RuntimeCtx{
		Surface: "wechat", State: fs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fs.reset {
		t.Fatal("Reset not called")
	}
}

func TestHelpReturnsListing(t *testing.T) {
	res, err := (&HelpCommand{}).Execute(context.Background(), "", RuntimeCtx{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ChunksText) != 1 || res.ChunksText[0] == "" {
		t.Fatalf("empty help text")
	}
}

func TestStatusReportsCurrentState(t *testing.T) {
	fs := &fakeState{paperID: 5, title: "Paper", figID: 8, papers: []int64{1, 2}}
	res, err := (&StatusCommand{}).Execute(context.Background(), "", RuntimeCtx{
		Surface: "wechat", ConversationID: 42, State: fs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ChunksText) != 1 {
		t.Fatal("want one chunk")
	}
	txt := res.ChunksText[0]
	for _, want := range []string{"#42", "5 Paper", "8", "2"} {
		if !contains(txt, want) {
			t.Errorf("status missing %q in %q", want, txt)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

type fakeNoteAppender struct {
	paperCalls  []paperNoteCall
	figureCalls []figureNoteCall
}
type paperNoteCall struct {
	ID   int64
	Text string
}
type figureNoteCall struct {
	ID   int64
	Text string
}

func (f *fakeNoteAppender) AppendPaperNote(id int64, text string) error {
	f.paperCalls = append(f.paperCalls, paperNoteCall{id, text})
	return nil
}
func (f *fakeNoteAppender) AppendFigureNote(id int64, text string) error {
	f.figureCalls = append(f.figureCalls, figureNoteCall{id, text})
	return nil
}

func TestNoteRequiresSelection(t *testing.T) {
	fs := &fakeState{} // nothing selected
	_, err := (&NoteCommand{Library: &fakeNoteAppender{}}).Execute(context.Background(), "test", RuntimeCtx{State: fs, Surface: "wechat"})
	if err == nil {
		t.Fatal("expected error when nothing selected")
	}
}

func TestNoteAppendsToCurrentFigureFirst(t *testing.T) {
	fa := &fakeNoteAppender{}
	fs := &fakeState{paperID: 3, title: "P", figID: 9}
	_, err := (&NoteCommand{Library: fa}).Execute(context.Background(), "记录", RuntimeCtx{State: fs, Surface: "wechat"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fa.figureCalls) != 1 || fa.figureCalls[0].ID != 9 {
		t.Fatalf("want figure note on 9, got %v", fa.figureCalls)
	}
	if len(fa.paperCalls) != 0 {
		t.Fatalf("paper note should not fire when figure is selected")
	}
}

func TestNoteFallsBackToPaper(t *testing.T) {
	fa := &fakeNoteAppender{}
	fs := &fakeState{paperID: 3, title: "P"}
	_, err := (&NoteCommand{Library: fa}).Execute(context.Background(), "记录", RuntimeCtx{State: fs, Surface: "wechat"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fa.paperCalls) != 1 || fa.paperCalls[0].ID != 3 {
		t.Fatalf("want paper note on 3, got %v", fa.paperCalls)
	}
}
