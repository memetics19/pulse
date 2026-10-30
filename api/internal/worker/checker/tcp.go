package checker

import (
	"context"
	"net"
	"time"

	"github.com/memetics19/pulse/api/internal/netguard"
)

type tcpChecker struct {
	allowPrivate bool
	retry        Retry
}

// NewTCP creates a TCP-connect checker. allowPrivate permits targets on
// private/internal networks; otherwise the dial is rejected by netguard (the
// same SSRF guard the HTTP checker uses), enforced on the resolved IP. It does
// not retry until WithRetry sets a policy.
func NewTCP(allowPrivate bool) Retrier { return &tcpChecker{allowPrivate: allowPrivate} }

// WithRetry returns a copy of the checker that uses r.
func (c *tcpChecker) WithRetry(r Retry) Checker {
	cp := *c
	cp.retry = r
	return &cp
}

// Check connects to target. Every failed connect counts as transient and is
// retried under the checker's policy.
func (c *tcpChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	return c.retry.run(ctx, func() (Result, bool) {
		res := c.attempt(ctx, target, timeoutSec)
		return res, res.Status != "up"
	})
}

func (c *tcpChecker) attempt(ctx context.Context, target string, timeoutSec int64) Result {
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
	// The connect itself is the check. Closing only releases the socket and
	// cannot change the result.
	_ = conn.Close()
	return Result{Status: "up", ResponseTimeMs: elapsed, CheckedAt: time.Now()}
}
