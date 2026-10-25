package scheduler

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/require"
)

// urlChecker records each checked URL.
type urlChecker struct {
	mu   sync.Mutex
	urls []string
}

func (c *urlChecker) Check(_ context.Context, url string, _ int64) checker.Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.urls = append(c.urls, url)
	return checker.Result{Status: "up", CheckedAt: time.Now()}
}

func (c *urlChecker) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.urls)
}

func (c *urlChecker) checked(url string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, u := range c.urls {
		if u == url {
			return true
		}
	}
	return false
}

func TestFirstCheckDelayStaysInWindowAndIsStable(t *testing.T) {
	for _, interval := range []time.Duration{time.Second, 10 * time.Second, time.Minute, time.Hour} {
		window := min(interval, maxStartupJitter)
		for id := int64(1); id <= 1000; id++ {
			d := firstCheckDelay(id, interval)
			require.GreaterOrEqual(t, d, time.Duration(0))
			require.Less(t, d, window, "id %d interval %s", id, interval)
			require.LessOrEqual(t, d, interval)
			require.Equal(t, d, firstCheckDelay(id, interval), "delay must be deterministic")
		}
	}
	require.Zero(t, firstCheckDelay(7, 0))
}

func TestFirstCheckDelaySpreadsConsecutiveIDs(t *testing.T) {
	// 100 monitors with a one-minute interval should cover every 3s slice
	// of the 30s window, not pile up at the start.
	const slice = 3 * time.Second
	buckets := map[time.Duration]int{}
	for id := int64(1); id <= 100; id++ {
		buckets[firstCheckDelay(id, time.Minute)/slice]++
	}
	require.Len(t, buckets, int(maxStartupJitter/slice))
	for b, n := range buckets {
		require.LessOrEqual(t, n, 20, "bucket %d holds too many first checks", b)
	}
}

func TestStartupReconcileSpreadsFirstChecks(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	const n = 12
	var mons []store.Monitor
	for i := range n {
		m, err := q.CreateMonitor(ctx, store.CreateMonitorParams{
			Name: "m", Url: "tcp://" + string(rune('a'+i)) + ".example.com:80", Type: "tcp",
			IntervalSeconds: 60, TimeoutSeconds: 5, IsActive: true,
		})
		require.NoError(t, err)
		mons = append(mons, m)
	}

	clk := newFakeClock(time.Unix(1_700_000_000, 0))
	cc := &urlChecker{}
	s := New(db, nil, true)
	s.clock = clk
	s.SetChecker("tcp", cc)

	require.NoError(t, s.reconcile(ctx))

	// Every monitor waits on a startup timer. Without jitter, all of them
	// would check at once and no timer would be created.
	for range n {
		select {
		case <-clk.created:
		case <-time.After(2 * time.Second):
			t.Fatal("first checks were not delayed")
		}
	}
	require.Zero(t, cc.count(), "no check should run before its offset")

	delays := make([]time.Duration, 0, n)
	for _, m := range mons {
		delays = append(delays, firstCheckDelay(m.ID, time.Minute))
	}
	sort.Slice(delays, func(i, j int) bool { return delays[i] < delays[j] })
	require.Greater(t, delays[n-1]-delays[0], 10*time.Second, "offsets should spread across the window")

	// Step to each offset in turn. Exactly the monitors whose offset has
	// passed have checked, and the rest are still waiting.
	var elapsed time.Duration
	for i, d := range delays {
		clk.Advance(d - elapsed)
		elapsed = d
		want := i + 1
		for want < n && delays[want] == d {
			want++
		}
		require.Eventually(t, func() bool { return cc.count() == want },
			2*time.Second, 5*time.Millisecond, "after %s", elapsed)
	}
	require.Less(t, elapsed, maxStartupJitter)
}

func TestLaterReconcileChecksCreatedAndEditedMonitorsAtOnce(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	first, err := q.CreateMonitor(ctx, store.CreateMonitorParams{
		Name: "first", Url: "tcp://first.example.com:80", Type: "tcp",
		IntervalSeconds: 3600, TimeoutSeconds: 5, IsActive: true,
	})
	require.NoError(t, err)

	clk := newFakeClock(time.Unix(1_700_000_000, 0))
	cc := &urlChecker{}
	s := New(db, nil, true)
	s.clock = clk
	s.SetChecker("tcp", cc)
	require.NoError(t, s.reconcile(ctx))

	// A monitor created after startup checks without waiting on the clock.
	_, err = q.CreateMonitor(ctx, store.CreateMonitorParams{
		Name: "second", Url: "tcp://second.example.com:80", Type: "tcp",
		IntervalSeconds: 3600, TimeoutSeconds: 5, IsActive: true,
	})
	require.NoError(t, err)
	require.NoError(t, s.reconcile(ctx))
	require.Eventually(t, func() bool { return cc.checked("tcp://second.example.com:80") },
		2*time.Second, 5*time.Millisecond)

	// An edited monitor restarts and checks its new target at once.
	_, err = q.UpdateMonitor(ctx, store.UpdateMonitorParams{
		ID: first.ID, Name: first.Name, Url: "tcp://edited.example.com:80", Type: first.Type,
		IntervalSeconds: first.IntervalSeconds, TimeoutSeconds: first.TimeoutSeconds, IsActive: true,
	})
	require.NoError(t, err)
	require.NoError(t, s.reconcile(ctx))
	require.Eventually(t, func() bool { return cc.checked("tcp://edited.example.com:80") },
		2*time.Second, 5*time.Millisecond)
	require.False(t, cc.checked("tcp://first.example.com:80"), "the old config never reached its delayed first check")
}
