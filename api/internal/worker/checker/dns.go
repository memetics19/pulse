package checker

import (
	"context"
	"net"
	"time"
)

type dnsChecker struct{}

func NewDNS() Checker { return &dnsChecker{} }

func (c *dnsChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	r := &net.Resolver{}
	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	start := time.Now()
	addrs, err := r.LookupHost(tctx, target)
	elapsed := time.Since(start).Milliseconds()

	if err != nil || len(addrs) == 0 {
		msg := "no records found"
		if err != nil {
			msg = err.Error()
		}
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: msg, CheckedAt: time.Now()}
	}
	return Result{Status: "up", ResponseTimeMs: elapsed, CheckedAt: time.Now()}
}
