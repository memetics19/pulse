package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
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
	"github.com/memetics19/pulse/api/internal/server"
)

// clearDBEnv unsets every setting that decides where the database lives.
func clearDBEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"PULSE_DB", "SQLITE_PATH", "PULSE_DATA_DIR"} {
		t.Setenv(name, "")
	}
}

// captureLog sends the standard logger to a buffer for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prev)
		log.SetFlags(flags)
	})
	return &buf
}

func TestDatabaseFile(t *testing.T) {
	recorded := bootstrap.Config{Configured: true, SQLitePath: "/data/pulse.db"}
	cases := []struct {
		name string
		boot bootstrap.Config
		cfg  config.Config
		want string
		warn bool
	}{
		{"before setup it is the configured file", bootstrap.Config{}, config.Config{SQLitePath: "/srv/custom.db", SQLitePathSet: true}, "/srv/custom.db", false},
		{"after setup it is the recorded file", recorded, config.Config{SQLitePath: "/data/pulse.db"}, "/data/pulse.db", false},
		{"PULSE_DB naming the recorded file is quiet", recorded, config.Config{SQLitePath: "/data/./pulse.db", SQLitePathSet: true}, "/data/pulse.db", false},
		{"PULSE_DB naming another file keeps the recorded one and warns", recorded, config.Config{SQLitePath: "/data/custom.db", SQLitePathSet: true}, "/data/pulse.db", true},
		{"a record without a path falls back to the configured file", bootstrap.Config{Configured: true}, config.Config{SQLitePath: "/data/pulse.db"}, "/data/pulse.db", false},
		{"a file chosen in the wizard before #61 stays in use", bootstrap.Config{Configured: true, SQLitePath: "/srv/chosen.db"}, config.Config{SQLitePath: "/data/pulse.db"}, "/srv/chosen.db", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLog(t)
			if got := databaseFile("/data", tc.boot, tc.cfg); got != tc.want {
				t.Errorf("databaseFile = %q, want %q", got, tc.want)
			}
			if warned := strings.Contains(logs.String(), "Using the recorded file"); warned != tc.warn {
				t.Errorf("warned = %v, want %v. Logs: %q", warned, tc.warn, logs.String())
			}
		})
	}
}

