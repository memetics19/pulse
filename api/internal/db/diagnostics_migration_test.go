package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDiagnosticsMigrationUpAndDown covers migration 11, which adds the
// incident_diagnostics table used to attach agent-collected diagnostic bundles
// to an incident.
func TestDiagnosticsMigrationUpAndDown(t *testing.T) {
	db, migrator := openTestDBAtVersion(t, 11)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
		INSERT INTO infra_agents (id, name, host_label, token_hash)
		VALUES (1, 'proxmox', 'homeserver', 'hash');
		INSERT INTO incidents (id, title, severity, status)
		VALUES (7, 'Jellyfin is down', 'major', 'detected');
		INSERT INTO incident_diagnostics (incident_id, agent_id, payload)
		VALUES (7, 1, '{"docker":{"containers":[]}}');
	`)
	require.NoError(t, err)

	// Deleting the incident must take its diagnostics with it.
	_, err = db.ExecContext(ctx, `DELETE FROM incidents WHERE id = 7`)
	require.NoError(t, err)

	var count int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM incident_diagnostics`).Scan(&count))
	require.Zero(t, count, "diagnostics should cascade when the incident is deleted")

	// Down migration removes the table entirely.
	require.NoError(t, migrator.Steps(-1))
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master WHERE name = 'incident_diagnostics'
	`).Scan(&count))
	require.Zero(t, count)
}

// An on-demand bundle (pulse-agent --diagnose) is evidence about a host at a
// moment in time and may not belong to any incident, so incident_id is optional.
func TestDiagnostics_IncidentIsOptional(t *testing.T) {
	db, _ := openTestDBAtVersion(t, 11)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
		INSERT INTO infra_agents (id, name, host_label, token_hash)
		VALUES (1, 'proxmox', 'homeserver', 'hash');
		INSERT INTO incident_diagnostics (incident_id, agent_id, payload)
		VALUES (NULL, 1, '{"sections":{}}');
	`)
	require.NoError(t, err)

	var count int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM incident_diagnostics WHERE incident_id IS NULL`).Scan(&count))
	require.Equal(t, 1, count)
}
