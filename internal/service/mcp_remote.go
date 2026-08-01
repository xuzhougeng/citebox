package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/mcp"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

const remoteMCPSettingsKey = "remote_mcp_settings"

type oauthMetadata struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RegistrationEndpoint  string   `json:"registration_endpoint"`
	ScopesSupported       []string `json:"scopes_supported"`
}

type protectedResourceMetadata struct {
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
}

type oauthTokenError struct {
	Status      string
	Code        string
	Description string
}

func (e *oauthTokenError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("token endpoint returned %s: %s (%s)", e.Status, e.Code, e.Description)
	}
	return fmt.Sprintf("token endpoint returned %s: %s", e.Status, e.Code)
}

type oauthCredential struct {
	ResourceURL   string    `json:"resource_url"`
	ClientID      string    `json:"client_id"`
	ClientSecret  string    `json:"client_secret,omitempty"`
	TokenEndpoint string    `json:"token_endpoint"`
	AccessToken   string    `json:"access_token"`
	RefreshToken  string    `json:"refresh_token,omitempty"`
	Scope         string    `json:"scope,omitempty"`
	UserID        string    `json:"user_id,omitempty"`
	WorkspaceID   string    `json:"workspace_id,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	ToolNames     []string  `json:"tool_names,omitempty"`
}

type oauthFlow struct {
	ID           string
	State        string
	Verifier     string
	RedirectURI  string
	Mode         string
	Settings     model.RemoteMCPSettings
	Metadata     oauthMetadata
	ClientID     string
	ClientSecret string
	CreatedAt    time.Time
	Status       model.MCPAuthorizationStatus
}

// RemoteMCPService manages a generic remote MCP URL and its OAuth lifecycle.
type RemoteMCPService struct {
	repo           *repository.SettingRepository
	httpClient     *http.Client
	credentialPath string
	mu             sync.Mutex
	flows          map[string]*oauthFlow
	refreshMu      sync.Mutex
}

func NewRemoteMCPService(repo *repository.SettingRepository, storageDir string) *RemoteMCPService {
	return &RemoteMCPService{
		repo:           repo,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		credentialPath: filepath.Join(storageDir, "mcp", "oauth-credentials.json"),
		flows:          make(map[string]*oauthFlow),
	}
}

func (s *RemoteMCPService) GetSettings() (model.RemoteMCPSettingsView, error) {
	settings, err := s.loadSettings()
	if err != nil {
		return model.RemoteMCPSettingsView{}, err
	}
	view := model.RemoteMCPSettingsView{RemoteMCPSettings: settings}
	credential, err := s.loadCredential()
	if err != nil {
		return view, err
	}
	view.Authorized = credential != nil && sameMCPResource(credential.ResourceURL, settings.URL)
	if view.Authorized {
		view.ToolNames = append([]string(nil), credential.ToolNames...)
	}
	return view, nil
}

func (s *RemoteMCPService) StartAuthorization(ctx context.Context, settings model.RemoteMCPSettings, mode, redirectURI string) (model.MCPAuthorizationStart, error) {
	settings, err := normalizeRemoteMCPSettings(settings)
	if err != nil {
		return model.MCPAuthorizationStart{}, err
	}
	if mode != "test" && mode != "save" {
		return model.MCPAuthorizationStart{}, apperr.New(apperr.CodeInvalidArgument, "MCP 授权模式无效")
	}
	if _, err := url.ParseRequestURI(redirectURI); err != nil {
		return model.MCPAuthorizationStart{}, apperr.New(apperr.CodeInvalidArgument, "MCP OAuth 回调地址无效")
	}
	resource, metadata, scopes, err := s.discoverOAuth(ctx, settings.URL)
	if err != nil {
		return model.MCPAuthorizationStart{}, apperr.Wrap(apperr.CodeUnavailable, "MCP OAuth discovery failed", err)
	}
	clientID, clientSecret, err := s.registerClient(ctx, metadata, redirectURI)
	if err != nil {
		return model.MCPAuthorizationStart{}, apperr.Wrap(apperr.CodeUnavailable, "MCP OAuth client registration failed", err)
	}
	flowID, err := randomURLSafe(18)
	if err != nil {
		return model.MCPAuthorizationStart{}, err
	}
	state, err := randomURLSafe(32)
	if err != nil {
		return model.MCPAuthorizationStart{}, err
	}
	verifier, err := randomURLSafe(32)
	if err != nil {
		return model.MCPAuthorizationStart{}, err
	}
	challenge := sha256.Sum256([]byte(verifier))
	authURL, err := url.Parse(metadata.AuthorizationEndpoint)
	if err != nil {
		return model.MCPAuthorizationStart{}, err
	}
	query := authURL.Query()
	query.Set("response_type", "code")
	query.Set("client_id", clientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	query.Set("code_challenge_method", "S256")
	query.Set("resource", resource)
	if len(scopes) > 0 {
		query.Set("scope", strings.Join(scopes, " "))
	}
	authURL.RawQuery = query.Encode()

	s.mu.Lock()
	s.pruneFlowsLocked()
	s.flows[flowID] = &oauthFlow{
		ID: flowID, State: state, Verifier: verifier, RedirectURI: redirectURI, Mode: mode,
		Settings: settings, Metadata: metadata, ClientID: clientID, ClientSecret: clientSecret,
		CreatedAt: time.Now(), Status: model.MCPAuthorizationStatus{Status: "pending"},
	}
	s.mu.Unlock()
	return model.MCPAuthorizationStart{FlowID: flowID, AuthorizationURL: authURL.String()}, nil
}

func (s *RemoteMCPService) CompleteAuthorization(ctx context.Context, state, code, oauthError, errorDescription string) (string, bool) {
	s.mu.Lock()
	var flow *oauthFlow
	for _, candidate := range s.flows {
		if candidate.State == state {
			flow = candidate
			break
		}
	}
	if flow != nil && flow.Status.Status != "pending" {
		status := flow.Status
		s.mu.Unlock()
		return status.Message, status.Status == "complete"
	}
	s.mu.Unlock()
	if flow == nil || state == "" {
		return "OAuth state did not match; authorization was rejected.", false
	}
	if oauthError != "" {
		message := oauthError
		if errorDescription != "" {
			message += ": " + errorDescription
		}
		s.finishFlow(flow.ID, model.MCPAuthorizationStatus{Status: "error", Message: message})
		return message, false
	}
	if code == "" {
		s.finishFlow(flow.ID, model.MCPAuthorizationStatus{Status: "error", Message: "OAuth callback is missing code"})
		return "OAuth callback is missing code.", false
	}
	credential, err := s.exchangeCode(ctx, flow, code)
	if err == nil {
		var tools []mcp.Tool
		tools, err = s.listToolsWithCredential(ctx, credential)
		credential.ToolNames = toolNames(tools)
		if err == nil && flow.Mode == "save" {
			err = s.saveAuthorizedConnection(flow.Settings, credential)
		}
		if err == nil {
			status := model.MCPAuthorizationStatus{Status: "complete", Message: fmt.Sprintf("MCP connected; discovered %d tools", len(tools)), ToolNames: toolNames(tools)}
			s.finishFlow(flow.ID, status)
			return status.Message, true
		}
	}
	message := "MCP authorization failed: " + err.Error()
	s.finishFlow(flow.ID, model.MCPAuthorizationStatus{Status: "error", Message: message})
	return message, false
}

func (s *RemoteMCPService) AuthorizationStatus(flowID string) (model.MCPAuthorizationStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	flow := s.flows[flowID]
	if flow == nil {
		return model.MCPAuthorizationStatus{}, apperr.New(apperr.CodeNotFound, "MCP OAuth flow not found")
	}
	return flow.Status, nil
}

func (s *RemoteMCPService) Disconnect() error {
	if err := os.Remove(s.credentialPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return apperr.Wrap(apperr.CodeInternal, "删除 MCP OAuth 凭据失败", err)
	}
	settings, err := s.loadSettings()
	if err != nil {
		return err
	}
	settings.Enabled = false
	return s.saveSettings(settings)
}

func (s *RemoteMCPService) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	settings, err := s.loadSettings()
	if err != nil {
		return nil, err
	}
	if !settings.Enabled {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "Remote MCP is disabled")
	}
	credential, err := s.authorizedCredential(ctx, settings.URL)
	if err != nil {
		return nil, err
	}
	return s.listToolsWithCredential(ctx, *credential)
}

func (s *RemoteMCPService) CallTool(ctx context.Context, name string, arguments map[string]any) (mcp.CallResult, error) {
	settings, err := s.loadSettings()
	if err != nil {
		return mcp.CallResult{}, err
	}
	if !settings.Enabled {
		return mcp.CallResult{}, apperr.New(apperr.CodeFailedPrecondition, "Remote MCP is disabled")
	}
	credential, err := s.authorizedCredential(ctx, settings.URL)
	if err != nil {
		return mcp.CallResult{}, err
	}
	client := mcp.NewClient(settings.URL, credential.AccessToken, s.httpClient)
	if err := client.Initialize(ctx); err != nil {
		return mcp.CallResult{}, err
	}
	return client.CallTool(ctx, name, arguments)
}

func (s *RemoteMCPService) loadSettings() (model.RemoteMCPSettings, error) {
	raw, err := s.repo.GetAppSetting(remoteMCPSettingsKey)
	if err != nil {
		return model.RemoteMCPSettings{}, err
	}
	if raw == "" {
		return model.RemoteMCPSettings{Name: "Notion MCP", URL: "https://mcp.notion.com/mcp", AuthMethod: "oauth"}, nil
	}
	var settings model.RemoteMCPSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return model.RemoteMCPSettings{}, apperr.Wrap(apperr.CodeInternal, "解析 MCP 配置失败", err)
	}
	return settings, nil
}

func (s *RemoteMCPService) saveSettings(settings model.RemoteMCPSettings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return s.repo.UpsertAppSetting(remoteMCPSettingsKey, string(raw))
}

func (s *RemoteMCPService) saveAuthorizedConnection(settings model.RemoteMCPSettings, credential oauthCredential) error {
	if err := os.MkdirAll(filepath.Dir(s.credentialPath), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.credentialPath, raw, 0o600); err != nil {
		return err
	}
	_ = os.Chmod(s.credentialPath, 0o600)
	return s.saveSettings(settings)
}

func (s *RemoteMCPService) loadCredential() (*oauthCredential, error) {
	raw, err := os.ReadFile(s.credentialPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取 MCP OAuth 凭据失败", err)
	}
	var credential oauthCredential
	if err := json.Unmarshal(raw, &credential); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "解析 MCP OAuth 凭据失败", err)
	}
	return &credential, nil
}

func (s *RemoteMCPService) authorizedCredential(ctx context.Context, resourceURL string) (*oauthCredential, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	credential, err := s.loadCredential()
	if err != nil {
		return nil, err
	}
	if credential == nil || !sameMCPResource(credential.ResourceURL, resourceURL) {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "Remote MCP requires OAuth authorization")
	}
	if credential.ExpiresAt.IsZero() || time.Until(credential.ExpiresAt) > time.Minute {
		return credential, nil
	}
	if credential.RefreshToken == "" {
		_ = os.Remove(s.credentialPath)
		return nil, apperr.New(apperr.CodeFailedPrecondition, "MCP OAuth expired; reconnect the service")
	}
	values := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {credential.RefreshToken}, "client_id": {credential.ClientID}, "resource": {credential.ResourceURL}}
	refreshed, err := s.requestToken(ctx, credential.TokenEndpoint, credential.ClientID, credential.ClientSecret, values)
	if err != nil {
		var tokenErr *oauthTokenError
		if errors.As(err, &tokenErr) && (tokenErr.Code == "invalid_grant" || tokenErr.Code == "invalid_client") {
			_ = os.Remove(s.credentialPath)
			return nil, apperr.Wrap(apperr.CodeFailedPrecondition, "MCP OAuth refresh failed; reconnect the service", err)
		}
		return nil, apperr.Wrap(apperr.CodeUnavailable, "MCP OAuth refresh temporarily failed", err)
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = credential.RefreshToken
	}
	refreshed.ResourceURL = credential.ResourceURL
	refreshed.ClientID = credential.ClientID
	refreshed.ClientSecret = credential.ClientSecret
	refreshed.TokenEndpoint = credential.TokenEndpoint
	refreshed.UserID = credential.UserID
	refreshed.WorkspaceID = credential.WorkspaceID
	refreshed.ToolNames = append([]string(nil), credential.ToolNames...)
	settings, _ := s.loadSettings()
	if err := s.saveAuthorizedConnection(settings, refreshed); err != nil {
		return nil, err
	}
	return &refreshed, nil
}

func (s *RemoteMCPService) discoverOAuth(ctx context.Context, resourceURL string) (string, oauthMetadata, []string, error) {
	u, err := url.Parse(resourceURL)
	if err != nil {
		return "", oauthMetadata{}, nil, err
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	candidates := []string{
		u.Scheme + "://" + u.Host + "/.well-known/oauth-protected-resource" + path,
		strings.TrimRight(resourceURL, "/") + "/.well-known/oauth-protected-resource",
		u.Scheme + "://" + u.Host + "/.well-known/oauth-protected-resource",
	}
	if challenged := s.resourceMetadataFromChallenge(ctx, resourceURL); challenged != "" {
		candidates = append([]string{challenged}, candidates...)
	}
	var protected protectedResourceMetadata
	if err := s.getFirstJSON(ctx, candidates, &protected); err != nil {
		return "", oauthMetadata{}, nil, err
	}
	if len(protected.AuthorizationServers) == 0 {
		return "", oauthMetadata{}, nil, fmt.Errorf("protected resource metadata returned no authorization server")
	}
	issuer, err := url.Parse(protected.AuthorizationServers[0])
	if err != nil {
		return "", oauthMetadata{}, nil, err
	}
	issuerPath := strings.TrimRight(issuer.EscapedPath(), "/")
	base := issuer.Scheme + "://" + issuer.Host
	metadataCandidates := []string{
		base + "/.well-known/oauth-authorization-server" + issuerPath,
		strings.TrimRight(issuer.String(), "/") + "/.well-known/oauth-authorization-server",
		base + "/.well-known/openid-configuration" + issuerPath,
		strings.TrimRight(issuer.String(), "/") + "/.well-known/openid-configuration",
	}
	var metadata oauthMetadata
	if err := s.getFirstJSON(ctx, metadataCandidates, &metadata); err != nil {
		return "", oauthMetadata{}, nil, err
	}
	if metadata.AuthorizationEndpoint == "" || metadata.TokenEndpoint == "" || metadata.RegistrationEndpoint == "" {
		return "", oauthMetadata{}, nil, fmt.Errorf("authorization metadata is missing required endpoints")
	}
	scopes := protected.ScopesSupported
	if len(scopes) == 0 {
		scopes = metadata.ScopesSupported
	}
	return resourceURL, metadata, scopes, nil
}

func (s *RemoteMCPService) resourceMetadataFromChallenge(ctx context.Context, resourceURL string) string {
	payload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"CiteBox","version":"0.1"}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resourceURL, strings.NewReader(payload))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return ""
	}
	challenge := resp.Header.Get("WWW-Authenticate")
	marker := `resource_metadata="`
	start := strings.Index(challenge, marker)
	if start < 0 {
		return ""
	}
	value := challenge[start+len(marker):]
	end := strings.IndexByte(value, '"')
	if end < 0 {
		return ""
	}
	metadataURL := strings.TrimSpace(value[:end])
	parsed, err := url.Parse(metadataURL)
	if err != nil || parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return ""
	}
	return metadataURL
}

