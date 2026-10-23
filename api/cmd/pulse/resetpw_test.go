package main

import (
	"context"
	"testing"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/generated"
)

func TestRunResetPasswordHappyPath(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/pulse.db"
	t.Setenv("PULSE_DATA_DIR", dir)
	t.Setenv("PULSE_DB", path)

	// Create the DB (runs migrations) and an admin user to reset.
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	q := generated.New(conn)
	hash, _ := auth.HashPassword("original-pass")
	if _, err := q.CreateUser(context.Background(), generated.CreateUserParams{Username: "admin", PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	// Reset via the CLI entry point.
	runResetPassword([]string{"--username", "admin", "--password", "brand-new-pass"})

	// Verify the new password now validates.
	conn2, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn2.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	u, err := generated.New(conn2).GetUserByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := auth.VerifyPassword("brand-new-pass", u.PasswordHash)
	if err != nil || !ok {
		t.Fatalf("new password should validate: ok=%v err=%v", ok, err)
	}
}
