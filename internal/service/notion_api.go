package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

const (
	notionAPIBaseURL        = "https://api.notion.com"
	notionAPIVersion        = "2026-03-11"
	notionAPICredentialFile = "personal-access-token.json"
	notionAPIExportStateKey = "notion_api_export_pages"
	notionDirectUploadLimit = 20 * 1024 * 1024
)

type notionAPICredential struct {
	AccessToken string    `json:"access_token"`
	UserID      string    `json:"user_id"`
	UserName    string    `json:"user_name,omitempty"`
	ValidatedAt time.Time `json:"validated_at"`
}

type notionAPIPageRef struct {
	ID    string `json:"id"`
	URL   string `json:"url,omitempty"`
	Title string `json:"title,omitempty"`
}

type notionAPIUserExportState struct {
	Root   notionAPIPageRef            `json:"root"`
	Papers map[string]notionAPIPageRef `json:"papers"`
}

type notionAPIExportState struct {
	Users map[string]notionAPIUserExportState `json:"users"`
}

type notionAPIError struct {
	Status int
	Code   string
	Detail string
}

func (e *notionAPIError) Error() string {
	if e.Code != "" && e.Detail != "" {
		return fmt.Sprintf("Notion API returned %d %s: %s", e.Status, e.Code, e.Detail)
	}
	if e.Detail != "" {
		return fmt.Sprintf("Notion API returned %d: %s", e.Status, e.Detail)
	}
	return fmt.Sprintf("Notion API returned %d", e.Status)
}

// NotionAPIService owns the personal-access-token flow used for native image
// uploads. It is intentionally separate from RemoteMCPService: @Notion uses
// MCP OAuth, while figure export uses one PAT for upload and page writes.
type NotionAPIService struct {
	repo           *repository.SettingRepository
	httpClient     *http.Client
	baseURL        string
	credentialPath string
	exportMu       sync.Mutex
}

func NewNotionAPIService(repo *repository.SettingRepository, storageDir string) *NotionAPIService {
	return &NotionAPIService{
		repo:           repo,
		httpClient:     &http.Client{Timeout: 60 * time.Second},
		baseURL:        notionAPIBaseURL,
		credentialPath: filepath.Join(storageDir, "notion-api", notionAPICredentialFile),
	}
}

func (s *NotionAPIService) GetSettings() (model.NotionAPISettingsView, error) {
	credential, err := s.loadCredential()
	if err != nil || credential == nil {
		return model.NotionAPISettingsView{}, err
	}
	return model.NotionAPISettingsView{
		Configured: true,
		UserID:     credential.UserID,
		UserName:   credential.UserName,
	}, nil
}

func (s *NotionAPIService) TestToken(ctx context.Context, token string) (model.NotionAPITokenStatus, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		credential, err := s.loadCredential()
		if err != nil {
			return model.NotionAPITokenStatus{}, err
		}
		if credential == nil {
			return model.NotionAPITokenStatus{}, apperr.New(apperr.CodeFailedPrecondition, "请填写 Notion API 个人访问令牌")
		}
		token = credential.AccessToken
	}
	user, err := s.retrieveCurrentUser(ctx, token)
	if err != nil {
		return model.NotionAPITokenStatus{}, err
	}
	return model.NotionAPITokenStatus{
		Success:  true,
		Message:  "Notion API 个人访问令牌有效",
		UserID:   user.ID,
		UserName: user.Name,
	}, nil
}

func (s *NotionAPIService) SaveToken(ctx context.Context, token string) (model.NotionAPITokenStatus, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return model.NotionAPITokenStatus{}, apperr.New(apperr.CodeInvalidArgument, "请填写 Notion API 个人访问令牌")
	}
	user, err := s.retrieveCurrentUser(ctx, token)
	if err != nil {
		return model.NotionAPITokenStatus{}, err
	}
	credential := notionAPICredential{
		AccessToken: token,
		UserID:      firstNonEmpty(strings.TrimSpace(user.ID), tokenIdentity(token)),
		UserName:    strings.TrimSpace(user.Name),
		ValidatedAt: time.Now(),
	}
	if err := s.saveCredential(credential); err != nil {
		return model.NotionAPITokenStatus{}, err
	}
	return model.NotionAPITokenStatus{
		Success:  true,
		Message:  "Notion API 个人访问令牌已安全保存",
		UserID:   credential.UserID,
		UserName: credential.UserName,
	}, nil
}

