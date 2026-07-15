package middleware

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func RequestLogger(output io.Writer) func(http.Handler) http.Handler {
	logger := log.New(output, "", log.LstdFlags)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := requestLogPath(r.URL.Path)
			wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			started := time.Now()
			next.ServeHTTP(wrapped, r)
			status := wrapped.Status()
			if status == 0 {
				status = http.StatusOK
			}
			logger.Printf("%s %s status=%d bytes=%d duration=%s",
				r.Method, path, status, wrapped.BytesWritten(), time.Since(started))
		})
	}
}

func requestLogPath(path string) string {
	if path == "/api/push" || strings.HasPrefix(path, "/api/push/") {
		return "/api/push/[REDACTED]"
	}
	return path
}
