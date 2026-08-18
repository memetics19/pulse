package diagnostics

import (
	"context"
	"os/exec"
	"time"
)

// ExecRunner runs diagnostic commands against the real host, bounding each one
// so a wedged command cannot stall collection. A hung host is precisely when
// diagnostics matter most, so the timeout is not optional.
type ExecRunner struct {
	timeout time.Duration
}

// NewExecRunner returns a Runner that kills any command exceeding timeout.
func NewExecRunner(timeout time.Duration) *ExecRunner {
	return &ExecRunner{timeout: timeout}
}

// Run executes name with args and returns its combined output. Output is
// returned even on failure, because a command's stderr is often the diagnosis.
func (r *ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
