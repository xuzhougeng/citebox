package magpie

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestMagpieModelInfo(t *testing.T) {
	base := `{"baseURL":"http://localhost/v1","apiKey":"test-key","model":"p/reasoner"`
	c, ok := Parse([]byte(base + `,"modelInfo":{"id":"p/reasoner","context":400000,"output":128000,"efforts":["low","medium","high"]}}`))
	if !ok || c.Info == nil || c.Info.Context != 400000 || c.Info.Output != 128000 || !reflect.DeepEqual(c.Info.Efforts, []string{"low", "medium", "high"}) {
		t.Fatalf("matching snapshot: %+v", c.Info)
	}
	if !c.Info.SupportsEffort("high") || c.Info.SupportsEffort("max") || c.Info.SupportsEffort("") {
		t.Fatal("effort membership")
	}
	c, ok = Parse([]byte(base + `,"modelInfo":{"id":"p/reasoner","efforts":["high"]}}`))
	if !ok || c.Info == nil || c.Info.Output != 0 || c.Info.Context != 0 {
		t.Fatalf("partial snapshot: %+v", c.Info)
	}
	// A stale or malformed snapshot is ignored, never the connection itself.
	for _, info := range []string{
		`{"id":"p/other","output":128000,"efforts":["high"]}`,
		`{"output":128000}`,
		`null`, `[]`, `"p/reasoner"`, `{"id":7}`,
		`{"id":"p/reasoner","output":0}`,
		`{"id":"p/reasoner","output":-1}`,
		`{"id":"p/reasoner","output":8192.5}`,
		`{"id":"p/reasoner","context":4294967296}`,
		`{"id":"p/reasoner","context":"400000"}`,
		`{"id":"p/reasoner","efforts":"high"}`,
		`{"id":"p/reasoner","efforts":["high",false]}`,
		`{"id":"p/reasoner","efforts":["High"]}`,
		`{"id":"p/reasoner","efforts":["bad value"]}`,
	} {
		c, ok := Parse([]byte(base + `,"modelInfo":` + info + `}`))
		if !ok || c.Model != "p/reasoner" || c.APIKey != "test-key" {
			t.Fatalf("modelInfo %s invalidated the connection", info)
		}
		if c.Info != nil {
			t.Fatalf("modelInfo %s was used: %+v", info, c.Info)
		}
	}
	if c, ok := Parse([]byte(base + `}`)); !ok || c.Info != nil {
		t.Fatal("missing snapshot")
	}
}