func (s *NotionAPIService) DeleteToken() error {
	if err := os.Remove(s.credentialPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return apperr.Wrap(apperr.CodeInternal, "删除 Notion API 个人访问令牌失败", err)
	}
	return nil
}

type notionAPIUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *NotionAPIService) retrieveCurrentUser(ctx context.Context, token string) (notionAPIUser, error) {
	var user notionAPIUser
	if err := s.doJSON(ctx, http.MethodGet, "/v1/users/me", token, nil, &user); err != nil {
		return notionAPIUser{}, apperr.Wrap(apperr.CodeUnauthenticated, "Notion API 个人访问令牌验证失败", err)
	}
	if strings.TrimSpace(user.ID) == "" {
		return notionAPIUser{}, apperr.New(apperr.CodeUnauthenticated, "Notion API 用户响应缺少用户 ID")
	}
	return user, nil
}

func (s *NotionAPIService) saveCredential(credential notionAPICredential) error {
	if err := os.MkdirAll(filepath.Dir(s.credentialPath), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.credentialPath, raw, 0o600); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "保存 Notion API 个人访问令牌失败", err)
	}
	_ = os.Chmod(s.credentialPath, 0o600)
	return nil
}

func (s *NotionAPIService) loadCredential() (*notionAPICredential, error) {
	raw, err := os.ReadFile(s.credentialPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取 Notion API 个人访问令牌失败", err)
	}
	var credential notionAPICredential
	if err := json.Unmarshal(raw, &credential); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "解析 Notion API 个人访问令牌失败", err)
	}
	if strings.TrimSpace(credential.AccessToken) == "" {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "Notion API 个人访问令牌为空")
	}
	return &credential, nil
}

func tokenIdentity(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "token-" + hex.EncodeToString(sum[:8])
}

