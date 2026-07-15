package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
)

func TestImportFoundationMigrationDownAndUp(t *testing.T) {
	db, migrator := openTestDBAtVersion(t, 9)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
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

	var downSidecarCount int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'import_foundation_group_identities'
	`).Scan(&downSidecarCount))

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

	var groupSource, groupExternalID string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT source, external_id FROM monitor_groups WHERE id = 42
	`).Scan(&groupSource, &groupExternalID))
	require.Equal(t, "uptime-kuma", groupSource)
	require.Equal(t, "group:42", groupExternalID)
	require.Equal(t, 1, downSidecarCount)
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'import_foundation_group_identities'
	`).Scan(&count))
	require.Zero(t, count)

	assertSQLiteIntegrity(t, ctx, db)
}

func TestImportFoundationUpCollisionRollsBack(t *testing.T) {
	db, migrator := openTestDBAtVersion(t, 8)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
		INSERT INTO monitors (id, name, url, type, source, external_id)
		VALUES
			(10, 'First', 'https://first.example', 'http', 'uptime-kuma', 'monitor:7'),
			(11, 'Duplicate', 'https://duplicate.example', 'http', 'uptime-kuma', 'monitor:7');
	`)
	require.NoError(t, err)

	require.Error(t, migrator.Steps(1))
	assertLegacyImportFoundationSchema(t, ctx, db, 0)
	assertSQLiteIntegrity(t, ctx, db)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM monitors WHERE id IN (10, 11)`).Scan(&count))
	require.Equal(t, 2, count)

	require.NoError(t, migrator.Force(8))
	_, err = db.ExecContext(ctx, `UPDATE monitors SET external_id = 'monitor:8' WHERE id = 11`)
	require.NoError(t, err)
	require.NoError(t, migrator.Steps(1))

	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master
		WHERE name IN ('push_monitor_tokens', 'import_runs',
		               'idx_monitors_source_external', 'idx_groups_source_external')
	`).Scan(&count))
	require.Equal(t, 4, count)
	assertSQLiteIntegrity(t, ctx, db)
}

