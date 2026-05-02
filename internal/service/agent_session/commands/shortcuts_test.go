package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

// --- shared fakes -----------------------------------------------------------

type fakePaperListSource struct {
	resp     *model.PaperListResponse
	err      error
	lastArgs model.PaperFilter
	calls    int
}

func (f *fakePaperListSource) ListPapers(filter model.PaperFilter) (*model.PaperListResponse, error) {
	f.calls++
	f.lastArgs = filter
	return f.resp, f.err
}

type fakePaperFiguresSource struct {
	paper *model.Paper
	err   error
	calls int
	lastID int64
}

func (f *fakePaperFiguresSource) GetPaper(id int64) (*model.Paper, error) {
	f.calls++
	f.lastID = id
	return f.paper, f.err
}

type fakeRandomPicker struct {
	id      int64
	path    string
	caption string
	err     error
	called  bool
}

func (f *fakeRandomPicker) PickRandomFigure(_ context.Context) (int64, string, string, error) {
	f.called = true
	return f.id, f.path, f.caption, f.err
}

// --- /recent ---------------------------------------------------------------

func TestRecentReturnsFormattedList(t *testing.T) {
	fps := &fakePaperListSource{resp: &model.PaperListResponse{
		Papers: []model.Paper{
			{ID: 11, Title: "Paper Eleven"},
			{ID: 22, Title: "Paper Twenty Two"},
		},
		Total: 2,
	}}
	fs := &fakeState{}
	cmd := &RecentCommand{Lister: fps, Limit: 3}
	res, err := cmd.Execute(context.Background(), "", RuntimeCtx{
		Surface: "wechat", State: fs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.UsedShortcut {
		t.Fatal("want UsedShortcut=true")
	}
	if fps.lastArgs.PageSize != 3 {
		t.Fatalf("want PageSize=3, got %d", fps.lastArgs.PageSize)
	}
	if fps.lastArgs.SortBy != "created_at" {
		t.Fatalf("want SortBy=created_at, got %q", fps.lastArgs.SortBy)
	}
	if len(res.ChunksText) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(res.ChunksText))
	}
	txt := res.ChunksText[0]
	for _, want := range []string{"Paper Eleven", "Paper Twenty Two", "#11", "#22"} {
		if !contains(txt, want) {
			t.Errorf("recent missing %q in %q", want, txt)
		}
	}
	if len(fs.papers) != 2 || fs.papers[0] != 11 || fs.papers[1] != 22 {
		t.Fatalf("want state papers [11,22], got %v", fs.papers)
	}
	if fs.figs != nil {
		t.Fatalf("want state figs nil, got %v", fs.figs)
	}
}

func TestRecentDefaultsToFiveWhenLimitUnset(t *testing.T) {
	fps := &fakePaperListSource{resp: &model.PaperListResponse{}}
	cmd := &RecentCommand{Lister: fps}
	if _, err := cmd.Execute(context.Background(), "", RuntimeCtx{Surface: "wechat", State: &fakeState{}}); err != nil {
		t.Fatal(err)
	}
	if fps.lastArgs.PageSize != 5 {
		t.Fatalf("want default PageSize=5, got %d", fps.lastArgs.PageSize)
	}
}

