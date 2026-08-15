package zotero

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func ResolveLocalFilePath(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", os.ErrNotExist
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(parsed.Scheme, "file") {
			return "", os.ErrInvalid
		}
		value = fileURLPath(parsed)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", os.ErrNotExist
	}

	candidates := uniqueNonEmpty([]string{
		value,
		windowsPathToWSL(value),
	})
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	if mapped := windowsPathToWSL(value); mapped != "" {
		return mapped, os.ErrNotExist
	}
	return value, os.ErrNotExist
}

func fileURLPath(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	path := parsed.Path
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") && parsed.Host != "127.0.0.1" {
		// file://E:/foo or file://hostname/E:/foo
		if len(parsed.Host) == 1 {
			path = parsed.Host + ":" + path
		} else {
			path = "//" + parsed.Host + path
		}
	}
	if runtime.GOOS != "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		// /E:/foo
		path = path[1:]
	}
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	return strings.ReplaceAll(path, "/", string(filepath.Separator))
}

func windowsPathToWSL(path string) string {
	cleaned := strings.ReplaceAll(strings.TrimSpace(path), "/", "\\")
	if len(cleaned) < 3 || cleaned[1] != ':' {
		return ""
	}
	drive := strings.ToLower(string(cleaned[0]))
	if drive < "a" || drive > "z" {
		return ""
	}
	rest := strings.TrimPrefix(cleaned[2:], "\\")
	rest = strings.ReplaceAll(rest, "\\", "/")
	return "/mnt/" + drive + "/" + rest
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
