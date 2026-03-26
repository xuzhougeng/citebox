package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xuzhougeng/citebox/internal/model"
)

const (
	doubaoTTSEndpoint          = "https://openspeech.bytedance.com/api/v3/tts/unidirectional/sse"
	doubaoTTSDefaultResourceID = "seed-tts-2.0"
	doubaoTTSDefaultFormat     = "mp3"
	doubaoTTSDefaultSampleRate = 24000
	doubaoTTSReplyDirName      = "tts-replies"
)

var doubaoTTSHTTPClient = &http.Client{Timeout: 90 * time.Second}

type doubaoTTSSettings struct {
	AppID      string
	AccessKey  string
	ResourceID string
	Speaker    string
	Endpoint   string
}

type doubaoTTSRequest struct {
	User      doubaoTTSUser      `json:"user,omitempty"`
	Namespace string             `json:"namespace,omitempty"`
	ReqParams doubaoTTSReqParams `json:"req_params"`
}

type doubaoTTSUser struct {
	UID string `json:"uid,omitempty"`
}

type doubaoTTSReqParams struct {
	Text        string               `json:"text"`
	Speaker     string               `json:"speaker"`
	AudioParams doubaoTTSAudioParams `json:"audio_params"`
}

type doubaoTTSAudioParams struct {
	Format     string `json:"format"`
	SampleRate int    `json:"sample_rate"`
}

type doubaoTTSEvent struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Data    *string `json:"data"`
}

func synthesizeDoubaoTTSFile(ctx context.Context, stateDir, text, uid string, settings doubaoTTSSettings) (string, func(), error) {
	audio, extension, err := synthesizeDoubaoTTSAudio(ctx, doubaoTTSHTTPClient, settings, text, uid)
	if err != nil {
		return "", nil, err
	}
	if len(audio) == 0 {
		return "", nil, fmt.Errorf("doubao tts returned empty audio")
	}

	replyDir := filepath.Join(stateDir, doubaoTTSReplyDirName)
	if err := os.MkdirAll(replyDir, 0o755); err != nil {
		return "", nil, err
	}

	pattern := "ask-reply-*"
	if extension != "" {
		pattern += extension
	}
	file, err := os.CreateTemp(replyDir, pattern)
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		_ = os.Remove(file.Name())
	}
	if _, err := file.Write(audio); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return file.Name(), cleanup, nil
}

func synthesizeDoubaoTTSAudio(ctx context.Context, httpClient *http.Client, settings doubaoTTSSettings, text, uid string) ([]byte, string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, "", fmt.Errorf("tts text is empty")
	}
	if !settings.Enabled() {
		return nil, "", fmt.Errorf("doubao tts is not configured")
	}
	if httpClient == nil {
		httpClient = doubaoTTSHTTPClient
	}

	payload := doubaoTTSRequest{
		User:      doubaoTTSUser{UID: firstNonEmpty(strings.TrimSpace(uid), "citebox-weixin")},
		Namespace: "BidirectionalTTS",
		ReqParams: doubaoTTSReqParams{
			Text:    text,
			Speaker: settings.Speaker,
			AudioParams: doubaoTTSAudioParams{
				Format:     doubaoTTSDefaultFormat,
				SampleRate: doubaoTTSDefaultSampleRate,
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("marshal doubao tts request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("create doubao tts request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-App-Id", settings.AppID)
	req.Header.Set("X-Api-Access-Key", settings.AccessKey)
	req.Header.Set("X-Api-Resource-Id", settings.ResourceID)
	req.Header.Set("X-Api-Request-Id", uuid.NewString())

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("doubao tts request failed: %w", err)
	}
	defer resp.Body.Close()
	logID := strings.TrimSpace(resp.Header.Get("X-Tt-Logid"))

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		if logID != "" {
			return nil, "", fmt.Errorf("doubao tts HTTP %d (logid=%s): %s", resp.StatusCode, logID, strings.TrimSpace(string(responseBody)))
		}
		return nil, "", fmt.Errorf("doubao tts HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var audio bytes.Buffer
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		rawJSON := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if rawJSON == "" {
			continue
		}

		var event doubaoTTSEvent
		if err := json.Unmarshal([]byte(rawJSON), &event); err != nil {
			if logID != "" {
				return nil, "", fmt.Errorf("parse doubao tts event (logid=%s): %w", logID, err)
			}
			return nil, "", fmt.Errorf("parse doubao tts event: %w", err)
		}

		if event.Code != 0 && event.Code != 20000000 {
			if logID != "" {
				return nil, "", fmt.Errorf("doubao tts API %d (logid=%s): %s", event.Code, logID, strings.TrimSpace(event.Message))
			}
			return nil, "", fmt.Errorf("doubao tts API %d: %s", event.Code, strings.TrimSpace(event.Message))
		}
		if event.Data == nil || strings.TrimSpace(*event.Data) == "" {
			continue
		}

		chunk, err := base64.StdEncoding.DecodeString(*event.Data)
		if err != nil {
			if logID != "" {
				return nil, "", fmt.Errorf("decode doubao tts audio chunk (logid=%s): %w", logID, err)
			}
			return nil, "", fmt.Errorf("decode doubao tts audio chunk: %w", err)
		}
		audio.Write(chunk)
	}
	if err := scanner.Err(); err != nil {
		if logID != "" {
			return nil, "", fmt.Errorf("read doubao tts stream (logid=%s): %w", logID, err)
		}
		return nil, "", fmt.Errorf("read doubao tts stream: %w", err)
	}
	if audio.Len() == 0 {
		if logID != "" {
			return nil, "", fmt.Errorf("doubao tts returned no audio chunks (logid=%s)", logID)
		}
		return nil, "", fmt.Errorf("doubao tts returned no audio chunks")
	}

	return audio.Bytes(), "." + doubaoTTSDefaultFormat, nil
}

func newDoubaoTTSSettings(settings model.TTSSettings) doubaoTTSSettings {
	resourceID := strings.TrimSpace(settings.ResourceID)
	if resourceID == "" {
		resourceID = doubaoTTSDefaultResourceID
	}

	return doubaoTTSSettings{
		AppID:      strings.TrimSpace(settings.AppID),
		AccessKey:  strings.TrimSpace(settings.AccessKey),
		ResourceID: resourceID,
		Speaker:    strings.TrimSpace(settings.Speaker),
		Endpoint:   doubaoTTSEndpoint,
	}
}

func (s doubaoTTSSettings) Enabled() bool {
	return strings.TrimSpace(s.AppID) != "" &&
		strings.TrimSpace(s.AccessKey) != "" &&
		strings.TrimSpace(s.ResourceID) != "" &&
		strings.TrimSpace(s.Speaker) != "" &&
		strings.TrimSpace(s.Endpoint) != ""
}
