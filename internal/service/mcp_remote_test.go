package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestRemoteMCPOAuthTestAndSave(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/mcp", "/metadata-challenge":
			_ = json.NewEncoder(w).Encode(map[string]any{"authorization_servers": []string{server.URL}, "scopes_supported": []string{"mcp"}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize",
				"token_endpoint": server.URL + "/token", "registration_endpoint": server.URL + "/register",
			})
		case "/register":
			_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "citebox-test"})
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("code_verifier") == "" || r.Form.Get("resource") != server.URL+"/mcp" {
				t.Fatalf("unexpected token form: %v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "refresh_token": "refresh", "expires_in": 3600})
		case "/mcp":
			if r.Header.Get("Authorization") == "" {
				w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+server.URL+`/metadata-challenge"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Header.Get("Authorization") != "Bearer access" {
				t.Fatalf("missing MCP bearer token")
			}
			var request struct {
				ID     int64  `json:"id"`
				Method string `json:"method"`
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			switch request.Method {
			case "initialize":
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"protocolVersion": "2025-06-18"}})
			case "notifications/initialized":
				w.WriteHeader(http.StatusAccepted)
			case "tools/list":
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"tools": []map[string]any{{"name": "notion-search"}}}})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	repo, err := repository.NewLibraryRepository(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	remote := NewRemoteMCPService(repo.Setting, filepath.Join(root, "storage"))
	remote.httpClient = server.Client()
	settings := model.RemoteMCPSettings{Name: "Notion", URL: server.URL + "/mcp", AuthMethod: "oauth", Enabled: true}

	testFlow, err := remote.StartAuthorization(context.Background(), settings, "test", "http://127.0.0.1/callback")
	if err != nil {
		t.Fatal(err)
	}
	state := authorizationState(t, testFlow.AuthorizationURL)
	if _, ok := remote.CompleteAuthorization(context.Background(), state, "code", "", ""); !ok {
		t.Fatal("test authorization failed")
	}
	view, err := remote.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if view.Authorized {
		t.Fatal("test mode persisted credentials")
	}

	saveFlow, err := remote.StartAuthorization(context.Background(), settings, "save", "http://127.0.0.1/callback")
	if err != nil {
		t.Fatal(err)
	}
	state = authorizationState(t, saveFlow.AuthorizationURL)
	if _, ok := remote.CompleteAuthorization(context.Background(), state, "code", "", ""); !ok {
		t.Fatal("save authorization failed")
	}
	view, err = remote.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !view.Authorized || len(view.ToolNames) != 1 || view.ToolNames[0] != "notion-search" {
		t.Fatalf("view = %#v", view)
	}
}

func TestRemoteMCPOAuthRejectsWrongStateBeforeOAuthError(t *testing.T) {
	remote := &RemoteMCPService{flows: map[string]*oauthFlow{}}
	message, ok := remote.CompleteAuthorization(context.Background(), "wrong", "", "access_denied", "declined")
	if ok || message != "OAuth state did not match; authorization was rejected." {
		t.Fatalf("message=%q ok=%v", message, ok)
	}
}

func TestMCPCallbackHTMLEscapesError(t *testing.T) {
	html := MCPCallbackHTML(`<script>alert(1)</script>`, false)
	if strings.Contains(html, "<script>") || !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("callback HTML was not escaped: %s", html)
	}
}

func TestRemoteMCPRefreshOnlyClearsTerminalGrant(t *testing.T) {
	terminal := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if terminal {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"expired"}`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`temporarily unavailable`))
	}))
	defer server.Close()

	root := t.TempDir()
	repo, err := repository.NewLibraryRepository(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	remote := NewRemoteMCPService(repo.Setting, filepath.Join(root, "storage"))
	remote.httpClient = server.Client()
	settings := model.RemoteMCPSettings{Name: "Local", URL: server.URL, AuthMethod: "oauth", Enabled: true}
	credential := oauthCredential{
		ResourceURL: settings.URL, ClientID: "client", TokenEndpoint: server.URL,
		AccessToken: "expired", RefreshToken: "refresh", ExpiresAt: time.Now().Add(-time.Minute),
	}
	if err := remote.saveAuthorizedConnection(settings, credential); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.authorizedCredential(context.Background(), settings.URL); err == nil {
		t.Fatal("expected transient refresh error")
	}
	if _, err := remote.loadCredential(); err != nil {
		t.Fatal(err)
	} else if credential, _ := remote.loadCredential(); credential == nil {
		t.Fatal("transient refresh error cleared credential")
	}

	terminal = true
	if _, err := remote.authorizedCredential(context.Background(), settings.URL); err == nil {
		t.Fatal("expected invalid_grant refresh error")
	}
	if credential, err := remote.loadCredential(); err != nil || credential != nil {
		t.Fatalf("terminal refresh credential=%#v err=%v", credential, err)
	}
}

func authorizationState(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("state")
}
