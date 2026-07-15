package scheduler_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/memetics19/pulse/api/internal/worker/checkresult"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/internal/worker/scheduler"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingChecker struct{ count atomic.Int64 }

func (c *countingChecker) Check(_ context.Context, _ string, _ int64) checker.Result {
	c.count.Add(1)
	return checker.Result{Status: "up", ResponseTimeMs: 5, CheckedAt: time.Now()}
}

func TestScheduler_CallsCheckerAtInterval(t *testing.T) {
	cc := &countingChecker{}

	// Use a "tcp" type so the registered mock checker is exercised directly;
	// the scheduler builds a fresh per-monitor checker only for http(s).
	mon := store.Monitor{
		ID: 1, Name: "test", Url: "tcp://x.com:80", Type: "tcp",
		IntervalSeconds: 1, TimeoutSeconds: 5,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true,
		Source: "internal",
	}

	// nil DB — scheduler should handle nil gracefully when no DB ops needed
	s := scheduler.New(nil, nil, true)
	s.SetChecker("tcp", cc)

	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	go s.RunMonitor(ctx, mon)

	<-ctx.Done()
	count := cc.count.Load()
	assert.GreaterOrEqual(t, count, int64(2), "expected at least 2 checks in 2.5s with 1s interval")
}

func TestScheduler_ZeroIntervalDoesNotPanic(t *testing.T) {
	cc := &countingChecker{}

	// IntervalSeconds: 0 would make time.NewTicker(0) panic without the clamp.
	mon := store.Monitor{
		ID: 1, Name: "test", Url: "tcp://x.com:80", Type: "tcp",
		IntervalSeconds: 0, TimeoutSeconds: 5,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true,
		Source: "internal",
	}

	s := scheduler.New(nil, nil, true)
	s.SetChecker("tcp", cc)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		s.RunMonitor(ctx, mon)
		close(done)
	}()

	select {
	case <-done:
		// Returned cleanly after the immediate check and ctx cancellation.
		assert.GreaterOrEqual(t, cc.count.Load(), int64(1), "expected the immediate check to run")
	case <-time.After(time.Second):
		t.Fatal("RunMonitor did not return after context cancellation")
	}
}

func TestSchedulerRecordsEachCheckExactlyOnce(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon, err := q.CreateMonitor(context.Background(), store.CreateMonitorParams{
		Name: "test", Url: "db:5432", Type: "tcp",
		IntervalSeconds: 60, TimeoutSeconds: 5,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true,
		Source: "internal",
	})
	require.NoError(t, err)

	recorder := checkresult.New(db, incident.NewDetector(q), nil)
	s := scheduler.New(db, recorder, true)
	s.SetChecker("tcp", &countingChecker{})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	s.RunMonitor(ctx, mon)

	results, err := q.LatestTwoCheckResults(context.Background(), mon.ID)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "up", results[0].Status)
}

func TestSchedulerStartRejectsNilRecorder(t *testing.T) {
	db := testutil.NewTestDB(t)
	s := scheduler.New(db, nil, true)

	err := s.Start(context.Background())

	require.EqualError(t, err, "scheduler: no check result recorder")
}
