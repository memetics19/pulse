package db

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
)

const foreignKeysOffTransactionMarker = "-- pulse:foreign-keys-off-transaction"

// migrationDriver preserves the sqlite driver's normal transaction wrapping,
// except for explicitly marked table rebuilds that must disable foreign keys
// before their own atomic transaction begins.
type migrationDriver struct {
	database.Driver
	db *sql.DB
}

func newMigrationDriver(db *sql.DB) (database.Driver, error) {
	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		return nil, err
	}
	return &migrationDriver{Driver: driver, db: db}, nil
}

func (d *migrationDriver) Run(migration io.Reader) (err error) {
	contents, err := io.ReadAll(migration)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.TrimSpace(string(contents)), foreignKeysOffTransactionMarker) {
		return d.Driver.Run(bytes.NewReader(contents))
	}

	ctx := context.Background()
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer func() {
		err = errors.Join(err, setForeignKeys(ctx, conn, true))
	}()
	if err := setForeignKeys(ctx, conn, false); err != nil {
		return err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin marked migration: %w", err)
	}
	if _, err := tx.Exec(string(contents)); err != nil {
		err = errors.Join(err, rollback(tx))
		return &database.Error{OrigErr: err, Query: contents}
	}
	if err := tx.Commit(); err != nil {
		return errors.Join(fmt.Errorf("commit marked migration: %w", err), rollback(tx))
	}
	return nil
}

func rollback(tx *sql.Tx) error {
	err := tx.Rollback()
	if errors.Is(err, sql.ErrTxDone) {
		return nil
	}
	return err
}

func setForeignKeys(ctx context.Context, conn *sql.Conn, enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA foreign_keys = %d", value)); err != nil {
		return err
	}

	var actual int
	if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&actual); err != nil {
		return err
	}
	if actual != value {
		return fmt.Errorf("set PRAGMA foreign_keys = %d: got %d", value, actual)
	}
	return nil
}
