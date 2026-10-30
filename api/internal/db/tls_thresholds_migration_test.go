package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTLSThresholdsMigrationUpAndDown covers migration 16 (issue #77). It
// adds per-monitor TLS thresholds with the old fixed values as defaults, and
// copies the days left of older ssl rows out of status_code.
func TestTLSThresholdsMigrationUpAndDown(t *testing.T) {
	db, migrator := openTestDBAtVersion(t, 14)
	ctx := context.Background()
	exec := func(query string) {
		t.Helper()
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err)
	}
	exec(`INSERT INTO monitors (id, name, url, type) VALUES (1, 'cert', 'example.com:443', 'ssl')`)
	exec(`INSERT INTO monitors (id, name, url, type) VALUES (2, 'web', 'https://example.com', 'http')`)
	exec(`INSERT INTO check_results (monitor_id, status, status_code) VALUES (1, 'up', 40)`)
	exec(`INSERT INTO check_results (monitor_id, status, status_code) VALUES (1, 'down', NULL)`)
	exec(`INSERT INTO check_results (monitor_id, status, status_code) VALUES (2, 'up', 200)`)

	require.NoError(t, migrator.Migrate(16))

	var warn, critical int64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT tls_warn_days, tls_critical_days FROM monitors WHERE id = 1`).Scan(&warn, &critical))
	require.EqualValues(t, 14, warn)
	require.EqualValues(t, 7, critical)

	rows, err := db.QueryContext(ctx, `SELECT monitor_id, cert_days_left FROM check_results ORDER BY id`)
	require.NoError(t, err)
	type row struct {
		monitorID int64
		days      sql.NullInt64
	}
	var got []row
	for rows.Next() {
		var r row
		require.NoError(t, rows.Scan(&r.monitorID, &r.days))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, []row{
		{1, sql.NullInt64{Int64: 40, Valid: true}},
		{1, sql.NullInt64{}},
		{2, sql.NullInt64{}},
	}, got, "only ssl rows with a status_code are copied")

	require.NoError(t, migrator.Steps(-1))
	for _, col := range []struct{ table, column string }{
		{"monitors", "tls_warn_days"},
		{"monitors", "tls_critical_days"},
		{"check_results", "cert_days_left"},
	} {
		var count int
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`, col.table, col.column).Scan(&count))
		require.Zero(t, count, "down migration drops "+col.table+"."+col.column)
	}
}
