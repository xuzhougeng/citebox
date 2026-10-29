package magpie

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMagpieParse(t *testing.T) {
	valid := `{"baseURL":"http://127.0.0.1:3425/v1","apiKey":"test-key","model":"provider/model","future":true}`
	c, ok := Parse([]byte(valid))
	if !ok || c.Model != "provider/model" {
		t.Fatal("valid connection rejected")
	}
	for _, suffix := range []string{"?", "#", "?x=1", "#part"} {
		if _, ok := Parse([]byte(strings.Replace(valid, "/v1", "/v1"+suffix, 1))); ok {
			t.Fatal("query or fragment accepted")
		}
	}
	for _, content := range []string{`{}`, `null`, `[]`, `{`, `{"baseURL":false}`, strings.Repeat(" ", MaxFileBytes+1),
		strings.Replace(valid, "test-key", "", 1), strings.Replace(valid, "http://127.0.0.1:3425/v1", "file:///tmp/x", 1),
		strings.Replace(valid, "test-key", `bad\r\nheader`, 1), strings.Replace(valid, "http://127.0.0.1:3425/v1", "http://user:pass@localhost/v1", 1)} {
		if _, ok := Parse([]byte(content)); ok {
			t.Fatal("invalid connection accepted")
		}
	}
}

func TestMagpieLoadSwitchDisconnect(t *testing.T) {
	file := filepath.Join(t.TempDir(), "magpie.json")
	if _, ok := Load(file); ok {
		t.Fatal("missing file is connected")
	}
	for _, m := range []string{"p/first", "p/second"} {
		data := `{"baseURL":"http://localhost/v1","apiKey":"test-key","model":"` + m + `"}`
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		c, ok := Load(file)
		if !ok || c.Model != m {
			t.Fatal("model switch was not read")
		}
		after, _ := os.ReadFile(file)
		if string(after) != data {
			t.Fatal("reader modified the file")
		}
	}
	if err := os.WriteFile(file, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(file); ok {
		t.Fatal("empty restored file is connected")
	}
}
