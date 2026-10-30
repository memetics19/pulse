package metrics_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/metrics"
	"github.com/memetics19/pulse/api/internal/worker/checkresult"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scrape(t *testing.T, h http.Handler, auth string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	return rec.Code, string(body)
}

func createMonitor(t *testing.T, q *generated.Queries, name, typ, target string, groupID *int64, active bool) generated.Monitor {
	t.Helper()
	m, err := q.CreateMonitor(context.Background(), generated.CreateMonitorParams{
		Name: name, Url: target, Type: typ, IntervalSeconds: 60, TimeoutSeconds: 10,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: active,
		GroupID: groupID, Source: "internal",
	})
	require.NoError(t, err)
	return m
}

func insertCheck(t *testing.T, q *generated.Queries, monitorID int64, status string, ms *int64, at time.Time) {
	t.Helper()
	_, err := q.InsertCheckResult(context.Background(), generated.InsertCheckResultParams{
		MonitorID: monitorID, CheckedAt: at, Status: status, ResponseTimeMs: ms,
	})
	require.NoError(t, err)
}

func ms(v int64) *int64 { return &v }

func TestSettingsFromEnv(t *testing.T) {
	tests := []struct {
		enabled, token string
		want           metrics.Settings
	}{
		{"", "", metrics.Settings{}},
		{"false", "x", metrics.Settings{Token: "x"}},
		{"true", "", metrics.Settings{Enabled: true}},
		{"TRUE", " s3cret ", metrics.Settings{Enabled: true, Token: "s3cret"}},
		{"1", "", metrics.Settings{Enabled: true}},
		{"yes", "", metrics.Settings{Enabled: true}},
	}
	for _, tt := range tests {
		t.Setenv("PULSE_METRICS", tt.enabled)
		t.Setenv("PULSE_METRICS_TOKEN", tt.token)
		assert.Equal(t, tt.want, metrics.SettingsFromEnv(), "PULSE_METRICS=%q", tt.enabled)
	}
}

func TestHandlerExposesMonitorStateWithoutTargets(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	ctx := context.Background()
	grp, err := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "Core"})
	require.NoError(t, err)

	api := createMonitor(t, q, "Labels API", "http", "https://secret-api.internal.example/health?key=hunter2", &grp.ID, true)
	dbm := createMonitor(t, q, "Labels DB", "tcp", "db.secret-lan.example:5432", nil, true)
	slow := createMonitor(t, q, "Labels Slow", "ping", "10.9.8.7", &grp.ID, true)
	createMonitor(t, q, "Labels Fresh", "dns", "fresh.secret.example", nil, true)
	paused := createMonitor(t, q, "Labels Paused", "http", "https://paused.secret.example", nil, false)

	now := time.Now().UTC()
	// An older result must not win over the latest one.
	insertCheck(t, q, api.ID, "down", ms(9000), now.Add(-time.Minute))
	insertCheck(t, q, dbm.ID, "down", nil, now)
	insertCheck(t, q, slow.ID, "degraded", ms(750), now)
	insertCheck(t, q, paused.ID, "up", ms(10), now)

	// The recorder feeds the checks counter as well as the database.
	rec := checkresult.New(db, nil, nil)
	require.NoError(t, rec.Record(ctx, api, checkresult.Input{Status: "up", ResponseTimeMs: ms(120), CheckedAt: now}))

	_, err = q.CreateIncident(ctx, generated.CreateIncidentParams{
		Title: "open one", Severity: "minor", StartedAt: now, Source: "manual",
	})
	require.NoError(t, err)
	resolved, err := q.CreateIncident(ctx, generated.CreateIncidentParams{
		Title: "closed one", Severity: "minor", StartedAt: now, Source: "manual",
	})
	require.NoError(t, err)
	_, err = q.UpdateIncidentStatus(ctx, generated.UpdateIncidentStatusParams{Status: "resolved", ResolvedAt: &now, ID: resolved.ID})
	require.NoError(t, err)

	a := app.New()
	a.SetDB(db)
	a.MarkWorkerAlive()

	code, body := scrape(t, metrics.Handler(a, ""), "")
	require.Equal(t, http.StatusOK, code, body)

	for _, line := range []string{
		`pulse_monitor_up{group="Core",monitor="Labels API",type="http"} 1`,
		`pulse_monitor_response_seconds{group="Core",monitor="Labels API",type="http"} 0.12`,
		`pulse_monitor_up{group="",monitor="Labels DB",type="tcp"} 0`,
		`pulse_monitor_up{group="Core",monitor="Labels Slow",type="ping"} 0.5`,
		`pulse_monitor_response_seconds{group="Core",monitor="Labels Slow",type="ping"} 0.75`,
		`pulse_monitor_checks_total{monitor="Labels API",result="up"} 1`,
		"pulse_incidents_open 1",
		"go_goroutines ",
	} {
		assert.Contains(t, body, line)
	}
	// No data yet, paused, or no response time: the series is absent.
	assert.NotContains(t, body, `monitor="Labels Fresh"`)
	assert.NotContains(t, body, `monitor="Labels Paused"`)
	assert.NotContains(t, body, `pulse_monitor_response_seconds{group="",monitor="Labels DB"`)
	assert.NotContains(t, body, "pulse_worker_last_beat_timestamp_seconds 0\n")
	assert.Contains(t, body, "pulse_worker_last_beat_timestamp_seconds ")

	// Targets never reach the output. The port is matched with its colon: a
	// bare "5432" also turns up inside runtime numbers such as 1.9335432e+07.
	for _, leak := range []string{"secret", "hunter2", "10.9.8.7", ":5432", "https://"} {
		assert.NotContains(t, body, leak)
	}
}

