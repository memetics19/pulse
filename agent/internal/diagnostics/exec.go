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

// waitDelay bounds how long Wait may block after the context is cancelled,
// covering descendants that outlive the direct child and hold its pipes.
const waitDelay = 2 * time.Second

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
	parent := ctx
	ctx, cancel := context.WithTimeout(parent, r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	configureProcessGroup(cmd)
	// CommandContext kills only the direct child. A descendant that inherited
	// the pipe would otherwise keep CombinedOutput blocked well past the
	// deadline, defeating the timeout on exactly the wedged hosts this package
	// exists for. WaitDelay bounds that wait and forces the pipes closed.
	cmd.WaitDelay = waitDelay

	out, err := cmd.CombinedOutput()
	if err == nil {
		return out, nil
	}

	// A cancelled caller is not this runner's timeout; reporting Ctrl-C as a
	// timeout sends an operator looking for a slow command that was never slow.
	if parent.Err() != nil {
		return out, parent.Err()
	}

	// A killed process reports "signal: killed", which does not tell an
	// operator the command hit its time limit — the likeliest failure on a
	// wedged host.
	if ctx.Err() != nil {
		// A command that explains itself on stderr and then hangs is the most
		// useful kind of failure; classifying it must not discard what it said.
		if detail := boundedDetail(out); detail != "" {
			return out, fmt.Errorf("timed out after %s: %s", r.timeout, detail)
		}
		return out, fmt.Errorf("timed out after %s", r.timeout)
	}

	detail := boundedDetail(out)
	if detail == "" {
		return out, err
	}
	return out, fmt.Errorf("%w: %s", err, detail)
}

// boundedDetail trims a command's output and caps it, so a noisy command
// cannot bloat the bundle through its error message.
func boundedDetail(out []byte) string {
	detail := strings.TrimSpace(string(out))
	if len(detail) > maxErrorDetail {
		return detail[:maxErrorDetail] + "…"
	}
	return detail
}
