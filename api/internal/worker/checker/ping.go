package checker

import (
	"context"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	"github.com/memetics19/pulse/api/internal/netguard"
)

type pingChecker struct{ allowPrivate bool }

// NewPing creates an ICMP/UDP ping checker. allowPrivate permits targets on
// private/internal networks. pro-bing has no dial hook, so unless allowPrivate
// is set the target is resolved and rejected here if it points at an internal
// address (SSRF guard).
func NewPing(allowPrivate bool) Checker { return &pingChecker{allowPrivate: allowPrivate} }

func (c *pingChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	if err := netguard.ValidateTarget(target, c.allowPrivate); err != nil {
		return Result{Status: "down", ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}
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