func TestRecentEmptyLibraryReturnsFriendlyMessage(t *testing.T) {
	fps := &fakePaperListSource{resp: &model.PaperListResponse{Papers: nil}}
	cmd := &RecentCommand{Lister: fps}
	res, err := cmd.Execute(context.Background(), "", RuntimeCtx{Surface: "wechat", State: &fakeState{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ChunksText) != 1 || !contains(res.ChunksText[0], "为空") {
		t.Fatalf("want empty-library message, got %q", res.ChunksText)
	}
}

// --- /search ---------------------------------------------------------------

func TestSearchPersistsAndUpdatesState(t *testing.T) {
	fps := &fakePaperListSource{resp: &model.PaperListResponse{
		Papers: []model.Paper{{ID: 1, Title: "Hit: diffusion"}},
	}}
	fs := &fakeState{}
	cmd := &SearchCommand{Lister: fps}
	res, err := cmd.Execute(context.Background(), "diffusion", RuntimeCtx{
		Surface: "wechat", State: fs, ConversationID: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fps.lastArgs.Keyword != "diffusion" {
		t.Fatalf("want Keyword=diffusion, got %q", fps.lastArgs.Keyword)
	}
	if !res.UsedShortcut {
		t.Fatal("want shortcut")
	}
	if len(fs.papers) != 1 || fs.papers[0] != 1 {
		t.Fatalf("state not updated: %v", fs.papers)
	}
	if !contains(res.ChunksText[0], "Hit: diffusion") {
		t.Fatalf("missing title in %q", res.ChunksText[0])
	}
}

func TestSearchEmptyQueryErrors(t *testing.T) {
	cmd := &SearchCommand{Lister: &fakePaperListSource{}}
	if _, err := cmd.Execute(context.Background(), "   ", RuntimeCtx{Surface: "wechat", State: &fakeState{}}); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestSearchNoResultsFriendlyMessage(t *testing.T) {
	fps := &fakePaperListSource{resp: &model.PaperListResponse{Papers: nil}}
	cmd := &SearchCommand{Lister: fps}
	res, err := cmd.Execute(context.Background(), "absent", RuntimeCtx{Surface: "wechat", State: &fakeState{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ChunksText) != 1 || !contains(res.ChunksText[0], "未找到") {
		t.Fatalf("want no-result message, got %q", res.ChunksText)
	}
}

// --- /figures --------------------------------------------------------------

func TestFiguresRequiresCurrentPaper(t *testing.T) {
	fs := &fakeState{} // no current paper
	cmd := &FiguresCommand{Library: &fakePaperFiguresSource{}}
	if _, err := cmd.Execute(context.Background(), "", RuntimeCtx{Surface: "wechat", State: fs}); err == nil {
		t.Fatal("expected error when no current paper")
	}
}

func TestFiguresLists(t *testing.T) {
	subParent := int64(1)
	paper := &model.Paper{
		ID: 9, Title: "P9",
		Figures: []model.Figure{
			{ID: 1, FigureIndex: 1, PageNumber: 3, Caption: "Top fig 1"},
			{ID: 2, FigureIndex: 2, PageNumber: 4, Caption: "Top fig 2", DisplayLabel: "Fig 2"},
			{ID: 3, FigureIndex: 1, PageNumber: 4, Caption: "subfig", ParentFigureID: &subParent},
		},
	}
	fls := &fakePaperFiguresSource{paper: paper}
	fs := &fakeState{paperID: 9, title: "P9"}
	cmd := &FiguresCommand{Library: fls}
	res, err := cmd.Execute(context.Background(), "", RuntimeCtx{Surface: "wechat", State: fs})
	if err != nil {
		t.Fatal(err)
	}
	if !res.UsedShortcut {
		t.Fatal("want UsedShortcut=true")
	}
	if fls.lastID != 9 {
		t.Fatalf("want GetPaper(9), got GetPaper(%d)", fls.lastID)
	}
	txt := res.ChunksText[0]
	for _, want := range []string{"#1", "#2", "Top fig 1", "Fig 2"} {
		if !contains(txt, want) {
			t.Errorf("figures listing missing %q in %q", want, txt)
		}
	}
	if contains(txt, "#3") {
		t.Errorf("subfigure should not appear in top-level listing: %q", txt)
	}
	if len(fs.figs) != 2 || fs.figs[0] != 1 || fs.figs[1] != 2 {
		t.Fatalf("want state figs [1,2], got %v", fs.figs)
	}
}

func TestFiguresEmptyPaperReturnsFriendlyMessage(t *testing.T) {
	paper := &model.Paper{ID: 9, Title: "P9", Figures: nil}
	fls := &fakePaperFiguresSource{paper: paper}
	fs := &fakeState{paperID: 9, title: "P9"}
	cmd := &FiguresCommand{Library: fls}
	res, err := cmd.Execute(context.Background(), "", RuntimeCtx{Surface: "wechat", State: fs})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ChunksText) != 1 || !contains(res.ChunksText[0], "尚无") {
		t.Fatalf("want empty-figures message, got %q", res.ChunksText)
	}
}

// --- /random ---------------------------------------------------------------

func TestRandomCallsPickerAndSetsCurrent(t *testing.T) {
	fp := &fakeRandomPicker{id: 42, path: "/tmp/x.png", caption: "Random caption"}
	fs := &fakeState{}
	cmd := &RandomCommand{Picker: fp}
	res, err := cmd.Execute(context.Background(), "", RuntimeCtx{Surface: "wechat", State: fs})
	if err != nil {
		t.Fatal(err)
	}
	if !fp.called {
		t.Fatal("picker not called")
	}
	if fs.figID != 42 {
		t.Fatalf("want CurrentFigure=42, got %d", fs.figID)
	}
	if res.ImagePath != "/tmp/x.png" {
		t.Fatalf("want image path /tmp/x.png, got %q", res.ImagePath)
	}
	if len(res.ChunksText) != 1 || res.ChunksText[0] != "Random caption" {
		t.Fatalf("want caption text, got %q", res.ChunksText)
	}
	if !res.UsedShortcut {
		t.Fatal("want UsedShortcut=true")
	}
}

func TestRandomPropagatesPickerError(t *testing.T) {
	fp := &fakeRandomPicker{err: errors.New("nope")}
	cmd := &RandomCommand{Picker: fp}
	if _, err := cmd.Execute(context.Background(), "", RuntimeCtx{Surface: "wechat", State: &fakeState{}}); err == nil {
		t.Fatal("expected error from picker")
	}
}

// --- /paper N --------------------------------------------------------------

func TestPaperSelectsByIndex(t *testing.T) {
	fs := &fakeState{papers: []int64{10, 20, 30}}
	cmd := &PaperCommand{}
	res, err := cmd.Execute(context.Background(), "2", RuntimeCtx{Surface: "wechat", State: fs})
	if err != nil {
		t.Fatal(err)
	}
	if fs.paperID != 20 {
		t.Fatalf("want CurrentPaperID=20, got %d", fs.paperID)
	}
	if !res.UsedShortcut {
		t.Fatal("want UsedShortcut=true")
	}
	if !contains(res.ChunksText[0], "20") {
		t.Fatalf("want id 20 in chunk, got %q", res.ChunksText[0])
	}
}

func TestPaperOutOfRangeErrors(t *testing.T) {
	fs := &fakeState{papers: []int64{1, 2}}
	if _, err := (&PaperCommand{}).Execute(context.Background(), "5", RuntimeCtx{Surface: "wechat", State: fs}); err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestPaperBadArgErrors(t *testing.T) {
	fs := &fakeState{papers: []int64{1, 2}}
	for _, arg := range []string{"", "abc", "-1", "0"} {
		if _, err := (&PaperCommand{}).Execute(context.Background(), arg, RuntimeCtx{Surface: "wechat", State: fs}); err == nil {
			t.Fatalf("expected error for arg %q", arg)
		}
	}
}

// --- /figure N -------------------------------------------------------------

func TestFigureSelectsByIndex(t *testing.T) {
	fs := &fakeState{figs: []int64{100, 200, 300}}
	cmd := &FigureCommand{}
	res, err := cmd.Execute(context.Background(), "3", RuntimeCtx{Surface: "wechat", State: fs})
	if err != nil {
		t.Fatal(err)
	}
	if fs.figID != 300 {
		t.Fatalf("want CurrentFigureID=300, got %d", fs.figID)
	}
	if !res.UsedShortcut {
		t.Fatal("want UsedShortcut=true")
	}
}

func TestFigureOutOfRangeErrors(t *testing.T) {
	fs := &fakeState{figs: []int64{10}}
	if _, err := (&FigureCommand{}).Execute(context.Background(), "5", RuntimeCtx{Surface: "wechat", State: fs}); err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestFigureBadArgErrors(t *testing.T) {
	fs := &fakeState{figs: []int64{10, 20}}
	for _, arg := range []string{"", "abc", "0"} {
		if _, err := (&FigureCommand{}).Execute(context.Background(), arg, RuntimeCtx{Surface: "wechat", State: fs}); err == nil {
			t.Fatalf("expected error for arg %q", arg)
		}
	}
}
