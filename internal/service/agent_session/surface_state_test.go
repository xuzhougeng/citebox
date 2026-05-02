package agent_session

import (
	"path/filepath"
	"testing"
)

func TestSurfaceStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSurfaceStateStore(filepath.Join(dir, "wx_state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrentPaper("wechat", 42, "Title"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrentFigure("wechat", 7); err != nil {
		t.Fatal(err)
	}
	s2, _ := NewSurfaceStateStore(filepath.Join(dir, "wx_state.json"))
	sc, _ := s2.Get("wechat")
	if sc.CurrentPaperID != 42 || sc.CurrentPaperTitle != "Title" || sc.CurrentFigureID != 7 {
		t.Fatalf("unexpected state: %+v", sc)
	}
}

func TestSurfaceStateReset(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewSurfaceStateStore(filepath.Join(dir, "wx.json"))
	_ = s.SetCurrentPaper("wechat", 9, "X")
	if err := s.Reset("wechat"); err != nil {
		t.Fatal(err)
	}
	sc, _ := s.Get("wechat")
	if sc.CurrentPaperID != 0 || sc.CurrentPaperTitle != "" {
		t.Fatalf("reset failed: %+v", sc)
	}
}
