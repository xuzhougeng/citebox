package main

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
)

// WKWebView ignores Set-Cookie on responses from a custom URL scheme, so the
// login session would be dropped on every request. Every webview request goes
// through this in-process handler, so it keeps the cookies on the Go side
// instead. The page never reads document.cookie, so it does not notice.
var cookieJarURL = &url.URL{Scheme: "https", Host: "citebox.desktop", Path: "/"}

func withCookieJar(next http.Handler) http.Handler {
	jar, _ := cookiejar.New(nil)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("Cookie")
		for _, cookie := range jar.Cookies(cookieJarURL) {
			r.AddCookie(cookie)
		}
		next.ServeHTTP(&cookieCapture{ResponseWriter: w, jar: jar}, r)
	})
}

type cookieCapture struct {
	http.ResponseWriter
	jar         *cookiejar.Jar
	wroteHeader bool
}

func (c *cookieCapture) WriteHeader(status int) {
	if !c.wroteHeader {
		c.wroteHeader = true
		response := http.Response{Header: c.Header()}
		if cookies := response.Cookies(); len(cookies) > 0 {
			c.jar.SetCookies(cookieJarURL, cookies)
			c.Header().Del("Set-Cookie")
		}
	}
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
