package checker

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"time"

	"github.com/memetics19/pulse/api/internal/netguard"
)

// Default certificate expiry thresholds, in days left. A monitor that does not
// set its own thresholds uses these.
const (
	DefaultTLSWarnDays     = 14
	DefaultTLSCriticalDays = 7
)

type sslChecker struct {
	allowPrivate bool
	warnDays     int
	criticalDays int
	rootCAs      *x509.CertPool // nil verifies against the system roots
}

// NewSSL creates a TLS certificate-expiry checker with the default thresholds.
// allowPrivate permits targets on private/internal networks; otherwise the dial
// is rejected by netguard on the resolved IP (SSRF guard).
func NewSSL(allowPrivate bool) Checker {
	return NewSSLThresholds(DefaultTLSWarnDays, DefaultTLSCriticalDays, allowPrivate)
}

// NewSSLThresholds creates a TLS certificate-expiry checker that reports
// degraded with warnDays or fewer days left and down with criticalDays or
// fewer. Thresholds that break warnDays > criticalDays >= 0 fall back to the
// defaults, so a row written outside the API cannot mark every check down.
func NewSSLThresholds(warnDays, criticalDays int, allowPrivate bool) Checker {
	if criticalDays < 0 || warnDays <= criticalDays {
		warnDays, criticalDays = DefaultTLSWarnDays, DefaultTLSCriticalDays
	}
	return &sslChecker{allowPrivate: allowPrivate, warnDays: warnDays, criticalDays: criticalDays}
}

func (c *sslChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{
			Timeout: time.Duration(timeoutSec) * time.Second,
			Control: netguard.DialControl(c.allowPrivate),
		},
		Config: &tls.Config{RootCAs: c.rootCAs},
	}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", target)
	elapsed := elapsedMs(start)
	if err != nil {
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}
	// The handshake already happened; closing only releases the socket.
	defer func() { _ = conn.Close() }()

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
	if daysUntilExpiry <= c.criticalDays {
		status = "down"
	} else if daysUntilExpiry <= c.warnDays {
		status = "degraded"
	}

	return Result{
		Status:         status,
		ResponseTimeMs: elapsed,
		// StatusCode repeats CertDaysLeft for one release, so API clients
		// that read status_code have time to move to cert_days_left.
		StatusCode:   daysUntilExpiry,
		CertDaysLeft: &daysUntilExpiry,
		CheckedAt:    now,
	}
}
