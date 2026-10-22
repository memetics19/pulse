package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDiagnosticsMigrationUpAndDown covers migration 11, which adds the
// agent_diagnostics table holding agent-collected diagnostic bundles.
func TestDiagnosticsMigrationUpAndDown(t *testing.T) {
	db, migrator := openTestDBAtVersion(t, 11)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
		INSERT INTO infra_agents (id, name, host_label, token_hash)
		VALUES (1, 'proxmox', 'homeserver', 'hash');
		INSERT INTO agent_diagnostics (agent_id, payload)
		VALUES (1, '{"sections":{}}');
	`)
	require.NoError(t, err)

	// A bundle belongs to the agent that produced it and to nothing else.
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM pragma_table_info('agent_diagnostics')
		WHERE name = 'incident_id'
	`).Scan(&count))
	require.Zero(t, count, "diagnostics must not carry an unvalidated incident link")

	// Deleting the agent takes its diagnostics with it.
	_, err = db.ExecContext(ctx, `DELETE FROM infra_agents WHERE id = 1`)
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM agent_diagnostics`).Scan(&count))
	require.Zero(t, count)

	// Down migration removes the table entirely.
	require.NoError(t, migrator.Steps(-1))
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master WHERE name = 'agent_diagnostics'
	`).Scan(&count))
	require.Zero(t, count)
}
