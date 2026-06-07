package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
)

func TestSetupGateRedirectsWhenUnconfigured(t *testing.T) {
	a := app.New()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := SetupGate(a)(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/setup/" {
		t.Fatalf("unconfigured / should redirect to /setup/, got %d %q", rec.Code, rec.Header().Get("Location"))
	}

	// allowed-during-setup paths pass through (not redirected)
	for _, p := range []string{"/setup/", "/api/setup/state", "/_next/x.js", "/healthz"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != 200 {
			t.Fatalf("%s should pass through, got %d", p, rec.Code)
		}
	}
}
