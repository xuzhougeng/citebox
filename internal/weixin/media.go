package weixin

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/md5"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	UploadMediaTypeImage   = 1
	mediaEncryptTypeBundle = 1
)

type uploadedImage struct {
	DownloadEncryptedQueryParam string
	AESKey                      []byte
	FileSize                    int
	FileSizeCiphertext          int
}

func (c *Client) GetUploadURL(ctx context.Context, body GetUploadURLRequest) (*GetUploadURLResponse, error) {
	body.BaseInfo = baseInfo()

	var result GetUploadURLResponse
	if err := c.post(ctx, "/ilink/bot/getuploadurl", body, &result); err != nil {
		return nil, err
	}
	if result.Ret != 0 {
		return nil, fmt.Errorf("getuploadurl ret=%d errcode=%d: %s", result.Ret, result.ErrCode, result.Message)
	}
	return &result, nil
}

func (c *Client) SendImageFile(ctx context.Context, toUserID, filePath, contextToken string) error {
	if strings.TrimSpace(contextToken) == "" {
		return fmt.Errorf("context token is required for image sends")
	}

	uploaded, err := c.uploadImageFile(ctx, filePath, toUserID)
	if err != nil {
		return err
	}
	return c.SendUploadedImage(ctx, toUserID, uploaded, contextToken)
}

func (c *Client) SendUploadedImage(ctx context.Context, toUserID string, uploaded *uploadedImage, contextToken string) error {
	if uploaded == nil {
		return fmt.Errorf("uploaded image is required")
	}

	body := SendMessageRequest{
		Msg: Message{
			ToUserID:     toUserID,
			ClientID:     generateClientID(),
			MessageType:  MessageTypeBot,
			MessageState: MessageStateFinish,
			ContextToken: contextToken,
			ItemList: []MessageItem{
				{
					Type: ItemTypeImage,
					ImageItem: &ImageItem{
						Media: &CDNMedia{
							EncryptQueryParam: uploaded.DownloadEncryptedQueryParam,
							AESKey:            base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(uploaded.AESKey))),
							EncryptType:       mediaEncryptTypeBundle,
						},
						MidSize: uploaded.FileSizeCiphertext,
					},
				},
			},
		},
		BaseInfo: baseInfo(),
	}

	var result SendMessageResponse
	if err := c.post(ctx, "/ilink/bot/sendmessage", body, &result); err != nil {
		return err
	}
	if result.Ret != 0 {
		return fmt.Errorf("sendmessage(image) ret=%d errcode=%d: %s", result.Ret, result.ErrCode, result.Message)
	}
	return nil
}

func (c *Client) uploadImageFile(ctx context.Context, filePath, toUserID string) (*uploadedImage, error) {
	plaintext, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	rawSize := len(plaintext)
	rawMD5 := fmt.Sprintf("%x", md5.Sum(plaintext))
	fileSize := aesECBEncryptedSize(rawSize)
	fileKey, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	aesKey, err := randomBytes(aes.BlockSize)
	if err != nil {
		return nil, err
	}

	resp, err := c.GetUploadURL(ctx, GetUploadURLRequest{
		FileKey:     fileKey,
		MediaType:   UploadMediaTypeImage,
		ToUserID:    toUserID,
		RawSize:     rawSize,
		RawFileMD5:  rawMD5,
		FileSize:    fileSize,
		NoNeedThumb: true,
		AESKey:      hex.EncodeToString(aesKey),
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resp.UploadParam) == "" {
		return nil, fmt.Errorf("getuploadurl returned empty upload_param")
	}

	downloadParam, err := c.uploadBufferToCDN(ctx, plaintext, resp.UploadParam, fileKey, aesKey, "")
	if err != nil {
		return nil, err
	}

	return &uploadedImage{
		DownloadEncryptedQueryParam: downloadParam,
		AESKey:                      aesKey,
		FileSize:                    rawSize,
		FileSizeCiphertext:          fileSize,
	}, nil
}

func (c *Client) uploadBufferToCDN(ctx context.Context, plaintext []byte, uploadParam, fileKey string, aesKey []byte, cdnBaseURL string) (string, error) {
	ciphertext, err := encryptAESECB(plaintext, aesKey)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		buildCDNUploadURL(uploadParam, fileKey, cdnBaseURL),
		bytes.NewReader(ciphertext),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("cdn upload returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	downloadParam := strings.TrimSpace(resp.Header.Get("x-encrypted-param"))
	if downloadParam == "" {
		return "", fmt.Errorf("cdn upload response missing x-encrypted-param header")
	}
	return downloadParam, nil
}

func buildCDNUploadURL(uploadParam, fileKey, cdnBaseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(cdnBaseURL), "/")
	if base == "" {
		base = DefaultCDNBaseURL
	}
	return fmt.Sprintf(
		"%s/upload?encrypted_query_param=%s&filekey=%s",
		base,
		url.QueryEscape(uploadParam),
		url.QueryEscape(fileKey),
	)
}

func encryptAESECB(plaintext, key []byte) ([]byte, error) {
	if len(key) != aes.BlockSize {
		return nil, fmt.Errorf("invalid AES-128 key length %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	padding := aes.BlockSize - (len(plaintext) % aes.BlockSize)
	if padding == 0 {
		padding = aes.BlockSize
	}

	padded := make([]byte, len(plaintext)+padding)
	copy(padded, plaintext)
	for index := len(plaintext); index < len(padded); index++ {
		padded[index] = byte(padding)
	}

	ciphertext := make([]byte, len(padded))
	for offset := 0; offset < len(padded); offset += aes.BlockSize {
		block.Encrypt(ciphertext[offset:offset+aes.BlockSize], padded[offset:offset+aes.BlockSize])
	}
	return ciphertext, nil
}

func aesECBEncryptedSize(plaintextSize int) int {
	padding := aes.BlockSize - (plaintextSize % aes.BlockSize)
	if padding == 0 {
		padding = aes.BlockSize
	}
	return plaintextSize + padding
}

func randomBytes(length int) ([]byte, error) {
	buf := make([]byte, length)
	if _, err := io.ReadFull(crand.Reader, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func randomHex(length int) (string, error) {
	buf, err := randomBytes(length)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
