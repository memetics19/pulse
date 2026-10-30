package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMonitorSensitivityMigrationUpAndDown covers migration 17, which adds the
// per-monitor failure, recovery and retry settings. Existing monitors must get
// the values Pulse used before the settings existed.
func TestMonitorSensitivityMigrationUpAndDown(t *testing.T) {
	db, migrator := openTestDBAtVersion(t, 16)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `INSERT INTO monitors (name, url, type) VALUES ('api', 'http://example.com', 'http')`)
	require.NoError(t, err)

	// Migrate to 17 by version, not by step count, so the test does not
	// depend on which other migrations sit above it.
	require.NoError(t, migrator.Migrate(17))
	var failures, recoveries, retries, retryMs int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT failures_before_incident, recoveries_before_resolve, retries, retry_interval_ms
		FROM monitors
	`).Scan(&failures, &recoveries, &retries, &retryMs))
	require.EqualValues(t, 2, failures)
	require.EqualValues(t, 2, recoveries)
	require.EqualValues(t, 1, retries)
	require.EqualValues(t, 500, retryMs)

	require.NoError(t, migrator.Steps(-1))
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM pragma_table_info('monitors')
		WHERE name IN ('failures_before_incident', 'recoveries_before_resolve', 'retries', 'retry_interval_ms')
	`).Scan(&count))
	require.Zero(t, count, "down migration drops the columns")
}
