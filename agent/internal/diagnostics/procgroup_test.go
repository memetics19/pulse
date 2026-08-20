//go:build unix

package diagnostics

import (
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Killing only the direct child leaves descendants running on a host that is
// already struggling. Repeated diagnoses would accumulate them.
func TestExecRunner_KillsDescendantsNotJustTheDirectChild(t *testing.T) {
	r := NewExecRunner(150 * time.Millisecond)

	out, err := r.Run(context.Background(), "sh", "-c", "sleep 30 & echo $!; wait")
	require.Error(t, err, "the command must still hit its timeout")

	fields := strings.Fields(string(out))
	require.NotEmpty(t, fields, "the descendant PID should have been printed")
	pid, convErr := strconv.Atoi(fields[0])
	require.NoError(t, convErr)

	// Signal 0 probes liveness without delivering anything.
	proc, findErr := os.FindProcess(pid)
	require.NoError(t, findErr)
	assert.Error(t, proc.Signal(syscall.Signal(0)),
		"descendant %d should be dead once the group was terminated", pid)
}