// SaveFigureNoteToNotion uploads the original raster and attaches it as a
// native Notion image block. All page and file operations use the same PAT.
func (s *NotionAPIService) SaveFigureNoteToNotion(
	ctx context.Context,
	figure *model.FigureListItem,
	imageData []byte,
	imageMIME string,
	notesText string,
) (model.NotionSaveFigureNoteResponse, error) {
	if figure == nil {
		return model.NotionSaveFigureNoteResponse{}, apperr.New(apperr.CodeNotFound, "figure not found")
	}
	notesText = strings.TrimSpace(notesText)
	if notesText == "" {
		return model.NotionSaveFigureNoteResponse{}, apperr.New(apperr.CodeInvalidArgument, "图片笔记为空，无法保存到 Notion")
	}
	if len(imageData) == 0 {
		return model.NotionSaveFigureNoteResponse{}, apperr.New(apperr.CodeInvalidArgument, "图片文件为空，无法保存到 Notion")
	}
	if len(imageData) > notionDirectUploadLimit {
		return model.NotionSaveFigureNoteResponse{}, apperr.New(apperr.CodeResourceExhausted, "图片超过 Notion 20 MB 直接上传限制")
	}
	imageMIME = strings.ToLower(strings.TrimSpace(strings.Split(imageMIME, ";")[0]))
	if !strings.HasPrefix(imageMIME, "image/") {
		return model.NotionSaveFigureNoteResponse{}, apperr.New(apperr.CodeUnsupportedMedia, "Notion 原图上传仅支持图片文件")
	}

	credential, err := s.loadCredential()
	if err != nil {
		return model.NotionSaveFigureNoteResponse{}, err
	}
	if credential == nil {
		return model.NotionSaveFigureNoteResponse{}, apperr.New(apperr.CodeFailedPrecondition, "请先在设置中保存 Notion API 个人访问令牌")
	}
	filename := notionFigureFilename(figure, imageMIME)
	fileUploadID, err := s.uploadFile(ctx, credential.AccessToken, filename, imageMIME, imageData)
	if err != nil {
		return model.NotionSaveFigureNoteResponse{}, err
	}

	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	state, err := s.loadExportState()
	if err != nil {
		return model.NotionSaveFigureNoteResponse{}, err
	}
	userKey := firstNonEmpty(strings.TrimSpace(credential.UserID), tokenIdentity(credential.AccessToken))
	workspace := state.Users[userKey]
	if workspace.Papers == nil {
		workspace.Papers = make(map[string]notionAPIPageRef)
	}
	paperKey := strconv.FormatInt(figure.PaperID, 10)
	stateChanged := false
	if workspace.Root.ID != "" {
		root, usable, checkErr := s.retrievePageRef(ctx, credential.AccessToken, workspace.Root)
		if checkErr != nil {
			return model.NotionSaveFigureNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "检查 Notion 图片笔记目录失败", checkErr)
		}
		if !usable {
			workspace.Root = notionAPIPageRef{}
			workspace.Papers = make(map[string]notionAPIPageRef)
			stateChanged = true
		} else {
			workspace.Root = root
		}
	}
	if paperPage := workspace.Papers[paperKey]; workspace.Root.ID != "" && paperPage.ID != "" {
		paperPage, usable, checkErr := s.retrievePageRef(ctx, credential.AccessToken, paperPage)
		if checkErr != nil {
			return model.NotionSaveFigureNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "检查 Notion 文献页面失败", checkErr)
		}
		if !usable {
			delete(workspace.Papers, paperKey)
			stateChanged = true
		} else {
			workspace.Papers[paperKey] = paperPage
		}
	}
	if stateChanged {
		state.Users[userKey] = workspace
		if err := s.saveExportState(state); err != nil {
			return model.NotionSaveFigureNoteResponse{}, err
		}
	}
	blocks := buildNotionAPIFigureBlocks(figure, fileUploadID, notesText)
	retriedMissingPaper := false
	retriedMissingRoot := false

ensureExportPages:
	if workspace.Root.ID == "" {
		root, createErr := s.createPage(ctx, credential.AccessToken, "", "CiteBox 图片笔记", []any{
			notionParagraph("由 CiteBox 通过 Notion API 导出的原始图片笔记。每篇文献对应一个子页面。"),
		})
		if createErr != nil {
			return model.NotionSaveFigureNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "创建 Notion 图片笔记目录失败", createErr)
		}
		workspace.Root = root
		workspace.Root.Title = "CiteBox 图片笔记"
		state.Users[userKey] = workspace
		if err := s.saveExportState(state); err != nil {
			return model.NotionSaveFigureNoteResponse{}, err
		}
	}

	paperPage := workspace.Papers[paperKey]
	if paperPage.ID == "" {
		pageTitle := "文献｜" + firstNonEmpty(strings.TrimSpace(figure.PaperTitle), "未命名文献")
		children := append([]any{notionParagraph("CiteBox 文献 ID：" + paperKey)}, blocks...)
		created, createErr := s.createPage(ctx, credential.AccessToken, workspace.Root.ID, pageTitle, children)
		if createErr != nil {
			if isNotionAPIPageMissing(createErr) && !retriedMissingRoot {
				retriedMissingRoot = true
				workspace.Root = notionAPIPageRef{}
				workspace.Papers = make(map[string]notionAPIPageRef)
				state.Users[userKey] = workspace
				if err := s.saveExportState(state); err != nil {
					return model.NotionSaveFigureNoteResponse{}, err
				}
				goto ensureExportPages
			}
			return model.NotionSaveFigureNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "创建 Notion 文献页面失败", createErr)
		}
		paperPage = created
		paperPage.Title = pageTitle
		workspace.Papers[paperKey] = paperPage
		state.Users[userKey] = workspace
		if err := s.saveExportState(state); err != nil {
			return model.NotionSaveFigureNoteResponse{}, err
		}
	} else if appendErr := s.appendBlocks(ctx, credential.AccessToken, paperPage.ID, blocks); appendErr != nil {
		if isNotionAPIPageMissing(appendErr) && !retriedMissingPaper {
			retriedMissingPaper = true
			delete(workspace.Papers, paperKey)
			state.Users[userKey] = workspace
			if err := s.saveExportState(state); err != nil {
				return model.NotionSaveFigureNoteResponse{}, err
			}
			goto ensureExportPages
		}
		return model.NotionSaveFigureNoteResponse{}, apperr.Wrap(apperr.CodeUnavailable, "追加 Notion 图片笔记失败", appendErr)
	}

	return model.NotionSaveFigureNoteResponse{
		Success: true, Message: "原始图片与笔记已保存到 Notion",
		TargetPageID: paperPage.ID, TargetPageURL: paperPage.URL, ImageEmbedded: true,
	}, nil
}

