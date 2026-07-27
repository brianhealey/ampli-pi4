package auth

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

const (
	sessionCookieName = "amplipi-session"
	apiKeyHeader      = "X-Api-Key"
	authHeader        = "Authorization"
	// apiKeyQueryParam is deprecated but still supported for backwards compatibility
	apiKeyQueryParam = "api-key"
)

// Middleware returns an http.Handler middleware that enforces authentication.
// In open mode (no passwords configured), all requests pass through.
// Otherwise, checks (in order):
//  1. X-Api-Key header (recommended)
//  2. Authorization: Bearer <key> header
//  3. Session cookie
//  4. api-key query parameter (deprecated, logs warning)
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.IsOpenMode() {
			next.ServeHTTP(w, r)
			return
		}

		// Check X-Api-Key header (preferred method)
		if key := r.Header.Get(apiKeyHeader); key != "" {
			if s.VerifyKey(key) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check Authorization: Bearer <key> header
		if auth := r.Header.Get(authHeader); strings.HasPrefix(auth, "Bearer ") {
			key := strings.TrimPrefix(auth, "Bearer ")
			if s.VerifyKey(key) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check session cookie
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			if s.VerifyKey(cookie.Value) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check api-key query parameter (deprecated - logs warning)
		if key := r.URL.Query().Get(apiKeyQueryParam); key != "" {
			if s.VerifyKey(key) {
				slog.Warn("auth: API key in query parameter is deprecated, use X-Api-Key header instead",
					"path", r.URL.Path)
				next.ServeHTTP(w, r)
				return
			}
		}

		// Not authenticated — redirect to login
		loginURL := "/auth/login?next=" + url.QueryEscape(r.URL.RequestURI())
		http.Redirect(w, r, loginURL, http.StatusFound)
	})
}

// SetSessionCookie sets a secure session cookie with proper attributes.
func SetSessionCookie(w http.ResponseWriter, accessKey string, secure bool) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    accessKey,
		Path:     "/",
		HttpOnly: true,                    // Prevent XSS access
		SameSite: http.SameSiteStrictMode, // Prevent CSRF
		MaxAge:   86400 * 30,              // 30 days
	}
	if secure {
		cookie.Secure = true // Only send over HTTPS
	}
	http.SetCookie(w, cookie)
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}
