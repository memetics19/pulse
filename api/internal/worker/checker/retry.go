package checker

import (
	"context"
	"time"
)

// Retry is how many more attempts a checker makes after a transient failure,
// and how long it waits before each one, before it reports the target down.
type Retry struct {
	Count    int
	Interval time.Duration
}

// DefaultRetry is one more attempt 500ms later. It is the HTTP checker's
// long-standing behaviour and the default for a new monitor.
var DefaultRetry = Retry{Count: 1, Interval: 500 * time.Millisecond}

// Retrier is a checker whose retry policy can be set per monitor. The
// scheduler uses it to apply each monitor's retries and retry interval.
type Retrier interface {
	Checker
	WithRetry(Retry) Checker
}

// run calls attempt, then repeats it up to r.Count more times while attempt
// reports a transient failure. It stops early when ctx ends and returns the
// last result.
func (r Retry) run(ctx context.Context, attempt func() (res Result, transient bool)) Result {
	res, transient := attempt()
	for i := 0; i < r.Count && transient; i++ {
		t := time.NewTimer(r.Interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return res
		case <-t.C:
		}
		res, transient = attempt()
	}
	return res
}
