//go:build windows

package desktopruntime

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var windowsFilenameSanitizer = strings.NewReplacer(
	"<", "_",
	">", "_",
	":", "_",
	"\"", "_",
	"/", "_",
	"\\", "_",
	"|", "_",
	"?", "_",
	"*", "_",
)

func saveFile(filename string, dataBase64 string) (bool, error) {
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return false, fmt.Errorf("decode file data: %w", err)
	}

	downloadsDir, err := windowsDownloadsDir()
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(downloadsDir, 0o755); err != nil {
		return false, fmt.Errorf("prepare downloads directory: %w", err)
	}

	targetPath := nextAvailableWindowsDownloadPath(filepath.Join(downloadsDir, sanitizeWindowsFilename(filename)))
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		return false, fmt.Errorf("save file: %w", err)
	}

	return true, nil
}

func windowsDownloadsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(homeDir, "Downloads"), nil
}

func sanitizeWindowsFilename(filename string) string {
	name := strings.TrimSpace(filename)
	if name == "" {
		return "download.bin"
	}

	name = filepath.Base(name)
	name = windowsFilenameSanitizer.Replace(name)
	name = strings.Trim(name, ". ")
	if name == "" {
		return "download.bin"
	}

	stem := strings.TrimSuffix(name, filepath.Ext(name))
	switch strings.ToUpper(stem) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return "_" + name
	default:
		return name
	}
}

func nextAvailableWindowsDownloadPath(targetPath string) string {
	if _, err := os.Stat(targetPath); errors.Is(err, os.ErrNotExist) {
		return targetPath
	}

	dir := filepath.Dir(targetPath)
	ext := filepath.Ext(targetPath)
	base := strings.TrimSuffix(filepath.Base(targetPath), ext)

	for index := 1; ; index++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, index, ext))
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}
