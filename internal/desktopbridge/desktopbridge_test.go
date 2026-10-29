package desktopbridge

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestScriptFillsInternalRoutes(t *testing.T) {
	script, err := Script()
	if err != nil {
		t.Fatalf("Script() error = %v", err)
	}
	if strings.Contains(script, "%s") || !strings.Contains(script, `"/library"`) {
		t.Fatalf("Script() did not fill in the route list")
	}
}

func TestInjectScriptAddsTagAfterHead(t *testing.T) {
	page := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", "48")
		_, _ = io.WriteString(w, `<html><head lang="en"><title>x</title></head></html>`)
	})
	handler := InjectScript(page, "window.x = 1;")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/library", nil))

	want := `<head lang="en"><script src="` + ScriptPath + `"></script><title>`
	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("body = %q, want it to contain %q", rec.Body.String(), want)
	}
	if got := rec.Header().Get("Content-Length"); got != "" && got != strconv.Itoa(rec.Body.Len()) {
		t.Fatalf("Content-Length = %s, body is %d bytes", got, rec.Body.Len())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ScriptPath, nil))
	if rec.Body.String() != "window.x = 1;" {
		t.Fatalf("script body = %q", rec.Body.String())
	}
}

func TestInjectScriptTurnsRedirectIntoClientRedirect(t *testing.T) {
	handler := InjectScript(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusFound)
	}), "")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Location") != "" {
		t.Fatalf("Location header should be removed")
	}
	if !strings.Contains(rec.Body.String(), `location.replace("/login")`) {
		t.Fatalf("body = %q, want a client redirect to /login", rec.Body.String())
	}
}

func TestInjectScriptPassesAPIAndStreamsThrough(t *testing.T) {
	handler := InjectScript(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: one\n\n")
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, "data: two\n\n")
	}), "")

	for _, path := range []string{"/api/ai/stream", "/events"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Body.String() != "data: one\n\ndata: two\n\n" || !rec.Flushed {
			t.Fatalf("%s: body = %q flushed = %v", path, rec.Body.String(), rec.Flushed)
		}
	}
}

func TestCookieStoreRoundTripsAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")
	store := NewCookieStore(path, discardLogger())

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "s1", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "remember", Value: "r1", Path: "/", MaxAge: 3600})
		case "/logout":
			http.SetCookie(w, &http.Cookie{Name: "session", Path: "/", MaxAge: -1})
			http.SetCookie(w, &http.Cookie{Name: "remember", Path: "/", MaxAge: -1})
		default:
			var names []string
			for _, cookie := range r.Cookies() {
				names = append(names, cookie.Name+"="+cookie.Value)
			}
			_, _ = io.WriteString(w, strings.Join(sortedStrings(names), ";"))
		}
	})
	handler := WithCookieStore(inner, store)

	serve := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	if rec := serve("/login"); rec.Header().Get("Set-Cookie") != "" {
		t.Fatalf("Set-Cookie should not reach the webview")
	}
	if got := serve("/").Body.String(); got != "remember=r1;session=s1" {
		t.Fatalf("cookies sent = %q", got)
	}

	// A restart keeps only the persistent cookie.
	restarted := WithCookieStore(inner, NewCookieStore(path, discardLogger()))
	rec := httptest.NewRecorder()
	restarted.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Body.String(); got != "remember=r1" {
		t.Fatalf("cookies after restart = %q", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat cookie file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("cookie file mode = %v, want 0600", info.Mode().Perm())
	}

	serve("/logout")
	if got := serve("/").Body.String(); got != "" {
		t.Fatalf("cookies after logout = %q", got)
	}
	if got := NewCookieStore(path, discardLogger()); len(got.cookies) != 0 {
		t.Fatalf("saved cookies after logout = %v", got.cookies)
	}
}

func TestCookieStoreDropsExpiredCookies(t *testing.T) {
	store := NewCookieStore(filepath.Join(t.TempDir(), "cookies.json"), discardLogger())
	store.update([]*http.Cookie{{Name: "old", Value: "v", Expires: time.Now().Add(-time.Minute)}})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	store.addTo(req)
	if len(req.Cookies()) != 0 {
		t.Fatalf("expired cookie was sent: %v", req.Cookies())
	}
}

func sortedStrings(values []string) []string {
	sort.Strings(values)
	return values
}
