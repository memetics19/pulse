//go:build !unix

package diagnostics

import "os/exec"

// configureProcessGroup is a no-op off Unix, where CommandContext's default
// termination is all the platform offers here.
func configureProcessGroup(_ *exec.Cmd) {}