func (s *NotionAPIService) retrievePageRef(
	ctx context.Context,
	token string,
	page notionAPIPageRef,
) (notionAPIPageRef, bool, error) {
	var response struct {
		ID         string `json:"id"`
		URL        string `json:"url"`
		InTrash    bool   `json:"in_trash"`
		Archived   bool   `json:"archived"`
		IsArchived bool   `json:"is_archived"`
	}
	err := s.doJSON(ctx, http.MethodGet, "/v1/pages/"+url.PathEscape(page.ID), token, nil, &response)
	if err != nil {
		if isNotionAPIPageMissing(err) {
			return notionAPIPageRef{}, false, nil
		}
		return notionAPIPageRef{}, false, err
	}
	if response.InTrash || response.Archived || response.IsArchived {
		return notionAPIPageRef{}, false, nil
	}
	if strings.TrimSpace(response.ID) == "" {
		return notionAPIPageRef{}, false, nil
	}
	page.ID = strings.ReplaceAll(response.ID, "-", "")
	if strings.TrimSpace(response.URL) != "" {
		page.URL = response.URL
	}
	return page, true, nil
}

func (s *NotionAPIService) uploadFile(ctx context.Context, token, filename, contentType string, data []byte) (string, error) {
	var upload struct {
		ID        string `json:"id"`
		UploadURL string `json:"upload_url"`
		Status    string `json:"status"`
	}
	if err := s.doJSON(ctx, http.MethodPost, "/v1/file_uploads", token, map[string]any{
		"mode": "single_part", "filename": filename, "content_type": contentType,
	}, &upload); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "创建 Notion 原图上传任务失败", err)
	}
	if upload.ID == "" || upload.UploadURL == "" {
		return "", apperr.New(apperr.CodeUnavailable, "Notion 文件上传响应缺少 ID 或上传地址")
	}
	if err := s.sendFile(ctx, token, upload.UploadURL, filename, contentType, data); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "上传 Notion 原始图片失败", err)
	}
	return upload.ID, nil
}

func (s *NotionAPIService) sendFile(ctx context.Context, token, uploadURL, filename, contentType string, data []byte) error {
	target, err := s.safeUploadURL(uploadURL)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeMultipartFilename(filename)))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, &body)
	if err != nil {
		return err
	}
	s.setHeaders(req, token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseNotionAPIError(resp.StatusCode, responseBody)
	}
	return nil
}

func (s *NotionAPIService) safeUploadURL(raw string) (string, error) {
	base, err := url.Parse(strings.TrimRight(s.baseURL, "/"))
	if err != nil {
		return "", err
	}
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if !target.IsAbs() {
		target = base.ResolveReference(target)
	}
	if !strings.EqualFold(target.Scheme, base.Scheme) || !strings.EqualFold(target.Host, base.Host) {
		return "", apperr.New(apperr.CodeUnavailable, "Notion 返回了不受信任的文件上传地址")
	}
	return target.String(), nil
}