func (s *RemoteMCPService) getFirstJSON(ctx context.Context, candidates []string, target any) error {
	var lastErr error
	for _, candidate := range candidates {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, candidate, nil)
		req.Header.Set("Accept", "application/json")
		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("%s returned %s", candidate, resp.Status)
			continue
		}
		if err := json.Unmarshal(data, target); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func (s *RemoteMCPService) registerClient(ctx context.Context, metadata oauthMetadata, redirectURI string) (string, string, error) {
	payload := map[string]any{
		"client_name":                "CiteBox",
		"client_uri":                 "https://github.com/xuzhougeng/citebox",
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	}
	raw, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, metadata.RegistrationEndpoint, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("registration returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var out struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.ClientID == "" {
		return "", "", fmt.Errorf("registration response is missing client_id")
	}
	return out.ClientID, out.ClientSecret, nil
}

func (s *RemoteMCPService) exchangeCode(ctx context.Context, flow *oauthFlow, code string) (oauthCredential, error) {
	values := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {flow.RedirectURI},
		"client_id": {flow.ClientID}, "code_verifier": {flow.Verifier}, "resource": {flow.Settings.URL},
	}
	credential, err := s.requestToken(ctx, flow.Metadata.TokenEndpoint, flow.ClientID, flow.ClientSecret, values)
	if err != nil {
		return oauthCredential{}, err
	}
	credential.ResourceURL = flow.Settings.URL
	credential.ClientID = flow.ClientID
	credential.ClientSecret = flow.ClientSecret
	credential.TokenEndpoint = flow.Metadata.TokenEndpoint
	return credential, nil
}

