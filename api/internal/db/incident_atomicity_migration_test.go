package db_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncidentAtomicityMigration(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	insert := func(source, externalID, status string) error {
		_, err := db.ExecContext(ctx, `INSERT INTO incidents
			(title, severity, status, affected_monitor_ids, source, external_id)
			VALUES ('API down', 'major', ?, '[42]', ?, ?)`, status, source, externalID)
		return err
	}

	require.NoError(t, insert("monitor", "42", "detected"))
	require.Error(t, insert("monitor", "42", "investigating"),
		"only one unresolved automatic incident may exist per monitor")
	require.NoError(t, insert("internal", "42", "detected"),
		"manual incidents must not share the automatic-incident uniqueness rule")

	_, err := db.ExecContext(ctx, `UPDATE incidents SET status = 'resolved' WHERE source = 'monitor' AND external_id = '42'`)
	require.NoError(t, err)
	require.NoError(t, insert("monitor", "42", "detected"),
		"a resolved automatic incident must permit a later incident")

	rows, err := db.QueryContext(ctx, `PRAGMA index_xinfo('idx_check_results_monitor_checked')`)
	require.NoError(t, err)
	defer rows.Close()
	type indexedColumn struct {
		name string
		desc int
	}
	var columns []indexedColumn
	for rows.Next() {
		var seqno, cid, desc, key int
		var name, coll sql.NullString
		require.NoError(t, rows.Scan(&seqno, &cid, &name, &desc, &coll, &key))
		if key == 1 && name.Valid {
			columns = append(columns, indexedColumn{name: name.String, desc: desc})
		}
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []indexedColumn{
		{name: "monitor_id", desc: 0},
		{name: "checked_at", desc: 1},
		{name: "id", desc: 1},
	}, columns)
}
