package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/server"
	"github.com/memetics19/pulse/api/testutil"
)

// One request per route family, exercising the full router wiring: public
// endpoints answer, auth-gated endpoints reject anonymous callers, and the
// admin/setup redirects fire.
func TestRouterWiring(t *testing.T) {
	a := app.New()
	a.SetDB(testutil.NewTestDB(t))
	a.MarkWorkerAlive()
	h := server.New(a, t.TempDir(), config.Config{})

	cases := []struct {
		method, path string
		want         int
	}{
		{"GET", "/healthz", http.StatusOK},
		{"GET", "/api/status", http.StatusOK},
		{"GET", "/", http.StatusOK},
		{"GET", "/api/setup/state", http.StatusOK},
		{"GET", "/feed.xml", http.StatusOK},
		// auth-gated: anonymous must be rejected
		{"GET", "/api/monitors", http.StatusUnauthorized},
		{"GET", "/api/keys", http.StatusUnauthorized},
		{"GET", "/api/overview", http.StatusUnauthorized},
		// agent ingest without a bearer token
		{"POST", "/api/ingest/metrics", http.StatusUnauthorized},
		// redirects
		{"GET", "/admin", http.StatusMovedPermanently},
		{"GET", "/setup", http.StatusMovedPermanently},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.want {
			t.Errorf("%s %s = %d, want %d", c.method, c.path, rec.Code, c.want)
		}
	}
}
