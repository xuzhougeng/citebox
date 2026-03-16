package config

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ServerPort              string
	UploadDir               string
	StorageDir              string
	MaxUploadSize           int64
	DatabasePath            string
	AdminUsername           string
	AdminPassword           string
	AllowedTypes            []string
	ExtractorURL            string
	ExtractorJobsURL        string
	ExtractorToken          string
	ExtractorFileField      string
	ExtractorTimeoutSeconds int
	ExtractorPollInterval   int
}

func Load() *Config {
	return &Config{
		ServerPort:              getEnv("SERVER_PORT", "8080"),
		UploadDir:               getEnv("UPLOAD_DIR", "./uploads"),
		StorageDir:              getEnv("STORAGE_DIR", "./data/library"),
		MaxUploadSize:           getEnvInt64("MAX_UPLOAD_SIZE", 250*1024*1024),
		DatabasePath:            getEnv("DATABASE_PATH", "./data/library.db"),
		AdminUsername:           getEnv("ADMIN_USERNAME", "wanglab"),
		AdminPassword:           getEnv("ADMIN_PASSWORD", "wanglab789"),
		AllowedTypes:            []string{"application/pdf"},
		ExtractorURL:            getEnv("PDF_EXTRACTOR_URL", ""),
		ExtractorJobsURL:        getEnv("PDF_EXTRACTOR_JOBS_URL", ""),
		ExtractorToken:          getEnv("PDF_EXTRACTOR_TOKEN", ""),
		ExtractorFileField:      getEnv("PDF_EXTRACTOR_FILE_FIELD", "file"),
		ExtractorTimeoutSeconds: getEnvInt("PDF_EXTRACTOR_TIMEOUT_SECONDS", 300),
		ExtractorPollInterval:   getEnvInt("PDF_EXTRACTOR_POLL_INTERVAL_SECONDS", 2),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func (c *Config) IsAllowedType(contentType string) bool {
	contentType = strings.TrimSpace(strings.ToLower(contentType))
	for _, t := range c.AllowedTypes {
		if strings.ToLower(t) == contentType {
			return true
		}
	}
	return false
}

func (c *Config) PapersDir() string {
	return filepath.Join(c.StorageDir, "papers")
}

func (c *Config) FiguresDir() string {
	return filepath.Join(c.StorageDir, "figures")
}

func (c *Config) EffectiveExtractorURL() string {
	return normalizeExtractorEndpoint(c.ExtractorURL, "/api/v1/extract")
}

func (c *Config) EffectiveExtractorJobsURL() string {
	if value := strings.TrimSpace(c.ExtractorJobsURL); value != "" {
		return normalizeExtractorEndpoint(value, "/api/v1/jobs")
	}

	extractURL := strings.TrimSpace(c.EffectiveExtractorURL())
	if extractURL == "" {
		return ""
	}

	parsed, err := url.Parse(extractURL)
	if err != nil {
		return ""
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/api/v1/jobs"
		return parsed.String()
	}
	if strings.HasSuffix(parsed.Path, "/api/v1/jobs") {
		return parsed.String()
	}
	if strings.HasSuffix(parsed.Path, "/api/v1/extract") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/api/v1/extract") + "/api/v1/jobs"
		return parsed.String()
	}
	if !strings.HasSuffix(parsed.Path, "/extract") {
		return ""
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/extract") + "/jobs"
	return parsed.String()
}

func normalizeExtractorEndpoint(rawURL, defaultPath string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	switch {
	case parsed.Path == "", parsed.Path == "/":
		parsed.Path = defaultPath
	case defaultPath == "/api/v1/extract" && strings.HasSuffix(parsed.Path, "/api/v1/jobs"):
		parsed.Path = strings.TrimSuffix(parsed.Path, "/api/v1/jobs") + "/api/v1/extract"
	case defaultPath == "/api/v1/jobs" && strings.HasSuffix(parsed.Path, "/api/v1/extract"):
		parsed.Path = strings.TrimSuffix(parsed.Path, "/api/v1/extract") + "/api/v1/jobs"
	}

	return parsed.String()
}
