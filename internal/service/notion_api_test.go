package service

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestNotionAPITokenSettingsDoNotExposeCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/users/me" || r.Header.Get("Authorization") != "Bearer ntn_test_secret" {
			t.Fatalf("unexpected request: %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		writeJSON(t, w, map[string]any{"id": "user-1", "name": "CiteBox Tester"})
	}))
	defer server.Close()

	service, cleanup := newTestNotionAPIService(t, server)
	defer cleanup()
	status, err := service.SaveToken(context.Background(), "ntn_test_secret")
	if err != nil || !status.Success || status.UserID != "user-1" {
		t.Fatalf("SaveToken() status=%#v err=%v", status, err)
	}
	view, err := service.GetSettings()
	if err != nil || !view.Configured || view.UserName != "CiteBox Tester" {
		t.Fatalf("GetSettings() view=%#v err=%v", view, err)
	}
	encoded, _ := json.Marshal(view)
	if bytes.Contains(encoded, []byte("ntn_test_secret")) || bytes.Contains(encoded, []byte("token")) {
		t.Fatalf("settings response leaked credential: %s", encoded)
	}
	raw, err := os.ReadFile(service.credentialPath)
	if err != nil || !bytes.Contains(raw, []byte("ntn_test_secret")) {
		t.Fatalf("credential file was not written correctly: err=%v body=%s", err, raw)
	}
	if info, err := os.Stat(service.credentialPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("credential mode=%v err=%v", info.Mode().Perm(), err)
	}
	if _, err := service.TestToken(context.Background(), ""); err != nil {
		t.Fatalf("TestToken(saved) error=%v", err)
	}
	if err := service.DeleteToken(); err != nil {
		t.Fatal(err)
	}
	view, err = service.GetSettings()
	if err != nil || view.Configured {
		t.Fatalf("settings after delete=%#v err=%v", view, err)
	}
}

