package checker

import (
	"context"
	"net"
	"time"
)

type tcpChecker struct{}

func NewTCP() Checker { return &tcpChecker{} }

func (c *tcpChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	start := time.Now()
	d := net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second}
	conn, err := d.DialContext(ctx, "tcp", target)
	elapsed := elapsedMs(start)
	if err != nil {
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}
	conn.Close()
	return Result{Status: "up", ResponseTimeMs: elapsed, CheckedAt: time.Now()}
}
