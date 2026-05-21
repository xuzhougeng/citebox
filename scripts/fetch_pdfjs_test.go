package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectedPathIncludesOfficialViewerAssets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tarPath  string
		wantPath string
		wantOK   bool
	}{
		{name: "license", tarPath: "package/LICENSE", wantPath: "LICENSE", wantOK: true},
		{name: "modern build", tarPath: "package/build/pdf.mjs", wantPath: "build/pdf.mjs", wantOK: true},
		{name: "modern worker", tarPath: "package/build/pdf.worker.mjs", wantPath: "build/pdf.worker.mjs", wantOK: true},
		{name: "legacy build remains available", tarPath: "package/legacy/build/pdf.min.mjs", wantPath: "legacy/build/pdf.min.mjs", wantOK: true},
		{name: "legacy worker remains available", tarPath: "package/legacy/build/pdf.worker.min.mjs", wantPath: "legacy/build/pdf.worker.min.mjs", wantOK: true},
		{name: "official viewer module", tarPath: "package/web/pdf_viewer.mjs", wantPath: "web/pdf_viewer.mjs", wantOK: true},
		{name: "official viewer css", tarPath: "package/web/pdf_viewer.css", wantPath: "web/pdf_viewer.css", wantOK: true},
		{name: "viewer image asset", tarPath: "package/web/images/loading-icon.gif", wantPath: "web/images/loading-icon.gif", wantOK: true},
		{name: "cmaps", tarPath: "package/cmaps/78-EUC-H.bcmap", wantPath: "cmaps/78-EUC-H.bcmap", wantOK: true},
		{name: "unrelated web app file", tarPath: "package/web/viewer.html", wantPath: "", wantOK: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotPath, gotOK := selectedPath(tt.tarPath)
			if gotOK != tt.wantOK {
				t.Fatalf("selectedPath(%q) ok = %v, want %v", tt.tarPath, gotOK, tt.wantOK)
			}
			if gotPath != tt.wantPath {
				t.Fatalf("selectedPath(%q) path = %q, want %q", tt.tarPath, gotPath, tt.wantPath)
			}
		})
	}
}

func TestAssetsReadyRequiresOfficialViewerAssets(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	required := []string{
		"LICENSE",
		"build/pdf.mjs",
		"build/pdf.worker.mjs",
		"legacy/build/pdf.min.mjs",
		"legacy/build/pdf.worker.min.mjs",
		"web/pdf_viewer.mjs",
		"web/pdf_viewer.css",
		"web/images/loading-icon.gif",
		"cmaps/LICENSE",
		"standard_fonts/LiberationSans-Regular.ttf",
		"wasm/qcms_bg.wasm",
	}

	for _, relative := range required {
		fullPath := filepath.Join(targetDir, relative)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("asset"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ready, err := assetsReady(targetDir)
	if err != nil {
		t.Fatalf("assetsReady returned error: %v", err)
	}
	if !ready {
		t.Fatal("assetsReady returned false with every required asset present")
	}

	if err := os.Remove(filepath.Join(targetDir, "web/pdf_viewer.css")); err != nil {
		t.Fatal(err)
	}
	ready, err = assetsReady(targetDir)
	if err != nil {
		t.Fatalf("assetsReady returned error after removing css: %v", err)
	}
	if ready {
		t.Fatal("assetsReady returned true without web/pdf_viewer.css")
	}
}
