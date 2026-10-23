package rollup

import (
	"context"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seed(t *testing.T, q *store.Queries) int64 {
	t.Helper()
	a, err := q.CreateAgent(context.Background(), store.CreateAgentParams{
		Name: "host", HostLabel: "host", TokenHash: "tok",
	})
	require.NoError(t, err)
	insertRaw(t, q, a.ID, time.Now().Add(-time.Hour), 10)
	return a.ID
}

func insertRaw(t *testing.T, q *store.Queries, agentID int64, at time.Time, cpu float64) {
	t.Helper()
	require.NoError(t, q.InsertRawMetric(context.Background(), store.InsertRawMetricParams{
		AgentID: agentID, CollectedAt: at, CpuPercent: cpu,
	}))
}

// Each Run makes these calls in order, so failing after n of them reaches
// each error branch in turn.
func TestRunErrorBranches(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name      string
		primed    bool
		failAfter int
	}{
		{"max id", false, 0},
		{"list agents", false, 1},
		{"list raw metrics", false, 2},
		{"upsert", false, 3},
		{"prune raw", false, 4},
		{"list new rows", true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := testutil.NewTestDB(t)
			live := store.New(db)
			seed(t, live)
			r := New(live)
			if tc.primed {
				require.NoError(t, r.Run(ctx))
				insertRaw(t, live, 1, time.Now(), 20)
			}
			before := r.watermark
			r.q = store.New(testutil.FailAfter(db, tc.failAfter))
			require.Error(t, r.Run(ctx))
			if tc.name != "prune raw" {
				assert.Equal(t, before, r.watermark, "watermark must not move past rows that were not rolled up")
			}
		})
	}
}

// A failed upsert leaves the watermark where it was, so the next run rebuilds
// the same bucket instead of skipping it forever.
func TestRunRetriesBucketsAfterFailure(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	live := store.New(db)
	agentID := seed(t, live)
	r := New(live)
	require.NoError(t, r.Run(ctx))
	primedAt := r.watermark

	now := time.Now()
	insertRaw(t, live, agentID, now, 30)
	// max id, list new rows, list raw metrics succeed; the upsert fails.
	r.q = store.New(testutil.FailAfter(db, 3))
	require.Error(t, r.Run(ctx))
	assert.Equal(t, primedAt, r.watermark)

	r.q = live
	require.NoError(t, r.Run(ctx))
	assert.Greater(t, r.watermark, primedAt)
	buckets, err := live.List1mMetrics(ctx, store.List1mMetricsParams{
		AgentID: agentID, BucketAt: now.Add(-time.Minute),
	})
	require.NoError(t, err)
	require.Len(t, buckets, 1)
	assert.Equal(t, 30.0, buckets[0].CpuPercent)
}

// A bucket that starts before the window is only read from the window start,
// the same as a full pass.
func TestChangedBucketsClampsToWindow(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	q := store.New(db)
	agentID := seed(t, q)
	r := &Rollup{q: q, primed: true}

	since := time.Now().Truncate(time.Minute).Add(-time.Hour + 30*time.Second)
	insertRaw(t, q, agentID, since.Add(10*time.Second), 40)
	through, err := q.MaxRawMetricID(ctx)
	require.NoError(t, err)

	plans, err := r.changedBuckets(ctx, since, through)
	require.NoError(t, err)
	require.Contains(t, plans, agentID)
	assert.True(t, plans[agentID].from.Equal(since))

	// Nothing new past the watermark means nothing to plan.
	r.watermark = through
	plans, err = r.changedBuckets(ctx, since, through)
	require.NoError(t, err)
	assert.Empty(t, plans)
}
