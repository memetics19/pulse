package db_test

import (
	"context"
	"os"
	"testing"

	"github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/keyauth"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/require"
)

func TestMigrationsCreateUsersTable(t *testing.T) {
	conn, err := db.Open(t.TempDir() + "/m.db")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM users`).Scan(&n); err != nil {
		t.Fatalf("users table missing: %v", err)
	}
}

func TestUsersHaveProfileColumns(t *testing.T) {
	conn, err := db.Open(t.TempDir() + "/p.db")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`INSERT INTO users (username, password_hash, name, email) VALUES ('a','h','A','a@b.c')`); err != nil {
		t.Fatalf("insert with name/email failed: %v", err)
	}
}

func TestAPIKeysTableExists(t *testing.T) {
	conn, err := db.Open(t.TempDir() + "/k.db")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`INSERT INTO api_keys (name, key_hash, prefix, scopes) VALUES ('ci','h','pulse_live_ab','["monitors:read"]')`); err != nil {
		t.Fatalf("api_keys insert failed: %v", err)
	}
}

func TestMaintenanceTableExists(t *testing.T) {
	conn, err := db.Open(t.TempDir() + "/m.db")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`INSERT INTO maintenance_windows (title, starts_at, ends_at) VALUES ('x', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert failed: %v", err)
	}
}

func TestDefaultStatusPageSeeded(t *testing.T) {
	conn, err := db.Open(t.TempDir() + "/sp.db")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM status_pages WHERE is_default=1 AND domain=''`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expected exactly one default page, got n=%d err=%v", n, err)
	}
}

func TestOpen(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.db")
	require.NoError(t, err)
	f.Close()
	conn, err := db.Open(f.Name())
	require.NoError(t, err)
	require.NoError(t, conn.Ping())
	conn.Close()
}

func TestImportFoundationSchema(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `INSERT INTO monitors
		(name, url, type, interval_seconds, timeout_seconds, source, external_id)
		VALUES ('Push', '', 'push', 60, 10, 'uptime-kuma', 'monitor:7')`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO monitors
		(name, url, type, interval_seconds, timeout_seconds, source, external_id)
		VALUES ('Duplicate', '', 'push', 60, 10, 'uptime-kuma', 'monitor:7')`)
	require.Error(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO import_runs
		(source, source_version, input_hash, idempotency_key, conflict_policy, status)
		VALUES ('uptime-kuma', '1.23.16', 'hash', 'request-1', 'fail', 'running')`)
	require.NoError(t, err)

	var foreignKeys int
	require.NoError(t, db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys))
	require.Equal(t, 1, foreignKeys)
}

func TestLegacyAgentTokensHashedOnOpen(t *testing.T) {
	path := t.TempDir() + "/legacy.db"

	conn, err := db.Open(path)
	require.NoError(t, err)
	plaintext := "0123456789abcdef0123456789abcdef0123456789abcdef" // 48 hex chars
	_, err = conn.Exec(`INSERT INTO infra_agents (name, host_label, token_hash) VALUES ('legacy', 'host', ?)`, plaintext)
	require.NoError(t, err)
	conn.Close()

	// Reopen: Open must hash the plaintext row in place.
	conn, err = db.Open(path)
	require.NoError(t, err)
	defer conn.Close()

	var stored string
	require.NoError(t, conn.QueryRow(`SELECT token_hash FROM infra_agents WHERE name = 'legacy'`).Scan(&stored))
	require.Equal(t, keyauth.Hash(plaintext), stored)
	require.Len(t, stored, 64)
}
