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

func TestLooksLikeDOI(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"10.1038/s41586-023-06000-1", true},
		{"https://doi.org/10.1038/s41586-023-06000-1", true},
		{"doi:10.1038/s41586-023-06000-1", true},
		{"DOI:10.1000/xyz123", true},
		{"  10.1234/abc.def  ", true},
		{"hello world", false},
		{"/help", false},
		{"", false},
		{"just text mentioning 10.something", false},
	}
	for _, tc := range cases {
		if got := looksLikeDOI(tc.in); got != tc.want {
			t.Errorf("looksLikeDOI(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}
