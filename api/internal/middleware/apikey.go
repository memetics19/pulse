package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/keyauth"
)

// requiredScope maps a request to the scope an API key must hold. Returns ""
// when API keys may NOT access the path (session-only: key management, auth, setup).
func requiredScope(method, path string) string {
	var resource string
	switch {
	case strings.HasPrefix(path, "/api/monitors"), strings.HasPrefix(path, "/api/groups"):
		resource = "monitors"
	case strings.HasPrefix(path, "/api/incidents"):
		resource = "incidents"
	case strings.HasPrefix(path, "/api/notifications"):
		resource = "notifications"
	case strings.HasPrefix(path, "/api/agents"):
		resource = "agents"
	case strings.HasPrefix(path, "/api/theme"):
		resource = "theme"
	case strings.HasPrefix(path, "/api/pages"):
		resource = "pages"
	case strings.HasPrefix(path, "/api/maintenance"):
		resource = "maintenance"
	case strings.HasPrefix(path, "/api/overview"):
		return "status:read"
	default:
		return ""
	}
	if method == http.MethodGet {
		return resource + ":read"
	}
	return resource + ":write"
}

// RequireSessionOrAPIKey allows a request with a valid session cookie (full
// access) OR a valid, unrevoked API key holding the scope required for the
// method+path.
func RequireSessionOrAPIKey(q *generated.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie(auth.SessionCookieName); err == nil {
				if sess, err := q.GetSession(r.Context(), c.Value); err == nil && sess.ExpiresAt.After(time.Now()) {
					next.ServeHTTP(w, r)
					return
				}
			}

			authz := r.Header.Get("Authorization")
			if strings.HasPrefix(authz, "Bearer pulse_") {
				token := strings.TrimPrefix(authz, "Bearer ")
				key, err := q.GetAPIKeyByHash(r.Context(), keyauth.Hash(token))
				if err != nil {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				need := requiredScope(r.Method, r.URL.Path)
				if need == "" || !strings.Contains(key.Scopes, `"`+need+`"`) {
					http.Error(w, "insufficient scope", http.StatusForbidden)
					return
				}
				now := time.Now()
				_ = q.TouchAPIKey(r.Context(), generated.TouchAPIKeyParams{LastUsedAt: &now, ID: key.ID})
				next.ServeHTTP(w, r)
				return
			}

			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
}
