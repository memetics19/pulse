package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPruneIndexesMigrationUpAndDown covers migration 12, which indexes the
// timestamp column each pruner DELETE filters on.
func TestPruneIndexesMigrationUpAndDown(t *testing.T) {
	db, migrator := openTestDBAtVersion(t, 12)
	ctx := context.Background()

	indexes := []string{
		"idx_check_results_checked_at",
		"idx_agent_diagnostics_collected_at",
		"idx_infra_metrics_raw_collected_at",
		"idx_infra_metrics_1m_bucket_at",
		"idx_sessions_expires_at",
	}
	countIndexes := func() int {
		var n int
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT count(*) FROM sqlite_master
			WHERE type = 'index' AND name IN (?, ?, ?, ?, ?)
		`, indexes[0], indexes[1], indexes[2], indexes[3], indexes[4]).Scan(&n))
		return n
	}
	require.Equal(t, len(indexes), countIndexes())

	// The batched prune finds expired rows through the new index rather than
	// scanning the table.
	var plan string
	rows, err := db.QueryContext(ctx, `EXPLAIN QUERY PLAN
		SELECT id FROM infra_metrics_1m WHERE bucket_at < ? LIMIT 10`, "x")
	require.NoError(t, err)
	for rows.Next() {
		var id, parent, notused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &notused, &detail))
		plan += detail
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Contains(t, plan, "idx_infra_metrics_1m_bucket_at")

	require.NoError(t, migrator.Steps(-1))
	require.Zero(t, countIndexes())
}
