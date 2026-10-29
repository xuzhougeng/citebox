package desktopbridge

import (
	"bytes"
	"encoding/json"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// ScriptPath is where InjectScript serves the injected script.
const ScriptPath = "/__citebox_desktop/bridge.js"

var headTagPattern = regexp.MustCompile(`(?i)<head[^>]*>`)

// InjectScript serves script at ScriptPath and adds a <script> tag for it to
// the start of every HTML page, so it runs before the page's own scripts.
// Navigation redirects become client-side redirects, because the macOS
// webview renders the body of a 3xx from a custom-scheme handler instead of
// following it.
func InjectScript(next http.Handler, script string) http.Handler {
	scriptTag := []byte(`<script src="` + ScriptPath + `"></script>`)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == ScriptPath {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(script))
			return
		}
		if r.Method != http.MethodGet || strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		rec := &htmlRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.redirect != "" {
			writeClientRedirect(w, rec.redirect)
			return
		}
		if !rec.buffering {
			return
		}

		body := rec.buf.Bytes()
		if loc := headTagPattern.FindIndex(body); loc != nil {
			patched := make([]byte, 0, len(body)+len(scriptTag))
			patched = append(patched, body[:loc[1]]...)
			patched = append(patched, scriptTag...)
			patched = append(patched, body[loc[1]:]...)
			body = patched
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(rec.status)
		_, _ = w.Write(body)
	})
}

// writeClientRedirect turns a server redirect into an HTML page that
// navigates on the client. The macOS webview does not follow 3xx responses
// from the custom-scheme asset handler and renders their body instead.
func writeClientRedirect(w http.ResponseWriter, location string) {
	escaped := html.EscapeString(location)
	quoted, _ := json.Marshal(location)
	body := `<!doctype html><meta http-equiv="refresh" content="0;url=` + escaped + `">` +
		`<script>location.replace(` + string(quoted) + `);</script>`
	w.Header().Del("Location")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// htmlRecorder buffers 200 text/html responses, captures redirects, and
// passes everything else (including streamed responses) straight through.
type htmlRecorder struct {
	http.ResponseWriter
	buf         bytes.Buffer
	status      int
	wroteHeader bool
	buffering   bool
	redirect    string
}

func (h *htmlRecorder) WriteHeader(status int) {
	if h.wroteHeader {
		return
	}
	h.wroteHeader = true
	h.status = status
	if status >= 300 && status < 400 && h.Header().Get("Location") != "" {
		h.redirect = h.Header().Get("Location")
		h.buffering = true
		return
	}
	contentType := h.Header().Get("Content-Type")
	if status == http.StatusOK && strings.HasPrefix(contentType, "text/html") {
		h.buffering = true
		h.Header().Del("Content-Length")
		return
	}
	h.ResponseWriter.WriteHeader(status)
}

func (h *htmlRecorder) Write(p []byte) (int, error) {
	if !h.wroteHeader {
		if h.Header().Get("Content-Type") == "" {
			h.Header().Set("Content-Type", http.DetectContentType(p))
		}
		h.WriteHeader(http.StatusOK)
	}
	if h.buffering {
		return h.buf.Write(p)
	}
	return h.ResponseWriter.Write(p)
}

func (h *htmlRecorder) Flush() {
	if h.buffering {
		return
	}
	if flusher, ok := h.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
