package service

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

func TestSynthesizeDoubaoTTSAudioReadsSSEChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/tts/unidirectional/sse" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/v3/tts/unidirectional/sse")
		}
		if got := r.Header.Get("X-Api-App-Id"); got != "app-id" {
			t.Fatalf("X-Api-App-Id = %q, want %q", got, "app-id")
		}
		if got := r.Header.Get("X-Api-Access-Key"); got != "access-key" {
			t.Fatalf("X-Api-Access-Key = %q, want %q", got, "access-key")
		}
		if got := r.Header.Get("X-Api-Resource-Id"); got != "seed-tts-2.0" {
			t.Fatalf("X-Api-Resource-Id = %q, want %q", got, "seed-tts-2.0")
		}

		var body doubaoTTSRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.ReqParams.Text != "请朗读这段回答" {
			t.Fatalf("req_params.text = %q, want %q", body.ReqParams.Text, "请朗读这段回答")
		}
		if body.ReqParams.Speaker != "speaker-id" {
			t.Fatalf("req_params.speaker = %q, want %q", body.ReqParams.Speaker, "speaker-id")
		}
		if body.ReqParams.AudioParams.Format != doubaoTTSDefaultFormat {
			t.Fatalf("audio_params.format = %q, want %q", body.ReqParams.AudioParams.Format, doubaoTTSDefaultFormat)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: 352\n"))
		_, _ = w.Write([]byte("data: {\"code\":0,\"message\":\"chunk\",\"data\":\"" + base64.StdEncoding.EncodeToString([]byte("hello ")) + "\"}\n\n"))
		_, _ = w.Write([]byte("event: 352\n"))
		_, _ = w.Write([]byte("data: {\"code\":0,\"message\":\"chunk\",\"data\":\"" + base64.StdEncoding.EncodeToString([]byte("world")) + "\"}\n\n"))
		_, _ = w.Write([]byte("event: 152\n"))
		_, _ = w.Write([]byte("data: {\"code\":20000000,\"message\":\"finished\",\"data\":null}\n\n"))
	}))
	defer server.Close()

	settings := newDoubaoTTSSettings(model.TTSSettings{
		AppID:      "app-id",
		AccessKey:  "access-key",
		ResourceID: "seed-tts-2.0",
		Speaker:    "speaker-id",
	})
	settings.Endpoint = server.URL + "/api/v3/tts/unidirectional/sse"

	audio, extension, err := synthesizeDoubaoTTSAudio(context.Background(), server.Client(), settings, "请朗读这段回答", "user@im.wechat")
	if err != nil {
		t.Fatalf("synthesizeDoubaoTTSAudio() error = %v", err)
	}
	if extension != ".mp3" {
		t.Fatalf("extension = %q, want %q", extension, ".mp3")
	}
	if got := string(audio); got != "hello world" {
		t.Fatalf("audio = %q, want %q", got, "hello world")
	}
}

func TestSynthesizeDoubaoTTSAudioRequiresConfiguredFields(t *testing.T) {
	audio, extension, err := synthesizeDoubaoTTSAudio(context.Background(), nil, doubaoTTSSettings{}, "hello", "user")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("error = %v, want not configured error", err)
	}
	if len(audio) != 0 || extension != "" {
		t.Fatalf("audio/extension = %q/%q, want empty values", string(audio), extension)
	}
}
