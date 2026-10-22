package checker_test

import (
	"context"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/stretchr/testify/assert"
)

// With the guard on (allowPrivate=false), every non-HTTP checker must refuse an
// internal target instead of turning up/down into an internal-port-scan oracle.
func TestCheckers_RejectPrivateTargets(t *testing.T) {
	ctx := context.Background()

	tcpRes := checker.NewTCP(false).Check(ctx, "127.0.0.1:6379", 2)
	assert.Equal(t, "down", tcpRes.Status)
	assert.Contains(t, strings.ToLower(tcpRes.ErrorMessage), "netguard")

	sslRes := checker.NewSSL(false).Check(ctx, "10.0.0.5:443", 2)
	assert.Equal(t, "down", sslRes.Status)
	assert.Contains(t, strings.ToLower(sslRes.ErrorMessage), "netguard")

	pingRes := checker.NewPing(false).Check(ctx, "169.254.169.254", 2)
	assert.Equal(t, "down", pingRes.Status)
	assert.Contains(t, strings.ToLower(pingRes.ErrorMessage), "private or internal")
}
