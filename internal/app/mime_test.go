package app

import (
	"mime"
	"testing"
)

func TestRegisterWebAssetMIMETypes(t *testing.T) {
	if err := registerWebAssetMIMETypes(); err != nil {
		t.Fatalf("registerWebAssetMIMETypes() error = %v", err)
	}

	if got := mime.TypeByExtension(".mjs"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("mime.TypeByExtension(.mjs) = %q, want %q", got, "text/javascript; charset=utf-8")
	}

	if got := mime.TypeByExtension(".wasm"); got != "application/wasm" {
		t.Fatalf("mime.TypeByExtension(.wasm) = %q, want %q", got, "application/wasm")
	}
}
