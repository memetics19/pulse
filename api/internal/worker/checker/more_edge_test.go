package checker_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/stretchr/testify/assert"
)

func TestSSLAgainstNonTLS(t *testing.T) {
	srv := httptest.NewServer(nil) // plain HTTP, no TLS
	defer srv.Close()
	addr := srv.Listener.Addr().String()
	res := checker.NewSSL(true).Check(context.Background(), addr, 3)
	assert.Equal(t, "down", res.Status)
	assert.NotEmpty(t, res.ErrorMessage)
}

func TestPingUnresolvable(t *testing.T) {
	res := checker.NewPing(true).Check(context.Background(),
		"this-host-should-not-resolve-pulse.invalid", 2)
	assert.Equal(t, "down", res.Status)
}

func TestDNSDownOnNXDOMAIN(t *testing.T) {
	res := checker.NewDNS(true).Check(context.Background(),
		"this-domain-should-not-exist-pulse.invalid", 3)
	assert.Equal(t, "down", res.Status)
}
