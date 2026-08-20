//go:build unix

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Killing only the direct child leaves descendants running on a host that is
// already struggling, and repeated diagnoses would accumulate them.
//
// Liveness is checked by observing that the descendant stops doing work rather
// than by probing its PID: a killed process lingers as a zombie until reaped,
// so kill(pid, 0) still succeeds and the timing of reaping differs by platform.
func TestExecRunner_KillsDescendantsNotJustTheDirectChild(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ticks")
	r := NewExecRunner(150 * time.Millisecond)

	_, err := r.Run(context.Background(), "sh", "-c",
		"while :; do echo tick >> "+marker+"; sleep 0.02; done & wait")
	require.Error(t, err, "the command must still hit its timeout")

	sizeAfterRun := fileSize(t, marker)
	require.NotZero(t, sizeAfterRun, "the descendant should have been writing")

	time.Sleep(300 * time.Millisecond)

	assert.Equal(t, sizeAfterRun, fileSize(t, marker),
		"the descendant kept working after the command was cancelled")
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	return info.Size()
}
