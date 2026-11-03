package backup_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/backup"
	"github.com/memetics19/pulse/api/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

// newDB creates a migrated database at path with a table of n rows, and
// returns the open connection, as the server would hold it.
func newDB(t *testing.T, path string, n int) *sql.DB {
	t.Helper()
	conn, err := db.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`)
	require.NoError(t, err)
	insert(t, conn, n)
	return conn
}

func insert(t *testing.T, conn *sql.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		_, err := conn.Exec(`INSERT INTO t (v) VALUES (?)`, strings.Repeat("x", 200))
		require.NoError(t, err)
	}
}

// rows counts the rows of t in the database file at path.
func rows(t *testing.T, path string) int {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	var n int
	require.NoError(t, conn.QueryRow(`SELECT count(*) FROM t`).Scan(&n))
	return n
}

// snapshot backs up the database at path through its own connection, as
// pulse backup does from a second process.
func snapshot(t *testing.T, path, out, configFile string) backup.Result {
	t.Helper()
	src, err := backup.Open(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, src.Close()) }()
	res, err := backup.Snapshot(ctx, src, out, configFile)
	require.NoError(t, err)
	return res
}

// corrupt overwrites the third page of the file at path with garbage.
func corrupt(t *testing.T, path string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte(strings.Repeat("\xff", 4096)), 2*4096)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestSnapshotOfLiveDatabaseWithOpenWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pulse.db")
	conn := newDB(t, path, 100)

	// A write transaction stays open during the snapshot. Its rows are not
	// committed, so the snapshot must not hold them, and it must not wait.
	tx, err := conn.Begin()
	require.NoError(t, err)
	_, err = tx.Exec(`INSERT INTO t (v) VALUES ('uncommitted')`)
	require.NoError(t, err)

	out := filepath.Join(dir, "snap.db")
	res := snapshot(t, path, out, filepath.Join(dir, "pulse.json"))
	require.NoError(t, tx.Rollback())
	assert.Equal(t, out, res.Path)
	assert.Positive(t, res.Size)
	latest, err := db.LatestVersion()
	require.NoError(t, err)
	assert.Equal(t, latest, res.Version)
	assert.Empty(t, res.Config, "no pulse.json to copy")
	assert.Equal(t, 100, rows(t, out))

	// Writers that commit during the snapshot are not blocked, and the
	// snapshot is a consistent state between before and after.
	var wg sync.WaitGroup
	var stop atomic.Bool
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			if _, err := conn.Exec(`INSERT INTO t (v) VALUES ('during')`); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	out2 := filepath.Join(dir, "snap2.db")
	snapshot(t, path, out2, "")
	stop.Store(true)
	wg.Wait()
	n := rows(t, out2)
	assert.GreaterOrEqual(t, n, 100)
	_, err = backup.Verify(ctx, out2)
	require.NoError(t, err)

	// A snapshot is one file. No -wal, -shm or temporary file is left.
	for _, name := range names(t, dir) {
		assert.False(t, strings.HasPrefix(name, "snap") && name != "snap.db" && name != "snap2.db", "left %s", name)
		assert.False(t, strings.HasPrefix(name, ".pulse-backup-"), "left %s", name)
	}
}

func TestSnapshotCopiesPulseJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pulse.db")
	newDB(t, path, 1)
	configFile := filepath.Join(dir, "pulse.json")
	require.NoError(t, os.WriteFile(configFile, []byte(`{"configured":true}`), 0o600))

	out := filepath.Join(dir, "snap.db")
	res := snapshot(t, path, out, configFile)
	assert.Equal(t, backup.ConfigPath(out), res.Config)
	b, err := os.ReadFile(res.Config)
	require.NoError(t, err)
	assert.JSONEq(t, `{"configured":true}`, string(b))
}

func TestSnapshotRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pulse.db")
	newDB(t, path, 1)
	src, err := backup.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	out := filepath.Join(dir, "snap.db")
	require.NoError(t, os.WriteFile(out, []byte("keep me"), 0o600))
	_, err = backup.Snapshot(ctx, src, out, "")
	require.ErrorIs(t, err, backup.ErrExists)
	b, _ := os.ReadFile(out)
	assert.Equal(t, "keep me", string(b))

	// The copy of pulse.json is not replaced either.
	out2 := filepath.Join(dir, "snap2.db")
	require.NoError(t, os.WriteFile(backup.ConfigPath(out2), []byte("keep me"), 0o600))
	_, err = backup.Snapshot(ctx, src, out2, "")
	require.ErrorIs(t, err, backup.ErrExists)
	assert.NoFileExists(t, out2)
}

func TestSnapshotErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pulse.db")
	newDB(t, path, 1)
	src, err := backup.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	_, err = backup.Snapshot(ctx, src, filepath.Join(dir, "missing", "snap.db"), "")
	require.Error(t, err, "a directory that does not exist")

	_, err = backup.Open(filepath.Join(dir, "nope.db"))
	require.ErrorIs(t, err, os.ErrNotExist)

	closed, err := backup.Open(path)
	require.NoError(t, err)
	require.NoError(t, closed.Close())
	_, err = backup.Snapshot(ctx, closed, filepath.Join(dir, "snap.db"), "")
	require.Error(t, err, "a closed connection")
	assert.ElementsMatch(t, []string{"pulse.db", "pulse.db-shm", "pulse.db-wal"}, names(t, dir),
		"a failed snapshot leaves nothing behind")

	// pulse.json that cannot be read fails the snapshot after it is written.
	_, err = backup.Snapshot(ctx, src, filepath.Join(dir, "snap.db"), dir)
	require.Error(t, err)
}

func TestName(t *testing.T) {
	at := time.Date(2026, 10, 30, 9, 15, 0, 0, time.FixedZone("IST", 5*3600+1800))
	assert.Equal(t, "pulse-backup-20261030T034500Z.db", backup.Name(at))
}

func TestVerifyFailsOnCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pulse.db")
	newDB(t, path, 500)
	out := filepath.Join(dir, "snap.db")
	snapshot(t, path, out, "")
	corrupt(t, out)

	_, err := backup.Verify(ctx, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integrity check")
}

func TestVerifyFailsOnOtherDatabases(t *testing.T) {
	dir := t.TempDir()

	// A SQLite file that is not a Pulse database.
	plain := filepath.Join(dir, "plain.db")
	conn, err := sql.Open("sqlite", plain)
	require.NoError(t, err)
	_, err = conn.Exec(`CREATE TABLE x (id INTEGER)`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	_, err = backup.Verify(ctx, plain)
	require.ErrorContains(t, err, "not a Pulse database")

	// A Pulse database whose last migration stopped halfway.
	dirty := filepath.Join(dir, "dirty.db")
	conn = newDB(t, dirty, 1)
	_, err = conn.Exec(`UPDATE schema_migrations SET dirty = 1`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	_, err = backup.Verify(ctx, dirty)
	require.ErrorContains(t, err, "did not finish")

	// Not a database at all.
	text := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(text, []byte(strings.Repeat("not sqlite ", 100)), 0o600))
	_, err = backup.Verify(ctx, text)
	require.ErrorContains(t, err, "integrity check")
}
