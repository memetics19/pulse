package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
)

func TestImportFoundationMigrationDownAndUp(t *testing.T) {
	db, err := Open(t.TempDir() + "/round-trip.db")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	ctx := context.Background()

	_, err = db.ExecContext(ctx, `
		INSERT INTO monitor_groups (id, name, source, external_id)
		VALUES (42, 'Imported group', 'uptime-kuma', 'group:42');
		INSERT INTO monitors (id, name, url, type, group_id, source, external_id)
		VALUES (100, 'HTTPS', 'https://example.com/health', 'https', 42, 'uptime-kuma', 'monitor:100');
		INSERT INTO check_results (monitor_id, status) VALUES (100, 'up');
		INSERT INTO monitors (id, name, url, type, source, external_id)
		VALUES (101, 'Push', '', 'push', 'uptime-kuma', 'monitor:101');
		INSERT INTO push_monitor_tokens (monitor_id, token_hash, prefix)
		VALUES (101, 'token-hash', 'pulse_push_ab');
		INSERT INTO import_runs
			(source, source_version, input_hash, idempotency_key, conflict_policy, status)
		VALUES ('uptime-kuma', '1.23.16', 'input-hash', 'request-1', 'fail', 'completed');
	`)
	require.NoError(t, err)

	source, err := iofs.New(migrations, "migrations")
	require.NoError(t, err)
	driver, err := sqlite.WithInstance(db, &sqlite.Config{NoTxWrap: true})
	require.NoError(t, err)
	migrator, err := migrate.NewWithInstance("iofs", source, "sqlite", driver)
	require.NoError(t, err)

	require.NoError(t, migrator.Steps(-1))

	var monitorType, monitorURL string
	var groupID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT type, url, group_id FROM monitors WHERE id = 100
	`).Scan(&monitorType, &monitorURL, &groupID))
	require.Equal(t, "http", monitorType)
	require.Equal(t, "https://example.com/health", monitorURL)
	require.Equal(t, int64(42), groupID)

	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM monitors WHERE id = 101`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master
		WHERE name IN ('push_monitor_tokens', 'import_runs',
		               'idx_monitors_source_external', 'idx_groups_source_external')
	`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM pragma_table_info('monitor_groups')
		WHERE name IN ('source', 'external_id')
	`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM check_results WHERE monitor_id = 100`).Scan(&count))
	require.Equal(t, 1, count)

	assertSQLiteIntegrity(t, ctx, db)

	require.NoError(t, migrator.Steps(1))

	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master
		WHERE name IN ('push_monitor_tokens', 'import_runs',
		               'idx_monitors_source_external', 'idx_groups_source_external')
	`).Scan(&count))
	require.Equal(t, 4, count)
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM pragma_table_info('monitor_groups')
		WHERE name IN ('source', 'external_id')
	`).Scan(&count))
	require.Equal(t, 2, count)
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT type, url, group_id FROM monitors WHERE id = 100
	`).Scan(&monitorType, &monitorURL, &groupID))
	require.Equal(t, "http", monitorType)
	require.Equal(t, "https://example.com/health", monitorURL)
	require.Equal(t, int64(42), groupID)

	assertSQLiteIntegrity(t, ctx, db)
}

func assertSQLiteIntegrity(t *testing.T, ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) {
	t.Helper()

	var foreignKeys, violations int
	require.NoError(t, db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys))
	require.Equal(t, 1, foreignKeys)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_foreign_key_check`).Scan(&violations))
	require.Zero(t, violations)

	var integrity string
	require.NoError(t, db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity))
	require.Equal(t, "ok", integrity)
}
