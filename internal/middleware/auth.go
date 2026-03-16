package middleware

import (
	"net/http"
	"strings"

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

// PublicPath represents a path that can be accessed without authentication
type PublicPath struct {
	Path      string
	Prefix    bool // if true, matches any path starting with Path
}

// AuthMiddleware creates a middleware that protects routes with Basic Auth,
// but allows public paths to be accessed without authentication.
// When authentication fails on protected HTML pages, it redirects to login.
func AuthMiddleware(svc *service.LibraryService, publicPaths []PublicPath) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Check if this is a public path
			isPublic := false
			for _, pp := range publicPaths {
				if pp.Prefix {
					if strings.HasPrefix(r.URL.Path, pp.Path) {
						isPublic = true
						break
					}
				} else {
					if r.URL.Path == pp.Path || r.URL.Path == pp.Path+"/" {
						isPublic = true
						break
					}
				}
			}

			if isPublic {
				next.ServeHTTP(w, r)
				return
			}

			// Check authentication
			username, password, ok := r.BasicAuth()
			if !ok || !svc.ValidateCredentials(username, password) {
				// For HTML pages, redirect to login
				if isHTMLPage(r) {
					http.Redirect(w, r, "/login", http.StatusFound)
					return
				}
				// For API requests, return 401
				w.Header().Set("WWW-Authenticate", `Basic realm="Restricted", charset="UTF-8"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isHTMLPage checks if the request is for an HTML page
func isHTMLPage(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	path := r.URL.Path

	// Check Accept header
	if strings.Contains(accept, "text/html") {
		return true
	}

	// Check common HTML page paths
	htmlPaths := []string{"/", "/index.html", "/upload", "/figures", "/groups", "/tags", "/ai", "/settings"}
	for _, p := range htmlPaths {
		if path == p || path == p+".html" {
			return true
		}
	}

	return false
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