func escapeMultipartFilename(value string) string {
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, `"`, "_")
	return filepath.Base(value)
}

func (s *NotionAPIService) createPage(ctx context.Context, token, parentID, title string, children []any) (notionAPIPageRef, error) {
	payload := map[string]any{
		"properties": map[string]any{
			"title": map[string]any{"title": notionRichText(title)},
		},
		"children": children,
	}
	if parentID != "" {
		payload["parent"] = map[string]any{"type": "page_id", "page_id": parentID}
	}
	var page struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := s.doJSON(ctx, http.MethodPost, "/v1/pages", token, payload, &page); err != nil {
		return notionAPIPageRef{}, err
	}
	if strings.TrimSpace(page.ID) == "" {
		return notionAPIPageRef{}, apperr.New(apperr.CodeUnavailable, "Notion 创建页面响应缺少页面 ID")
	}
	return notionAPIPageRef{ID: strings.ReplaceAll(page.ID, "-", ""), URL: page.URL, Title: title}, nil
}

func (s *NotionAPIService) appendBlocks(ctx context.Context, token, pageID string, children []any) error {
	return s.doJSON(ctx, http.MethodPatch, "/v1/blocks/"+url.PathEscape(pageID)+"/children", token, map[string]any{
		"children": children,
		"position": map[string]any{"type": "end"},
	}, nil)
}

func (s *NotionAPIService) doJSON(ctx context.Context, method, path, token string, payload any, target any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.baseURL, "/")+path, body)
	if err != nil {
		return err
	}
	s.setHeaders(req, token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseNotionAPIError(resp.StatusCode, data)
	}
	if target != nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, target); err != nil {
			return fmt.Errorf("decode Notion API response: %w", err)
		}
	}
	return nil
}

func (s *NotionAPIService) setHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", notionAPIVersion)
	req.Header.Set("Accept", "application/json")
}

func parseNotionAPIError(status int, data []byte) error {
	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &payload)
	detail := strings.TrimSpace(payload.Message)
	if detail == "" {
		detail = strings.TrimSpace(string(data))
	}
	return &notionAPIError{Status: status, Code: payload.Code, Detail: detail}
}

func isNotionAPIPageMissing(err error) bool {
	var apiErr *notionAPIError
	if errors.As(err, &apiErr) {
		message := strings.ToLower(apiErr.Detail)
		return apiErr.Status == http.StatusNotFound || apiErr.Code == "object_not_found" ||
			strings.Contains(message, "archived") || strings.Contains(message, "in trash")
	}
	return false
}

func (s *NotionAPIService) loadExportState() (notionAPIExportState, error) {
	state := notionAPIExportState{Users: make(map[string]notionAPIUserExportState)}
	raw, err := s.repo.GetAppSetting(notionAPIExportStateKey)
	if err != nil {
		return state, err
	}
	if strings.TrimSpace(raw) == "" {
		return state, nil
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return state, apperr.Wrap(apperr.CodeInternal, "解析 Notion API 页面映射失败", err)
	}
	if state.Users == nil {
		state.Users = make(map[string]notionAPIUserExportState)
	}
	return state, nil
}

func (s *NotionAPIService) saveExportState(state notionAPIExportState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.repo.UpsertAppSetting(notionAPIExportStateKey, string(raw))
}

func notionFigureFilename(figure *model.FigureListItem, contentType string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(figure.Filename)))
	if ext == "" {
		switch contentType {
		case "image/jpeg":
			ext = ".jpg"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		case "image/svg+xml":
			ext = ".svg"
		default:
			if extensions, _ := mime.ExtensionsByType(contentType); len(extensions) > 0 {
				ext = extensions[0]
			} else {
				ext = ".png"
			}
		}
	}
	return fmt.Sprintf("citebox-paper-%d-figure-%d%s", figure.PaperID, figure.ID, ext)
}

