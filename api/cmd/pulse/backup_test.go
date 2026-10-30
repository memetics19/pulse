package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/backup"
	"github.com/memetics19/pulse/api/internal/bootstrap"
	"github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installed sets up a configured install in a temporary data directory, with
// one admin, and returns the data directory, the database file and the open
// connection the server would hold.
func installed(t *testing.T) (dataDir, path string, conn *sql.DB) {
	t.Helper()
	dataDir = t.TempDir()
	path = filepath.Join(dataDir, "pulse.db")
	t.Setenv("PULSE_DATA_DIR", dataDir)
	t.Setenv("PULSE_DB", path)
	conn, err := db.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	addUser(t, conn, "admin")
	require.NoError(t, bootstrap.Save(dataDir, bootstrap.Config{Configured: true, SQLitePath: path}))
	return dataDir, path, conn
}

func addUser(t *testing.T, conn *sql.DB, name string) {
	t.Helper()
	hash, err := auth.HashPassword("password-123")
	require.NoError(t, err)
	_, err = generated.New(conn).CreateUser(context.Background(), generated.CreateUserParams{Username: name, PasswordHash: hash})
	require.NoError(t, err)
}

func users(t *testing.T, path string) int64 {
	t.Helper()
	conn, err := db.Open(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	n, err := generated.New(conn).CountUsers(context.Background())
	require.NoError(t, err)
	return n
}

func TestBackupCommandWhileServerRuns(t *testing.T) {
	dataDir, _, conn := installed(t)
	server, err := lockDataDir(dataDir) // the server holds the instance lock
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	out := filepath.Join(t.TempDir(), "snap.db")
	var buf strings.Builder
	require.NoError(t, backupDatabase([]string{"--out", out}, &buf))
	assert.Contains(t, buf.String(), "backup written: "+out)
	assert.Contains(t, buf.String(), "pulse.json copied to "+out+".json")
	_, err = backup.Verify(context.Background(), out)
	require.NoError(t, err)

	// The server keeps writing after the backup.
	addUser(t, conn, "later")

	// Without --out the snapshot goes beside the database.
	buf.Reset()
	require.NoError(t, backupDatabase(nil, &buf))
	matches, err := filepath.Glob(filepath.Join(dataDir, "pulse-backup-*.db"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Contains(t, buf.String(), matches[0])

	// An existing file is never replaced.
	err = backupDatabase([]string{"--out", out}, &buf)
	require.ErrorIs(t, err, backup.ErrExists)
}

func TestBackupCommandErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PULSE_DATA_DIR", dir)
	t.Setenv("PULSE_DB", filepath.Join(dir, "pulse.db"))
	var buf strings.Builder

	require.ErrorContains(t, backupDatabase(nil, &buf), "no database to back up")
	require.ErrorContains(t, backupDatabase([]string{"extra"}, &buf), "usage: pulse backup")
	require.Error(t, backupDatabase([]string{"--nope"}, &buf))
	require.NoError(t, backupDatabase([]string{"-h"}, &buf))

	require.NoError(t, os.WriteFile(bootstrap.Path(dir), []byte("{"), 0o600))
	require.ErrorContains(t, backupDatabase(nil, &buf), "bootstrap")
}

func TestRestoreCommand(t *testing.T) {
	dataDir, path, conn := installed(t)
	snap := filepath.Join(t.TempDir(), "snap.db")
	var buf strings.Builder
	require.NoError(t, backupDatabase([]string{"--out", snap}, &buf))
	addUser(t, conn, "after-backup")

	// The running server holds the lock, so restore refuses.
	server, err := lockDataDir(dataDir)
	require.NoError(t, err)
	err = restoreDatabase([]string{snap}, &buf)
	require.ErrorContains(t, err, "pulse is running")
	assert.Contains(t, err.Error(), lockPath(dataDir))

	// The server stops.
	require.NoError(t, server.Close())
	require.NoError(t, conn.Close())

	buf.Reset()
	require.NoError(t, restoreDatabase([]string{snap}, &buf))
	assert.Contains(t, buf.String(), "restored "+snap+" into "+path)
	assert.Contains(t, buf.String(), "the old database is kept as "+path+".pre-restore-")
	assert.NotContains(t, buf.String(), "migrates it", "the snapshot is at the latest version")
	assert.EqualValues(t, 1, users(t, path), "the account added after the backup is gone")
}

func TestRestoreCommandMentionsPendingMigrations(t *testing.T) {
	_, path, conn := installed(t)
	snap := filepath.Join(t.TempDir(), "snap.db")
	var buf strings.Builder
	require.NoError(t, backupDatabase([]string{"--out", snap}, &buf))
	require.NoError(t, conn.Close())

	old, err := sql.Open("sqlite", snap)
	require.NoError(t, err)
	_, err = old.Exec(`UPDATE schema_migrations SET version = version - 1`)
	require.NoError(t, err)
	require.NoError(t, old.Close())

	buf.Reset()
	require.NoError(t, restoreDatabase([]string{snap}, &buf))
	assert.Contains(t, buf.String(), "pulse migrates it to schema version")
	assert.FileExists(t, path)
}

func TestRestoreCommandErrors(t *testing.T) {
	dataDir, _, _ := installed(t)
	var buf strings.Builder

	require.ErrorContains(t, restoreDatabase(nil, &buf), "usage: pulse restore")
	require.ErrorContains(t, restoreDatabase([]string{"a", "b"}, &buf), "usage: pulse restore")
	require.Error(t, restoreDatabase([]string{"--nope"}, &buf))
	require.NoError(t, restoreDatabase([]string{"-h"}, &buf))

	err := restoreDatabase([]string{filepath.Join(t.TempDir(), "missing.db")}, &buf)
	require.ErrorContains(t, err, "restore:")

	require.NoError(t, os.WriteFile(bootstrap.Path(dataDir), []byte("{"), 0o600))
	require.ErrorContains(t, restoreDatabase([]string{"x.db"}, &buf), "bootstrap")

	t.Setenv("PULSE_DATA_DIR", filepath.Join(bootstrap.Path(dataDir), "sub"))
	require.ErrorContains(t, restoreDatabase([]string{"x.db"}, &buf), "lock ")
}

func TestRestoreCommandRefusesOpenDatabaseWithoutLock(t *testing.T) {
	_, _, _ = installed(t)
	snap := filepath.Join(t.TempDir(), "snap.db")
	var buf strings.Builder
	require.NoError(t, backupDatabase([]string{"--out", snap}, &buf))

	// The connection from installed is still open, as a server from a
	// release without the instance lock would hold it.
	err := restoreDatabase([]string{snap}, &buf)
	require.ErrorIs(t, err, backup.ErrInUse)
}

func TestLockDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	first, err := lockDataDir(dir)
	require.NoError(t, err)
	assert.FileExists(t, lockPath(dir))

	_, err = lockDataDir(dir)
	require.ErrorIs(t, err, errLocked)

	// The lock goes with the process (here, the file), not the file on disk.
	require.NoError(t, first.Close())
	again, err := lockDataDir(dir)
	require.NoError(t, err)
	require.NoError(t, again.Close())

	notADir := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(notADir, nil, 0o600))
	_, err = lockDataDir(filepath.Join(notADir, "data"))
	require.Error(t, err)

	loop := t.TempDir()
	require.NoError(t, os.Symlink("pulse.lock", lockPath(loop)))
	_, err = lockDataDir(loop)
	require.Error(t, err, "pulse.lock is a symlink to itself")
}
