package service

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func decodeBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, ","); strings.HasPrefix(value, "data:") && idx >= 0 {
		value = value[idx+1:]
	}

	if data, err := base64.StdEncoding.DecodeString(value); err == nil {
		return data, nil
	}
	if data, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		return data, nil
	}
	return nil, errors.New("无法解码提取图片的 base64 数据")
}

func normalizeExtractionStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "", "completed", "queued", "running", "failed", "cancelled":
		return status
	default:
		return status
	}
}

func normalizeExtractionMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", extractionModeAuto, extractionModeManual:
		return mode
	default:
		return mode
	}
}

func normalizeColor(color string) string {
	color = strings.TrimSpace(color)
	if color == "" {
		return "#A45C40"
	}
	if !strings.HasPrefix(color, "#") {
		color = "#" + color
	}
	if len(color) != 7 {
		return "#A45C40"
	}
	return strings.ToUpper(color)
}

func colorForName(name string) string {
	palette := []string{
		"#A45C40",
		"#7B8C5A",
		"#416788",
		"#C67B5C",
		"#6C4E80",
		"#B98B2F",
		"#3E7C6B",
	}
	sum := 0
	for _, r := range name {
		sum += int(r)
	}
	return palette[sum%len(palette)]
}

func removeFiles(paths []string) {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		_ = os.Remove(path)
	}
}

func clearDirectoryContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}

	return nil
}
