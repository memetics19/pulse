package checker_test

import (
	"context"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/stretchr/testify/assert"
)

func TestSSLChecker_ValidCert(t *testing.T) {
	c := checker.NewSSL()
	result := c.Check(context.Background(), "example.com:443", 10)
	assert.Equal(t, "up", result.Status)
	assert.Empty(t, result.ErrorMessage)
	assert.Greater(t, result.StatusCode, 0) // days until expiry stored here
}

func TestSSLChecker_InvalidHost(t *testing.T) {
	c := checker.NewSSL()
	result := c.Check(context.Background(), "127.0.0.1:19997", 1)
	assert.Equal(t, "down", result.Status)
	assert.NotEmpty(t, result.ErrorMessage)
}
