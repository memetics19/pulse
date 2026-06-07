package scheduler_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/memetics19/pulse/api/internal/worker/scheduler"
	"github.com/memetics19/pulse/api/store"
	"github.com/stretchr/testify/assert"
)

type countingChecker struct{ count atomic.Int64 }

func (c *countingChecker) Check(_ context.Context, _ string, _ int64) checker.Result {
	c.count.Add(1)
	return checker.Result{Status: "up", ResponseTimeMs: 5, CheckedAt: time.Now()}
}

func TestScheduler_CallsCheckerAtInterval(t *testing.T) {
	cc := &countingChecker{}

	mon := store.Monitor{
		ID: 1, Name: "test", Url: "http://x.com", Type: "http",
		IntervalSeconds: 1, TimeoutSeconds: 5,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: 1,
		Source: "internal",
	}

	// nil DB — scheduler should handle nil gracefully when no DB ops needed
	s := scheduler.New(nil, nil, nil)
	s.SetChecker("http", cc)

	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	go s.RunMonitor(ctx, mon)

	<-ctx.Done()
	count := cc.count.Load()
	assert.GreaterOrEqual(t, count, int64(2), "expected at least 2 checks in 2.5s with 1s interval")
}
