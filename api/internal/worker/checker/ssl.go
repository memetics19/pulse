package checker

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/memetics19/pulse/api/internal/netguard"
)

type sslChecker struct {
	allowPrivate bool
	retry        Retry
}

// NewSSL creates a TLS certificate-expiry checker. allowPrivate permits targets
// on private/internal networks; otherwise the dial is rejected by netguard on
// the resolved IP (SSRF guard). It does not retry until WithRetry sets a
// policy.
func NewSSL(allowPrivate bool) Retrier { return &sslChecker{allowPrivate: allowPrivate} }

// WithRetry returns a copy of the checker that uses r.
func (c *sslChecker) WithRetry(r Retry) Checker {
	cp := *c
	cp.retry = r
	return &cp
}

// Check dials target and reads its certificate. Only a failed dial or
// handshake is retried. An expired or expiring certificate is a stable answer.
func (c *sslChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	return c.retry.run(ctx, func() (Result, bool) {
		return c.attempt(ctx, target, timeoutSec)
	})
}

// attempt runs one check. transient is true when the dial or handshake
// failed, so another attempt might succeed.
func (c *sslChecker) attempt(ctx context.Context, target string, timeoutSec int64) (res Result, transient bool) {
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{
			Timeout: time.Duration(timeoutSec) * time.Second,
			Control: netguard.DialControl(c.allowPrivate),
		},
		Config: &tls.Config{},
	}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", target)
	elapsed := elapsedMs(start)
	if err != nil {
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: err.Error(), CheckedAt: time.Now()}, true
	}
	// The handshake already happened; closing only releases the socket.
	defer func() { _ = conn.Close() }()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: "not a TLS connection", CheckedAt: time.Now()}, false
	}

	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: "no peer certificates", CheckedAt: time.Now()}, false
	}

	leaf := certs[0]
	now := time.Now()
	if now.After(leaf.NotAfter) {
		return Result{
			Status:         "down",
			ResponseTimeMs: elapsed,
			ErrorMessage:   fmt.Sprintf("certificate expired on %s", leaf.NotAfter.Format(time.DateOnly)),
			CheckedAt:      now,
		}, false
	}

	daysUntilExpiry := int(leaf.NotAfter.Sub(now).Hours() / 24)
	status := "up"
	if daysUntilExpiry <= 7 {
		status = "down"
	} else if daysUntilExpiry <= 14 {
		status = "degraded"
	}

	return Result{
		Status:         status,
		ResponseTimeMs: elapsed,
		StatusCode:     daysUntilExpiry,
		CheckedAt:      now,
	}, false
}
