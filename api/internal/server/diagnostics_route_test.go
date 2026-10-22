package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/keyauth"
	"github.com/memetics19/pulse/api/internal/server"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// Diagnostic bundles carry journal entries, container logs, process names and
// filesystem paths — materially more sensitive than agent inventory. A key
// issued for agents:read before this feature existed must not silently gain
// access to them.
func TestDiagnosticsHistoryNeedsItsOwnScope(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	a := app.New()
	a.SetDB(db)
	h := server.New(a, t.TempDir(), config.Config{})

	unauth := httptest.NewRequest(http.MethodGet, "/api/agents/1/diagnostics", nil)
	unauthRec := httptest.NewRecorder()
	h.ServeHTTP(unauthRec, unauth)
	assert.Equal(t, http.StatusUnauthorized, unauthRec.Code)

	// Chi serves both spellings, so both must demand the scope. Matching on a
	// path suffix alone let the trailing-slash form fall through to the
	// generic /api/agents rule.
	for _, path := range []string{"/api/agents/1/diagnostics", "/api/agents/1/diagnostics/"} {
		legacy := httptest.NewRequest(http.MethodGet, path, nil)
		legacy.Header.Set("Authorization", "Bearer "+createServerAPIKey(t, q, "agents:read"))
		legacyRec := httptest.NewRecorder()
		h.ServeHTTP(legacyRec, legacy)
		assert.Equal(t, http.StatusForbidden, legacyRec.Code,
			"an agents:read key must not inherit access to logs via %s", path)
	}

	granted := httptest.NewRequest(http.MethodGet, "/api/agents/1/diagnostics", nil)
	granted.Header.Set("Authorization", "Bearer "+createServerAPIKey(t, q, "diagnostics:read"))
	grantedRec := httptest.NewRecorder()
	h.ServeHTTP(grantedRec, granted)
	assert.Equal(t, http.StatusOK, grantedRec.Code, grantedRec.Body.String())

	// Agent inventory itself is unaffected.
	inventory := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	inventory.Header.Set("Authorization", "Bearer "+createServerAPIKey(t, q, "agents:read"))
	inventoryRec := httptest.NewRecorder()
	h.ServeHTTP(inventoryRec, inventory)
	assert.Equal(t, http.StatusOK, inventoryRec.Code)
}

// The point of the feature: an agent pushes evidence with its own token, and
// an operator reads it back with an API key, without touching the host.
func TestDiagnosticsRoundTripThroughRouter(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	a := app.New()
	a.SetDB(db)
	h := server.New(a, t.TempDir(), config.Config{})

	agentToken := "0123456789abcdef0123456789abcdef0123456789abcdef"
	agent, err := q.CreateAgent(t.Context(), generated.CreateAgentParams{
		Name: "proxmox", HostLabel: "homeserver", TokenHash: keyauth.Hash(agentToken),
	})
	require.NoError(t, err)

	push := httptest.NewRequest(http.MethodPost, "/api/ingest/diagnostics",
		strings.NewReader(`{"bundle":{"sections":{"kernel":{"data":{"oom_kills":[{"process":"jellyfin"}]}}}}}`))
	push.Header.Set("Authorization", "Bearer "+agentToken)
	pushRec := httptest.NewRecorder()
	h.ServeHTTP(pushRec, push)
	require.Equal(t, http.StatusNoContent, pushRec.Code, pushRec.Body.String())

	read := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/agents/%d/diagnostics", agent.ID), nil)
	read.Header.Set("Authorization", "Bearer "+createServerAPIKey(t, q, "diagnostics:read"))
	readRec := httptest.NewRecorder()
	h.ServeHTTP(readRec, read)

	require.Equal(t, http.StatusOK, readRec.Code, readRec.Body.String())
	assert.Contains(t, readRec.Body.String(), "jellyfin",
		"the operator must get back what the agent collected")
}
