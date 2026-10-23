package rollup_test

import (
	"context"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/rollup"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRollup_AggregatesRawInto1mBucket(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)

	agent, err := q.CreateAgent(context.Background(), store.CreateAgentParams{
		Name: "test-host", HostLabel: "host-1", TokenHash: "tok1",
	})
	require.NoError(t, err)

	now := time.Now().Truncate(time.Minute)

	require.NoError(t, q.InsertRawMetric(context.Background(), store.InsertRawMetricParams{
		AgentID: agent.ID, CollectedAt: now.Add(-50 * time.Second),
		CpuPercent: 20, MemPercent: 40, MemUsedMb: 800, DiskPercent: 50, DiskUsedGb: 100,
		NetRxBytes: 1000, NetTxBytes: 500,
	}))
	require.NoError(t, q.InsertRawMetric(context.Background(), store.InsertRawMetricParams{
		AgentID: agent.ID, CollectedAt: now.Add(-20 * time.Second),
		CpuPercent: 40, MemPercent: 60, MemUsedMb: 1200, DiskPercent: 70, DiskUsedGb: 120,
		NetRxBytes: 2000, NetTxBytes: 1000,
	}))

	r := rollup.New(q)
	err = r.Run(context.Background())
	require.NoError(t, err)

	buckets, err := q.List1mMetrics(context.Background(), store.List1mMetricsParams{
		AgentID:  agent.ID,
		BucketAt: now.Add(-2 * time.Minute),
	})
	require.NoError(t, err)
	require.Len(t, buckets, 1)
	assert.InDelta(t, 30.0, buckets[0].CpuPercent, 0.1)
	assert.InDelta(t, 50.0, buckets[0].MemPercent, 0.1)
}

// rawSample is one raw row in the incremental-vs-full comparison.
type rawSample struct {
	agent int // index into the agents slice
	at    time.Time
	cpu   float64
}

func insertSamples(t *testing.T, q *store.Queries, agentIDs []int64, samples []rawSample) {
	t.Helper()
	for _, s := range samples {
		require.NoError(t, q.InsertRawMetric(context.Background(), store.InsertRawMetricParams{
			AgentID: agentIDs[s.agent], CollectedAt: s.at,
			CpuPercent: s.cpu, MemPercent: s.cpu / 3, MemUsedMb: 512,
			DiskPercent: 100 - s.cpu, DiskUsedGb: 50,
			// Odd byte counts so the integer averages truncate.
			NetRxBytes: 1000 + s.at.Unix()%97, NetTxBytes: 333 + s.at.Unix()%31,
		}))
	}
}

func createAgents(t *testing.T, q *store.Queries, n int) []int64 {
	t.Helper()
	ids := make([]int64, n)
	for i := range ids {
		a, err := q.CreateAgent(context.Background(), store.CreateAgentParams{
			Name: "host", HostLabel: "host", TokenHash: string(rune('a' + i)),
		})
		require.NoError(t, err)
		ids[i] = a.ID
	}
	return ids
}

func allBuckets(t *testing.T, q *store.Queries, agentIDs []int64) []store.InfraMetrics1m {
	t.Helper()
	var out []store.InfraMetrics1m
	for _, id := range agentIDs {
		rows, err := q.List1mMetrics(context.Background(), store.List1mMetricsParams{
			AgentID: id, BucketAt: time.Now().Add(-48 * time.Hour),
		})
		require.NoError(t, err)
		out = append(out, rows...)
	}
	return out
}

