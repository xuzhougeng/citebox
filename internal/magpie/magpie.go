// Package magpie reads the independent connection file written by Magpie.
package magpie

import (
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const MaxFileBytes = 64 * 1024

type Connection struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

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
	if len(data) > MaxFileBytes || json.Unmarshal(data, &c) != nil {
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
	return c, true
}
