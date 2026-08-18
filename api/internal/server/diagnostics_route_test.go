package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/server"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
)

// The agent posts bundles with its own bearer token, so the route sits outside
// the session/API-key group. A 401 (rather than 404) proves it is wired.
func TestDiagnosticsIngestRouteIsRegisteredAndRequiresAgentToken(t *testing.T) {
	a := app.New()
	a.SetDB(testutil.NewTestDB(t))
	h := server.New(a, t.TempDir(), config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/api/ingest/diagnostics",
		strings.NewReader(`{"bundle":{}}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
}