func TestImportFoundationLateFailureRollsBack(t *testing.T) {
	db, migrator := openTestDBAtVersion(t, 9)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
		INSERT INTO monitor_groups (id, name, source, external_id)
		VALUES
			(41, 'First', 'uptime-kuma', 'group:41'),
			(42, 'Second', 'uptime-kuma', 'group:42');
	`)
	require.NoError(t, err)

	require.NoError(t, migrator.Steps(-1))
	_, err = db.ExecContext(ctx, `
		UPDATE import_foundation_group_identities
		SET external_id = 'group:duplicate'
		WHERE group_id IN (41, 42)
	`)
	require.NoError(t, err)

	require.Error(t, migrator.Steps(1))
	assertLegacyImportFoundationSchema(t, ctx, db, 1)
	assertSQLiteIntegrity(t, ctx, db)

	require.NoError(t, migrator.Force(8))
	_, err = db.ExecContext(ctx, `
		UPDATE import_foundation_group_identities
		SET external_id = CASE group_id WHEN 41 THEN 'group:41' ELSE 'group:42' END
		WHERE group_id IN (41, 42)
	`)
	require.NoError(t, err)
	require.NoError(t, migrator.Steps(1))
	assertSQLiteIntegrity(t, ctx, db)
}

func TestImportFoundationPreservesAutoincrementHighWater(t *testing.T) {
	db, migrator := openTestDBAtVersion(t, 8)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
		INSERT INTO monitor_groups (id, name) VALUES (42, 'Live group');
		INSERT INTO monitor_groups (id, name) VALUES (2000, 'Deleted high group');
		DELETE FROM monitor_groups WHERE id = 2000;
		INSERT INTO monitors (id, name, url, type, group_id)
		VALUES (100, 'Live monitor', 'https://example.com', 'http', 42);
		INSERT INTO monitors (id, name, url, type)
		VALUES (1000, 'Deleted high monitor', 'https://deleted.example', 'http');
		DELETE FROM monitors WHERE id = 1000;
	`)
	require.NoError(t, err)
	require.Equal(t, int64(1000), sqliteSequence(t, ctx, db, "monitors"))
	require.Equal(t, int64(2000), sqliteSequence(t, ctx, db, "monitor_groups"))

	require.NoError(t, migrator.Steps(1))
	require.Equal(t, int64(1000), sqliteSequence(t, ctx, db, "monitors"))
	require.Equal(t, int64(2000), sqliteSequence(t, ctx, db, "monitor_groups"))

	result, err := db.ExecContext(ctx, `
		INSERT INTO monitors (name, url, type) VALUES ('After up', 'https://after-up.example', 'http')
	`)
	require.NoError(t, err)
	nextMonitorID, err := result.LastInsertId()
	require.NoError(t, err)
	require.Greater(t, nextMonitorID, int64(1000))

	_, err = db.ExecContext(ctx, `
		INSERT INTO monitors (id, name, url, type)
		VALUES (5000, 'Highest push', '', 'push')
	`)
	require.NoError(t, err)
	require.NoError(t, migrator.Steps(-1))
	require.Equal(t, int64(5000), sqliteSequence(t, ctx, db, "monitors"))
	require.Equal(t, int64(2000), sqliteSequence(t, ctx, db, "monitor_groups"))

	result, err = db.ExecContext(ctx, `
		INSERT INTO monitors (name, url, type) VALUES ('After down', 'https://after-down.example', 'http')
	`)
	require.NoError(t, err)
	nextMonitorID, err = result.LastInsertId()
	require.NoError(t, err)
	require.Greater(t, nextMonitorID, int64(5000))

	result, err = db.ExecContext(ctx, `INSERT INTO monitor_groups (name) VALUES ('After down')`)
	require.NoError(t, err)
	nextGroupID, err := result.LastInsertId()
	require.NoError(t, err)
	require.Greater(t, nextGroupID, int64(2000))
	assertSQLiteIntegrity(t, ctx, db)
}

func openTestDBAtVersion(t *testing.T, version int) (*sql.DB, *migrate.Migrate) {
	t.Helper()

	dsn, err := sqliteDSN(t.TempDir() + "/migration.db")
	require.NoError(t, err)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	migrator := newTestMigrator(t, db)
	require.NoError(t, migrator.Steps(version))
	return db, migrator
}

func newTestMigrator(t *testing.T, db *sql.DB) *migrate.Migrate {
	t.Helper()

	source, err := iofs.New(migrations, "migrations")
	require.NoError(t, err)
	driver, err := newMigrationDriver(db)
	require.NoError(t, err)
	migrator, err := migrate.NewWithInstance("iofs", source, "sqlite", driver)
	require.NoError(t, err)
	return migrator
}

func assertLegacyImportFoundationSchema(t *testing.T, ctx context.Context, db *sql.DB, sidecarCount int) {
	t.Helper()

	var count int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM pragma_table_info('monitor_groups')
		WHERE name IN ('source', 'external_id')
	`).Scan(&count))
	require.Zero(t, count)
	var monitorSchema string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'monitors'
	`).Scan(&monitorSchema))
	require.NotContains(t, monitorSchema, "'push'")
	require.NotContains(t, monitorSchema, "'https'")
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master
		WHERE name IN ('push_monitor_tokens', 'import_runs',
		               'idx_monitors_source_external', 'idx_groups_source_external',
		               'monitors_new', 'monitors_old', 'monitor_groups_old',
		               'import_foundation_sequence_state')
	`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master
		WHERE name = 'import_foundation_group_identities'
	`).Scan(&count))
	require.Equal(t, sidecarCount, count)
}

func sqliteSequence(t *testing.T, ctx context.Context, db *sql.DB, table string) int64 {
	t.Helper()

	var sequence int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT seq FROM sqlite_sequence WHERE name = ?`, table).Scan(&sequence))
	return sequence
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
