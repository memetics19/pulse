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

// A bare "exit status 1" is useless in a tool whose entire job is explaining
// failures. The command's own stderr is the diagnosis and must survive.
func TestExecRunner_ErrorIncludesCommandOutput(t *testing.T) {
	r := NewExecRunner(5 * time.Second)

	_, err := r.Run(context.Background(), "sh", "-c",
		"echo 'cannot connect to the docker daemon' >&2; exit 1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot connect to the docker daemon")
}

func TestExecRunner_ErrorStaysBoundedForNoisyCommands(t *testing.T) {
	r := NewExecRunner(5 * time.Second)

	_, err := r.Run(context.Background(), "sh", "-c",
		"head -c 20000 /dev/zero | tr '\\0' 'x' >&2; exit 1")

	require.Error(t, err)
	assert.Less(t, len(err.Error()), 2000, "a noisy command must not bloat the bundle")
}

// "signal: killed" does not tell an operator the command hit its time limit,
// which is the likeliest failure on the wedged hosts worth diagnosing.
func TestExecRunner_TimeoutErrorSaysItTimedOut(t *testing.T) {
	r := NewExecRunner(50 * time.Millisecond)

	_, err := r.Run(context.Background(), "sleep", "5")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

// A cancelled caller is not this runner's timeout. Reporting Ctrl-C as
// "timed out after 10s" sends an operator looking for a slow command that
// was never slow.
func TestExecRunner_ReportsCallerCancellationDistinctly(t *testing.T) {
	r := NewExecRunner(10 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Run(ctx, "sleep", "5")

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "timed out", "no 10s timer elapsed")
	assert.ErrorIs(t, err, context.Canceled)
}

// exec.CommandContext kills only the direct child. A descendant that inherits
// the pipe keeps CombinedOutput blocked past the deadline, which breaks the
// no-hang guarantee on exactly the wedged hosts this package targets.
func TestExecRunner_TimeoutIsNotDefeatedByDescendantProcesses(t *testing.T) {
	r := NewExecRunner(100 * time.Millisecond)

	started := time.Now()
	_, err := r.Run(context.Background(), "sh", "-c", "sleep 5 & echo started; wait")

	require.Error(t, err)
	assert.Less(t, time.Since(started), 3*time.Second,
		"a descendant holding the pipe must not outlast the timeout")
}

// A command that explains itself on stderr and then hangs is the most useful
// kind of failure. Classifying it as a timeout must not discard what it said.
func TestExecRunner_TimeoutKeepsOutputAlreadyWritten(t *testing.T) {
	r := NewExecRunner(150 * time.Millisecond)

	_, err := r.Run(context.Background(), "sh", "-c",
		"echo 'cannot reach the docker daemon' >&2; sleep 5")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Contains(t, err.Error(), "cannot reach the docker daemon")
}
