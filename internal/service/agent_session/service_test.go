package agent_session

import "testing"

func TestParseSlashRecognizesFullwidth(t *testing.T) {
	cmd, arg, ok := parseSlash("／help")
	if !ok || cmd != "/help" || arg != "" {
		t.Fatalf("got cmd=%q arg=%q ok=%v", cmd, arg, ok)
	}
}

func TestParseSlashSplitsArg(t *testing.T) {
	cmd, arg, ok := parseSlash("/note 这是笔记")
	if !ok || cmd != "/note" || arg != "这是笔记" {
		t.Fatalf("got cmd=%q arg=%q ok=%v", cmd, arg, ok)
	}
}

func TestParseSlashRejectsPlainText(t *testing.T) {
	_, _, ok := parseSlash("hello")
	if ok {
		t.Fatal("plain text should not be recognized as slash")
	}
}
