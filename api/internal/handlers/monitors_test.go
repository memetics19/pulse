package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/internal/worker/checkresult"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitorsCRUD(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewMonitors(q, true)

	body, _ := json.Marshal(map[string]any{
		"name":                  "API Gateway",
		"url":                   "https://api.example.com",
		"type":                  "http",
		"interval_seconds":      60,
		"timeout_seconds":       10,
		"degraded_threshold_ms": 500,
		"down_threshold_ms":     2000,
		"is_active":             true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/monitors", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	var m generated.Monitor
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&m))
	assert.Equal(t, "API Gateway", m.Name)
	assert.Equal(t, "http", m.Type)

	req2 := httptest.NewRequest(http.MethodGet, "/api/monitors", nil)
	rr2 := httptest.NewRecorder()
	h.List(rr2, req2)
	require.Equal(t, http.StatusOK, rr2.Code)
	var monitors []generated.Monitor
	require.NoError(t, json.NewDecoder(rr2.Body).Decode(&monitors))
	assert.Len(t, monitors, 1)
}

func TestMonitorsCreateRejectsZeroInterval(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewMonitors(q, true)

	body, _ := json.Marshal(map[string]any{
		"name":             "Bad Monitor",
		"url":              "https://api.example.com",
		"type":             "http",
		"interval_seconds": 0,
		"timeout_seconds":  10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/monitors", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)

	// Nothing should have been persisted.
	req2 := httptest.NewRequest(http.MethodGet, "/api/monitors", nil)
	rr2 := httptest.NewRecorder()
	h.List(rr2, req2)
	var monitors []generated.Monitor
	require.NoError(t, json.NewDecoder(rr2.Body).Decode(&monitors))
	assert.Len(t, monitors, 0)
}

func TestMonitorsCreateRejectsPrivateURL(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewMonitors(q, false)

	body, _ := json.Marshal(map[string]any{
		"name":             "Metadata",
		"url":              "http://127.0.0.1/x",
		"type":             "http",
		"interval_seconds": 60,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/monitors", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "private or internal")
}

func TestMonitorsCreateAllowsPushWithoutURL(t *testing.T) {
	db := testutil.NewTestDB(t)
	h := handlers.NewMonitors(generated.New(db), true)

	body, _ := json.Marshal(map[string]any{
		"name":             "Heartbeat",
		"type":             "push",
		"interval_seconds": 60,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/monitors", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
}

func TestMonitorsCreateRejectsTCPURLSyntax(t *testing.T) {
	db := testutil.NewTestDB(t)
	h := handlers.NewMonitors(generated.New(db), true)

	body, _ := json.Marshal(map[string]any{
		"name":             "Database",
		"url":              "tcp://db:5432",
		"type":             "tcp",
		"interval_seconds": 60,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/monitors", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "tcp target must be host:port")
}

func TestMonitorsCreateDefaultsThresholdsForPushRecording(t *testing.T) {
	db := testutil.NewTestDB(t)
	generatedQueries := generated.New(db)
	h := handlers.NewMonitors(generatedQueries, true)
	body, _ := json.Marshal(map[string]any{
		"name":             "Heartbeat",
		"type":             "push",
		"interval_seconds": 60,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/monitors", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var mon store.Monitor
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&mon))
	require.Equal(t, int64(500), mon.DegradedThresholdMs)
	require.Equal(t, int64(2000), mon.DownThresholdMs)

	q := store.New(db)
	r := checkresult.New(db, incident.NewDetector(q), nil)
	ping := int64(25)
	require.NoError(t, r.Record(context.Background(), mon, checkresult.Input{
		Status: "up", ResponseTimeMs: &ping, CheckedAt: time.Now(),
	}))
	results, err := q.LatestTwoCheckResults(context.Background(), mon.ID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "up", results[0].Status)
}

func TestMonitorsCreateRejectsPrivateConnectionTargets(t *testing.T) {
	tests := []struct {
		monType string
		target  string
	}{
		{monType: "tcp", target: "127.0.0.1:5432"},
		{monType: "ssl", target: "127.0.0.1:443"},
		{monType: "ping", target: "127.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.monType, func(t *testing.T) {
			db := testutil.NewTestDB(t)
			h := handlers.NewMonitors(generated.New(db), false)
			body, _ := json.Marshal(map[string]any{
				"name": tt.monType, "url": tt.target, "type": tt.monType, "interval_seconds": 60,
			})
			req := httptest.NewRequest(http.MethodPost, "/api/monitors", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			h.Create(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Body.String(), "private or internal")
		})
	}
}
