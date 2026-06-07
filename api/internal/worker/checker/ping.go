package checker

import (
	"context"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

type pingChecker struct{}

func NewPing() Checker { return &pingChecker{} }

func (c *pingChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	pinger, err := probing.NewPinger(target)
	if err != nil {
		return Result{Status: "down", ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}
	pinger.Count = 3
	pinger.Timeout = time.Duration(timeoutSec) * time.Second
	pinger.SetPrivileged(false)

	start := time.Now()
	if err := pinger.RunWithContext(ctx); err != nil {
		return Result{Status: "down", ResponseTimeMs: time.Since(start).Milliseconds(), ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}
	stats := pinger.Statistics()
	if stats.PacketsRecv == 0 {
		return Result{Status: "down", ResponseTimeMs: time.Since(start).Milliseconds(), ErrorMessage: "all packets lost", CheckedAt: time.Now()}
	}
	return Result{Status: "up", ResponseTimeMs: stats.AvgRtt.Milliseconds(), CheckedAt: time.Now()}
}
