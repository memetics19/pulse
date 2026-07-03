package middleware

import (
	"net/http"

	"github.com/go-chi/cors"
)

// CORS returns a middleware that allows cross-origin requests from the given
// origins. With no origins configured it is a no-op: the embedded admin UI is
// served same-origin, so no CORS headers are needed by default.
func CORS(origins []string) func(http.Handler) http.Handler {
	if len(origins) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return cors.Handler(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	})
}
