package middleware

import (
	"net/http"

	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/service"
)

type AuthChecker interface {
	ValidateCredentials(username, password string) bool
}

// BasicAuth wraps an http.Handler and enforces HTTP Basic authentication using
// the admin credentials from configuration. OPTIONS requests bypass the check
// so that CORS preflight can succeed.
func BasicAuth(next http.Handler, cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		username, password, ok := r.BasicAuth()
		if !ok || username != cfg.AdminUsername || password != cfg.AdminPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// BasicAuthWithService wraps an http.Handler and enforces HTTP Basic authentication
// using the service to validate credentials, which supports runtime password changes.
func BasicAuthWithService(next http.Handler, svc *service.LibraryService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		username, password, ok := r.BasicAuth()
		if !ok || !svc.ValidateCredentials(username, password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// OptionalBasicAuth allows requests without authentication to proceed to login page
func OptionalBasicAuth(next http.Handler, svc *service.LibraryService, publicPaths []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		// 检查是否是公开路径
		for _, path := range publicPaths {
			if r.URL.Path == path || (path[len(path)-1] == '/' && len(r.URL.Path) > len(path) && r.URL.Path[:len(path)] == path) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// 检查认证
		username, password, ok := r.BasicAuth()
		if !ok || !svc.ValidateCredentials(username, password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
