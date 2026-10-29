// Package magpie reads the independent connection file written by Magpie.
package magpie

import (
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const MaxFileBytes = 64 * 1024

type Connection struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
	// Info holds what Magpie knows about Model; nil when it is absent, stale or malformed.
	Info *ModelInfo `json:"-"`
}

// ModelInfo is the optional modelInfo snapshot written beside the model.
// Zero limits and empty efforts are unknown, never a capacity to enforce.
type ModelInfo struct {
	ID      string
	Context int
	Output  int
	Efforts []string
}

var effortPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "citebox", "magpie.json")
}

// Load treats missing, partial, oversized and invalid files as disconnected.
// It neither writes settings nor logs credentials.
func Load(path string) (Connection, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Connection{}, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return Connection{}, false
	}
	return Parse(data)
}

func Parse(data []byte) (Connection, bool) {
	var c Connection
	var extra struct {
		ModelInfo json.RawMessage `json:"modelInfo"`
	}
	if len(data) > MaxFileBytes || json.Unmarshal(data, &c) != nil || json.Unmarshal(data, &extra) != nil {
		return Connection{}, false
	}
	for _, s := range []string{c.BaseURL, c.APIKey, c.Model} {
		if len(s) == 0 || len(s) > 8192 || strings.TrimSpace(s) != s {
			return Connection{}, false
		}
		for _, r := range s {
			if r < 32 || r == 127 {
				return Connection{}, false
			}
		}
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || strings.ContainsAny(c.BaseURL, "?#") {
		return Connection{}, false
	}
	c.Info = parseModelInfo(extra.ModelInfo, c.Model)
	return c, true
}

// parseModelInfo never invalidates the connection. A snapshot for another ID
// (for example after the model was edited by hand) or with a malformed field
// is ignored as a whole.
func parseModelInfo(data json.RawMessage, model string) *ModelInfo {
	var raw struct {
		ID      string   `json:"id"`
		Context *uint32  `json:"context"`
		Output  *uint32  `json:"output"`
		Efforts []string `json:"efforts"`
	}
	if len(data) == 0 || json.Unmarshal(data, &raw) != nil || raw.ID != model || len(raw.Efforts) > 32 {
		return nil
	}
	info := &ModelInfo{ID: raw.ID}
	for _, field := range []struct {
		value *uint32
		dest  *int
	}{{raw.Context, &info.Context}, {raw.Output, &info.Output}} {
		if field.value == nil {
			continue
		}
		if *field.value == 0 {
			return nil
		}
		*field.dest = int(*field.value)
	}
	for _, effort := range raw.Efforts {
		if !effortPattern.MatchString(effort) {
			return nil
		}
		if !info.SupportsEffort(effort) {
			info.Efforts = append(info.Efforts, effort)
		}
	}
	return info
}

func (i *ModelInfo) SupportsEffort(effort string) bool {
	if i == nil || effort == "" {
		return false
	}
	for _, item := range i.Efforts {
		if item == effort {
			return true
		}
	}
	return false
}
