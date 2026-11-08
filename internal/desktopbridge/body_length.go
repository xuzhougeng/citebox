package desktopbridge

import "net/http"

// WithUnknownBodyLength marks a request body of unknown size as such.
// WebView2 sends no Content-Length, and Wails v3.0.0 then leaves
// ContentLength at 0 even though the body has data (wailsapp/wails#6316).
// Code that trusts ContentLength would treat the body as empty, so a body
// with no declared length gets -1 ("unknown"), as net/http's server does.
func WithUnknownBodyLength(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength == 0 && r.Body != nil && r.Body != http.NoBody && r.Header.Get("Content-Length") == "" {
			r.ContentLength = -1
		}
		next.ServeHTTP(w, r)
	})
}
