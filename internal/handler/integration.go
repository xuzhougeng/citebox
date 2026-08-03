package handler

import (
	"encoding/json"
	"net/http"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/integration"
	"github.com/xuzhougeng/citebox/internal/mcpserver"
)

// IntegrationHandler 负责外部集成（内置 MCP 服务）的设置与令牌管理
type IntegrationHandler struct {
	svc    *integration.Service
	tokens *integration.TokenService
	mcp    *mcpserver.Server
}

func NewIntegrationHandler(svc *integration.Service, tokens *integration.TokenService, mcp *mcpserver.Server) *IntegrationHandler {
	return &IntegrationHandler{svc: svc, tokens: tokens, mcp: mcp}
}

// Settings 处理 GET/PUT /api/settings/integration
func (h *IntegrationHandler) Settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.sendSettingsView(w, nil)
	case http.MethodPut:
		var req struct {
			Enabled bool `json:"enabled"`
			Port    int  `json:"port"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
			return
		}
		if req.Port < 1 || req.Port > 65535 {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "集成服务端口无效"))
			return
		}
		if err := h.svc.Update(req.Enabled, req.Port); err != nil {
			sendError(w, err)
			return
		}
		// 按新设置重启 MCP 服务（启用/停止/重新绑定端口）
		if err := h.mcp.Restart(); err != nil {
			sendError(w, err)
			return
		}
		h.sendSettingsView(w, nil)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// RotateToken 处理 POST /api/settings/integration/token/rotate
func (h *IntegrationHandler) RotateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	plaintext, _, err := h.tokens.Rotate()
	if err != nil {
		sendError(w, err)
		return
	}
	h.sendSettingsView(w, map[string]any{"new_token": plaintext})
}

// DeleteToken 处理 DELETE /api/settings/integration/token
func (h *IntegrationHandler) DeleteToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.tokens.Revoke(); err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]any{"success": true})
}

// sendSettingsView 返回设置视图；extra 中的字段会合并进响应（如轮换后的新令牌明文）
func (h *IntegrationHandler) sendSettingsView(w http.ResponseWriter, extra map[string]any) {
	settings := h.svc.Get()
	view := map[string]any{
		"enabled": settings.Enabled,
		"port":    settings.Port,
		"url":     h.svc.BaseURL() + "/mcp",
		"token":   nil,
	}
	token, err := h.tokens.Status()
	switch {
	case err == nil:
		view["token"] = map[string]any{
			"active":       true,
			"created_at":   token.CreatedAt,
			"last_used_at": token.LastUsedAt,
			"scopes":       token.Scopes,
		}
	case apperr.IsCode(err, apperr.CodeNotFound):
		// 尚未创建令牌，token 保持 null
	default:
		sendError(w, err)
		return
	}
	for key, value := range extra {
		view[key] = value
	}
	sendJSON(w, http.StatusOK, view)
}
