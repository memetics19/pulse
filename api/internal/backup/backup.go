// Package backup copies the SQLite database while Pulse runs, and swaps a
// copy back in while it is stopped.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/memetics19/pulse/api/internal/db"
	"modernc.org/sqlite"
)

// timeLayout stamps snapshot and kept file names. It sorts in time order.
const timeLayout = "20060102T150405Z"

var (
	// ErrExists is returned instead of replacing a file.
	ErrExists = errors.New("file already exists")
	// ErrInUse means another process still has the database open.
	ErrInUse = errors.New("database is open in another process")
	// ErrTooNew means a snapshot was written by a newer release.
	ErrTooNew = errors.New("snapshot is newer than this binary")
)

// Result describes a snapshot written by Snapshot.
type Result struct {
	Path    string
	Size    int64
	Version uint
	// Config is the copy of pulse.json beside the snapshot, or "" when the
	// install has no pulse.json.
	Config string
}

// Name returns the default file name of a snapshot taken at now.
func Name(now time.Time) string {
	return "pulse-backup-" + now.UTC().Format(timeLayout) + ".db"
}

// ConfigPath returns the copy of pulse.json taken with the snapshot at path.
func ConfigPath(path string) string { return path + ".json" }

// dsn builds a modernc.org/sqlite file URI for path with the given query.
func dsn(path string, query url.Values) string {
	u := url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}
	return u.String()
}

// Open opens the database file at path for Snapshot. Unlike db.Open it runs
// no migrations and leaves the journal mode alone, so a backup never changes
// the database it copies, even when this binary is newer than the server
// that owns the file.
func Open(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return sql.Open("sqlite", dsn(path, url.Values{"_pragma": {"busy_timeout(5000)"}}))
}

// Snapshot writes a consistent copy of conn's database to out with VACUUM
// INTO. It is safe while the server runs: the copy is one read transaction,
// so writers carry on, and the result is a single file with no -wal or -shm.
// The copy goes to a temporary file beside out, is synced and checked (see
// Verify), and only then gets its name. Snapshot never replaces an existing
// file. When configFile (pulse.json) exists, it is copied to ConfigPath(out).
func Snapshot(ctx context.Context, conn *sql.DB, out, configFile string) (Result, error) {
	for _, p := range []string{out, ConfigPath(out)} {
		if _, err := os.Lstat(p); err == nil {
			return Result{}, fmt.Errorf("%s: %w", p, ErrExists)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return Result{}, err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(out), ".pulse-backup-*.tmp")
	if err != nil {
		return Result{}, err
	}
	name := tmp.Name()
	// After publish the temporary name is a second link to out, or gone.
	defer func() { _ = os.Remove(name) }()
	if err := tmp.Close(); err != nil {
		return Result{}, err
	}
	// VACUUM INTO accepts an empty file and does not fsync what it writes.
	if _, err := conn.ExecContext(ctx, `VACUUM INTO ?`, name); err != nil {
		return Result{}, fmt.Errorf("vacuum into %s: %w", name, err)
	}
	if err := syncFile(name); err != nil {
		return Result{}, err
	}
	version, err := Verify(ctx, name)
	if err != nil {
		return Result{}, err
	}
	if err := publish(name, out); err != nil {
		return Result{}, err
	}
	info, err := os.Stat(out)
	if err != nil {
		return Result{}, err
	}
	res := Result{Path: out, Size: info.Size(), Version: version}
	res.Config, err = copyConfig(configFile, ConfigPath(out))
	return res, err
}

// publish gives the finished snapshot tmp the name out without replacing a
// file that appeared meanwhile: a hard link fails when out exists. A
// filesystem without hard links falls back to a rename.
func publish(tmp, out string) error {
	err := os.Link(tmp, out)
	if err == nil {
		return nil
	}
	if _, serr := os.Lstat(out); serr == nil {
		return fmt.Errorf("%s: %w", out, ErrExists)
	}
	return os.Rename(tmp, out)
}

// copyConfig copies pulse.json to dst. A missing pulse.json copies nothing
// and returns "".
func copyConfig(src, dst string) (string, error) {
	b, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	return dst, errors.Join(err, f.Close())
}

func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

// Verify checks that path is an intact Pulse database: PRAGMA
// integrity_check passes and a finished migration is recorded. It returns
// that migration version. The file is opened read-only and immutable, so a
// check never writes to it and never creates a -wal or -shm file.
func Verify(ctx context.Context, path string) (version uint, err error) {
	conn, err := sql.Open("sqlite", dsn(path, url.Values{"mode": {"ro"}, "immutable": {"1"}}))
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, conn.Close()) }()

	rows, err := conn.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return 0, fmt.Errorf("%s fails the integrity check: %w", path, err)
	}
	var problems []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return 0, errors.Join(err, rows.Close())
		}
		problems = append(problems, s)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return 0, fmt.Errorf("%s fails the integrity check: %w", path, err)
	}
	if len(problems) != 1 || problems[0] != "ok" {
		if len(problems) > 5 {
			problems = problems[:5]
		}
		return 0, fmt.Errorf("%s fails the integrity check: %s", path, strings.Join(problems, ", "))
	}

	version, dirty, err := db.SchemaVersion(ctx, conn)
	if err != nil {
		return 0, fmt.Errorf("%s is not a Pulse database: %w", path, err)
	}
	if dirty {
		return 0, fmt.Errorf("%s: migration %d did not finish", path, version)
	}
	return version, nil
}

// Restored describes a database swapped in by Restore.
type Restored struct {
	// Version is the migration version of the restored database. Latest is
	// the newest one this binary carries. The server applies the ones in
	// between when it starts.
	Version, Latest uint
	// Kept is the old database under its new name, or "" when there was none.
	Kept string
}

