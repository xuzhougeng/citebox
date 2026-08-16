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

var htmlI18nAttrRE = regexp.MustCompile(`\bdata-i18n(?:-[a-z-]+)?="([^"]+)"`)

func TestLocaleFilesHaveMatchingKeys(t *testing.T) {
	repoRoot := testRepoRoot(t)
	zhDir := filepath.Join(repoRoot, "web", "static", "locales", "zh-CN")
	enDir := filepath.Join(repoRoot, "web", "static", "locales", "en")

	zhFiles := localeFileNames(t, zhDir)
	enFiles := localeFileNames(t, enDir)
	if !sameStrings(zhFiles, enFiles) {
		t.Fatalf("locale file sets differ:\nzh-CN only: %s\nen only: %s",
			strings.Join(difference(zhFiles, enFiles), ", "),
			strings.Join(difference(enFiles, zhFiles), ", "))
	}

	for _, name := range zhFiles {
		zh := readLocaleFile(t, filepath.Join(zhDir, name))
		en := readLocaleFile(t, filepath.Join(enDir, name))
		assertSameKeys(t, name, zh, en)
	}
}

func TestHTMLPagesI18nKeysHaveLocaleEntries(t *testing.T) {
	repoRoot := testRepoRoot(t)
	pages, err := filepath.Glob(filepath.Join(repoRoot, "web", "*.html"))
	if err != nil {
		t.Fatalf("Glob web/*.html error = %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("no web HTML pages found")
	}

	sort.Strings(pages)
	for _, pagePath := range pages {
		pagePath := pagePath
		pageName := strings.TrimSuffix(filepath.Base(pagePath), ".html")
		t.Run(pageName, func(t *testing.T) {
			html, err := os.ReadFile(pagePath)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", pagePath, err)
			}

			keys := uniqueStrings(htmlI18nAttrRE.FindAllStringSubmatch(string(html), -1))
			if len(keys) == 0 {
				return
			}

			for _, lang := range []string{"zh-CN", "en"} {
				locale := mergePageLocales(t, repoRoot, lang, pageName)
				var missing []string
				for _, key := range keys {
					if _, ok := locale[key]; !ok {
						missing = append(missing, key)
					}
				}
				if len(missing) > 0 {
					t.Fatalf("%s/%s is missing HTML i18n keys:\n%s", lang, pageName, strings.Join(missing, "\n"))
				}
			}
		})
	}
}

func TestExtensionLocaleKeysMatch(t *testing.T) {
	repoRoot := testRepoRoot(t)
	zhPath := filepath.Join(repoRoot, "extension", "_locales", "zh_CN", "messages.json")
	enPath := filepath.Join(repoRoot, "extension", "_locales", "en", "messages.json")

	zh := readExtensionMessages(t, zhPath)
	en := readExtensionMessages(t, enPath)
	assertSameKeys(t, "extension messages.json", zh, en)

	pages, err := filepath.Glob(filepath.Join(repoRoot, "extension", "*.html"))
	if err != nil {
		t.Fatalf("Glob extension/*.html error = %v", err)
	}
	sort.Strings(pages)
	for _, pagePath := range pages {
		html, err := os.ReadFile(pagePath)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", pagePath, err)
		}
		keys := uniqueStrings(htmlI18nAttrRE.FindAllStringSubmatch(string(html), -1))
		var missing []string
		for _, key := range keys {
			if _, ok := zh[key]; !ok {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			t.Fatalf("%s is missing extension i18n keys:\n%s", filepath.Base(pagePath), strings.Join(missing, "\n"))
		}
	}
}

func mergePageLocales(t *testing.T, repoRoot, lang, page string) map[string]string {
	t.Helper()

	merged := map[string]string{}
	for _, name := range []string{"common.json", "shared.json", page + ".json"} {
		path := filepath.Join(repoRoot, "web", "static", "locales", lang, name)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("Stat(%s) error = %v", path, err)
		}
		for key, value := range readLocaleFile(t, path) {
			merged[key] = value
		}
	}
	return merged
}

func localeFileNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", dir, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatalf("%s contains no locale JSON files", dir)
	}
	return names
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
	if len(locale) == 0 {
		t.Fatalf("%s is empty", path)
	}
	return locale
}

func readExtensionMessages(t *testing.T, path string) map[string]string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	var raw map[string]struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", path, err)
	}
	if len(raw) == 0 {
		t.Fatalf("%s is empty", path)
	}

	messages := make(map[string]string, len(raw))
	for key, entry := range raw {
		if strings.TrimSpace(entry.Message) == "" {
			t.Fatalf("%s key %q has an empty message", path, key)
		}
		messages[key] = entry.Message
	}
	return messages
}

func assertSameKeys(t *testing.T, label string, left, right map[string]string) {
	t.Helper()

	leftOnly := difference(sortedKeys(left), sortedKeys(right))
	rightOnly := difference(sortedKeys(right), sortedKeys(left))
	if len(leftOnly) == 0 && len(rightOnly) == 0 {
		return
	}
	t.Fatalf("%s keys differ\nleft only: %s\nright only: %s",
		label, strings.Join(leftOnly, ", "), strings.Join(rightOnly, ", "))
}

func uniqueStrings(matches [][]string) []string {
	seen := make(map[string]struct{})
	for _, match := range matches {
		if len(match) < 2 || match[1] == "" {
			continue
		}
		seen[match[1]] = struct{}{}
	}
	return sortedKeys(seen)
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func difference(left, right []string) []string {
	skip := make(map[string]struct{}, len(right))
	for _, value := range right {
		skip[value] = struct{}{}
	}
	var extra []string
	for _, value := range left {
		if _, ok := skip[value]; !ok {
			extra = append(extra, value)
		}
	}
	return extra
}

func testRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
