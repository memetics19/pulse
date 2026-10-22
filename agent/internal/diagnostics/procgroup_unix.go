//go:build unix

package diagnostics

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup puts the command in its own process group and kills the
// whole group when the context is cancelled.
//
// exec.CommandContext signals only the direct child, so a descendant survives
// its parent and keeps running on a host that is already degraded. Repeated
// diagnoses would accumulate orphans. Negating the pid targets the group.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
