package weixin

import (
	"context"
	"crypto/aes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const DefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"

type DownloadedFile struct {
	Filename    string
	ContentType string
	Data        []byte
}

func DownloadFileItem(ctx context.Context, item MessageItem, httpClient *http.Client, cdnBaseURL string) (*DownloadedFile, error) {
	if item.Type != ItemTypeFile || item.FileItem == nil {
		return nil, errors.New("weixin message item is not a file")
	}

	fileItem := item.FileItem
	if fileItem.Media == nil {
		return nil, errors.New("weixin file item is missing media metadata")
	}

	data, err := DownloadAndDecryptBuffer(
		ctx,
		httpClient,
		fileItem.Media.EncryptQueryParam,
		fileItem.Media.AESKey,
		cdnBaseURL,
	)
	if err != nil {
		return nil, err
	}

	filename := strings.TrimSpace(fileItem.FileName)
	if filename == "" {
		filename = "wechat-file.bin"
	}

	return &DownloadedFile{
		Filename:    filename,
		ContentType: mimeFromFilename(filename),
		Data:        data,
	}, nil
}

func DownloadAndDecryptBuffer(ctx context.Context, httpClient *http.Client, encryptedQueryParam, aesKeyBase64, cdnBaseURL string) ([]byte, error) {
	if strings.TrimSpace(encryptedQueryParam) == "" {
		return nil, errors.New("missing encrypted_query_param")
	}
	if strings.TrimSpace(aesKeyBase64) == "" {
		return nil, errors.New("missing aes_key")
	}

	key, err := parseAESKey(aesKeyBase64)
	if err != nil {
		return nil, err
	}

	encrypted, err := downloadCDNBytes(ctx, httpClient, buildCDNDownloadURL(encryptedQueryParam, cdnBaseURL))
	if err != nil {
		return nil, err
	}
	return decryptAESECB(encrypted, key)
}

func buildCDNDownloadURL(encryptedQueryParam, cdnBaseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(cdnBaseURL), "/")
	if base == "" {
		base = DefaultCDNBaseURL
	}
	return base + "/download?encrypted_query_param=" + url.QueryEscape(encryptedQueryParam)
}

func downloadCDNBytes(ctx context.Context, httpClient *http.Client, targetURL string) ([]byte, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("weixin CDN download returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return io.ReadAll(resp.Body)
}

func parseAESKey(value string) ([]byte, error) {
	decoded, err := decodeBase64String(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("decode weixin aes key failed: %w", err)
	}

	switch {
	case len(decoded) == 16:
		return decoded, nil
	case len(decoded) == 32 && isASCIIHex(decoded):
		key, err := decodeHexBytes(string(decoded))
		if err != nil {
			return nil, fmt.Errorf("decode hex weixin aes key failed: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("unexpected weixin aes key length %d", len(decoded))
	}
}

func decryptAESECB(ciphertext, key []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, errors.New("empty ciphertext")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext)%block.BlockSize() != 0 {
		return nil, fmt.Errorf("ciphertext size %d is not a multiple of AES block size", len(ciphertext))
	}

	plaintext := make([]byte, len(ciphertext))
	for offset := 0; offset < len(ciphertext); offset += block.BlockSize() {
		block.Decrypt(plaintext[offset:offset+block.BlockSize()], ciphertext[offset:offset+block.BlockSize()])
	}

	return pkcs7Unpad(plaintext, block.BlockSize())
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty padded plaintext")
	}

	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, errors.New("invalid PKCS7 padding")
	}

	for _, value := range data[len(data)-padding:] {
		if int(value) != padding {
			return nil, errors.New("invalid PKCS7 padding")
		}
	}

	return data[:len(data)-padding], nil
}

func mimeFromFilename(filename string) string {
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

func decodeBase64String(value string) ([]byte, error) {
	if data, err := base64.StdEncoding.DecodeString(value); err == nil {
		return data, nil
	}
	return base64.RawStdEncoding.DecodeString(value)
}

func isASCIIHex(data []byte) bool {
	for _, value := range data {
		switch {
		case value >= '0' && value <= '9':
		case value >= 'a' && value <= 'f':
		case value >= 'A' && value <= 'F':
		default:
			return false
		}
	}
	return true
}

func decodeHexBytes(value string) ([]byte, error) {
	if len(value)%2 != 0 {
		return nil, errors.New("hex string must have even length")
	}

	data := make([]byte, len(value)/2)
	for index := 0; index < len(value); index += 2 {
		high, ok := fromHexChar(value[index])
		if !ok {
			return nil, fmt.Errorf("invalid hex character %q", value[index])
		}
		low, ok := fromHexChar(value[index+1])
		if !ok {
			return nil, fmt.Errorf("invalid hex character %q", value[index+1])
		}
		data[index/2] = high<<4 | low
	}
	return data, nil
}

func fromHexChar(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}
