package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/server"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
)

// 2FA belongs to the logged-in user's session. An API key, whatever its
// scopes, must not be able to start setup, enable or turn 2FA off.
func TestTwoFARoutesRefuseAPIKeys(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	a := app.New()
	a.SetDB(db)
	h := server.New(a, t.TempDir(), config.Config{})

	scopes := []string{
		"monitors:write", "incidents:write", "notifications:write", "agents:write",
		"theme:write", "pages:write", "maintenance:write", "status:read",
		"imports:write", "diagnostics:read",
	}
	for _, path := range []string{"/api/auth/2fa/setup", "/api/auth/2fa/enable", "/api/auth/2fa/disable"} {
		for _, scope := range scopes {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"code":"000000"}`))
			req.Header.Set("Authorization", "Bearer "+createServerAPIKey(t, q, scope))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusUnauthorized, rec.Code, "%s with a %s key", path, scope)
		}
	}
}