func buildNotionAPIFigureBlocks(figure *model.FigureListItem, fileUploadID, notesText string) []any {
	label := strings.TrimSpace(figure.DisplayLabel)
	if label == "" && figure.FigureIndex > 0 {
		label = fmt.Sprintf("Fig %d", figure.FigureIndex)
	}
	label = firstNonEmpty(label, "图片")
	blocks := []any{
		map[string]any{"object": "block", "type": "divider", "divider": map[string]any{}},
		notionTextBlock("heading_2", fmt.Sprintf("%s · 第 %d 页", label, figure.PageNumber)),
		map[string]any{
			"object": "block", "type": "image",
			"image": map[string]any{
				"type":        "file_upload",
				"file_upload": map[string]any{"id": fileUploadID},
				"caption":     notionRichText(truncateRunes(strings.TrimSpace(figure.Caption), 1900)),
			},
		},
		notionParagraph("导出时间：" + time.Now().Format("2006-01-02 15:04:05")),
	}
	if caption := strings.TrimSpace(figure.Caption); caption != "" {
		blocks = append(blocks, notionTextBlock("heading_3", "图片说明"))
		blocks = append(blocks, notionParagraphChunks(caption)...)
	}
	if tags := strings.TrimSpace(joinTagNames(figure.Tags)); tags != "" {
		blocks = append(blocks, notionParagraph("图片标签："+tags))
	}
	blocks = append(blocks, notionTextBlock("heading_3", "图片笔记"))
	blocks = append(blocks, notionMarkdownBlocks(truncateRunes(notesText, 30000), 80)...)
	return blocks
}

func notionMarkdownBlocks(text string, maxBlocks int) []any {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	blocks := make([]any, 0, minInt(len(lines), maxBlocks))
	var paragraph []string
	flush := func() {
		if len(paragraph) == 0 || len(blocks) >= maxBlocks {
			paragraph = nil
			return
		}
		blocks = append(blocks, notionParagraphChunks(strings.Join(paragraph, "\n"))...)
		if len(blocks) > maxBlocks {
			blocks = blocks[:maxBlocks]
		}
		paragraph = nil
	}
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(blocks) >= maxBlocks-1 {
			flush()
			remaining := strings.Join(lines[index:], "\n")
			if len(blocks) < maxBlocks {
				blocks = append(blocks, notionParagraph(truncateRunes(remaining, 1900)))
			}
			break
		}
		if trimmed == "" {
			flush()
			continue
		}
		blockType, content := "", ""
		switch {
		case strings.HasPrefix(trimmed, "### "):
			blockType, content = "heading_3", strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))
		case strings.HasPrefix(trimmed, "## "):
			blockType, content = "heading_2", strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
		case strings.HasPrefix(trimmed, "# "):
			blockType, content = "heading_1", strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			blockType, content = "bulleted_list_item", strings.TrimSpace(trimmed[2:])
		case strings.HasPrefix(trimmed, "> "):
			blockType, content = "quote", strings.TrimSpace(strings.TrimPrefix(trimmed, "> "))
		}
		if blockType == "" {
			paragraph = append(paragraph, line)
			continue
		}
		flush()
		blocks = append(blocks, notionTextBlock(blockType, content))
	}
	flush()
	if len(blocks) == 0 {
		blocks = append(blocks, notionParagraph("（无内容）"))
	}
	return blocks
}

func notionParagraphChunks(text string) []any {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}
	var blocks []any
	for len(runes) > 0 {
		size := minInt(len(runes), 1900)
		blocks = append(blocks, notionParagraph(string(runes[:size])))
		runes = runes[size:]
	}
	return blocks
}

func notionParagraph(text string) map[string]any {
	return notionTextBlock("paragraph", text)
}

func notionTextBlock(blockType, text string) map[string]any {
	return map[string]any{
		"object": "block", "type": blockType,
		blockType: map[string]any{"rich_text": notionRichText(truncateRunes(text, 1900))},
	}
}

func notionRichText(text string) []any {
	if text == "" {
		return []any{}
	}
	return []any{map[string]any{"type": "text", "text": map[string]any{"content": text}}}
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
