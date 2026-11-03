package backup_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/backup"
	"github.com/memetics19/pulse/api/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixture is a stopped install: target holds n+extra rows, and snap is a
// snapshot taken when it held n.
func fixture(t *testing.T, n, extra int) (target, snap string) {
	t.Helper()
	target = filepath.Join(t.TempDir(), "pulse.db")
	conn := newDB(t, target, n)
	snap = filepath.Join(t.TempDir(), "snap.db")
	snapshot(t, target, snap, "")
	insert(t, conn, extra)
	require.NoError(t, conn.Close())
	return target, snap
}

var now = time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)

const kept = "pulse.db.pre-restore-20261030T100000Z"

func TestRestoreRoundTrip(t *testing.T) {
	target, snap := fixture(t, 10, 5)
	before, err := os.Stat(target)
	require.NoError(t, err)

	res, err := backup.Restore(ctx, snap, target, now)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(filepath.Dir(target), kept), res.Kept)
	assert.Equal(t, res.Latest, res.Version)
	assert.Equal(t, 10, rows(t, target), "the snapshot is back")
	assert.Equal(t, 15, rows(t, res.Kept), "the old database is kept whole")

	after, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, before.Mode().Perm(), after.Mode().Perm(), "the restored file keeps the old mode")

	// The swap leaves no -wal, -shm or staged file, and the snapshot is
	// untouched.
	assert.ElementsMatch(t, []string{"pulse.db", kept}, names(t, filepath.Dir(target)))
	_, err = backup.Verify(ctx, snap)
	require.NoError(t, err)

	// The server opens the restored file as usual.
	conn, err := db.Open(target)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	// A second restore in the same second does not replace the kept file.
	_, err = backup.Restore(ctx, snap, target, now)
	require.ErrorIs(t, err, backup.ErrExists)
	assert.Equal(t, 15, rows(t, res.Kept))
}

func TestRestoreIntoEmptyDirectory(t *testing.T) {
	_, snap := fixture(t, 3, 0)
	target := filepath.Join(t.TempDir(), "new", "pulse.db")
	res, err := backup.Restore(ctx, snap, target, now)
	require.NoError(t, err)
	assert.Empty(t, res.Kept)
	assert.Equal(t, 3, rows(t, target))
}

func TestRestoreRefusesOpenDatabase(t *testing.T) {
	target, snap := fixture(t, 10, 5)
	conn, err := db.Open(target) // the server, still running
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	_, err = backup.Restore(ctx, snap, target, now)
	require.ErrorIs(t, err, backup.ErrInUse)
	require.NoError(t, conn.Close())
	assert.Equal(t, 15, rows(t, target), "the database is unchanged")
	assert.ElementsMatch(t, []string{"pulse.db"}, names(t, filepath.Dir(target)), "no staged file is left")
}

// setVersion moves the recorded migration of the database at path by delta.
func setVersion(t *testing.T, path string, delta int) {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = conn.Exec(`UPDATE schema_migrations SET version = version + ?`, delta)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func TestRestoreRefusesNewerSchema(t *testing.T) {
	target, snap := fixture(t, 10, 5)
	setVersion(t, snap, 1)

	_, err := backup.Restore(ctx, snap, target, now)
	require.ErrorIs(t, err, backup.ErrTooNew)
	assert.Equal(t, 15, rows(t, target), "the database is unchanged")
	assert.ElementsMatch(t, []string{"pulse.db"}, names(t, filepath.Dir(target)))
}

func TestRestoreAcceptsOlderSchema(t *testing.T) {
	target, snap := fixture(t, 10, 5)
	setVersion(t, snap, -1)

	res, err := backup.Restore(ctx, snap, target, now)
	require.NoError(t, err)
	assert.Equal(t, res.Latest-1, res.Version)
}

func TestRestoreRefusesCorruptSnapshot(t *testing.T) {
	target := filepath.Join(t.TempDir(), "pulse.db")
	conn := newDB(t, target, 500)
	snap := filepath.Join(t.TempDir(), "snap.db")
	snapshot(t, target, snap, "")
	require.NoError(t, conn.Close())
	corrupt(t, snap)

	_, err := backup.Restore(ctx, snap, target, now)
	require.ErrorContains(t, err, "integrity check")
	assert.Equal(t, 500, rows(t, target), "the database is unchanged")
	assert.ElementsMatch(t, []string{"pulse.db"}, names(t, filepath.Dir(target)))
}

func TestRestoreRefusesLiveOrMissingSource(t *testing.T) {
	target, snap := fixture(t, 1, 0)
	require.NoError(t, os.WriteFile(snap+"-wal", []byte("frames"), 0o600))
	_, err := backup.Restore(ctx, snap, target, now)
	require.ErrorContains(t, err, "write-ahead log")

	_, err = backup.Restore(ctx, filepath.Join(t.TempDir(), "missing.db"), target, now)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRestoreKeepsUnreadableDatabaseWithItsLog(t *testing.T) {
	_, snap := fixture(t, 4, 0)
	dir := t.TempDir()
	target := filepath.Join(dir, "pulse.db")
	require.NoError(t, os.WriteFile(target, []byte(strings.Repeat("garbage ", 1000)), 0o600))
	require.NoError(t, os.WriteFile(target+"-wal", []byte("stale log"), 0o600))
	require.NoError(t, os.WriteFile(target+"-shm", []byte("stale shm"), 0o600))

	res, err := backup.Restore(ctx, snap, target, now)
	require.NoError(t, err)
	assert.Equal(t, 4, rows(t, target))
	assert.ElementsMatch(t, []string{"pulse.db", kept, kept + "-wal", kept + "-shm"}, names(t, dir),
		"the stale log moves with the old file and never meets the restored one")
	b, err := os.ReadFile(res.Kept + "-wal")
	require.NoError(t, err)
	assert.Equal(t, "stale log", string(b))
}
