package handlers_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestMonitorsCRUD(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewMonitors(q, func() *sql.DB { return db }, true)

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
	h := handlers.NewMonitors(q, func() *sql.DB { return db }, true)

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
	h := handlers.NewMonitors(q, func() *sql.DB { return db }, false)

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
	h := handlers.NewMonitors(generated.New(db), func() *sql.DB { return db }, true)

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
	h := handlers.NewMonitors(generated.New(db), func() *sql.DB { return db }, true)

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
	h := handlers.NewMonitors(generatedQueries, func() *sql.DB { return db }, true)
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
			h := handlers.NewMonitors(generated.New(db), func() *sql.DB { return db }, false)
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

func TestMonitorsCreatePushAtomicallyReturnsOneTimeCredential(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewMonitors(q, func() *sql.DB { return db }, true)
	body, _ := json.Marshal(map[string]any{
		"name": "Heartbeat", "type": "push", "interval_seconds": 60, "is_active": true,
	})
	req := httptest.NewRequest(http.MethodPost, "http://pulse.example/api/monitors", bytes.NewReader(body))
	req.Host = "pulse.example"
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var response struct {
		generated.Monitor
		PushToken string `json:"push_token"`
		PushURL   string `json:"push_url"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	require.True(t, pushauth.ValidToken(response.PushToken))
	assert.Equal(t, "http://pulse.example/api/push/"+response.PushToken+"?status=up&msg=OK&ping=", response.PushURL)
	credential, err := q.GetPushTokenByMonitor(t.Context(), response.ID)
	require.NoError(t, err)
	assert.Equal(t, pushauth.HashToken(response.PushToken), credential.TokenHash)
	assert.Equal(t, pushauth.Prefix(response.PushToken), credential.Prefix)
	assert.NotEqual(t, response.PushToken, credential.TokenHash)

	var storedPlaintext int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM push_monitor_tokens WHERE token_hash = ? OR prefix = ?`, response.PushToken, response.PushToken).Scan(&storedPlaintext))
	assert.Zero(t, storedPlaintext)
}

func TestMonitorsCreatePushRollsBackMonitorWhenCredentialInsertFails(t *testing.T) {
	db := testutil.NewTestDB(t)
	_, err := db.Exec(`
		CREATE TRIGGER reject_push_token
		BEFORE INSERT ON push_monitor_tokens
		BEGIN
			SELECT RAISE(FAIL, 'injected push token failure');
		END`)
	require.NoError(t, err)
	h := handlers.NewMonitors(generated.New(db), func() *sql.DB { return db }, true)
	body := bytes.NewBufferString(`{"name":"Heartbeat","type":"push","interval_seconds":60,"is_active":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/monitors", body)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	var monitorCount, tokenCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM monitors`).Scan(&monitorCount))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM push_monitor_tokens`).Scan(&tokenCount))
	assert.Zero(t, monitorCount)
	assert.Zero(t, tokenCount)
}

func TestMonitorsCreatePushUsesSafeForwardedScheme(t *testing.T) {
	tests := []struct {
		name                string
		tls                 bool
		forwarded           string
		additionalForwarded string
		want                string
	}{
		{name: "tls", tls: true, want: "https"},
		{name: "trusted exact override", forwarded: "https", want: "https"},
		{name: "comma list ignored", forwarded: "https,http", want: "http"},
		{name: "multiple values ignored", forwarded: "https", additionalForwarded: "http", want: "http"},
		{name: "invalid ignored", forwarded: "javascript", want: "http"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.NewTestDB(t)
			h := handlers.NewMonitors(generated.New(db), func() *sql.DB { return db }, true)
			body := bytes.NewBufferString(`{"name":"Heartbeat","type":"push","interval_seconds":60,"is_active":true}`)
			req := httptest.NewRequest(http.MethodPost, "http://pulse.example/api/monitors", body)
			req.Host = "pulse.example"
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forwarded)
			}
			if tt.additionalForwarded != "" {
				req.Header.Add("X-Forwarded-Proto", tt.additionalForwarded)
			}
			rec := httptest.NewRecorder()
			h.Create(rec, req)
			require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
			var response map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
			assert.True(t, strings.HasPrefix(response["push_url"].(string), tt.want+"://"))
		})
	}
}

func TestMonitorsCreateNonPushPreservesFlatResponseWithoutCredential(t *testing.T) {
	db := testutil.NewTestDB(t)
	h := handlers.NewMonitors(generated.New(db), func() *sql.DB { return db }, true)
	body := bytes.NewBufferString(`{"name":"Web","url":"https://example.com","type":"http","interval_seconds":60,"is_active":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/monitors", body)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var response map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	assert.Equal(t, "Web", response["name"])
	assert.NotContains(t, response, "Monitor")
	assert.NotContains(t, response, "push_token")
	assert.NotContains(t, response, "push_url")
}

func TestMonitorsRotatePushTokenInvalidatesOldTokenAndDeleteCascades(t *testing.T) {
	f := newPushFixture(t, "push", true)
	h := handlers.NewMonitors(f.q, func() *sql.DB { return f.db }, true)
	r := chi.NewRouter()
	r.Post("/api/monitors/{id}/push-token/rotate", h.RotatePushToken)
	pushHandler := handlers.NewPush(f.q, checkresult.New(f.db, incident.NewDetector(store.New(f.db)), nil))
	r.Get("/api/push/{token}", pushHandler.Heartbeat)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/monitors/%d/push-token/rotate", f.monitor.ID), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var rotated struct {
		PushToken string `json:"push_token"`
		PushURL   string `json:"push_url"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&rotated))
	require.True(t, pushauth.ValidToken(rotated.PushToken))
	assert.NotEqual(t, f.token, rotated.PushToken)

	oldReq := httptest.NewRequest(http.MethodGet, "/api/push/"+f.token, nil)
	oldRec := httptest.NewRecorder()
	r.ServeHTTP(oldRec, oldReq)
	assert.Equal(t, http.StatusNotFound, oldRec.Code)
	assert.Equal(t, "{\"error\":\"push monitor not found\"}\n", oldRec.Body.String())

	newReq := httptest.NewRequest(http.MethodGet, "/api/push/"+rotated.PushToken, nil)
	newRec := httptest.NewRecorder()
	r.ServeHTTP(newRec, newReq)
	assert.Equal(t, http.StatusOK, newRec.Code, newRec.Body.String())

	require.NoError(t, f.q.DeleteMonitor(t.Context(), f.monitor.ID))
	_, err := f.q.GetPushTokenByMonitor(t.Context(), f.monitor.ID)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestMonitorsRotatePushTokenRejectsInvalidOrNonPushMonitor(t *testing.T) {
	tests := []struct {
		name        string
		monitorType string
		id          string
		want        int
	}{
		{name: "invalid id", monitorType: "push", id: "bad", want: http.StatusBadRequest},
		{name: "missing", monitorType: "push", id: "99999", want: http.StatusNotFound},
		{name: "non push", monitorType: "http", want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPushFixture(t, tt.monitorType, true)
			h := handlers.NewMonitors(f.q, func() *sql.DB { return f.db }, true)
			r := chi.NewRouter()
			r.Post("/api/monitors/{id}/push-token/rotate", h.RotatePushToken)
			id := tt.id
			if id == "" {
				id = strconv.FormatInt(f.monitor.ID, 10)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/monitors/"+id+"/push-token/rotate", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			assert.Equal(t, tt.want, rec.Code, rec.Body.String())
		})
	}
}
