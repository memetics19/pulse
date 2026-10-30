package checker_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSLChecker_ValidCert(t *testing.T) {
	c := checker.NewSSL(false)
	result := c.Check(context.Background(), "example.com:443", 10)
	assert.Equal(t, "up", result.Status)
	assert.Empty(t, result.ErrorMessage)
	require.NotNil(t, result.CertDaysLeft)
	assert.Greater(t, *result.CertDaysLeft, 0)
	assert.Equal(t, *result.CertDaysLeft, result.StatusCode) // kept for one release
}

func TestSSLChecker_InvalidHost(t *testing.T) {
	c := checker.NewSSL(true)
	result := c.Check(context.Background(), "127.0.0.1:19997", 1)
	assert.Equal(t, "down", result.Status)
	assert.NotEmpty(t, result.ErrorMessage)
	assert.Nil(t, result.CertDaysLeft)
}

func TestSSLChecker_BlocksPrivateTargetAtDialTime(t *testing.T) {
	result := checker.NewSSL(false).Check(context.Background(), "127.0.0.1:443", 1)

	assert.Equal(t, "down", result.Status)
	assert.Contains(t, result.ErrorMessage, "private or internal")
}

// serveCert serves a self-signed certificate for 127.0.0.1 that expires at
// notAfter. It returns the listener address and a pool that trusts the
// certificate.
func serveCert(t *testing.T, notAfter time.Time) (string, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "pulse-test"},
		NotBefore:             time.Now().Add(-48 * time.Hour),
		NotAfter:              notAfter,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(cert)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.(*tls.Conn).Handshake()
			_ = conn.Close()
		}
	}()
	return ln.Addr().String(), pool
}

// daysLeft is a certificate end time that the checker reads as exactly days
// days left. The extra 12 hours keep the value away from a day boundary.
func daysLeft(days int) time.Time {
	return time.Now().Add(time.Duration(days)*24*time.Hour + 12*time.Hour)
}

func TestSSLChecker_Thresholds(t *testing.T) {
	cases := []struct {
		name           string
		warn, critical int
		days           int
		want           string
	}{
		{"default well before warn", 14, 7, 30, "up"},
		{"default at warn", 14, 7, 14, "degraded"},
		{"default just above critical", 14, 7, 8, "degraded"},
		{"default at critical", 14, 7, 7, "down"},
		{"default last day", 14, 7, 0, "down"},
		{"custom above warn", 30, 0, 31, "up"},
		{"custom at warn", 30, 0, 30, "degraded"},
		{"custom one day left", 30, 0, 1, "degraded"},
		{"custom critical zero on last day", 30, 0, 0, "down"},
		{"custom critical", 21, 10, 10, "down"},
		{"warn not above critical uses defaults", 5, 10, 10, "degraded"},
		{"negative critical uses defaults", 14, -1, 3, "down"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr, pool := serveCert(t, daysLeft(tc.days))
			res := checker.NewSSLWithRoots(tc.warn, tc.critical, pool).Check(context.Background(), addr, 3)

			assert.Equal(t, tc.want, res.Status)
			assert.Empty(t, res.ErrorMessage)
			require.NotNil(t, res.CertDaysLeft)
			assert.Equal(t, tc.days, *res.CertDaysLeft)
			assert.Equal(t, tc.days, res.StatusCode, "status_code keeps the days left for one release")
		})
	}
}

func TestSSLChecker_ExpiredCertHasNoDaysLeft(t *testing.T) {
	addr, pool := serveCert(t, time.Now().Add(-time.Hour))
	res := checker.NewSSLWithRoots(14, 7, pool).Check(context.Background(), addr, 3)

	assert.Equal(t, "down", res.Status)
	assert.Contains(t, res.ErrorMessage, "expired")
	assert.Nil(t, res.CertDaysLeft)
}
