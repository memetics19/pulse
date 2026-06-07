package db_test

import (
	"os"
	"testing"

	"github.com/memetics19/pulse/api/internal/db"
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
