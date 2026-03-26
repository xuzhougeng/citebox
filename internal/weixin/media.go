package weixin

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/md5"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	UploadMediaTypeImage   = 1
	UploadMediaTypeFile    = 3
	UploadMediaTypeVoice   = 4
	mediaEncryptTypeBundle = 1
	voiceEncodeTypePCM     = 1
	voiceEncodeTypeAMR     = 5
	voiceEncodeTypeSILK    = 6
	voiceEncodeTypeMP3     = 7
	voiceEncodeTypeOGG     = 8
)

type uploadedImage struct {
	DownloadEncryptedQueryParam string
	AESKey                      []byte
	FileSize                    int
	FileSizeCiphertext          int
}

type uploadedVoice struct {
	DownloadEncryptedQueryParam string
	AESKey                      []byte
	FileSize                    int
	FileSizeCiphertext          int
	EncodeType                  int
	SampleRate                  int
	PlayTime                    int
}

type uploadedFile struct {
	DownloadEncryptedQueryParam string
	AESKey                      []byte
	FileName                    string
	FileSize                    int
	FileSizeCiphertext          int
}

type probedVoiceMetadata struct {
	EncodeType int
	SampleRate int
	PlayTime   int
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

func (c *Client) SendVoiceFile(ctx context.Context, toUserID, filePath, contextToken string) error {
	if strings.TrimSpace(contextToken) == "" {
		return fmt.Errorf("context token is required for voice sends")
	}

	uploaded, err := c.uploadVoiceFile(ctx, filePath, toUserID)
	if err != nil {
		return err
	}
	return c.SendUploadedVoice(ctx, toUserID, uploaded, contextToken)
}

func (c *Client) SendFileAttachment(ctx context.Context, toUserID, filePath, contextToken string) error {
	if strings.TrimSpace(contextToken) == "" {
		return fmt.Errorf("context token is required for file sends")
	}

	uploaded, err := c.uploadFileAttachment(ctx, filePath, toUserID)
	if err != nil {
		return err
	}
	return c.SendUploadedFileAttachment(ctx, toUserID, uploaded, contextToken)
}

func (c *Client) SendUploadedVoice(ctx context.Context, toUserID string, uploaded *uploadedVoice, contextToken string) error {
	if uploaded == nil {
		return fmt.Errorf("uploaded voice is required")
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
					Type: ItemTypeVoice,
					VoiceItem: &VoiceItem{
						Media: &CDNMedia{
							EncryptQueryParam: uploaded.DownloadEncryptedQueryParam,
							AESKey:            base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(uploaded.AESKey))),
							EncryptType:       mediaEncryptTypeBundle,
						},
						EncodeType: uploaded.EncodeType,
						SampleRate: uploaded.SampleRate,
						PlayTime:   uploaded.PlayTime,
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
		return fmt.Errorf("sendmessage(voice) ret=%d errcode=%d: %s", result.Ret, result.ErrCode, result.Message)
	}
	return nil
}

func (c *Client) SendUploadedFileAttachment(ctx context.Context, toUserID string, uploaded *uploadedFile, contextToken string) error {
	if uploaded == nil {
		return fmt.Errorf("uploaded file is required")
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
					Type: ItemTypeFile,
					FileItem: &FileItem{
						Media: &CDNMedia{
							EncryptQueryParam: uploaded.DownloadEncryptedQueryParam,
							AESKey:            base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(uploaded.AESKey))),
							EncryptType:       mediaEncryptTypeBundle,
						},
						FileName: uploaded.FileName,
						Len:      strconv.Itoa(uploaded.FileSize),
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
		return fmt.Errorf("sendmessage(file) ret=%d errcode=%d: %s", result.Ret, result.ErrCode, result.Message)
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

func (c *Client) uploadVoiceFile(ctx context.Context, filePath, toUserID string) (*uploadedVoice, error) {
	plaintext, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	metadata, err := probeVoiceFile(ctx, filePath)
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
		MediaType:   UploadMediaTypeVoice,
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

	return &uploadedVoice{
		DownloadEncryptedQueryParam: downloadParam,
		AESKey:                      aesKey,
		FileSize:                    rawSize,
		FileSizeCiphertext:          fileSize,
		EncodeType:                  metadata.EncodeType,
		SampleRate:                  metadata.SampleRate,
		PlayTime:                    metadata.PlayTime,
	}, nil
}

func (c *Client) uploadFileAttachment(ctx context.Context, filePath, toUserID string) (*uploadedFile, error) {
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
		MediaType:   UploadMediaTypeFile,
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

	return &uploadedFile{
		DownloadEncryptedQueryParam: downloadParam,
		AESKey:                      aesKey,
		FileName:                    filepath.Base(strings.TrimSpace(filePath)),
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

func probeVoiceFile(ctx context.Context, filePath string) (*probedVoiceMetadata, error) {
	cmd := exec.CommandContext(
		ctx,
		"ffprobe",
		"-v", "error",
		"-show_entries", "stream=codec_name,sample_rate:format=duration",
		"-of", "json",
		filePath,
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe %s: %w", filePath, err)
	}

	var payload struct {
		Streams []struct {
			CodecName  string `json:"codec_name"`
			SampleRate string `json:"sample_rate"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, fmt.Errorf("parse ffprobe output for %s: %w", filePath, err)
	}
	if len(payload.Streams) == 0 {
		return nil, fmt.Errorf("ffprobe returned no audio streams for %s", filePath)
	}

	stream := payload.Streams[0]
	encodeType, err := detectVoiceEncodeType(filePath, stream.CodecName)
	if err != nil {
		return nil, err
	}
	sampleRate, err := strconv.Atoi(strings.TrimSpace(stream.SampleRate))
	if err != nil || sampleRate <= 0 {
		return nil, fmt.Errorf("invalid sample rate %q for %s", stream.SampleRate, filePath)
	}

	durationSeconds, err := strconv.ParseFloat(strings.TrimSpace(payload.Format.Duration), 64)
	if err != nil || durationSeconds <= 0 {
		return nil, fmt.Errorf("invalid duration %q for %s", payload.Format.Duration, filePath)
	}

	return &probedVoiceMetadata{
		EncodeType: encodeType,
		SampleRate: sampleRate,
		PlayTime:   int(durationSeconds * 1000),
	}, nil
}

func detectVoiceEncodeType(filePath, codecName string) (int, error) {
	codec := strings.ToLower(strings.TrimSpace(codecName))
	switch codec {
	case "mp3":
		return voiceEncodeTypeMP3, nil
	case "amr_nb", "amr_wb", "amr":
		return voiceEncodeTypeAMR, nil
	case "speex":
		return voiceEncodeTypeOGG, nil
	case "silk":
		return voiceEncodeTypeSILK, nil
	}

	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".mp3":
		return voiceEncodeTypeMP3, nil
	case ".amr":
		return voiceEncodeTypeAMR, nil
	case ".ogg":
		return voiceEncodeTypeOGG, nil
	case ".silk":
		return voiceEncodeTypeSILK, nil
	case ".wav":
		return voiceEncodeTypePCM, nil
	}

	if strings.HasPrefix(codec, "pcm_") {
		return voiceEncodeTypePCM, nil
	}

	return 0, fmt.Errorf("unsupported voice codec %q for %s", codecName, filePath)
}
