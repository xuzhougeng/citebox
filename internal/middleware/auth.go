package middleware

import (
	"net/http"

	"paper_image_db/internal/config"
)

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
