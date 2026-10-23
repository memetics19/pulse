package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTOTPLastStepMigrationUpAndDown covers migration 13, which records the
// last accepted TOTP time step per user so a code works only once.
func TestTOTPLastStepMigrationUpAndDown(t *testing.T) {
	dsn, err := sqliteDSN(t.TempDir() + "/migration.db")
	require.NoError(t, err)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	migrator := newTestMigrator(t, db)

	// Migrate to 13 by version, not by step count, so the test does not
	// depend on which other migrations sit below it.
	require.NoError(t, migrator.Migrate(13))
	ctx := context.Background()

	_, err = db.ExecContext(ctx, `INSERT INTO users (username, password_hash) VALUES ('admin', 'h')`)
	require.NoError(t, err)
	var step int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT totp_last_step FROM users`).Scan(&step))
	require.Zero(t, step, "existing and new users start with no accepted step")

	require.NoError(t, migrator.Steps(-1))
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM pragma_table_info('users') WHERE name = 'totp_last_step'
	`).Scan(&count))
	require.Zero(t, count, "down migration drops the column")
}
