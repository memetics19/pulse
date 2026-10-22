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
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close test db: %v", err)
		}
	})
	return conn
}
