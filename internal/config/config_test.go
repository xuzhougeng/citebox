package config

import "testing"

func TestEffectiveExtractorURLFromBaseURL(t *testing.T) {
	cfg := &Config{ExtractorURL: "http://127.0.0.1:8000"}

	if got := cfg.EffectiveExtractorURL(); got != "http://127.0.0.1:8000/api/v1/extract" {
		t.Fatalf("unexpected extract url: %s", got)
	}
	if got := cfg.EffectiveExtractorJobsURL(); got != "http://127.0.0.1:8000/api/v1/jobs" {
		t.Fatalf("unexpected jobs url: %s", got)
	}
}

func TestEffectiveExtractorJobsURLOverrideFromBaseURL(t *testing.T) {
	cfg := &Config{
		ExtractorURL:     "http://127.0.0.1:8000/api/v1/extract",
		ExtractorJobsURL: "http://127.0.0.1:9000",
	}

	if got := cfg.EffectiveExtractorJobsURL(); got != "http://127.0.0.1:9000/api/v1/jobs" {
		t.Fatalf("unexpected overridden jobs url: %s", got)
	}
}

func TestEffectiveExtractorURLKeepsExplicitEndpoint(t *testing.T) {
	cfg := &Config{ExtractorURL: "http://127.0.0.1:8000/api/analyze"}

	if got := cfg.EffectiveExtractorURL(); got != "http://127.0.0.1:8000/api/analyze" {
		t.Fatalf("unexpected explicit extract url: %s", got)
	}
	if got := cfg.EffectiveExtractorJobsURL(); got != "" {
		t.Fatalf("unexpected jobs url for legacy endpoint: %s", got)
	}
}
