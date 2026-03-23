package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

type AuthHandler struct {
	libraryService *service.LibraryService
	sessionManager *service.SessionManager
}

func NewAuthHandler(libraryService *service.LibraryService, sessionManager *service.SessionManager) *AuthHandler {
	return &AuthHandler{
		libraryService: libraryService,
		sessionManager: sessionManager,
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请输入用户名和密码"))
		return
	}

	if !h.libraryService.ValidateCredentials(username, req.Password) {
		sendError(w, apperr.New(apperr.CodeUnauthenticated, "用户名或密码错误"))
		return
	}

	if cookie, err := r.Cookie(service.SessionCookieName); err == nil {
		h.sessionManager.Delete(cookie.Value)
	}

	session, err := h.sessionManager.Create(username)
	if err != nil {
		sendError(w, apperr.Wrap(apperr.CodeInternal, "创建登录会话失败", err))
		return
	}

	http.SetCookie(w, buildSessionCookie(r, session, h.sessionManager.TTL()))
	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "登录成功",
	})
}

func (h *AuthHandler) GetAuthSettings(w http.ResponseWriter, r *http.Request) {
	settings := h.libraryService.GetAuthSettings()
	sendJSON(w, http.StatusOK, settings)
}

func (h *AuthHandler) StartWeixinBinding(w http.ResponseWriter, r *http.Request) {
	result, err := h.libraryService.StartWeixinBinding(r.Context())
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
}

func (h *AuthHandler) GetWeixinBindingStatus(w http.ResponseWriter, r *http.Request) {
	result, err := h.libraryService.GetWeixinBindingStatus(r.Context(), r.URL.Query().Get("qrcode"))
	if err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, result)
}

func (h *AuthHandler) UnbindWeixin(w http.ResponseWriter, r *http.Request) {
	if err := h.libraryService.UnbindWeixin(); err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "微信绑定已解除",
	})
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

	h.sessionManager.DeleteAll()
	http.SetCookie(w, clearSessionCookie(r))
	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "密码修改成功，请使用新密码重新登录",
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(service.SessionCookieName); err == nil {
		h.sessionManager.Delete(cookie.Value)
	}

	http.SetCookie(w, clearSessionCookie(r))
	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "已登出",
		"action":  "logout",
	})
}

func buildSessionCookie(r *http.Request, session service.Session, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     service.SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		MaxAge:   int(ttl.Seconds()),
		Expires:  session.ExpiresAt,
	}
}

func clearSessionCookie(r *http.Request) *http.Cookie {
	return &http.Cookie{
		Name:     service.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}

	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}
