package checker

import (
	"context"
	"net"
	"time"

	"github.com/memetics19/pulse/api/internal/netguard"
)

type dnsChecker struct{ allowPrivate bool }

// NewDNS creates a DNS-resolution checker. Unless allowPrivate is set, a target
// that resolves to a private/internal address is rejected so Pulse cannot be
// used as an oracle for internal name resolution (SSRF guard).
func NewDNS(allowPrivate bool) Checker { return &dnsChecker{allowPrivate: allowPrivate} }

func (c *dnsChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	r := &net.Resolver{}
	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	start := time.Now()
	addrs, err := r.LookupHost(tctx, target)
	elapsed := elapsedMs(start)

	if err != nil || len(addrs) == 0 {
		msg := "no records found"
		if err != nil {
			msg = err.Error()
		}
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: msg, CheckedAt: time.Now()}
	}
	if !c.allowPrivate {
		for _, a := range addrs {
			if ip := net.ParseIP(a); ip != nil && netguard.IsForbiddenIP(ip) {
				return Result{Status: "down", ResponseTimeMs: elapsed,
					ErrorMessage: "resolves to a private or internal address (set PULSE_ALLOW_PRIVATE_MONITORS=true to allow)",
					CheckedAt:    time.Now()}
			}
		}
	}
	return Result{Status: "up", ResponseTimeMs: elapsed, CheckedAt: time.Now()}
}
