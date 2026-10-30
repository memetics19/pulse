package handlers

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/bootstrap"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/generated"
)

// breakCommit makes every later transaction that inserts a user fail at
// COMMIT, not before. A trigger adds a row that breaks a deferred foreign
// key, and SQLite checks deferred keys only when the transaction commits.
// It returns a function that removes the trigger again.
func breakCommit(t *testing.T, path string) func() {
	t.Helper()
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	for _, stmt := range []string{
		`CREATE TABLE commit_breaker_parent (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE commit_breaker (parent_id INTEGER REFERENCES commit_breaker_parent(id) DEFERRABLE INITIALLY DEFERRED)`,
		`CREATE TRIGGER commit_breaker_on_user AFTER INSERT ON users BEGIN INSERT INTO commit_breaker VALUES (1); END`,
	} {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return func() {
		if _, err := conn.Exec(`DROP TRIGGER commit_breaker_on_user`); err != nil {
			t.Fatal(err)
		}
	}
}

func countSetupUsers(t *testing.T, path string) int64 {
	t.Helper()
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	n, err := generated.New(conn).CountUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// noStagedFiles fails when dataDir holds anything but pulse.json and the
// database files, such as a staged file left behind.
func noStagedFiles(t *testing.T, dataDir string) {
	t.Helper()
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "pulse.json" && e.Name() != "pulse.db" && !strings.HasPrefix(e.Name(), "pulse.db-") {
			t.Errorf("unexpected file left in the data directory: %s", e.Name())
		}
	}
}

// Issue #72: pulse.json used to be saved before the admin committed. When the
// commit failed, the next start found pulse.json, booted as configured, and
// nobody could log in. Now a failed commit leaves no pulse.json, and setup
// can be retried.
func TestSetupCommitFailureWritesNoPulseJSON(t *testing.T) {
	logs := captureLog(t)
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "pulse.db")
	restore := breakCommit(t, path)

	a := app.New()
	h := NewSetup(a, dataDir, config.Config{SQLitePath: path, SetupToken: config.SetupTokenOff})
	rec := postSetup(h, admin(nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("setup with a failing commit = %d %s, want 500", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs.String(), "setup: commit") {
		t.Fatalf("the commit should be what failed, logs: %q", logs.String())
	}
	if _, err := os.Stat(bootstrap.Path(dataDir)); !os.IsNotExist(err) {
		t.Fatalf("pulse.json exists after a failed commit (stat err %v)", err)
	}
	noStagedFiles(t, dataDir)
	if a.Configured() {
		t.Fatal("the app must stay in setup mode after a failed commit")
	}
	if n := countSetupUsers(t, path); n != 0 {
		t.Fatalf("users after a failed commit = %d, want 0", n)
	}

	// Once the database accepts the commit, a retry completes setup.
	restore()
	if rec := postSetup(h, admin(nil)); rec.Code != http.StatusCreated {
		t.Fatalf("retry = %d %s, want 201", rec.Code, rec.Body.String())
	}
	t.Cleanup(func() { _ = a.DB().Close() })
	boot, err := bootstrap.Load(dataDir)
	if err != nil || !boot.Configured || boot.SQLitePath != path {
		t.Fatalf("pulse.json after the retry = %+v (err %v)", boot, err)
	}
	noStagedFiles(t, dataDir)
}

// If pulse.json cannot be put in place after the admin committed, setup has
// still succeeded: the app serves the database, the admin is logged in, and
// the next start adopts the database.
func TestSetupSucceedsWhenPulseJSONCannotBeRenamed(t *testing.T) {
	logs := captureLog(t)
	dataDir := t.TempDir()
	path := filepath.Join(t.TempDir(), "pulse.db")
	// A non-empty directory where pulse.json goes makes the rename fail,
	// while the staged file can still be written next to it.
	if err := os.MkdirAll(filepath.Join(bootstrap.Path(dataDir), "blocker"), 0o755); err != nil {
		t.Fatal(err)
	}

	a := app.New()
	h := NewSetup(a, dataDir, config.Config{SQLitePath: path, SetupToken: config.SetupTokenOff})
	rec := postSetup(h, admin(nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d %s, want 201", rec.Code, rec.Body.String())
	}
	t.Cleanup(func() { _ = a.DB().Close() })
	if !a.Configured() || len(rec.Result().Cookies()) == 0 {
		t.Fatal("setup should configure the app and log the admin in")
	}
	if !strings.Contains(logs.String(), "adopts the database on the next start") {
		t.Errorf("the failed pulse.json write should be logged, logs: %q", logs.String())
	}
	if n := countSetupUsers(t, path); n != 1 {
		t.Fatalf("users = %d, want 1", n)
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("the staged file was left behind: %v", entries)
	}
}

// A server still in setup mode whose database already has an admin answers
// 409 with a message that says what to do, and logs the file.
func TestSetupOnDatabaseWithAdminSaysRestart(t *testing.T) {
	logs := captureLog(t)
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "pulse.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("original-pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generated.New(conn).CreateUser(context.Background(), generated.CreateUserParams{Username: "old", PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()

	a := app.New()
	h := NewSetup(a, dataDir, config.Config{SQLitePath: path, SetupToken: config.SetupTokenOff})
	rec := postSetup(h, admin(nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("setup = %d, want 409", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != setupDoneRestart {
		t.Errorf("body = %q, want %q", got, setupDoneRestart)
	}
	if !strings.Contains(logs.String(), path) {
		t.Errorf("the database file should be logged, logs: %q", logs.String())
	}
	if _, err := os.Stat(bootstrap.Path(dataDir)); !os.IsNotExist(err) {
		t.Errorf("a refused setup wrote pulse.json (stat err %v)", err)
	}
	noStagedFiles(t, dataDir)
}
