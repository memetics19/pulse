package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHTTPMonitorOptionsMigrationUpAndDown covers migration 18, which adds the
// optional HTTP monitor options with defaults that keep the old behaviour.
func TestHTTPMonitorOptionsMigrationUpAndDown(t *testing.T) {
	dsn, err := sqliteDSN(t.TempDir() + "/migration.db")
	require.NoError(t, err)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	migrator := newTestMigrator(t, db)
	ctx := context.Background()

	// A monitor that exists before the migration gets the defaults.
	require.NoError(t, migrator.Migrate(16))
	_, err = db.ExecContext(ctx, `INSERT INTO monitors (name, url, type) VALUES ('old', 'https://example.com', 'http')`)
	require.NoError(t, err)

	require.NoError(t, migrator.Migrate(18))
	var method, headers, keywordMode, accepted string
	var tlsVerify, followRedirects sql.NullInt64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT method, headers, keyword_mode, accepted_statuses, tls_verify, follow_redirects
		FROM monitors WHERE name = 'old'
	`).Scan(&method, &headers, &keywordMode, &accepted, &tlsVerify, &followRedirects))
	require.Equal(t, "GET", method)
	require.Empty(t, headers)
	require.Equal(t, "contains", keywordMode)
	require.Empty(t, accepted)
	require.False(t, tlsVerify.Valid, "NULL tls_verify means verify")
	require.False(t, followRedirects.Valid, "NULL follow_redirects means follow")

	require.NoError(t, migrator.Migrate(16))
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM pragma_table_info('monitors')
		WHERE name IN ('method', 'headers', 'body', 'auth_type', 'auth_username',
		  'auth_secret', 'tls_verify', 'ca_pem', 'follow_redirects',
		  'accepted_statuses', 'keyword_mode')
	`).Scan(&count))
	require.Zero(t, count, "down migration drops every column")
	var name string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT name FROM monitors`).Scan(&name))
	require.Equal(t, "old", name, "down migration keeps the monitor")
}
