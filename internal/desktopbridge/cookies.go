package desktopbridge

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

// WKWebView ignores Set-Cookie on responses from a custom URL scheme, so the
// login session would be dropped on every request. Every webview request goes
// through this in-process handler, so it keeps the cookies on the Go side
// instead. The page never reads document.cookie, so it does not notice.
//
// The app has one origin and sets every cookie on "/", so cookies are keyed by
// name only. Cookies with an expiry (the remember-login cookie) are saved to
// disk so a restart keeps the user signed in, as the old webview did.

type storedCookie struct {
	Value   string    `json:"value"`
	Expires time.Time `json:"expires,omitempty"`
}

// CookieStore keeps the webview's cookies on the Go side.
type CookieStore struct {
	mu      sync.Mutex
	path    string
	logger  *slog.Logger
	cookies map[string]storedCookie
}

// NewCookieStore loads saved cookies from path, if any.
func NewCookieStore(path string, logger *slog.Logger) *CookieStore {
	store := &CookieStore{path: path, logger: logger, cookies: map[string]storedCookie{}}

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("failed to read desktop cookies", "error", err)
		}
		return store
	}
	if err := json.Unmarshal(data, &store.cookies); err != nil {
		logger.Warn("failed to parse desktop cookies", "error", err)
		store.cookies = map[string]storedCookie{}
	}
	return store
}

func (s *CookieStore) addTo(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for name, cookie := range s.cookies {
		if !cookie.Expires.IsZero() && !cookie.Expires.After(now) {
			continue
		}
		r.AddCookie(&http.Cookie{Name: name, Value: cookie.Value})
	}
}

func (s *CookieStore) update(cookies []*http.Cookie) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	persistentChanged := false
	for _, cookie := range cookies {
		previous, existed := s.cookies[cookie.Name]
		if existed && !previous.Expires.IsZero() {
			persistentChanged = true
		}

		var expires time.Time
		switch {
		case cookie.MaxAge < 0:
			delete(s.cookies, cookie.Name)
			continue
		case cookie.MaxAge > 0:
			expires = now.Add(time.Duration(cookie.MaxAge) * time.Second)
		case !cookie.Expires.IsZero():
			if !cookie.Expires.After(now) {
				delete(s.cookies, cookie.Name)
				continue
			}
			expires = cookie.Expires
		}

		s.cookies[cookie.Name] = storedCookie{Value: cookie.Value, Expires: expires}
		if !expires.IsZero() {
			persistentChanged = true
		}
	}

	if persistentChanged {
		s.saveLocked()
	}
}

func (s *CookieStore) saveLocked() {
	persistent := map[string]storedCookie{}
	for name, cookie := range s.cookies {
		if !cookie.Expires.IsZero() {
			persistent[name] = cookie
		}
	}

	data, err := json.Marshal(persistent)
	if err != nil {
		s.logger.Warn("failed to encode desktop cookies", "error", err)
		return
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		s.logger.Warn("failed to save desktop cookies", "error", err)
	}
}

// WithCookieStore adds the stored cookies to each request and stores the
// cookies each response sets, removing Set-Cookie from what the webview sees.
func WithCookieStore(next http.Handler, store *CookieStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("Cookie")
		store.addTo(r)
		capture := &cookieCapture{ResponseWriter: w, store: store}
		next.ServeHTTP(capture, r)
		// A handler that sets cookies but writes nothing never calls WriteHeader.
		capture.captureCookies()
	})
}

type cookieCapture struct {
	http.ResponseWriter
	store       *CookieStore
	wroteHeader bool
}

func (c *cookieCapture) captureCookies() {
	if c.wroteHeader {
		return
	}
	c.wroteHeader = true
	response := http.Response{Header: c.Header()}
	if cookies := response.Cookies(); len(cookies) > 0 {
		c.store.update(cookies)
		c.Header().Del("Set-Cookie")
	}
}

func (c *cookieCapture) WriteHeader(status int) {
	c.captureCookies()
	c.ResponseWriter.WriteHeader(status)
}

func (c *cookieCapture) Write(p []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	return c.ResponseWriter.Write(p)
}

func (c *cookieCapture) Flush() {
	if flusher, ok := c.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
