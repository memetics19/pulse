package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestLoggerRedactsPushCredentialAndQuery(t *testing.T) {
	var output bytes.Buffer
	h := RequestLogger(&output)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/push/abcdefghij?msg=secret", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	logLine := output.String()
	assert.NotContains(t, logLine, "abcdefghij")
	assert.NotContains(t, logLine, "secret")
	assert.Contains(t, logLine, "GET /api/push/[REDACTED]")
	assert.Contains(t, logLine, "status=202")
	assert.Contains(t, logLine, "bytes=2")
	assert.Contains(t, logLine, "duration=")
}

func TestRequestLoggerRedactsPushPathVariantsBeforeFormatting(t *testing.T) {
	paths := []string{
		"/api/push/abcdefghij/",
		"/api/push/abcdefghij/extra?msg=secret",
		"/api/push/abc%2Fdefghij?msg=secret",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			var output bytes.Buffer
			h := RequestLogger(&output)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			line := strings.TrimSpace(output.String())
			require.NotEmpty(t, line)
			assert.Contains(t, line, "GET /api/push/[REDACTED] status=404")
			assert.NotContains(t, line, "secret")
			assert.NotContains(t, line, "abcdefghij")
			assert.NotContains(t, line, "abc%2Fdefghij")
			assert.NotContains(t, line, "/extra")
		})
	}
}

func TestRequestLoggerLogsOrdinaryMethodPathAndResponse(t *testing.T) {
	var output bytes.Buffer
	h := RequestLogger(&output)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/monitors?view=all", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	line := output.String()
	assert.Contains(t, line, "POST /api/monitors status=200")
	assert.Contains(t, line, "bytes=5")
	assert.NotContains(t, line, "view=all")
}
