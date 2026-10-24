package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/stretchr/testify/require"
)

// TestUTCTimestampsMigration covers migration 14 (issue #48). A database from
// 3.0.0 sits at version 13 and may hold timestamps written with the host's
// offset. The migration rewrites them into the text the driver writes for a
// UTC time and leaves every other value alone.
func TestUTCTimestampsMigration(t *testing.T) {
	db, migrator := openTestDBAtVersion(t, 13)
	ctx := context.Background()
	exec := func(query string, args ...any) {
		t.Helper()
		_, err := db.ExecContext(ctx, query, args...)
		require.NoError(t, err)
	}

	exec(`INSERT INTO users (id, username, password_hash) VALUES (1, 'admin', 'h')`)
	exec(`INSERT INTO monitors (id, name, type) VALUES (1, 'm', 'http')`)
	exec(`INSERT INTO infra_agents (id, name, host_label, token_hash) VALUES (1, 'a', 'h', 't')`)

	// Each input is raw text as an older build could have stored it.
	cases := []struct{ in, want string }{
		{"2026-10-23 10:00:00 +0530 IST", "2026-10-23 04:30:00 +0000 UTC"},
		{"2026-10-23 10:00:00 +0530 +0530", "2026-10-23 04:30:00 +0000 UTC"},
		{"2026-10-23 10:00:00.5 +0530 IST m=+0.016177418", "2026-10-23 04:30:00.5 +0000 UTC"},
		{"2026-10-23 21:15:30.123456789 -0400 EDT", "2026-10-24 01:15:30.123456789 +0000 UTC"},
		{"2026-10-23 04:30:00 +0000 GMT", "2026-10-23 04:30:00 +0000 UTC"},
		{"2026-10-23 04:30:00 +0000 UTC m=+1.5", "2026-10-23 04:30:00 +0000 UTC"},
		// Already canonical, or written by CURRENT_TIMESTAMP: unchanged.
		{"2026-10-23 04:30:00.25 +0000 UTC", "2026-10-23 04:30:00.25 +0000 UTC"},
		{"2026-10-23 04:30:00", "2026-10-23 04:30:00"},
		// Not a driver-written time: unchanged rather than lost.
		{"not a time", "not a time"},
	}
	for i, c := range cases {
		id := i + 1
		exec(`INSERT INTO maintenance_windows (id, title, starts_at, ends_at) VALUES (?, 't', ?, ?)`, id, c.in, c.in)
		exec(`INSERT INTO check_results (id, monitor_id, checked_at, status) VALUES (?, 1, ?, 'up')`, id, c.in)
		exec(`INSERT INTO agent_diagnostics (id, agent_id, collected_at) VALUES (?, 1, ?)`, id, c.in)
		exec(`INSERT INTO infra_metrics_raw (id, agent_id, collected_at) VALUES (?, 1, ?)`, id, c.in)
		exec(`INSERT INTO incidents (id, title, severity, status, started_at, resolved_at)
		      VALUES (?, 't', 'minor', 'resolved', ?, ?)`, id, c.in, c.in)
		exec(`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, 1, ?)`, id, c.in)
		exec(`INSERT INTO api_keys (id, name, key_hash, prefix, last_used_at) VALUES (?, 'k', ?, 'p', ?)`, id, id, c.in)
	}
	// One bucket per agent: the 1m table is unique on (agent_id, bucket_at).
	exec(`INSERT INTO infra_agents (id, name, host_label, token_hash) VALUES (2, 'b', 'h', 't2')`)
	exec(`INSERT INTO infra_metrics_1m (agent_id, bucket_at, cpu_percent) VALUES (1, '2026-10-23 10:00:00 +0530 IST', 1)`)
	// The same instant stored twice for one agent collapses into one row.
	exec(`INSERT INTO infra_metrics_1m (agent_id, bucket_at, cpu_percent) VALUES (2, '2026-10-23 10:00:00 +0530 IST', 1)`)
	exec(`INSERT INTO infra_metrics_1m (agent_id, bucket_at, cpu_percent) VALUES (2, '2026-10-23 04:30:00 +0000 UTC', 2)`)

	columns := []struct{ table, column, key string }{
		{"maintenance_windows", "starts_at", "id"},
		{"maintenance_windows", "ends_at", "id"},
		{"check_results", "checked_at", "id"},
		{"agent_diagnostics", "collected_at", "id"},
		{"infra_metrics_raw", "collected_at", "id"},
		{"incidents", "started_at", "id"},
		{"incidents", "resolved_at", "id"},
		{"sessions", "expires_at", "CAST(token AS INTEGER)"},
		{"api_keys", "last_used_at", "id"},
	}
	snapshot := func() map[string][]string {
		t.Helper()
		out := map[string][]string{}
		for _, col := range columns {
			out[col.table+"."+col.column] = textColumn(t, db,
				`SELECT CAST(`+col.column+` AS TEXT) FROM `+col.table+` ORDER BY `+col.key)
		}
		out["infra_metrics_1m.bucket_at"] = textColumn(t, db,
			`SELECT CAST(bucket_at AS TEXT) || ' agent=' || agent_id FROM infra_metrics_1m ORDER BY agent_id`)
		return out
	}

	require.NoError(t, migrator.Migrate(14))
	want := make([]string, len(cases))
	for i, c := range cases {
		want[i] = c.want
	}
	got := snapshot()
	for _, col := range columns {
		require.Equal(t, want, got[col.table+"."+col.column], col.table+"."+col.column)
	}
	require.Equal(t, []string{
		"2026-10-23 04:30:00 +0000 UTC agent=1",
		"2026-10-23 04:30:00 +0000 UTC agent=2",
	}, got["infra_metrics_1m.bucket_at"])

	// The rewritten rows read back as the same instants.
	q := generated.New(db)
	due, err := q.DueToStart(ctx, time.Date(2026, 10, 23, 5, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NotEmpty(t, due)
	for _, m := range due {
		require.False(t, m.StartsAt.After(time.Date(2026, 10, 23, 5, 0, 0, 0, time.UTC)), m.StartsAt)
	}

	// Running the migration again changes nothing.
	require.NoError(t, migrator.Steps(-1))
	require.Equal(t, got, snapshot(), "down migration keeps the UTC text")
	require.NoError(t, migrator.Steps(1))
	require.Equal(t, got, snapshot(), "second run is a no-op")
}

// TestUTCTimestampsMigrationEmptyDB runs every migration on a new database.
func TestUTCTimestampsMigrationEmptyDB(t *testing.T) {
	db, migrator := openTestDBAtVersion(t, 13)
	require.NoError(t, migrator.Migrate(14))
	version, dirty, err := migrator.Version()
	require.NoError(t, err)
	require.False(t, dirty)
	require.EqualValues(t, 14, version)
	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM maintenance_windows`).Scan(&n))
	require.Zero(t, n)
}

// TestOpenStoresTimesInUTC checks the connection writes every time.Time in
// UTC, whatever zone the caller's value carries.
func TestOpenStoresTimesInUTC(t *testing.T) {
	conn, err := Open(t.TempDir() + "/utc.db")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })

	ist := time.FixedZone("IST", 5*3600+30*60)
	var stored string
	require.NoError(t, conn.QueryRow(`SELECT CAST(? AS TEXT)`,
		time.Date(2026, 10, 23, 10, 0, 0, 500, ist)).Scan(&stored))
	require.Equal(t, "2026-10-23 04:30:00.0000005 +0000 UTC", stored)
	require.NoError(t, conn.QueryRow(`SELECT CAST(? AS TEXT)`, time.Now()).Scan(&stored))
	require.Regexp(t, `^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d(\.\d+)? \+0000 UTC$`, stored,
		"no local offset and no monotonic suffix")
}

func textColumn(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.Query(query)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	var out []string
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		out = append(out, s)
	}
	require.NoError(t, rows.Err())
	return out
}
