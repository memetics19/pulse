package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// errLocked means another process holds the instance lock.
var errLocked = errors.New("held by another process")

// lockPath returns the file the instance lock is taken on.
func lockPath(dataDir string) string { return filepath.Join(dataDir, "pulse.lock") }

// lockDataDir takes an exclusive flock on pulse.lock in dataDir and returns
// the open file that holds it. The server holds it while it runs, and pulse
// restore takes it to make sure the server is stopped. The kernel drops the
// lock when the process exits, even after a crash, so a leftover pulse.lock
// never blocks a restore. It returns errLocked when another process holds it.
//
// The file is opened read-only and created readable by all, so a lock file
// left by a restore run as root does not lock the server's user out.
func lockDataDir(dataDir string) (*os.File, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath(dataDir), os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			err = errLocked
		}
		return nil, errors.Join(err, f.Close())
	}
	return f, nil
}
