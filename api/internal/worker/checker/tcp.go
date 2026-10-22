package checker

import (
	"context"
	"net"
	"time"

	"github.com/memetics19/pulse/api/internal/netguard"
)

type tcpChecker struct{ allowPrivate bool }

// NewTCP creates a TCP-connect checker. allowPrivate permits targets on
// private/internal networks; otherwise the dial is rejected by netguard (the
// same SSRF guard the HTTP checker uses), enforced on the resolved IP.
func NewTCP(allowPrivate bool) Checker { return &tcpChecker{allowPrivate: allowPrivate} }

func (c *tcpChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	start := time.Now()
	d := net.Dialer{
		Timeout: time.Duration(timeoutSec) * time.Second,
		Control: netguard.DialControl(c.allowPrivate),
	}
	conn, err := d.DialContext(ctx, "tcp", target)
	elapsed := elapsedMs(start)
	if err != nil {
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}
	conn.Close()
	return Result{Status: "up", ResponseTimeMs: elapsed, CheckedAt: time.Now()}
}
