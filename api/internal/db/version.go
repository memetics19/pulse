package db

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"

	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// LatestVersion returns the newest migration this binary carries. A database
// at a higher version was written by a newer release.
func LatestVersion() (uint, error) {
	src, err := iofs.New(migrations, "migrations")
	if err != nil {
		return 0, err
	}
	v, err := src.First()
	if err != nil {
		return 0, err
	}
	for {
		next, err := src.Next(v)
		if errors.Is(err, fs.ErrNotExist) {
			return v, nil
		}
		if err != nil {
			return 0, err
		}
		v = next
	}
}

// SchemaVersion reads the migration version recorded in conn, and whether a
// migration stopped halfway (dirty). A database without the schema_migrations
// table is not a Pulse database.
func SchemaVersion(ctx context.Context, conn *sql.DB) (version uint, dirty bool, err error) {
	err = conn.QueryRowContext(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty)
	return version, dirty, err
}
