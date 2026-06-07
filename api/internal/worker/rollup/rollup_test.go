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
		Name: "test-host", HostLabel: "host-1", Token: "tok1",
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
