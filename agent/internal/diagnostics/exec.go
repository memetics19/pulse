package diagnostics

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// maxErrorDetail bounds how much of a failing command's output is folded into
// the error, so a noisy command cannot bloat the bundle.
const maxErrorDetail = 512

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

// Run executes name with args and returns its combined output.
//
// On failure the command's own output is folded into the error. A bare
// "exit status 1" is useless in a tool whose whole job is explaining failures:
// "cannot connect to the docker daemon" is the actual diagnosis.
func (r *ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err == nil {
		return out, nil
	}

	detail := strings.TrimSpace(string(out))
	if detail == "" {
		return out, err
	}
	if len(detail) > maxErrorDetail {
		detail = detail[:maxErrorDetail] + "…"
	}
	return out, fmt.Errorf("%w: %s", err, detail)
}