func TestHandlerSkipsDuplicateLabelSets(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	first := createMonitor(t, q, "Dup", "http", "https://a.example", nil, true)
	second := createMonitor(t, q, "Dup", "http", "https://b.example", nil, true)
	now := time.Now().UTC()
	insertCheck(t, q, first.ID, "up", ms(100), now)
	insertCheck(t, q, second.ID, "down", ms(200), now)
	a := app.New()
	a.SetDB(db)

	code, body := scrape(t, metrics.Handler(a, ""), "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, 1, strings.Count(body, `pulse_monitor_up{group="",monitor="Dup",type="http"}`))
	assert.Contains(t, body, `pulse_monitor_up{group="",monitor="Dup",type="http"} 1`)
}

func TestHandlerBeforeSetupServesProcessMetrics(t *testing.T) {
	code, body := scrape(t, metrics.Handler(app.New(), ""), "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, "pulse_worker_last_beat_timestamp_seconds 0\n")
	assert.Contains(t, body, "go_goroutines ")
	assert.NotContains(t, body, "pulse_monitor_up{")
	assert.NotContains(t, body, "pulse_incidents_open ")
}

func TestHandlerDatabaseErrorDropsOnlyStateMetrics(t *testing.T) {
	db := testutil.NewTestDB(t)
	a := app.New()
	a.SetDB(db)
	require.NoError(t, db.Close())

	code, body := scrape(t, metrics.Handler(a, ""), "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, "go_goroutines ")
	assert.Contains(t, body, "pulse_worker_last_beat_timestamp_seconds ")
	assert.NotContains(t, body, "pulse_incidents_open ")
	assert.NotContains(t, body, "pulse_monitor_up{")
}

func TestHandlerBearerToken(t *testing.T) {
	h := metrics.Handler(app.New(), "s3cret")
	tests := []struct {
		auth string
		want int
	}{
		{"", http.StatusUnauthorized},
		{"Bearer wrong", http.StatusUnauthorized},
		{"Basic s3cret", http.StatusUnauthorized},
		{"Bearer", http.StatusUnauthorized},
		{"Bearer s3cret", http.StatusOK},
		{"bearer s3cret", http.StatusOK},
	}
	for _, tt := range tests {
		code, body := scrape(t, h, tt.auth)
		assert.Equal(t, tt.want, code, "Authorization %q", tt.auth)
		if tt.want == http.StatusUnauthorized {
			assert.NotContains(t, body, "go_goroutines")
		}
	}
}
