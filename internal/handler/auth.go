package handler

import (
	"encoding/json"
	"net/http"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

type AuthHandler struct {
	libraryService *service.LibraryService
}

func NewAuthHandler(libraryService *service.LibraryService) *AuthHandler {
	return &AuthHandler{libraryService: libraryService}
}

func (h *AuthHandler) GetAuthSettings(w http.ResponseWriter, r *http.Request) {
	settings := h.libraryService.GetAuthSettings()
	sendJSON(w, http.StatusOK, settings)
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req model.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	if err := h.libraryService.ChangePassword(req.CurrentPassword, req.NewPassword); err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "密码修改成功，请使用新密码重新登录",
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Basic Auth 的登出主要是通过返回 401 让浏览器清除缓存
	// 但现代浏览器通常会记住凭据直到关闭
	// 这里我们返回一个特殊的响应，前端可以据此处理
	w.Header().Set("WWW-Authenticate", `Basic realm="Restricted", charset="UTF-8"`)
	sendJSON(w, http.StatusUnauthorized, map[string]interface{}{
		"success": true,
		"message": "已登出，请关闭浏览器或清除缓存以完全退出",
		"action":  "logout",
	})
}
