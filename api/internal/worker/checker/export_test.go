package checker

import "crypto/x509"

// NewSSLWithRoots is NewSSLThresholds for tests. It trusts roots instead of
// the system pool and allows private targets, so a test can serve its own
// certificate on localhost.
func NewSSLWithRoots(warnDays, criticalDays int, roots *x509.CertPool) Checker {
	c := NewSSLThresholds(warnDays, criticalDays, true).(*sslChecker)
	c.rootCAs = roots
	return c
}