func (s *RemoteMCPService) requestToken(ctx context.Context, endpoint, clientID, clientSecret string, values url.Values) (oauthCredential, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if clientSecret != "" {
		req.SetBasicAuth(clientID, clientSecret)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return oauthCredential{}, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if json.Unmarshal(data, &payload) == nil && payload.Error != "" {
			return oauthCredential{}, &oauthTokenError{Status: resp.Status, Code: payload.Error, Description: payload.ErrorDescription}
		}
		return oauthCredential{}, fmt.Errorf("token endpoint returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		UserID       string `json:"user_id"`
		WorkspaceID  string `json:"workspace_id"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(data, &token); err != nil || token.AccessToken == "" {
		return oauthCredential{}, fmt.Errorf("token response is missing access_token")
	}
	credential := oauthCredential{
		AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, Scope: token.Scope,
		UserID: token.UserID, WorkspaceID: token.WorkspaceID,
	}
	if token.ExpiresIn > 0 {
		credential.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	return credential, nil
}

func (s *RemoteMCPService) listToolsWithCredential(ctx context.Context, credential oauthCredential) ([]mcp.Tool, error) {
	client := mcp.NewClient(credential.ResourceURL, credential.AccessToken, s.httpClient)
	if err := client.Initialize(ctx); err != nil {
		return nil, err
	}
	return client.ListTools(ctx)
}

func (s *RemoteMCPService) finishFlow(flowID string, status model.MCPAuthorizationStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if flow := s.flows[flowID]; flow != nil {
		flow.Status = status
		flow.Verifier = ""
		flow.ClientSecret = ""
	}
}

func (s *RemoteMCPService) pruneFlowsLocked() {
	cutoff := time.Now().Add(-10 * time.Minute)
	for id, flow := range s.flows {
		if flow.CreatedAt.Before(cutoff) {
			delete(s.flows, id)
		}
	}
}

func normalizeRemoteMCPSettings(settings model.RemoteMCPSettings) (model.RemoteMCPSettings, error) {
	settings.Name = strings.TrimSpace(settings.Name)
	settings.URL = strings.TrimSpace(settings.URL)
	settings.AuthMethod = strings.ToLower(strings.TrimSpace(settings.AuthMethod))
	if settings.Name == "" || settings.URL == "" {
		return settings, apperr.New(apperr.CodeInvalidArgument, "请填写 MCP 名称和远程 URL")
	}
	u, err := url.Parse(settings.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname()))) {
		return settings, apperr.New(apperr.CodeInvalidArgument, "Remote MCP URL must use HTTPS (HTTP is allowed only for loopback)")
	}
	if settings.AuthMethod == "" {
		settings.AuthMethod = "oauth"
	}
	if settings.AuthMethod != "oauth" {
		return settings, apperr.New(apperr.CodeInvalidArgument, "Remote MCP currently supports OAuth authentication")
	}
	return settings, nil
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func randomURLSafe(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func sameMCPResource(a, b string) bool {
	return strings.TrimRight(strings.TrimSpace(a), "/") == strings.TrimRight(strings.TrimSpace(b), "/")
}

func toolNames(tools []mcp.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func MCPCallbackHTML(message string, success bool) string {
	title := "MCP connection failed"
	if success {
		title = "MCP connected"
	}
	return "<!doctype html><meta charset=utf-8><title>" + title + "</title><body><main><h1>" + title + "</h1><p>" + html.EscapeString(message) + "</p><p>You may close this window and return to CiteBox.</p></main></body>"
}