// Restore swaps the snapshot src in for the database file target. The server
// must be stopped. The caller holds the instance lock, and Restore also
// refuses while another process has target open.
//
// src is copied to a temporary file beside target and checked there (see
// Verify), so src is never changed and a file that fails the check replaces
// nothing. A snapshot at a migration version this binary does not carry was
// written by a newer release and is refused. The old database stays beside
// target as target.pre-restore-<time>. The checked copy takes the mode and
// owner of the old file, or the owner of the directory when there is none,
// and is renamed over target in one step.
func Restore(ctx context.Context, src, target string, now time.Time) (Restored, error) {
	if info, err := os.Stat(src + "-wal"); err == nil && info.Size() > 0 {
		return Restored{}, fmt.Errorf("%s has a write-ahead log beside it, so it may be a live database. Copy it with pulse backup first", src)
	}
	latest, err := db.LatestVersion()
	if err != nil {
		return Restored{}, err
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Restored{}, err
	}
	staged, err := stage(src, dir)
	if err != nil {
		return Restored{}, err
	}
	// After the swap the staged name no longer exists.
	defer func() { _ = os.Remove(staged) }()

	res := Restored{Latest: latest}
	if res.Version, err = Verify(ctx, staged); err != nil {
		return Restored{}, err
	}
	if res.Version > latest {
		return Restored{}, fmt.Errorf("%s is at schema version %d and this binary knows up to %d. Restore it with the release that wrote it or a newer one: %w",
			src, res.Version, latest, ErrTooNew)
	}

	old, err := os.Stat(target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// A new database gets the owner of its directory, the data volume.
		dirInfo, err := os.Stat(dir)
		if err != nil {
			return Restored{}, err
		}
		if err := chown(staged, dirInfo); err != nil {
			return Restored{}, err
		}
	case err != nil:
		return Restored{}, err
	default:
		if err := quiesce(ctx, target); err != nil {
			return Restored{}, err
		}
		if err := os.Chmod(staged, old.Mode().Perm()); err != nil {
			return Restored{}, err
		}
		if err := chown(staged, old); err != nil {
			return Restored{}, err
		}
		res.Kept = target + ".pre-restore-" + now.UTC().Format(timeLayout)
		if err := keep(target, res.Kept); err != nil {
			return Restored{}, err
		}
	}
	if err := os.Rename(staged, target); err != nil {
		return Restored{}, err
	}
	return res, syncDir(dir)
}

// stage copies src to a new temporary file in dir and returns its name.
func stage(src, dir string) (name string, err error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	out, err := os.CreateTemp(dir, ".pulse-restore-*.tmp")
	if err != nil {
		return "", err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if err = errors.Join(err, out.Close()); err != nil {
		_ = os.Remove(out.Name())
		return "", err
	}
	return out.Name(), nil
}

// quiesce makes sure no other process has path open and folds its
// write-ahead log into the main file. Leaving WAL mode needs the only
// connection to the database, so SQLite answers SQLITE_BUSY while the
// server, or anything else, still has the file open. That also catches a
// server from a release that does not take the instance lock. On success
// SQLite removes the -wal and -shm files itself.
//
// A file without the SQLite header is left alone: SQLite would drop its -wal
// and -shm on open, and they move along with it instead (see keep).
func quiesce(ctx context.Context, path string) (err error) {
	if !hasSQLiteHeader(path) {
		return nil
	}
	conn, err := sql.Open("sqlite", dsn(path, url.Values{"_pragma": {"busy_timeout(0)"}}))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, conn.Close()) }()
	var mode string
	err = conn.QueryRowContext(ctx, `PRAGMA journal_mode=DELETE`).Scan(&mode)
	switch code := sqliteCode(err); {
	case err == nil && mode != "delete", code == 5, code == 6: // SQLITE_BUSY, SQLITE_LOCKED
		return fmt.Errorf("%s: %w", path, ErrInUse)
	case code == 11, code == 26: // SQLITE_CORRUPT, SQLITE_NOTADB
		// No server can be using a file SQLite cannot read. It is kept as
		// it is, together with any -wal and -shm.
		return nil
	}
	return err
}

// hasSQLiteHeader reports whether the file at path starts like a database.
func hasSQLiteHeader(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	header := make([]byte, 16)
	_, err = io.ReadFull(f, header)
	return err == nil && string(header) == "SQLite format 3\x00"
}

// sqliteCode returns the primary SQLite result code of err, or 0.
func sqliteCode(err error) int {
	var se *sqlite.Error
	if errors.As(err, &se) {
		return se.Code() & 0xff
	}
	return 0
}

// chown gives path the owner of like, so a restore run as root leaves a file
// the server's user can open.
func chown(path string, like fs.FileInfo) error {
	st, ok := like.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if err := os.Chown(path, int(st.Uid), int(st.Gid)); err != nil {
		return fmt.Errorf("give the restored file the owner of %s (uid %d, gid %d). Run pulse restore as that user or as root: %w",
			like.Name(), st.Uid, st.Gid, err)
	}
	return nil
}

// keep gives the old database the second name kept while target still names
// it, so target always holds a whole database. A filesystem without hard
// links falls back to a rename. Any -wal and -shm move along, so they stay
// with the file they belong to and never meet the restored one.
func keep(target, kept string) error {
	if err := os.Link(target, kept); err != nil {
		if _, serr := os.Lstat(kept); serr == nil {
			return fmt.Errorf("%s: %w", kept, ErrExists)
		}
		if err := os.Rename(target, kept); err != nil {
			return err
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Rename(target+suffix, kept+suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// syncDir makes the renames in dir durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}
