package checker_test

import (
	"context"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/stretchr/testify/assert"
)

func TestDNSChecker_Up(t *testing.T) {
	c := checker.NewDNS()
	result := c.Check(context.Background(), "example.com", 5)
	assert.Equal(t, "up", result.Status)
}

func TestDNSChecker_Down(t *testing.T) {
	c := checker.NewDNS()
	result := c.Check(context.Background(), "this-domain-should-not-exist-pulse.invalid", 5)
	assert.Equal(t, "down", result.Status)
}