func TestSaveFigureNoteToNotionUploadsOriginalAndGroupsByPaper(t *testing.T) {
	var server *httptest.Server
	var uploadCount, createPageCount, appendCount int
	var uploadedBodies [][]byte
	var appendedPayload map[string]any
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Notion-Version") != notionAPIVersion {
			t.Fatalf("Notion-Version=%q", r.Header.Get("Notion-Version"))
		}
		switch {
		case r.URL.Path == "/v1/users/me":
			writeJSON(t, w, map[string]any{"id": "user-native", "name": "Native User"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/file_uploads":
			uploadCount++
			writeJSON(t, w, map[string]any{
				"id":         "upload-" + string(rune('0'+uploadCount)),
				"upload_url": server.URL + "/v1/file_uploads/upload-" + string(rune('0'+uploadCount)) + "/send",
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/send"):
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			body, _ := io.ReadAll(file)
			uploadedBodies = append(uploadedBodies, body)
			if header.Header.Get("Content-Type") != "image/png" {
				t.Fatalf("upload content type=%q", header.Header.Get("Content-Type"))
			}
			writeJSON(t, w, map[string]any{"id": filepath.Base(filepath.Dir(r.URL.Path)), "status": "uploaded"})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/pages/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/pages/")
			writeJSON(t, w, map[string]any{"id": id, "url": "https://notion.so/" + id, "in_trash": false})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pages":
			createPageCount++
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			if createPageCount == 1 {
				if _, exists := payload["parent"]; exists {
					t.Fatalf("private root page unexpectedly has parent: %#v", payload["parent"])
				}
			} else {
				id = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
				parent := payload["parent"].(map[string]any)
				if parent["page_id"] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
					t.Fatalf("paper parent=%#v", parent)
				}
				assertNativeImageBlock(t, payload, "upload-1")
			}
			writeJSON(t, w, map[string]any{"id": id, "url": "https://notion.so/" + id})
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/blocks/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/children":
			appendCount++
			if err := json.NewDecoder(r.Body).Decode(&appendedPayload); err != nil {
				t.Fatal(err)
			}
			assertNativeImageBlock(t, appendedPayload, "upload-2")
			writeJSON(t, w, map[string]any{"object": "list", "results": []any{}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	service, cleanup := newTestNotionAPIService(t, server)
	defer cleanup()
	if _, err := service.SaveToken(context.Background(), "ntn_native"); err != nil {
		t.Fatal(err)
	}
	imageData := tinyPNG(t)
	figure := &model.FigureListItem{
		ID: 7, PaperID: 42, PaperTitle: "Atlas Study", Filename: "figure.png",
		PageNumber: 3, FigureIndex: 2, DisplayLabel: "Fig 2", Caption: "Signal increases.",
		Tags: []model.Tag{{Name: "astrocyte"}},
	}
	first, err := service.SaveFigureNoteToNotion(context.Background(), figure, imageData, "image/png", "# First note")
	if err != nil {
		t.Fatal(err)
	}
	figure.ID = 8
	figure.DisplayLabel = "Fig 3"
	second, err := service.SaveFigureNoteToNotion(context.Background(), figure, imageData, "image/png", "second note")
	if err != nil {
		t.Fatal(err)
	}
	if first.TargetPageID != second.TargetPageID || first.TargetPageID != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	if uploadCount != 2 || createPageCount != 2 || appendCount != 1 {
		t.Fatalf("uploads=%d creates=%d appends=%d", uploadCount, createPageCount, appendCount)
	}
	if len(uploadedBodies) != 2 || !bytes.Equal(uploadedBodies[0], imageData) || !bytes.Equal(uploadedBodies[1], imageData) {
		t.Fatal("uploaded image bytes differ from the original")
	}
	if !strings.Contains(mustJSON(appendedPayload), "second note") {
		t.Fatalf("appended payload=%s", mustJSON(appendedPayload))
	}
}

func TestSaveFigureNoteToNotionRebuildsTrashedExportTreeBeforeAppending(t *testing.T) {
	var server *httptest.Server
	var createCalls int
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/users/me":
			writeJSON(t, w, map[string]any{"id": "user-retry", "name": "Retry User"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/file_uploads":
			writeJSON(t, w, map[string]any{"id": "upload-retry", "upload_url": server.URL + "/upload/send"})
		case r.Method == http.MethodPost && r.URL.Path == "/upload/send":
			writeJSON(t, w, map[string]any{"id": "upload-retry", "status": "uploaded"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/pages/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa":
			writeJSON(t, w, map[string]any{
				"id":       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"url":      "https://notion.so/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"in_trash": true,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/pages":
			createCalls++
			id := "cccccccccccccccccccccccccccccccc"
			if createCalls == 2 {
				id = "dddddddddddddddddddddddddddddddd"
			}
			writeJSON(t, w, map[string]any{"id": id, "url": "https://notion.so/" + id})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	service, cleanup := newTestNotionAPIService(t, server)
	defer cleanup()
	if _, err := service.SaveToken(context.Background(), "ntn_retry"); err != nil {
		t.Fatal(err)
	}
	state := notionAPIExportState{Users: map[string]notionAPIUserExportState{
		"user-retry": {
			Root:   notionAPIPageRef{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			Papers: map[string]notionAPIPageRef{"42": {ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
		},
	}}
	if err := service.saveExportState(state); err != nil {
		t.Fatal(err)
	}
	figure := &model.FigureListItem{ID: 7, PaperID: 42, PaperTitle: "Atlas Study", Filename: "figure.png", PageNumber: 3}
	result, err := service.SaveFigureNoteToNotion(context.Background(), figure, tinyPNG(t), "image/png", "recovered")
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetPageID != "dddddddddddddddddddddddddddddddd" || createCalls != 2 {
		t.Fatalf("result=%#v create=%d", result, createCalls)
	}
}

func assertNativeImageBlock(t *testing.T, payload map[string]any, uploadID string) {
	t.Helper()
	children, ok := payload["children"].([]any)
	if !ok {
		t.Fatalf("children missing: %#v", payload)
	}
	for _, child := range children {
		block, _ := child.(map[string]any)
		if block["type"] != "image" {
			continue
		}
		imageValue := block["image"].(map[string]any)
		fileUpload := imageValue["file_upload"].(map[string]any)
		if imageValue["type"] == "file_upload" && fileUpload["id"] == uploadID {
			return
		}
	}
	t.Fatalf("native image block for %q not found: %s", uploadID, mustJSON(payload))
}

func newTestNotionAPIService(t *testing.T, server *httptest.Server) (*NotionAPIService, func()) {
	t.Helper()
	root := t.TempDir()
	repo, err := repository.NewLibraryRepository(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewNotionAPIService(repo.Setting, filepath.Join(root, "storage"))
	service.baseURL = server.URL
	service.httpClient = server.Client()
	return service, func() { _ = repo.Close() }
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func writeNotionError(t *testing.T, w http.ResponseWriter, status int, code, message string) {
	t.Helper()
	w.WriteHeader(status)
	writeJSON(t, w, map[string]any{"code": code, "message": message})
}

func mustJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 50), G: uint8(y * 50), B: 100, A: 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
