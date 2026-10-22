package testutil

import "testing"

func TestNewTestDB(t *testing.T) {
	db := NewTestDB(t)
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM monitors").Scan(&n); err != nil {
		t.Fatalf("migrated schema query: %v", err)
	}
}
