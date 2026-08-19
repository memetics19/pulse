package pruner_test

import (
	"context"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/pruner"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruner_DeletesOldCheckResults(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)

	mon, err := q.CreateMonitor(context.Background(), store.CreateMonitorParams{
		Name: "Test", Url: "http://x.com", Type: "http",
		IntervalSeconds: 60, TimeoutSeconds: 10,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true,
		Source: "internal",
	})
	require.NoError(t, err)

	oldTime := time.Now().AddDate(0, 0, -91)
	recentTime := time.Now().Add(-time.Hour)
	statusCode := int64(200)
	respMs := int64(100)

	_, err = q.InsertCheckResult(context.Background(), store.InsertCheckResultParams{
		MonitorID: mon.ID, CheckedAt: oldTime, Status: "up",
		ResponseTimeMs: &respMs, StatusCode: &statusCode,
	})
	require.NoError(t, err)

	_, err = q.InsertCheckResult(context.Background(), store.InsertCheckResultParams{
		MonitorID: mon.ID, CheckedAt: recentTime, Status: "up",
		ResponseTimeMs: &respMs, StatusCode: &statusCode,
	})
	require.NoError(t, err)

	p := pruner.New(q)
	err = p.Run(context.Background())
	require.NoError(t, err)

	results, err := q.ListCheckResults(context.Background(), store.ListCheckResultsParams{
		MonitorID: mon.ID, CheckedAt: time.Now().AddDate(0, 0, -365), Limit: 100,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	// Compare instants, not the time.Time struct: the DB round-trips CheckedAt
	// in UTC while recentTime carries the Local location, so assert.Equal (which
	// compares the location pointer) would spuriously fail on non-UTC hosts.
	assert.WithinDuration(t, recentTime, results[0].CheckedAt, time.Second)
}

// Diagnostic bundles are the largest rows Pulse stores and every authenticated
// agent request adds one, so they must fall under the same retention window as
// check results.
func TestPruner_DeletesOldDiagnostics(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)

	agent, err := q.CreateAgent(context.Background(), store.CreateAgentParams{
		Name: "proxmox", HostLabel: "homeserver", TokenHash: "hash",
	})
	require.NoError(t, err)

	for _, at := range []time.Time{time.Now().AddDate(0, 0, -91), time.Now().Add(-time.Hour)} {
		require.NoError(t, q.InsertAgentDiagnostic(context.Background(),
			store.InsertAgentDiagnosticParams{
				AgentID: agent.ID, CollectedAt: at, Payload: `{"sections":{}}`,
			}))
	}

	require.NoError(t, pruner.New(q).Run(context.Background()))

	remaining, err := q.ListAgentDiagnostics(context.Background(),
		store.ListAgentDiagnosticsParams{AgentID: agent.ID, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, remaining, 1, "the 91-day-old bundle should be gone, the recent one kept")
}
