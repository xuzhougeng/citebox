package service

import (
	"net/http"
	"strings"
	"time"
)

func BuildSessionCookie(r *http.Request, session Session) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   IsSecureRequest(r),
	}
}

func ClearSessionCookie(r *http.Request) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   IsSecureRequest(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
}

func BuildRememberLoginCookie(r *http.Request, token string, expiresAt time.Time) *http.Cookie {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}

	return &http.Cookie{
		Name:     RememberLoginCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   IsSecureRequest(r),
		MaxAge:   maxAge,
		Expires:  expiresAt,
	}
}

func ClearRememberLoginCookie(r *http.Request) *http.Cookie {
	return &http.Cookie{
		Name:     RememberLoginCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   IsSecureRequest(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
}

func IsSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}

	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}
