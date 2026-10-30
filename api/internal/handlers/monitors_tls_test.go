package handlers_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createSSLMonitor(t *testing.T, h *handlers.Monitors, fields map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body := map[string]any{
		"name": "cert", "url": "example.com:443", "type": "ssl", "interval_seconds": 3600,
	}
	for k, v := range fields {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	h.Create(rr, httptest.NewRequest(http.MethodPost, "/api/monitors", bytes.NewReader(raw)))
	return rr
}

func TestMonitorsCreateDefaultsTLSThresholds(t *testing.T) {
	db := testutil.NewTestDB(t)
	h := handlers.NewMonitors(generated.New(db), func() *sql.DB { return db }, true)

	rr := createSSLMonitor(t, h, nil)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var m generated.Monitor
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&m))
	assert.Equal(t, int64(14), m.TlsWarnDays)
	assert.Equal(t, int64(7), m.TlsCriticalDays)
}

func TestMonitorsCreateStoresTLSThresholds(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewMonitors(q, func() *sql.DB { return db }, true)

	// A critical threshold of 0 is valid: down only on the last day.
	rr := createSSLMonitor(t, h, map[string]any{"tls_warn_days": 30, "tls_critical_days": 0})

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var m generated.Monitor
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&m))
	stored, err := q.GetMonitor(t.Context(), m.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(30), stored.TlsWarnDays)
	assert.Equal(t, int64(0), stored.TlsCriticalDays)
}

func TestMonitorsCreateRejectsInvalidTLSThresholds(t *testing.T) {
	cases := []struct {
		name   string
		fields map[string]any
		reason string
	}{
		{"warn equals critical", map[string]any{"tls_warn_days": 7, "tls_critical_days": 7}, "tls_warn_days must be greater than tls_critical_days"},
		{"warn below critical", map[string]any{"tls_warn_days": 5, "tls_critical_days": 10}, "tls_warn_days must be greater than tls_critical_days"},
		{"warn below default critical", map[string]any{"tls_warn_days": 3}, "tls_warn_days must be greater than tls_critical_days"},
		{"critical above default warn", map[string]any{"tls_critical_days": 20}, "tls_warn_days must be greater than tls_critical_days"},
		{"negative critical", map[string]any{"tls_warn_days": 14, "tls_critical_days": -1}, "tls_critical_days must be 0 or more"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := testutil.NewTestDB(t)
			q := generated.New(db)
			h := handlers.NewMonitors(q, func() *sql.DB { return db }, true)

			rr := createSSLMonitor(t, h, tc.fields)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.reason)
			monitors, err := q.ListMonitors(t.Context())
			require.NoError(t, err)
			assert.Empty(t, monitors, "a rejected request stores nothing")
		})
	}
}

func TestMonitorsUpdateTLSThresholds(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	monitor, err := q.CreateMonitor(t.Context(), generated.CreateMonitorParams{
		Name: "cert", Url: "example.com:443", Type: "ssl", IntervalSeconds: 3600,
		TimeoutSeconds: 10, DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true,
		TlsWarnDays: 14, TlsCriticalDays: 7,
	})
	require.NoError(t, err)
	h := handlers.NewMonitors(q, func() *sql.DB { return db }, true)
	r := chi.NewRouter()
	r.Put("/api/monitors/{id}", h.Update)
	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPut,
			fmt.Sprintf("/api/monitors/%d", monitor.ID), bytes.NewBufferString(body)))
		return rec
	}

	rec := put(`{"name":"cert","url":"example.com:443","type":"ssl","interval_seconds":3600,
		"tls_warn_days":21,"tls_critical_days":3}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	stored, err := q.GetMonitor(t.Context(), monitor.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(21), stored.TlsWarnDays)
	assert.Equal(t, int64(3), stored.TlsCriticalDays)

	rec = put(`{"name":"cert","url":"example.com:443","type":"ssl","interval_seconds":3600,
		"tls_warn_days":3,"tls_critical_days":21}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "tls_warn_days must be greater than tls_critical_days")
	stored, err = q.GetMonitor(t.Context(), monitor.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(21), stored.TlsWarnDays, "a rejected update keeps the stored thresholds")
}

func TestCheckResultsReturnCertDaysLeft(t *testing.T) {
	q := newQ(t)
	ctx := t.Context()
	mon, err := q.CreateMonitor(ctx, generated.CreateMonitorParams{
		Name: "cert", Url: "example.com:443", Type: "ssl", IntervalSeconds: 3600,
		TimeoutSeconds: 10, DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true,
		Source: "internal", TlsWarnDays: 14, TlsCriticalDays: 7,
	})
	require.NoError(t, err)
	days := int64(12)
	_, err = q.InsertCheckResult(ctx, generated.InsertCheckResultParams{
		MonitorID: mon.ID, CheckedAt: time.Now().UTC(), Status: "degraded",
		StatusCode: &days, CertDaysLeft: &days,
	})
	require.NoError(t, err)
	h := handlers.NewCheckResults(q)

	rr := do(func(w http.ResponseWriter, r *http.Request) {
		h.List(w, withChiID(r, "monitorID", itoa(mon.ID)))
	}, "GET", "/x", nil)

	require.Equal(t, http.StatusOK, rr.Code)
	var rows []map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&rows))
	require.Len(t, rows, 1)
	assert.EqualValues(t, 12, rows[0]["cert_days_left"])
	assert.EqualValues(t, 12, rows[0]["status_code"], "status_code keeps the days left for one release")
}
