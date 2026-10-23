package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/server"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
)

func getMetrics(h http.Handler, auth string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func configuredApp(t *testing.T) *app.App {
	t.Helper()
	a := app.New()
	a.SetDB(testutil.NewTestDB(t))
	return a
}

func TestMetricsOffByDefault(t *testing.T) {
	t.Setenv("PULSE_METRICS", "")
	t.Setenv("PULSE_METRICS_TOKEN", "")
	h := server.New(configuredApp(t), t.TempDir(), config.Config{})
	assert.Equal(t, http.StatusNotFound, getMetrics(h, "").Code)
}

func TestMetricsEnabled(t *testing.T) {
	t.Setenv("PULSE_METRICS", "true")
	t.Setenv("PULSE_METRICS_TOKEN", "")
	h := server.New(configuredApp(t), t.TempDir(), config.Config{})
	rec := getMetrics(h, "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "pulse_incidents_open 0")
}

func TestMetricsTokenRequired(t *testing.T) {
	t.Setenv("PULSE_METRICS", "true")
	t.Setenv("PULSE_METRICS_TOKEN", "scrape-token")
	h := server.New(configuredApp(t), t.TempDir(), config.Config{})
	assert.Equal(t, http.StatusUnauthorized, getMetrics(h, "").Code)
	assert.Equal(t, http.StatusUnauthorized, getMetrics(h, "Bearer nope").Code)
	assert.Equal(t, http.StatusOK, getMetrics(h, "Bearer scrape-token").Code)
}

func TestMetricsServedDuringSetup(t *testing.T) {
	t.Setenv("PULSE_METRICS", "true")
	t.Setenv("PULSE_METRICS_TOKEN", "")
	h := server.New(app.New(), t.TempDir(), config.Config{})
	rec := getMetrics(h, "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "pulse_worker_last_beat_timestamp_seconds 0")
}
