package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ?days is capped at 90, so a huge value cannot make one request scan the
// whole check_results table, and rows older than the cap are not returned.
func TestCheckResultsDaysIsClamped(t *testing.T) {
	q := newQ(t)
	ctx := t.Context()
	mon, err := q.CreateMonitor(ctx, generated.CreateMonitorParams{
		Name: "m", Url: "http://example.com", Type: "http", IntervalSeconds: 60,
		TimeoutSeconds: 10, DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, Source: "internal",
	})
	require.NoError(t, err)
	for _, at := range []time.Time{time.Now().AddDate(0, 0, -120), time.Now().Add(-time.Minute)} {
		_, err = q.InsertCheckResult(ctx, generated.InsertCheckResultParams{MonitorID: mon.ID, CheckedAt: at, Status: "up"})
		require.NoError(t, err)
	}
	h := handlers.NewCheckResults(q)
	id := itoa(mon.ID)

	list := do(func(w http.ResponseWriter, r *http.Request) {
		h.List(w, withChiID(r, "monitorID", id))
	}, "GET", "/x?days=100000", nil)
	require.Equal(t, http.StatusOK, list.Code)
	var rows []generated.CheckResult
	require.NoError(t, json.NewDecoder(list.Body).Decode(&rows))
	assert.Len(t, rows, 1, "the 120 day old row is outside the 90 day cap")

	for query, want := range map[string]float64{"?days=100000": 90, "?days=30": 30, "?days=-5": 90, "": 90} {
		up := do(func(w http.ResponseWriter, r *http.Request) {
			h.Uptime(w, withChiID(r, "monitorID", id))
		}, "GET", "/x"+query, nil)
		require.Equal(t, http.StatusOK, up.Code)
		var body map[string]any
		require.NoError(t, json.NewDecoder(up.Body).Decode(&body))
		assert.Equal(t, want, body["days"], "uptime %q", query)
	}
}
