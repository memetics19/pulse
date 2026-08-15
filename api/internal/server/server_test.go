package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/keyauth"
	pushauth "github.com/memetics19/pulse/api/internal/push"
	"github.com/memetics19/pulse/api/internal/server"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushHeartbeatIsPublicAndRotationIsProtected(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	monitor, err := q.CreateMonitor(t.Context(), generated.CreateMonitorParams{
		Name: "heartbeat", Type: "push", IntervalSeconds: 60, TimeoutSeconds: 30,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true,
	})
	require.NoError(t, err)
	token := "abcdefghij0123456789_-TOKEN"
	_, err = q.UpsertPushToken(t.Context(), generated.UpsertPushTokenParams{
		MonitorID: monitor.ID, TokenHash: pushauth.HashToken(token), Prefix: pushauth.Prefix(token),
	})
	require.NoError(t, err)
	a := app.New()
	a.SetDB(db)
	h := server.New(a, t.TempDir(), config.Config{})

	publicReq := httptest.NewRequest(http.MethodGet, "/api/push/"+token+"?status=up", nil)
	publicRec := httptest.NewRecorder()
	h.ServeHTTP(publicRec, publicReq)
	assert.Equal(t, http.StatusOK, publicRec.Code, publicRec.Body.String())

	rotatePath := fmt.Sprintf("/api/monitors/%d/push-token/rotate", monitor.ID)
	unauthReq := httptest.NewRequest(http.MethodPost, rotatePath, nil)
	unauthRec := httptest.NewRecorder()
	h.ServeHTTP(unauthRec, unauthReq)
	assert.Equal(t, http.StatusUnauthorized, unauthRec.Code)

	readKey := createServerAPIKey(t, q, "monitors:read")
	readReq := httptest.NewRequest(http.MethodPost, rotatePath, nil)
	readReq.Header.Set("Authorization", "Bearer "+readKey)
	readRec := httptest.NewRecorder()
	h.ServeHTTP(readRec, readReq)
	assert.Equal(t, http.StatusForbidden, readRec.Code)

	writeKey := createServerAPIKey(t, q, "monitors:write")
	writeReq := httptest.NewRequest(http.MethodPost, rotatePath, nil)
	writeReq.Header.Set("Authorization", "Bearer "+writeKey)
	writeRec := httptest.NewRecorder()
	h.ServeHTTP(writeRec, writeReq)
	assert.Equal(t, http.StatusOK, writeRec.Code, writeRec.Body.String())
}

func createServerAPIKey(t *testing.T, q *generated.Queries, scope string) string {
	t.Helper()
	full, prefix, hash, err := keyauth.Generate()
	require.NoError(t, err)
	scopes, err := json.Marshal([]string{scope})
	require.NoError(t, err)
	_, err = q.CreateAPIKey(t.Context(), generated.CreateAPIKeyParams{
		Name: scope, KeyHash: hash, Prefix: prefix, Scopes: string(scopes),
	})
	require.NoError(t, err)
	return full
}
