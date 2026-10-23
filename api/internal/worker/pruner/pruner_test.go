package pruner_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
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

func newAgent(t *testing.T, q *store.Queries) int64 {
	t.Helper()
	a, err := q.CreateAgent(context.Background(), store.CreateAgentParams{
		Name: "host", HostLabel: "host", TokenHash: "hash",
	})
	require.NoError(t, err)
	return a.ID
}

func upsertBucket(t *testing.T, q *store.Queries, agentID int64, at time.Time) {
	t.Helper()
	require.NoError(t, q.Upsert1mMetric(context.Background(), store.Upsert1mMetricParams{
		AgentID: agentID, BucketAt: at.Truncate(time.Minute), CpuPercent: 1,
	}))
}

func count1m(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM infra_metrics_1m`).Scan(&n))
	return n
}

// 1-minute infra metrics fall under the same retention as check results.
func TestPruner_Deletes1mMetricsPastRetention(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	q := store.New(db)
	agentID := newAgent(t, q)

	upsertBucket(t, q, agentID, time.Now().AddDate(0, 0, -91))
	recent := time.Now().Add(-time.Hour)
	upsertBucket(t, q, agentID, recent)

	require.NoError(t, pruner.New(q).Run(ctx))

	left, err := q.List1mMetrics(ctx, store.List1mMetricsParams{
		AgentID: agentID, BucketAt: time.Now().AddDate(-1, 0, 0),
	})
	require.NoError(t, err)
	require.Len(t, left, 1, "the 91-day-old bucket should be gone, the recent one kept")
	assert.WithinDuration(t, recent.Truncate(time.Minute), left[0].BucketAt, time.Second)
}

// A shorter retention_days setting applies to 1-minute metrics too.
func TestPruner_HonoursRetentionSetting(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	q := store.New(db)
	agentID := newAgent(t, q)
	require.NoError(t, q.SetSetting(ctx, generated.SetSettingParams{Key: "retention_days", Value: "7"}))

	upsertBucket(t, q, agentID, time.Now().AddDate(0, 0, -8))
	upsertBucket(t, q, agentID, time.Now().AddDate(0, 0, -6))

	require.NoError(t, pruner.New(q).Run(ctx))
	assert.Equal(t, 1, count1m(t, db), "only the 6-day-old bucket should remain")
}

// More expired rows than one batch holds are all removed, by looping.
func TestPruner_DeletesInBatches(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	q := store.New(db)
	agentID := newAgent(t, q)

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	qtx := q.WithTx(tx)
	old := time.Now().AddDate(0, 0, -100).Truncate(time.Minute)
	for i := 0; i < pruner.BatchSize+10; i++ {
		require.NoError(t, qtx.Upsert1mMetric(ctx, store.Upsert1mMetricParams{
			AgentID: agentID, BucketAt: old.Add(-time.Duration(i) * time.Minute),
		}))
	}
	require.NoError(t, tx.Commit())
	upsertBucket(t, q, agentID, time.Now())

	require.NoError(t, pruner.New(q).Run(ctx))
	assert.Equal(t, 1, count1m(t, db))
}

// Expired sessions are removed. Live ones, including one that expires soon,
// must keep working.
func TestPruner_DeletesOnlyExpiredSessions(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	q := store.New(db)
	u, err := q.CreateUser(ctx, store.CreateUserParams{Username: "admin", PasswordHash: "x"})
	require.NoError(t, err)

	sessions := map[string]time.Time{
		"expired-long-ago": time.Now().AddDate(0, 0, -30),
		"expired-just-now": time.Now().Add(-time.Second),
		"expires-soon":     time.Now().Add(time.Minute),
		"fresh":            time.Now().Add(24 * time.Hour),
	}
	for token, exp := range sessions {
		require.NoError(t, q.CreateSession(ctx, store.CreateSessionParams{
			Token: token, UserID: u.ID, ExpiresAt: exp,
		}))
	}

	require.NoError(t, pruner.New(q).Run(ctx))

	for _, token := range []string{"expired-long-ago", "expired-just-now"} {
		_, err := q.GetSession(ctx, token)
		assert.ErrorIs(t, err, sql.ErrNoRows, token)
	}
	for _, token := range []string{"expires-soon", "fresh"} {
		_, err := q.GetSession(ctx, token)
		assert.NoError(t, err, token)
	}
}

// A failing table is reported by name, and the other tables are still tried.
func TestPruner_ReportsEveryFailingTable(t *testing.T) {
	db := testutil.NewTestDB(t)
	// GetSetting succeeds, every DELETE after it fails.
	err := pruner.New(store.New(testutil.FailAfter(db, 1))).Run(context.Background())
	require.Error(t, err)
	for _, table := range []string{"check_results", "agent_diagnostics", "infra_metrics_1m", "sessions"} {
		assert.Contains(t, err.Error(), table)
	}
}

func TestDeleteInBatches(t *testing.T) {
	ctx := context.Background()

	t.Run("loops until a short batch", func(t *testing.T) {
		calls := 0
		total, err := pruner.DeleteInBatches(ctx, func(_ context.Context, limit int64) (int64, error) {
			calls++
			if calls < 3 {
				return limit, nil
			}
			return 7, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 3, calls)
		assert.Equal(t, int64(2*pruner.BatchSize+7), total)
	})

	t.Run("stops on error", func(t *testing.T) {
		boom := errors.New("boom")
		_, err := pruner.DeleteInBatches(ctx, func(context.Context, int64) (int64, error) {
			return 0, boom
		})
		assert.ErrorIs(t, err, boom)
	})

	t.Run("stops when the context is cancelled", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		calls := 0
		_, err := pruner.DeleteInBatches(cctx, func(_ context.Context, limit int64) (int64, error) {
			calls++
			cancel()
			return limit, nil
		})
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, calls)
	})
}
