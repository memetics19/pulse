package checker

import (
	"context"
	"time"

	"github.com/memetics19/pulse/api/internal/netguard"
	probing "github.com/prometheus-community/pro-bing"
)

type pingChecker struct{ allowPrivate bool }

func NewPing(allowPrivate bool) Checker { return &pingChecker{allowPrivate: allowPrivate} }

func (c *pingChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	ip, err := netguard.ResolveAllowedIP(ctx, target, c.allowPrivate)
	if err != nil {
		return Result{Status: "down", ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}
	pinger, err := probing.NewPinger(ip.String())
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
