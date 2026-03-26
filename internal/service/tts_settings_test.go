package service

import (
	"context"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func TestLibraryServiceTestTTSUsesCurrentInput(t *testing.T) {
	svc, _, _ := newTestService(t)

	var gotText string
	var gotSettings model.TTSSettings
	svc.ttsAudioSynthesizer = func(_ context.Context, text string, settings model.TTSSettings) ([]byte, string, error) {
		gotText = text
		gotSettings = settings
		return []byte("audio-data"), ".m4a", nil
	}

	audio, filename, contentType, err := svc.TestTTS(context.Background(), model.TTSSettings{
		AppID:     "app-id",
		AccessKey: "access-key",
		Speaker:   "speaker-id",
	})
	if err != nil {
		t.Fatalf("TestTTS() error = %v", err)
	}
	if string(audio) != "audio-data" {
		t.Fatalf("audio = %q, want %q", string(audio), "audio-data")
	}
	if filename != "tts-test.m4a" {
		t.Fatalf("filename = %q, want %q", filename, "tts-test.m4a")
	}
	if contentType != "audio/mp4" {
		t.Fatalf("contentType = %q, want %q", contentType, "audio/mp4")
	}
	if gotText != ttsTestDemoText {
		t.Fatalf("text = %q, want %q", gotText, ttsTestDemoText)
	}
	if gotSettings.ResourceID != doubaoTTSDefaultResourceID {
		t.Fatalf("ResourceID = %q, want %q", gotSettings.ResourceID, doubaoTTSDefaultResourceID)
	}
}

func TestLibraryServiceTestTTSRequiresConfig(t *testing.T) {
	svc, _, _ := newTestService(t)

	audio, filename, contentType, err := svc.TestTTS(context.Background(), model.TTSSettings{})
	if !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("error code = %q, want %q (err=%v)", apperr.CodeOf(err), apperr.CodeInvalidArgument, err)
	}
	if len(audio) != 0 || filename != "" || contentType != "" {
		t.Fatalf("unexpected return values: audio=%q filename=%q contentType=%q", string(audio), filename, contentType)
	}
}
