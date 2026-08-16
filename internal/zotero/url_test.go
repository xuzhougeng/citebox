package zotero

import "testing"

func TestParseAndValidateBaseURLAcceptsLoopback(t *testing.T) {
	got, err := NormalizeBaseURL("http://localhost:23119/api/")
	if err != nil {
		t.Fatalf("NormalizeBaseURL() error = %v", err)
	}
	if got != "http://localhost:23119/api" {
		t.Fatalf("NormalizeBaseURL() = %q", got)
	}
}

func TestParseAndValidateBaseURLRejectsRemote(t *testing.T) {
	if _, err := NormalizeBaseURL("http://example.com:23119/api"); err == nil {
		t.Fatal("expected remote URL to be rejected")
	}
}

func TestParseAndValidateBaseURLDefault(t *testing.T) {
	got, err := NormalizeBaseURL("   ")
	if err != nil {
		t.Fatalf("NormalizeBaseURL() error = %v", err)
	}
	if got != DefaultBaseURL {
		t.Fatalf("NormalizeBaseURL() = %q, want %q", got, DefaultBaseURL)
	}
}
