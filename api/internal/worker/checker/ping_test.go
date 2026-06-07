package checker_test

import (
	"context"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/stretchr/testify/assert"
)

func TestPingChecker_Localhost(t *testing.T) {
	c := checker.NewPing()
	result := c.Check(context.Background(), "127.0.0.1", 5)
	// Ping may require privileges — accept either up or down (not a panic)
	assert.Contains(t, []string{"up", "down"}, result.Status)
}
