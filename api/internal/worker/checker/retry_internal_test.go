package checker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryRun(t *testing.T) {
	tests := []struct {
		name      string
		retry     Retry
		transient func(call int) bool
		wantCalls int
	}{
		{name: "no retry", retry: Retry{}, transient: func(int) bool { return true }, wantCalls: 1},
		{name: "retries every transient failure", retry: Retry{Count: 3}, transient: func(int) bool { return true }, wantCalls: 4},
		{name: "stable failure is not retried", retry: Retry{Count: 3}, transient: func(int) bool { return false }, wantCalls: 1},
		{name: "stops once an attempt is final", retry: Retry{Count: 5}, transient: func(call int) bool { return call < 2 }, wantCalls: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			res := tt.retry.run(context.Background(), func() (Result, bool) {
				calls++
				return Result{Status: "down", StatusCode: calls}, tt.transient(calls)
			})
			assert.Equal(t, tt.wantCalls, calls)
			assert.Equal(t, tt.wantCalls, res.StatusCode, "the last attempt's result is returned")
		})
	}
}

func TestRetryRunStopsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	Retry{Count: 3, Interval: time.Hour}.run(ctx, func() (Result, bool) {
		calls++
		return Result{Status: "down"}, true
	})
	assert.Equal(t, 1, calls)
}

func TestRetryRunWaitsTheInterval(t *testing.T) {
	start := time.Now()
	Retry{Count: 2, Interval: 30 * time.Millisecond}.run(context.Background(), func() (Result, bool) {
		return Result{Status: "down"}, true
	})
	assert.GreaterOrEqual(t, time.Since(start), 60*time.Millisecond)
}
