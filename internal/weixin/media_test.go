package weixin

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadBufferToCDNEncryptsPayloadAndReturnsDownloadParam(t *testing.T) {
	key := []byte("0123456789abcdef")
	plaintext := []byte("preview image bytes")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/upload" {
			t.Fatalf("path = %q, want /upload", got)
		}
		if got := r.URL.Query().Get("encrypted_query_param"); got != "upload-token" {
			t.Fatalf("encrypted_query_param = %q, want %q", got, "upload-token")
		}
		if got := r.URL.Query().Get("filekey"); got != "file-key" {
			t.Fatalf("filekey = %q, want %q", got, "file-key")
		}

		ciphertext, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		decrypted, err := decryptAESECB(ciphertext, key)
		if err != nil {
			t.Fatalf("decryptAESECB() error = %v", err)
		}
		if string(decrypted) != string(plaintext) {
			t.Fatalf("decrypted = %q, want %q", string(decrypted), string(plaintext))
		}

		w.Header().Set("x-encrypted-param", "download-token")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient("https://example.invalid", "token", server.Client())
	downloadParam, err := client.uploadBufferToCDN(context.Background(), plaintext, "upload-token", "file-key", key, server.URL)
	if err != nil {
		t.Fatalf("uploadBufferToCDN() error = %v", err)
	}
	if downloadParam != "download-token" {
		t.Fatalf("downloadParam = %q, want %q", downloadParam, "download-token")
	}
}

func TestSendUploadedImagePostsImageMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/ilink/bot/sendmessage" {
			t.Fatalf("path = %q, want /ilink/bot/sendmessage", got)
		}

		var body SendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.Msg.ToUserID != "user@im.wechat" {
			t.Fatalf("to_user_id = %q, want %q", body.Msg.ToUserID, "user@im.wechat")
		}
		if body.Msg.ContextToken != "context-token" {
			t.Fatalf("context_token = %q, want %q", body.Msg.ContextToken, "context-token")
		}
		if len(body.Msg.ItemList) != 1 || body.Msg.ItemList[0].Type != ItemTypeImage || body.Msg.ItemList[0].ImageItem == nil {
			t.Fatalf("item_list = %+v, want one image item", body.Msg.ItemList)
		}
		media := body.Msg.ItemList[0].ImageItem.Media
		if media == nil {
			t.Fatal("image_item.media = nil, want populated media")
		}
		if media.EncryptQueryParam != "download-token" {
			t.Fatalf("encrypt_query_param = %q, want %q", media.EncryptQueryParam, "download-token")
		}
		decoded, err := base64.StdEncoding.DecodeString(media.AESKey)
		if err != nil {
			t.Fatalf("DecodeString() error = %v", err)
		}
		if got := strings.TrimSpace(string(decoded)); got != hex.EncodeToString([]byte("0123456789abcdef")) {
			t.Fatalf("decoded aes_key = %q, want %q", got, hex.EncodeToString([]byte("0123456789abcdef")))
		}
		if body.Msg.ItemList[0].ImageItem.MidSize != 48 {
			t.Fatalf("mid_size = %d, want %d", body.Msg.ItemList[0].ImageItem.MidSize, 48)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", server.Client())
	err := client.SendUploadedImage(context.Background(), "user@im.wechat", &uploadedImage{
		DownloadEncryptedQueryParam: "download-token",
		AESKey:                      []byte("0123456789abcdef"),
		FileSize:                    32,
		FileSizeCiphertext:          48,
	}, "context-token")
	if err != nil {
		t.Fatalf("SendUploadedImage() error = %v", err)
	}
}

func TestSendUploadedVoicePostsVoiceMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/ilink/bot/sendmessage" {
			t.Fatalf("path = %q, want /ilink/bot/sendmessage", got)
		}

		var body SendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.Msg.ToUserID != "user@im.wechat" {
			t.Fatalf("to_user_id = %q, want %q", body.Msg.ToUserID, "user@im.wechat")
		}
		if body.Msg.ContextToken != "context-token" {
			t.Fatalf("context_token = %q, want %q", body.Msg.ContextToken, "context-token")
		}
		if len(body.Msg.ItemList) != 1 || body.Msg.ItemList[0].Type != ItemTypeVoice || body.Msg.ItemList[0].VoiceItem == nil {
			t.Fatalf("item_list = %+v, want one voice item", body.Msg.ItemList)
		}
		voice := body.Msg.ItemList[0].VoiceItem
		if voice.Media == nil {
			t.Fatal("voice_item.media = nil, want populated media")
		}
		if voice.Media.EncryptQueryParam != "voice-download-token" {
			t.Fatalf("encrypt_query_param = %q, want %q", voice.Media.EncryptQueryParam, "voice-download-token")
		}
		decoded, err := base64.StdEncoding.DecodeString(voice.Media.AESKey)
		if err != nil {
			t.Fatalf("DecodeString() error = %v", err)
		}
		if got := strings.TrimSpace(string(decoded)); got != hex.EncodeToString([]byte("0123456789abcdef")) {
			t.Fatalf("decoded aes_key = %q, want %q", got, hex.EncodeToString([]byte("0123456789abcdef")))
		}
		if voice.EncodeType != voiceEncodeTypeMP3 {
			t.Fatalf("encode_type = %d, want %d", voice.EncodeType, voiceEncodeTypeMP3)
		}
		if voice.SampleRate != 24000 {
			t.Fatalf("sample_rate = %d, want %d", voice.SampleRate, 24000)
		}
		if voice.PlayTime != 2544 {
			t.Fatalf("playtime = %d, want %d", voice.PlayTime, 2544)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", server.Client())
	err := client.SendUploadedVoice(context.Background(), "user@im.wechat", &uploadedVoice{
		DownloadEncryptedQueryParam: "voice-download-token",
		AESKey:                      []byte("0123456789abcdef"),
		FileSize:                    32,
		FileSizeCiphertext:          48,
		EncodeType:                  voiceEncodeTypeMP3,
		SampleRate:                  24000,
		PlayTime:                    2544,
	}, "context-token")
	if err != nil {
		t.Fatalf("SendUploadedVoice() error = %v", err)
	}
}

func TestSendUploadedFileAttachmentPostsFileMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/ilink/bot/sendmessage" {
			t.Fatalf("path = %q, want /ilink/bot/sendmessage", got)
		}

		var body SendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.Msg.ToUserID != "user@im.wechat" {
			t.Fatalf("to_user_id = %q, want %q", body.Msg.ToUserID, "user@im.wechat")
		}
		if body.Msg.ContextToken != "context-token" {
			t.Fatalf("context_token = %q, want %q", body.Msg.ContextToken, "context-token")
		}
		if len(body.Msg.ItemList) != 1 || body.Msg.ItemList[0].Type != ItemTypeFile || body.Msg.ItemList[0].FileItem == nil {
			t.Fatalf("item_list = %+v, want one file item", body.Msg.ItemList)
		}
		file := body.Msg.ItemList[0].FileItem
		if file.Media == nil {
			t.Fatal("file_item.media = nil, want populated media")
		}
		if file.Media.EncryptQueryParam != "file-download-token" {
			t.Fatalf("encrypt_query_param = %q, want %q", file.Media.EncryptQueryParam, "file-download-token")
		}
		if file.FileName != "testvoice.ogg" {
			t.Fatalf("file_name = %q, want %q", file.FileName, "testvoice.ogg")
		}
		if file.Len != "7776" {
			t.Fatalf("len = %q, want %q", file.Len, "7776")
		}

		_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0})
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", server.Client())
	err := client.SendUploadedFileAttachment(context.Background(), "user@im.wechat", &uploadedFile{
		DownloadEncryptedQueryParam: "file-download-token",
		AESKey:                      []byte("0123456789abcdef"),
		FileName:                    "testvoice.ogg",
		FileSize:                    7776,
		FileSizeCiphertext:          7792,
	}, "context-token")
	if err != nil {
		t.Fatalf("SendUploadedFileAttachment() error = %v", err)
	}
}
