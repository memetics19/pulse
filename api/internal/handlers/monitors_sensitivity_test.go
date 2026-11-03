package handlers_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createMonitorJSON posts body merged over a valid HTTP monitor and returns
// the recorder.
func createMonitorJSON(t *testing.T, h *handlers.Monitors, extra map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	fields := map[string]any{
		"name": "API", "url": "https://api.example.com", "type": "http", "interval_seconds": 60,
	}
	for k, v := range extra {
		fields[k] = v
	}
	body, err := json.Marshal(fields)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	h.Create(rr, httptest.NewRequest(http.MethodPost, "/api/monitors", bytes.NewReader(body)))
	return rr
}

func TestMonitorsCreateDefaultsSensitivity(t *testing.T) {
	db := testutil.NewTestDB(t)
	h := handlers.NewMonitors(generated.New(db), func() *sql.DB { return db }, true)

	rr := createMonitorJSON(t, h, nil)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var m generated.Monitor
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&m))
	assert.EqualValues(t, 2, m.FailuresBeforeIncident)
	assert.EqualValues(t, 2, m.RecoveriesBeforeResolve)
	assert.EqualValues(t, 1, m.Retries)
	assert.EqualValues(t, 500, m.RetryIntervalMs)
}

func TestMonitorsCreateStoresSensitivity(t *testing.T) {
	db := testutil.NewTestDB(t)
	h := handlers.NewMonitors(generated.New(db), func() *sql.DB { return db }, true)

	rr := createMonitorJSON(t, h, map[string]any{
		"failures_before_incident": 5, "recoveries_before_resolve": 3,
		"retries": 0, "retry_interval_ms": 0,
	})

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var m generated.Monitor
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&m))
	assert.EqualValues(t, 5, m.FailuresBeforeIncident)
	assert.EqualValues(t, 3, m.RecoveriesBeforeResolve)
	assert.EqualValues(t, 0, m.Retries, "an explicit 0 turns retries off")
	assert.EqualValues(t, 0, m.RetryIntervalMs)
}

func TestMonitorsCreateRejectsSensitivityOutOfRange(t *testing.T) {
	tests := []struct {
		field string
		value int64
		want  string
	}{
		{field: "failures_before_incident", value: 0, want: "failures_before_incident must be between 1 and 10"},
		{field: "failures_before_incident", value: 11, want: "failures_before_incident must be between 1 and 10"},
		{field: "recoveries_before_resolve", value: 0, want: "recoveries_before_resolve must be between 1 and 10"},
		{field: "recoveries_before_resolve", value: 11, want: "recoveries_before_resolve must be between 1 and 10"},
		{field: "retries", value: -1, want: "retries must be between 0 and 5"},
		{field: "retries", value: 6, want: "retries must be between 0 and 5"},
		{field: "retry_interval_ms", value: -1, want: "retry_interval_ms must be between 0 and 60000"},
		{field: "retry_interval_ms", value: 60001, want: "retry_interval_ms must be between 0 and 60000"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s=%d", tt.field, tt.value), func(t *testing.T) {
			db := testutil.NewTestDB(t)
			q := generated.New(db)
			h := handlers.NewMonitors(q, func() *sql.DB { return db }, true)

			rr := createMonitorJSON(t, h, map[string]any{tt.field: tt.value})

			require.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.want)
			monitors, err := q.ListMonitors(t.Context())
			require.NoError(t, err)
			assert.Empty(t, monitors)
		})
	}
}

func TestMonitorsUpdateStoresSensitivity(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewMonitors(q, func() *sql.DB { return db }, true)
	rr := createMonitorJSON(t, h, nil)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var created generated.Monitor
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&created))

	r := chi.NewRouter()
	r.Put("/api/monitors/{id}", h.Update)
	body := bytes.NewBufferString(`{
		"name":"API","url":"https://api.example.com","type":"http","interval_seconds":60,
		"failures_before_incident":4,"recoveries_before_resolve":6,"retries":3,"retry_interval_ms":2000
	}`)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/monitors/%d", created.ID), body))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	stored, err := q.GetMonitor(t.Context(), created.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 4, stored.FailuresBeforeIncident)
	assert.EqualValues(t, 6, stored.RecoveriesBeforeResolve)
	assert.EqualValues(t, 3, stored.Retries)
	assert.EqualValues(t, 2000, stored.RetryIntervalMs)

	bad := bytes.NewBufferString(`{
		"name":"API","url":"https://api.example.com","type":"http","interval_seconds":60,
		"failures_before_incident":20
	}`)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/monitors/%d", created.ID), bad))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	stored, err = q.GetMonitor(t.Context(), created.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 4, stored.FailuresBeforeIncident, "a rejected update changes nothing")
}
