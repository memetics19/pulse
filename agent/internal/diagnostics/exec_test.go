package diagnostics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecRunner_ReturnsCommandOutput(t *testing.T) {
	r := NewExecRunner(5 * time.Second)

	out, err := r.Run(context.Background(), "echo", "hello")

	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(out))
}

// A wedged host is exactly when diagnostics matter most, so a hung command
// must not stall the collector indefinitely.
func TestExecRunner_KillsCommandThatExceedsTimeout(t *testing.T) {
	r := NewExecRunner(50 * time.Millisecond)

	started := time.Now()
	_, err := r.Run(context.Background(), "sleep", "5")

	require.Error(t, err)
	assert.Less(t, time.Since(started), 2*time.Second, "command should be killed at the timeout")
}

func TestExecRunner_ReturnsErrorForMissingCommand(t *testing.T) {
	r := NewExecRunner(5 * time.Second)

	_, err := r.Run(context.Background(), "pulse-no-such-command-exists")

	require.Error(t, err)
}
