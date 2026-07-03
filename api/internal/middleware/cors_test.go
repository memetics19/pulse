package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func corsProbe(t *testing.T, origins []string, origin string) *httptest.ResponseRecorder {
	t.Helper()
	h := CORS(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCORSDisabledByDefault(t *testing.T) {
	rec := corsProbe(t, nil, "https://evil.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	origins := []string{"https://status.example.com"}

	rec := corsProbe(t, origins, "https://status.example.com")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://status.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want configured origin", got)
	}

	rec = corsProbe(t, origins, "https://evil.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q for unlisted origin, want empty", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	h := CORS([]string{"https://status.example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodOptions, "/api/status", nil)
	req.Header.Set("Origin", "https://status.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://status.example.com" {
		t.Fatalf("preflight Access-Control-Allow-Origin = %q", got)
	}
}
