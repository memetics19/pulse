package handlers_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	pushauth "github.com/memetics19/pulse/api/internal/push"
	"github.com/memetics19/pulse/api/internal/worker/checkresult"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pushFixture struct {
	db      *sql.DB
	q       *generated.Queries
	monitor generated.Monitor
	token   string
	router  http.Handler
}

func newPushFixture(t *testing.T, monitorType string, active bool) pushFixture {
	t.Helper()
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	monitor, err := q.CreateMonitor(t.Context(), generated.CreateMonitorParams{
		Name: "heartbeat", Type: monitorType, Url: "", IntervalSeconds: 60,
		TimeoutSeconds: 30, DegradedThresholdMs: 500, DownThresholdMs: 2000,
		IsActive: active,
	})
	require.NoError(t, err)
	token := "abcdefghij0123456789_-TOKEN"
	_, err = q.UpsertPushToken(t.Context(), generated.UpsertPushTokenParams{
		MonitorID: monitor.ID, TokenHash: pushauth.HashToken(token), Prefix: pushauth.Prefix(token),
	})
	require.NoError(t, err)

	recorder := checkresult.New(db, incident.NewDetector(store.New(db)), nil)
	h := handlers.NewPush(q, recorder)
	r := chi.NewRouter()
	r.Get("/api/push/{token}", h.Heartbeat)
	r.Post("/api/push/{token}", h.Heartbeat)
	return pushFixture{db: db, q: q, monitor: monitor, token: token, router: r}
}

func (f pushFixture) request(t *testing.T, method, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/push/"+f.token+rawQuery, nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func checkResultCount(t *testing.T, db *sql.DB, monitorID int64) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM check_results WHERE monitor_id = ?`, monitorID).Scan(&count))
	return count
}

func TestPushHeartbeatGETRecordsUpWithPing(t *testing.T) {
	f := newPushFixture(t, "push", true)
	rec := f.request(t, http.MethodGet, "?status=up&msg=OK&ping=12")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"ok":true}`, rec.Body.String())
	results, err := f.q.LatestTwoCheckResults(t.Context(), f.monitor.ID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "up", results[0].Status)
	require.NotNil(t, results[0].ResponseTimeMs)
	assert.Equal(t, int64(12), *results[0].ResponseTimeMs)
	assert.Equal(t, "OK", results[0].ErrorMessage)
}

func TestPushHeartbeatPOSTUsesQueryParametersAndDefaultsStatus(t *testing.T) {
	f := newPushFixture(t, "push", true)
	rec := f.request(t, http.MethodPost, "?msg=posted")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	result, err := f.q.LatestCheckResult(t.Context(), f.monitor.ID)
	require.NoError(t, err)
	assert.Equal(t, "up", result.Status)
	assert.Equal(t, "posted", result.ErrorMessage)
	assert.Nil(t, result.ResponseTimeMs)
}

func TestPushHeartbeatDefaultsOmittedOrEmptyMessageToOK(t *testing.T) {
	for _, query := range []string{"", "?msg="} {
		t.Run(query, func(t *testing.T) {
			f := newPushFixture(t, "push", true)
			rec := f.request(t, http.MethodGet, query)

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			result, err := f.q.LatestCheckResult(t.Context(), f.monitor.ID)
			require.NoError(t, err)
			assert.Equal(t, "up", result.Status)
			assert.Equal(t, "OK", result.ErrorMessage)
			assert.Nil(t, result.ResponseTimeMs)
		})
	}
}

func TestPushHeartbeatTreatsEmptyPingAsOmitted(t *testing.T) {
	f := newPushFixture(t, "push", true)
	rec := f.request(t, http.MethodGet, "?ping=")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	result, err := f.q.LatestCheckResult(t.Context(), f.monitor.ID)
	require.NoError(t, err)
	assert.Nil(t, result.ResponseTimeMs)
}

func TestPushHeartbeatRejectsInvalidParametersWithoutPersistence(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "status", query: "?status=unknown"},
		{name: "degraded status", query: "?status=degraded"},
		{name: "negative ping", query: "?ping=-1"},
		{name: "non-number ping", query: "?ping=fast"},
		{name: "excessive ping", query: "?ping=100000000001"},
		{name: "message bytes", query: "?msg=" + url.QueryEscape(strings.Repeat("x", 1025))},
		{name: "message utf8", query: "?msg=%FF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPushFixture(t, "push", true)
			rec := f.request(t, http.MethodGet, tt.query)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.Zero(t, checkResultCount(t, f.db, f.monitor.ID))
		})
	}
}

func TestPushHeartbeatAcceptsInclusivePingAndMessageLimits(t *testing.T) {
	f := newPushFixture(t, "push", true)
	message := strings.Repeat("x", 1024)
	rec := f.request(t, http.MethodGet,
		"?status=down&ping=100000000000&msg="+url.QueryEscape(message))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	result, err := f.q.LatestCheckResult(t.Context(), f.monitor.ID)
	require.NoError(t, err)
	require.NotNil(t, result.ResponseTimeMs)
	assert.Equal(t, int64(100_000_000_000), *result.ResponseTimeMs)
	assert.Equal(t, message, result.ErrorMessage)
}

func TestPushHeartbeatHidesCredentialAndMonitorLookupFailures(t *testing.T) {
	wantBody := "{\"error\":\"push monitor not found\"}\n"
	tests := []struct {
		name        string
		monitorType string
		active      bool
		pathToken   string
	}{
		{name: "malformed short", monitorType: "push", active: true, pathToken: "short"},
		{name: "malformed long", monitorType: "push", active: true, pathToken: strings.Repeat("a", 129)},
		{name: "unknown", monitorType: "push", active: true, pathToken: "unknown-token"},
		{name: "inactive push", monitorType: "push", active: false},
		{name: "non-push", monitorType: "http", active: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPushFixture(t, tt.monitorType, tt.active)
			token := tt.pathToken
			if token == "" {
				token = f.token
			}
			req := httptest.NewRequest(http.MethodGet, "/api/push/"+token, nil)
			rec := httptest.NewRecorder()
			f.router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusNotFound, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.Equal(t, wantBody, rec.Body.String())
			assert.Zero(t, checkResultCount(t, f.db, f.monitor.ID))
		})
	}
}

type failingRecorder struct{}

func (failingRecorder) Record(context.Context, store.Monitor, checkresult.Input) error {
	return assert.AnError
}

func TestPushHeartbeatReturnsJSONInternalErrorWhenRecorderFails(t *testing.T) {
	f := newPushFixture(t, "push", true)
	h := handlers.NewPush(f.q, failingRecorder{})
	r := chi.NewRouter()
	r.Get("/api/push/{token}", h.Heartbeat)
	req := httptest.NewRequest(http.MethodGet, "/api/push/"+f.token, nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"error":"could not record heartbeat"}`, rec.Body.String())
	assert.Zero(t, checkResultCount(t, f.db, f.monitor.ID))
}
