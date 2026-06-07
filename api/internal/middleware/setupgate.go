package middleware

import (
	"net/http"
	"strings"

	"github.com/memetics19/pulse/api/internal/app"
)

// SetupGate redirects all traffic to /setup/ until the app is configured.
// Paths the wizard needs to render and submit pass through.
func SetupGate(a *app.App) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a.Configured() || allowedDuringSetup(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			http.Redirect(w, r, "/setup/", http.StatusFound)
		})
	}
}

func allowedDuringSetup(p string) bool {
	return p == "/setup" || strings.HasPrefix(p, "/setup/") ||
		strings.HasPrefix(p, "/_next/") || strings.HasPrefix(p, "/static/") ||
		strings.HasPrefix(p, "/api/setup") || p == "/favicon.ico" || p == "/healthz"
}
