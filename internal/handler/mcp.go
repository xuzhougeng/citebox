package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

type MCPHandler struct {
	service *service.RemoteMCPService
}

func NewMCPHandler(remote *service.RemoteMCPService) *MCPHandler {
	return &MCPHandler{service: remote}
}

func (h *MCPHandler) Settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := h.service.GetSettings()
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, settings)
	case http.MethodDelete:
		if err := h.service.Disconnect(); err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"success": true})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *MCPHandler) StartAuthorization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Mode string `json:"mode"`
		model.RemoteMCPSettings
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}
	start, err := h.service.StartAuthorization(r.Context(), req.RemoteMCPSettings, req.Mode, oauthCallbackURL(r))
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, start)
}

func (h *MCPHandler) AuthorizationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := h.service.AuthorizationStatus(strings.TrimSpace(r.URL.Query().Get("flow_id")))
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, status)
}

func (h *MCPHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	// The request logger runs after the handler; clear the authorization code
	// and state so neither OAuth secret is written to application logs.
	r.URL.RawQuery = ""
	message, ok := h.service.CompleteAuthorization(r.Context(), q.Get("state"), q.Get("code"), q.Get("error"), q.Get("error_description"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
	}
	_, _ = w.Write([]byte(service.MCPCallbackHTML(message, ok)))
}

func oauthCallbackURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	host := r.Host
	u := url.URL{Scheme: scheme, Host: host, Path: "/api/settings/mcp/oauth/callback"}
	return u.String()
}
