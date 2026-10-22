package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOverviewAggregates(t *testing.T) {
	q := newQ(t)
	ctx := context.Background()

	mk := func(name string) int64 {
		m, err := q.CreateMonitor(ctx, generated.CreateMonitorParams{
			Name: name, Url: "http://example.com", Type: "http", IntervalSeconds: 60,
			TimeoutSeconds: 10, DegradedThresholdMs: 500, DownThresholdMs: 2000,
			IsActive: true, Source: "internal",
		})
		require.NoError(t, err)
		return m.ID
	}
	downID := mk("down-svc")
	degID := mk("deg-svc")
	mk("up-svc") // no check result -> defaults up

	insert := func(id int64, status string) {
		_, err := q.InsertCheckResult(ctx, generated.InsertCheckResultParams{
			MonitorID: id, CheckedAt: time.Now(), Status: status,
		})
		require.NoError(t, err)
	}
	insert(downID, "down")
	insert(degID, "degraded")

	_, err := q.CreateIncident(ctx, generated.CreateIncidentParams{
		Title: "ongoing", Severity: "major", AffectedMonitorIds: "[]", StartedAt: time.Now(), Source: "internal",
	})
	require.NoError(t, err)

	rr := do(handlers.NewOverview(q).Get, "GET", "/api/overview", nil)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp struct {
		Overall string         `json:"overall"`
		Counts  map[string]int `json:"counts"`
		Total   int            `json:"total_monitors"`
		Active  []any          `json:"active_incidents"`
		Attn    []any          `json:"attention"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "outage", resp.Overall)
	assert.Equal(t, 1, resp.Counts["down"])
	assert.Equal(t, 1, resp.Counts["degraded"])
	assert.Equal(t, 1, resp.Counts["up"])
	assert.Equal(t, 3, resp.Total)
	assert.Len(t, resp.Active, 1)
	assert.NotEmpty(t, resp.Attn) // the down monitor produces an attention item
}
