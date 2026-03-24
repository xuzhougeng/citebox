package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/service"
)

func testPublicPaths() []PublicPath {
	return []PublicPath{
		{Path: "/login", Prefix: false},
		{Path: "/login.html", Prefix: false},
		{Path: "/api/auth/login", Prefix: false},
		{Path: "/static/", Prefix: true},
	}
}

func TestAuthMiddlewareRedirectsHTMLWithoutSession(t *testing.T) {
	sessionManager := service.NewSessionManager(time.Hour)
	protected := AuthMiddleware(sessionManager, testPublicPaths(), false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()

	protected.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusFound)
	}
	if location := resp.Header.Get("Location"); location != "/login" {
		t.Fatalf("Location = %q, want %q", location, "/login")
	}
}

func TestAuthMiddlewareReturnsJSONWithoutSession(t *testing.T) {
	sessionManager := service.NewSessionManager(time.Hour)
	protected := AuthMiddleware(sessionManager, testPublicPaths(), false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/papers", nil)
	w := httptest.NewRecorder()

	protected.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	if got := resp.Header.Get("WWW-Authenticate"); got != "" {
		t.Fatalf("WWW-Authenticate = %q, want empty", got)
	}
}

func TestAuthMiddlewareAllowsValidSession(t *testing.T) {
	sessionManager := service.NewSessionManager(time.Hour)
	session, err := sessionManager.Create("wanglab")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var called bool
	protected := AuthMiddleware(sessionManager, testPublicPaths(), false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/papers", nil)
	req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: session.ID})
	w := httptest.NewRecorder()

	protected.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if !called {
		t.Fatal("next handler was not called")
	}
}

func TestAuthMiddlewareRedirectsAuthenticatedLoginPage(t *testing.T) {
	sessionManager := service.NewSessionManager(time.Hour)
	session, err := sessionManager.Create("wanglab")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	protected := AuthMiddleware(sessionManager, testPublicPaths(), false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: session.ID})
	w := httptest.NewRecorder()

	protected.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusFound)
	}
	if location := resp.Header.Get("Location"); location != "/" {
		t.Fatalf("Location = %q, want %q", location, "/")
	}
}

func TestAuthMiddlewareBypassesProtectionWhenDisabled(t *testing.T) {
	sessionManager := service.NewSessionManager(time.Hour)
	var called bool
	protected := AuthMiddleware(sessionManager, testPublicPaths(), true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/papers", nil)
	w := httptest.NewRecorder()

	protected.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if !called {
		t.Fatal("next handler was not called")
	}
}

func TestAuthMiddlewareRedirectsLoginPageWhenDisabled(t *testing.T) {
	sessionManager := service.NewSessionManager(time.Hour)
	protected := AuthMiddleware(sessionManager, testPublicPaths(), true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()

	protected.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusFound)
	}
	if location := resp.Header.Get("Location"); location != "/" {
		t.Fatalf("Location = %q, want %q", location, "/")
	}
}
