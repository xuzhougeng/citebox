package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestSettingsPageI18nKeysHaveLocaleEntries(t *testing.T) {
	repoRoot := testRepoRoot(t)
	htmlPath := filepath.Join(repoRoot, "web", "settings.html")
	html, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", htmlPath, err)
	}

	keys := settingsI18nKeys(string(html))
	if len(keys) == 0 {
		t.Fatal("settings.html does not contain settings.* i18n keys")
	}

	for _, lang := range []string{"zh-CN", "en"} {
		localePath := filepath.Join(repoRoot, "web", "static", "locales", lang, "settings.json")
		locale := readLocaleFile(t, localePath)
		var missing []string
		for _, key := range keys {
			if _, ok := locale[key]; !ok {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			t.Fatalf("%s is missing settings.html i18n keys:\n%s", localePath, strings.Join(missing, "\n"))
		}
	}
}

func settingsI18nKeys(html string) []string {
	re := regexp.MustCompile(`\bdata-i18n(?:-[a-z-]+)?="([^"]+)"`)
	seen := make(map[string]struct{})
	for _, match := range re.FindAllStringSubmatch(html, -1) {
		key := match[1]
		if strings.HasPrefix(key, "settings.") {
			seen[key] = struct{}{}
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func readLocaleFile(t *testing.T, path string) map[string]string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	var locale map[string]string
	if err := json.Unmarshal(data, &locale); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", path, err)
	}
	return locale
}

func testRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
