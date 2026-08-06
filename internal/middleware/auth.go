package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/integration"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

// PublicPath represents a path that can be accessed without authentication.
type PublicPath struct {
	Path   string
	Prefix bool // if true, matches any path starting with Path
}

// BearerAuthenticator 校验请求携带的 Authorization: Bearer 令牌，通过返回 true
type BearerAuthenticator func(r *http.Request) bool

// IntegrationBearerAuthenticator 返回基于集成令牌的 Bearer 认证器，
// 仅接受带 library:write 权限的令牌（供浏览器插件等外部客户端一键入库使用）
func IntegrationBearerAuthenticator(tokens *integration.TokenService) BearerAuthenticator {
	return func(r *http.Request) bool {
		if tokens == nil {
			return false
		}
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return false
		}
		token, err := tokens.Authenticate(parts[1])
		if err != nil {
			return false
		}
		return integration.HasScope(token, integration.ScopeLibraryWrite)
	}
}

// AuthMiddleware protects routes with a session cookie and redirects
// unauthenticated HTML requests to the login page.
// Requests carrying a valid Bearer integration token (see bearerAuth) are
// treated as authenticated without a session cookie.
// When disableAuth is true, all requests pass through and the login page
// is redirected back to the app root.
func AuthMiddleware(sessionManager *service.SessionManager, libraryService *service.LibraryService, publicPaths []PublicPath, disableAuth bool, bearerAuth BearerAuthenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			if disableAuth {
				if isLoginPath(r.URL.Path) {
					http.Redirect(w, r, "/", http.StatusFound)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			isPublic := matchesPublicPath(r.URL.Path, publicPaths)
			_, authenticated := sessionFromRequest(r, sessionManager)
			if !authenticated {
				authenticated = authenticateRememberedSession(w, r, sessionManager, libraryService)
			}
			if !authenticated && bearerAuth != nil {
				authenticated = bearerAuth(r)
			}

			if isLoginPath(r.URL.Path) && authenticated {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}

			if isPublic {
				next.ServeHTTP(w, r)
				return
			}

			if !authenticated {
				if isHTMLPage(r) {
					http.Redirect(w, r, "/login", http.StatusFound)
					return
				}
				writeUnauthenticatedJSON(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isHTMLPage checks if the request is for an HTML page.
func isHTMLPage(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	path := r.URL.Path

	if strings.Contains(accept, "text/html") {
		return true
	}

	htmlPaths := []string{"/", "/index.html", "/library", "/guide", "/upload", "/manual", "/viewer", "/figures", "/groups", "/tags", "/notes", "/ai", "/settings"}
	for _, p := range htmlPaths {
		if path == p || path == p+".html" {
			return true
		}
	}

	return false
}

func matchesPublicPath(path string, publicPaths []PublicPath) bool {
	for _, pp := range publicPaths {
		if pp.Prefix {
			if strings.HasPrefix(path, pp.Path) {
				return true
			}
			continue
		}

		if path == pp.Path || path == pp.Path+"/" {
			return true
		}
	}

	return false
}

func sessionFromRequest(r *http.Request, sessionManager *service.SessionManager) (service.Session, bool) {
	cookie, err := r.Cookie(service.SessionCookieName)
	if err != nil {
		return service.Session{}, false
	}

	return sessionManager.Validate(cookie.Value)
}

func isLoginPath(path string) bool {
	return path == "/login" || path == "/login.html"
}

func authenticateRememberedSession(w http.ResponseWriter, r *http.Request, sessionManager *service.SessionManager, libraryService *service.LibraryService) bool {
	if libraryService == nil {
		return false
	}

	cookie, err := r.Cookie(service.RememberLoginCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false
	}

	if !libraryService.HasRememberLoginToken(cookie.Value) {
		http.SetCookie(w, service.ClearRememberLoginCookie(r))
		return false
	}

	session, err := sessionManager.Create(libraryService.AdminUsername())
	if err != nil {
		return false
	}

	http.SetCookie(w, service.BuildSessionCookie(r, session))
	return true
}

func writeUnauthenticatedJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(model.ErrorResponse{
		Success: false,
		Code:    string(apperr.CodeUnauthenticated),
		Error:   "请先登录",
	})
}
