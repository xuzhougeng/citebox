package main

import (
	"bytes"
	"encoding/json"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

const bridgeScriptPath = "/__citebox_desktop/bridge.js"

// shimScript maps the window.citeboxDesktop* functions the bridge script
// expects onto DesktopService methods, so the shared bridge and the pages
// stay unchanged. Wails's runtime.js is an ES module, so it is loaded with a
// dynamic import from this classic script.
const shimScript = `(function() {
    const runtime = import('/wails/runtime.js');
    const call = (method) => (...args) => runtime.then((wails) => wails.Call.ByName('main.DesktopService.' + method, ...args));
    window.citeboxDesktopOpenExternal = call('OpenExternal');
    window.citeboxDesktopMinimizeToTray = call('MinimizeToTray');
    window.citeboxDesktopExitApp = call('ExitApp');
    window.citeboxDesktopGetClosePreference = call('GetClosePreference');
    window.citeboxDesktopSetClosePreference = call('SetClosePreference');
    window.citeboxDesktopReadClipboardText = call('ReadClipboardText');
    window.citeboxDesktopWriteClipboardText = call('WriteClipboardText');
    window.citeboxDesktopSaveFile = call('SaveFile');
})();
`

var headTagPattern = regexp.MustCompile(`(?i)<head[^>]*>`)

// injectBridge serves the bridge script and adds a <script> tag for it to the
// start of every HTML page, which stands in for webview_go's Init().
func injectBridge(next http.Handler, script string) http.Handler {
	scriptTag := []byte(`<script src="` + bridgeScriptPath + `"></script>`)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == bridgeScriptPath {
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
