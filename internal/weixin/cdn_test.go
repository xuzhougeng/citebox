package weixin

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadFileItemDecryptsPDF(t *testing.T) {
	key := []byte("0123456789abcdef")
	plaintext := []byte("%PDF-1.4 inbound weixin file")
	ciphertext := encryptAESECBForTest(t, plaintext, key)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/download" {
			t.Fatalf("path = %q, want /download", got)
		}
		if got := r.URL.Query().Get("encrypted_query_param"); got != "abc" {
			t.Fatalf("encrypted_query_param = %q, want abc", got)
		}
		if _, err := w.Write(ciphertext); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}))
	defer server.Close()

	item := MessageItem{
		Type: ItemTypeFile,
		FileItem: &FileItem{
			FileName: "inbound.pdf",
			Media: &CDNMedia{
				EncryptQueryParam: "abc",
				AESKey:            base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(key))),
			},
		},
	}

	file, err := DownloadFileItem(context.Background(), item, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("DownloadFileItem() error = %v", err)
	}
	if file.ContentType != "application/pdf" {
		t.Fatalf("content type = %q, want application/pdf", file.ContentType)
	}
	if !bytes.Equal(file.Data, plaintext) {
		t.Fatalf("data = %q, want %q", string(file.Data), string(plaintext))
	}
}

func encryptAESECBForTest(t *testing.T, plaintext, key []byte) []byte {
	t.Helper()

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher() error = %v", err)
	}

	blockSize := block.BlockSize()
	padding := blockSize - (len(plaintext) % blockSize)
	if padding == 0 {
		padding = blockSize
	}

	padded := append([]byte(nil), plaintext...)
	padded = append(padded, bytes.Repeat([]byte{byte(padding)}, padding)...)

	ciphertext := make([]byte, len(padded))
	for offset := 0; offset < len(padded); offset += blockSize {
		block.Encrypt(ciphertext[offset:offset+blockSize], padded[offset:offset+blockSize])
	}
	return ciphertext
}
