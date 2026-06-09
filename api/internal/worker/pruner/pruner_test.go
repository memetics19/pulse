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
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: 1,
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
