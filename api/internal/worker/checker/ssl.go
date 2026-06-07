package checker

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

type sslChecker struct{}

func NewSSL() Checker { return &sslChecker{} }

func (c *sslChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second},
		Config:    &tls.Config{},
	}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", target)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: "not a TLS connection", CheckedAt: time.Now()}
	}

	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: "no peer certificates", CheckedAt: time.Now()}
	}

	leaf := certs[0]
	now := time.Now()
	if now.After(leaf.NotAfter) {
		return Result{
			Status:         "down",
			ResponseTimeMs: elapsed,
			ErrorMessage:   fmt.Sprintf("certificate expired on %s", leaf.NotAfter.Format(time.DateOnly)),
			CheckedAt:      now,
		}
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
	}
}
