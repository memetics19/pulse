package testutil

import (
	"database/sql"
	"os"
	"testing"

	"github.com/memetics19/pulse/api/internal/db"
)

func NewTestDB(t *testing.T) *sql.DB {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "pulse-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	conn, err := db.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}
