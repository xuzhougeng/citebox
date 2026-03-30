package handler

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

func newAuthHandlerForTest(t *testing.T) (*AuthHandler, *service.LibraryService, *service.SessionManager) {
	t.Helper()

	root := t.TempDir()
	cfg := &config.Config{
		StorageDir:              filepath.Join(root, "storage"),
		DatabasePath:            filepath.Join(root, "library.db"),
		AdminUsername:           "citebox",
		AdminPassword:           "citebox123",
		ExtractorTimeoutSeconds: 1,
		ExtractorPollInterval:   1,
		ExtractorFileField:      "file",
	}

	repo, err := repository.NewLibraryRepository(cfg.DatabasePath)
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	libraryService, err := service.NewLibraryService(repo, cfg, service.WithLogger(logger), service.WithoutBackgroundJobs())
	if err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}

	sessionManager := service.NewSessionManager(time.Hour)
	return NewAuthHandler(libraryService, sessionManager), libraryService, sessionManager
}

func TestLoginSetsSessionCookie(t *testing.T) {
	handler, _, sessionManager := newAuthHandlerForTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"citebox","password":"citebox123"}`))
	w := httptest.NewRecorder()

	handler.Login(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Login() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var sessionCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == service.SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("Login() did not set %q cookie", service.SessionCookieName)
	}
	if !sessionCookie.HttpOnly {
		t.Fatal("Login() cookie HttpOnly = false, want true")
	}
	if _, ok := sessionManager.Validate(sessionCookie.Value); !ok {
		t.Fatal("Login() session not found in session manager")
	}
}

func TestLoginSetsRememberLoginCookieWhenRequested(t *testing.T) {
	handler, libraryService, _ := newAuthHandlerForTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"citebox","password":"citebox123","remember_login":true}`))
	w := httptest.NewRecorder()

	handler.Login(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Login() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var rememberCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == service.RememberLoginCookieName {
			rememberCookie = cookie
			break
		}
	}
	if rememberCookie == nil {
		t.Fatalf("Login() did not set %q cookie", service.RememberLoginCookieName)
	}
	if !rememberCookie.HttpOnly {
		t.Fatal("Login() remember cookie HttpOnly = false, want true")
	}
	if rememberCookie.MaxAge <= 0 {
		t.Fatalf("Login() remember cookie MaxAge = %d, want positive", rememberCookie.MaxAge)
	}
	if !libraryService.HasRememberLoginToken(rememberCookie.Value) {
		t.Fatal("Login() remember login token not persisted")
	}
}

func TestLogoutClearsSessionCookie(t *testing.T) {
	handler, _, sessionManager := newAuthHandlerForTest(t)
	session, err := sessionManager.Create("citebox")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: session.ID})
	w := httptest.NewRecorder()

	handler.Logout(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Logout() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if _, ok := sessionManager.Validate(session.ID); ok {
		t.Fatal("Logout() session still valid")
	}

	var sessionCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == service.SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || sessionCookie.MaxAge >= 0 {
		t.Fatalf("Logout() cookie = %+v, want expired cookie", sessionCookie)
	}
}

func TestLogoutClearsRememberLoginCookie(t *testing.T) {
	handler, libraryService, sessionManager := newAuthHandlerForTest(t)
	session, err := sessionManager.Create("citebox")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	rememberToken, _, err := libraryService.IssueRememberLoginToken()
	if err != nil {
		t.Fatalf("IssueRememberLoginToken() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: session.ID})
	req.AddCookie(&http.Cookie{Name: service.RememberLoginCookieName, Value: rememberToken})
	w := httptest.NewRecorder()

	handler.Logout(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Logout() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if libraryService.HasRememberLoginToken(rememberToken) {
		t.Fatal("Logout() remember login token still valid")
	}

	var rememberCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == service.RememberLoginCookieName {
			rememberCookie = cookie
			break
		}
	}
	if rememberCookie == nil || rememberCookie.MaxAge >= 0 {
		t.Fatalf("Logout() remember cookie = %+v, want expired cookie", rememberCookie)
	}
}

func TestChangePasswordInvalidatesAllSessions(t *testing.T) {
	handler, libraryService, sessionManager := newAuthHandlerForTest(t)
	currentSession, err := sessionManager.Create("citebox")
	if err != nil {
		t.Fatalf("Create(current) error = %v", err)
	}
	otherSession, err := sessionManager.Create("citebox")
	if err != nil {
		t.Fatalf("Create(other) error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewBufferString(`{"current_password":"citebox123","new_password":"new-secret"}`))
	req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: currentSession.ID})
	w := httptest.NewRecorder()

	handler.ChangePassword(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ChangePassword() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if _, ok := sessionManager.Validate(currentSession.ID); ok {
		t.Fatal("ChangePassword() current session still valid")
	}
	if _, ok := sessionManager.Validate(otherSession.ID); ok {
		t.Fatal("ChangePassword() other session still valid")
	}
	if !libraryService.ValidateCredentials("citebox", "new-secret") {
		t.Fatal("ChangePassword() did not update runtime credentials")
	}
}

func TestChangePasswordRevokesRememberLoginTokens(t *testing.T) {
	handler, libraryService, sessionManager := newAuthHandlerForTest(t)
	currentSession, err := sessionManager.Create("citebox")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	rememberToken, _, err := libraryService.IssueRememberLoginToken()
	if err != nil {
		t.Fatalf("IssueRememberLoginToken() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewBufferString(`{"current_password":"citebox123","new_password":"new-secret"}`))
	req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: currentSession.ID})
	req.AddCookie(&http.Cookie{Name: service.RememberLoginCookieName, Value: rememberToken})
	w := httptest.NewRecorder()

	handler.ChangePassword(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ChangePassword() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if libraryService.HasRememberLoginToken(rememberToken) {
		t.Fatal("ChangePassword() remember login token still valid")
	}
}

func TestUpdateRememberLoginSetsCookie(t *testing.T) {
	handler, libraryService, _ := newAuthHandlerForTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/remember-login", bytes.NewBufferString(`{"enabled":true}`))
	w := httptest.NewRecorder()

	handler.UpdateRememberLogin(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("UpdateRememberLogin() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var rememberCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == service.RememberLoginCookieName {
			rememberCookie = cookie
			break
		}
	}
	if rememberCookie == nil {
		t.Fatalf("UpdateRememberLogin() did not set %q cookie", service.RememberLoginCookieName)
	}
	if !libraryService.HasRememberLoginToken(rememberCookie.Value) {
		t.Fatal("UpdateRememberLogin() remember login token not persisted")
	}
}
