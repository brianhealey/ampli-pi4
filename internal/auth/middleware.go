package auth

import (
	"net/http"
	"net/url"
)

const (
	sessionCookieName = "amplipi-session"
	apiKeyQueryParam  = "api-key"
)

// Middleware returns an http.Handler middleware that enforces authentication.
// In open mode (no passwords configured), all requests pass through.
// Otherwise, checks the session cookie and api-key query param.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.IsOpenMode() {
			next.ServeHTTP(w, r)
			return
		}

		// Check session cookie
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			if s.VerifyKey(cookie.Value) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check api-key query parameter
		if key := r.URL.Query().Get(apiKeyQueryParam); key != "" {
			if s.VerifyKey(key) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Not authenticated — redirect to login
		loginURL := "/auth/login?next=" + url.QueryEscape(r.URL.RequestURI())
		http.Redirect(w, r, loginURL, http.StatusFound)
	})
}
