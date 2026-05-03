package ai_image_gen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

func TestClient_GenerateImage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Errorf("path: %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("auth: %q", got)
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "gpt-image-2" {
			t.Errorf("model: %v", body["model"])
		}
		if _, exists := body["response_format"]; exists {
			t.Errorf("response_format should be omitted for gpt-image-2 payload: %v", body)
		}
		if body["size"] != "1024x1024" || body["quality"] != "high" || body["n"].(float64) != 1 {
			t.Errorf("payload: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("PNGDATA")) + `"}]}`))
	}))
	defer srv.Close()

	c := NewClient(http.DefaultClient)
	got, err := c.Generate(context.Background(), model.AIImageGenSettings{
		APIKey: "sk-test", BaseURL: srv.URL, Model: "gpt-image-2",
		Size: "1024x1024", Quality: "high",
	}, "draw something")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if string(got) != "PNGDATA" {
		t.Fatalf("png bytes mismatch: %q", got)
	}
}

func TestClient_GenerateImage_PropagatesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"prompt rejected"}}`))
	}))
	defer srv.Close()

	c := NewClient(http.DefaultClient)
	_, err := c.Generate(context.Background(), model.AIImageGenSettings{
		APIKey: "sk-test", BaseURL: srv.URL, Model: "gpt-image-2",
		Size: "1024x1024", Quality: "high",
	}, "x")
	if err == nil || !strings.Contains(err.Error(), "prompt rejected") {
		t.Fatalf("expected prompt-rejected error, got %v", err)
	}
}