// The setup wizard creates the database where the server and reset-password
// later look for it, for each way of naming that file.
func TestSetupAndResetPasswordAgree(t *testing.T) {
	cases := []struct {
		name string
		env  func(dir string) map[string]string
		want func(dir string) string
	}{
		{
			"PULSE_DB is the full path, outside PULSE_DATA_DIR",
			func(dir string) map[string]string {
				return map[string]string{"PULSE_DB": filepath.Join(dir, "db", "custom.db"), "PULSE_DATA_DIR": filepath.Join(dir, "state")}
			},
			func(dir string) string { return filepath.Join(dir, "db", "custom.db") },
		},
		{
			"PULSE_DB alone",
			func(dir string) map[string]string {
				return map[string]string{"PULSE_DB": filepath.Join(dir, "custom.db")}
			},
			func(dir string) string { return filepath.Join(dir, "custom.db") },
		},
		{
			"old SQLITE_PATH alone",
			func(dir string) map[string]string {
				return map[string]string{"SQLITE_PATH": filepath.Join(dir, "old.db")}
			},
			func(dir string) string { return filepath.Join(dir, "old.db") },
		},
		{
			"PULSE_DATA_DIR alone, as the image sets it",
			func(dir string) map[string]string { return map[string]string{"PULSE_DATA_DIR": dir} },
			func(dir string) string { return filepath.Join(dir, "pulse.db") },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearDBEnv(t)
			captureLog(t)
			dir := t.TempDir()
			for k, v := range tc.env(dir) {
				t.Setenv(k, v)
			}
			want := tc.want(dir)

			// The wizard shows the file the server chose, and setup creates it.
			t.Setenv("PULSE_SETUP_TOKEN", "off")
			h := server.New(app.New(), config.DataDir(), config.Load())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/setup/state", nil))
			var st struct {
				Path string `json:"sqlite_path"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
				t.Fatal(err)
			}
			if st.Path != want {
				t.Fatalf("wizard offers %q, want %q", st.Path, want)
			}
			body, _ := json.Marshal(map[string]string{"username": "admin", "password": "original-pass"})
			rec = httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body)))
			if rec.Code != http.StatusCreated {
				t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
			}
			if _, err := os.Stat(want); err != nil {
				t.Fatalf("setup did not create %s: %v", want, err)
			}

			// The server opens the same file on the next start.
			boot, err := bootstrap.Load(config.DataDir())
			if err != nil {
				t.Fatal(err)
			}
			if got := databaseFile(config.DataDir(), boot, config.Load()); got != want {
				t.Fatalf("serve would open %q, want %q", got, want)
			}

			// reset-password changes the password in that file.
			if err := resetPassword([]string{"--username", "admin", "--password", "brand-new-pass"}); err != nil {
				t.Fatal(err)
			}
			assertPassword(t, want, "admin", "brand-new-pass")
		})
	}
}

// Through the real router, a fresh server with no PULSE_SETUP_TOKEN logs a
// token and refuses setup without it.
func TestFreshServerRequiresSetupToken(t *testing.T) {
	clearDBEnv(t)
	t.Setenv("PULSE_SETUP_TOKEN", "")
	logs := captureLog(t)
	dir := t.TempDir()
	t.Setenv("PULSE_DATA_DIR", dir)
	a := app.New()
	h := server.New(a, config.DataDir(), config.Load())

	const prefix = "open /setup and enter this token: "
	i := strings.Index(logs.String(), prefix)
	if i < 0 {
		t.Fatalf("no setup token in the log: %q", logs.String())
	}
	token := strings.TrimSpace(strings.SplitN(logs.String()[i+len(prefix):], "\n", 2)[0])

	post := func(body map[string]string) int {
		b, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(b)))
		return rec.Code
	}
	if code := post(map[string]string{"username": "admin", "password": "original-pass"}); code != http.StatusUnauthorized {
		t.Fatalf("setup without the token = %d, want 401", code)
	}
	if code := post(map[string]string{"username": "admin", "password": "original-pass", "setup_token": token}); code != http.StatusCreated {
		t.Fatalf("setup with the token = %d, want 201", code)
	}
	t.Cleanup(func() { _ = a.DB().Close() })
	if _, err := os.Stat(filepath.Join(dir, "pulse.db")); err != nil {
		t.Fatalf("setup did not create the server's file: %v", err)
	}
}

// An install set up before PULSE_DB was a full path keeps the database it has:
// with SQLITE_PATH=<dir>/custom.db the wizard used to create <dir>/pulse.db.
func TestExistingInstallKeepsRecordedDatabase(t *testing.T) {
	clearDBEnv(t)
	logs := captureLog(t)
	dir := t.TempDir()
	recorded := filepath.Join(dir, "pulse.db")
	createAdmin(t, recorded, "admin", "original-pass")
	if err := bootstrap.Save(dir, bootstrap.Config{Configured: true, SQLitePath: recorded}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SQLITE_PATH", filepath.Join(dir, "custom.db"))

	if err := resetPassword([]string{"--username", "admin", "--password", "brand-new-pass"}); err != nil {
		t.Fatal(err)
	}
	assertPassword(t, recorded, "admin", "brand-new-pass")
	if _, err := os.Stat(filepath.Join(dir, "custom.db")); !os.IsNotExist(err) {
		t.Errorf("reset-password created the PULSE_DB file instead of using the recorded one (stat err %v)", err)
	}
	if !strings.Contains(logs.String(), "Using the recorded file") {
		t.Errorf("expected a warning about the differing PULSE_DB, logs: %q", logs.String())
	}
}

func TestResetPasswordErrors(t *testing.T) {
	clearDBEnv(t)
	if err := resetPassword([]string{"--username", "admin", "--password", "short"}); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("short password: err = %v, want usage", err)
	}
	if err := resetPassword([]string{"--bogus"}); err == nil {
		t.Error("unknown flag: want an error")
	}
	if err := resetPassword([]string{"-h"}); err != nil {
		t.Errorf("-h: err = %v, want nil", err)
	}

	dir := t.TempDir()
	t.Setenv("PULSE_DATA_DIR", dir)
	if err := os.WriteFile(bootstrap.Path(dir), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resetPassword([]string{"--username", "admin", "--password", "brand-new-pass"}); err == nil || !strings.Contains(err.Error(), "bootstrap") {
		t.Errorf("corrupt pulse.json: err = %v, want a bootstrap error", err)
	}

	// A directory is not a database file.
	if err := os.Remove(bootstrap.Path(dir)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PULSE_DB", dir)
	if err := resetPassword([]string{"--username", "admin", "--password", "brand-new-pass"}); err == nil || !strings.Contains(err.Error(), "db") {
		t.Errorf("directory as database: err = %v, want a db error", err)
	}
}

func createAdmin(t *testing.T, path, username, password string) {
	t.Helper()
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generated.New(conn).CreateUser(context.Background(), generated.CreateUserParams{Username: username, PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertPassword(t *testing.T, path, username, password string) {
	t.Helper()
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	}()
	u, err := generated.New(conn).GetUserByUsername(context.Background(), username)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := auth.VerifyPassword(password, u.PasswordHash); err != nil || !ok {
		t.Fatalf("password for %s in %s does not validate: ok=%v err=%v", username, path, ok, err)
	}
}
