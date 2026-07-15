package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaintenanceListRendersView(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	now := time.Now()
	_, err := q.CreateMaintenance(context.Background(), generated.CreateMaintenanceParams{
		Title: "win", Status: "scheduled", AffectedMonitorIds: "[1,2]",
		StartsAt: now, EndsAt: now.Add(time.Hour),
	})
	require.NoError(t, err)

	rr := do(handlers.NewMaintenance(q).List, "GET", "/api/maintenance", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	var views []struct {
		AffectedMonitorIds []int64 `json:"affected_monitor_ids"`
	}
	decode(t, rr, &views)
	require.Len(t, views, 1)
	assert.Equal(t, []int64{1, 2}, views[0].AffectedMonitorIds)
}

func TestOverviewFlagsOfflineAgent(t *testing.T) {
	db := testutil.NewTestDB(t)
	// An active agent last seen 10 minutes ago -> "agent_offline" attention.
	_, err := db.Exec(
		`INSERT INTO infra_agents (name, host_label, token_hash, is_active, last_seen_at) VALUES ('h','web','hash',1,?)`,
		time.Now().Add(-10*time.Minute))
	require.NoError(t, err)

	rr := do(handlers.NewOverview(generated.New(db)).Get, "GET", "/api/overview", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	var resp struct {
		AgentCount int `json:"agent_count"`
		Attention  []struct {
			Kind string `json:"kind"`
		} `json:"attention"`
	}
	decode(t, rr, &resp)
	assert.Equal(t, 1, resp.AgentCount)
	found := false
	for _, a := range resp.Attention {
		if a.Kind == "agent_offline" {
			found = true
		}
	}
	assert.True(t, found, "offline agent should raise an attention item")
}