// The incremental rollup must leave infra_metrics_1m exactly as a full
// recompute over the same raw data would, including when samples arrive late
// for an old bucket, when a later batch adds to a bucket already rolled up,
// and when a new agent appears after the first run.
func TestRollup_IncrementalMatchesFullRecompute(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Minute)

	var first []rawSample
	for i := 0; i < 3*60*4; i++ { // three hours, every 15s, two agents
		at := now.Add(-3 * time.Hour).Add(time.Duration(i) * 15 * time.Second)
		first = append(first, rawSample{agent: 0, at: at, cpu: float64(i%97) + 0.1})
		first = append(first, rawSample{agent: 1, at: at.Add(3 * time.Second), cpu: float64(i%89) + 0.7})
	}
	second := []rawSample{
		{agent: 0, at: now.Add(5 * time.Second), cpu: 11},                    // current minute
		{agent: 0, at: now.Add(-2*time.Hour + 7*time.Second), cpu: 99},       // late, into an old bucket
		{agent: 1, at: now.Add(-1*time.Minute + 50*time.Second), cpu: 42.5},  // previous minute
		{agent: 2, at: now.Add(-30 * time.Minute), cpu: 5},                   // agent with no data before
		{agent: 2, at: now.Add(-30*time.Minute + 20*time.Second), cpu: 6.25}, // same bucket
	}
	third := []rawSample{
		{agent: 0, at: now.Add(25 * time.Second), cpu: 13},                  // same bucket as second batch
		{agent: 1, at: now.Add(-20*time.Hour + 1*time.Second), cpu: 77},     // very late, new bucket
		{agent: 2, at: now.Add(-30*time.Minute + 40*time.Second), cpu: 7.5}, // grows a bucket again
	}

	// Incremental: roll up after every batch with one long-lived Rollup.
	incDB := store.New(testutil.NewTestDB(t))
	incAgents := createAgents(t, incDB, 3)
	r := rollup.New(incDB)
	for _, batch := range [][]rawSample{first, second, third} {
		insertSamples(t, incDB, incAgents, batch)
		require.NoError(t, r.Run(ctx))
	}

	// Full: a fresh Rollup's first run recomputes every bucket in the window.
	fullDB := store.New(testutil.NewTestDB(t))
	fullAgents := createAgents(t, fullDB, 3)
	insertSamples(t, fullDB, fullAgents, append(append(append([]rawSample{}, first...), second...), third...))
	require.NoError(t, rollup.New(fullDB).Run(ctx))

	inc := allBuckets(t, incDB, incAgents)
	full := allBuckets(t, fullDB, fullAgents)
	require.Len(t, inc, len(full))
	require.Greater(t, len(full), 3*60)
	for i := range full {
		f, g := full[i], inc[i]
		assert.Equal(t, f.AgentID, g.AgentID)
		assert.True(t, f.BucketAt.Equal(g.BucketAt), "bucket %d: %v vs %v", i, f.BucketAt, g.BucketAt)
		assert.Equal(t, f.CpuPercent, g.CpuPercent, "cpu at %v", f.BucketAt)
		assert.Equal(t, f.MemPercent, g.MemPercent, "mem at %v", f.BucketAt)
		assert.Equal(t, f.DiskPercent, g.DiskPercent, "disk at %v", f.BucketAt)
		assert.Equal(t, f.NetRxBytes, g.NetRxBytes, "rx at %v", f.BucketAt)
		assert.Equal(t, f.NetTxBytes, g.NetTxBytes, "tx at %v", f.BucketAt)
	}
}

// After the first run, a bucket with no new raw rows must not be rewritten.
func TestRollup_SkipsBucketsWithoutNewRows(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	q := store.New(db)
	agents := createAgents(t, q, 1)
	now := time.Now().Truncate(time.Minute)
	old := now.Add(-time.Hour)

	insertSamples(t, q, agents, []rawSample{{agent: 0, at: old.Add(10 * time.Second), cpu: 10}})
	r := rollup.New(q)
	require.NoError(t, r.Run(ctx))

	// Mark the old bucket. A full recompute would overwrite the marker.
	_, err := db.ExecContext(ctx, `UPDATE infra_metrics_1m SET cpu_percent = -1`)
	require.NoError(t, err)

	insertSamples(t, q, agents, []rawSample{{agent: 0, at: now.Add(10 * time.Second), cpu: 20}})
	require.NoError(t, r.Run(ctx))

	buckets := allBuckets(t, q, agents)
	require.Len(t, buckets, 2)
	assert.Equal(t, -1.0, buckets[0].CpuPercent, "untouched bucket was rewritten")
	assert.Equal(t, 20.0, buckets[1].CpuPercent)

	// A run with no new rows writes nothing.
	require.NoError(t, r.Run(ctx))
	assert.Equal(t, -1.0, allBuckets(t, q, agents)[0].CpuPercent)
}

// Raw rows older than the window are neither rolled up nor kept, as before.
func TestRollup_IgnoresAndPrunesRowsOutsideWindow(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	q := store.New(db)
	agents := createAgents(t, q, 1)
	r := rollup.New(q)
	require.NoError(t, r.Run(ctx))

	insertSamples(t, q, agents, []rawSample{
		{agent: 0, at: time.Now().Add(-30 * time.Hour), cpu: 50},
		{agent: 0, at: time.Now().Add(-time.Minute), cpu: 60},
	})
	require.NoError(t, r.Run(ctx))

	buckets := allBuckets(t, q, agents)
	require.Len(t, buckets, 1)
	assert.Equal(t, 60.0, buckets[0].CpuPercent)

	raws, err := q.ListRawMetrics(ctx, store.ListRawMetricsParams{
		AgentID: agents[0], CollectedAt: time.Now().Add(-48 * time.Hour),
	})
	require.NoError(t, err)
	require.Len(t, raws, 1, "the 30h-old raw row should be pruned")
}
