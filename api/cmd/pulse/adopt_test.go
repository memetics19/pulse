package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/bootstrap"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/db"
)

// runMainEnv makes the test binary run main() instead of the tests, so a test
// can start the real server in a child process.
const runMainEnv = "PULSE_TEST_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// startPulse runs main() in a child process with env added to the test's
// environment, and waits until it answers HTTP. It returns the base URL.
func startPulse(t *testing.T, env ...string) string {
	t.Helper()
	port := freePort(t)
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), append([]string{runMainEnv + "=1", "PULSE_PORT=" + port}, env...)...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		if t.Failed() {
			t.Logf("pulse output:\n%s", out.String())
		}
	})
	base := "http://localhost:" + port
	for range 250 {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			return base
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("pulse never came up. Output:\n%s", out.String())
	return ""
}

// Issue #72: a database that already has an admin, with no pulse.json next to
// it, used to start in setup mode, and setup answered 409 forever. The server
// now adopts it at startup: it serves it, records it in pulse.json, and the
// existing admin can log in.
func TestStartAdoptsDatabaseWithoutPulseJSON(t *testing.T) {
	clearDBEnv(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pulse.db")
	createAdmin(t, dbPath, "old", "original-pass")

	base := startPulse(t, "PULSE_DATA_DIR="+dir, "PULSE_DB="+dbPath, "PULSE_SETUP_TOKEN=off")

	resp, err := http.Get(base + "/api/setup/state")
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Configured bool `json:"configured"`
	}
	err = json.NewDecoder(resp.Body).Decode(&st)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Configured {
		t.Fatal("an existing database with an admin should start configured, not in setup mode")
	}

	boot, err := bootstrap.Load(dir)
	if err != nil || !boot.Configured || boot.SQLitePath != dbPath {
		t.Fatalf("pulse.json = %+v (err %v), want configured with %s", boot, err, dbPath)
	}

	body, _ := json.Marshal(map[string]string{"username": "old", "password": "original-pass"})
	resp, err = http.Post(base+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login as the existing admin = %d, want 200", resp.StatusCode)
	}
}

func TestAdoptDatabase(t *testing.T) {
	t.Run("no database file stays in setup mode and creates nothing", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "pulse.db")
		conn, err := adoptDatabase(dir, config.Config{SQLitePath: path})
		if err != nil || conn != nil {
			t.Fatalf("adoptDatabase = %v, %v, want nil, nil", conn, err)
		}
		for _, p := range []string{path, bootstrap.Path(dir)} {
			if _, err := os.Stat(p); !os.IsNotExist(err) {
				t.Errorf("%s was created (stat err %v)", p, err)
			}
		}
	})

	t.Run("a database without users stays in setup mode", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "pulse.db")
		empty, err := db.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := empty.Close(); err != nil {
			t.Fatal(err)
		}
		conn, err := adoptDatabase(dir, config.Config{SQLitePath: path})
		if err != nil || conn != nil {
			t.Fatalf("adoptDatabase = %v, %v, want nil, nil", conn, err)
		}
		if _, err := os.Stat(bootstrap.Path(dir)); !os.IsNotExist(err) {
			t.Errorf("pulse.json was written for a database without users (stat err %v)", err)
		}
	})

	t.Run("a database with an admin is adopted and recorded", func(t *testing.T) {
		logs := captureLog(t)
		dir := t.TempDir()
		path := filepath.Join(t.TempDir(), "kept.db")
		createAdmin(t, path, "old", "original-pass")
		conn, err := adoptDatabase(dir, config.Config{SQLitePath: path})
		if err != nil || conn == nil {
			t.Fatalf("adoptDatabase = %v, %v, want an open database", conn, err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		boot, err := bootstrap.Load(dir)
		if err != nil || !boot.Configured || boot.SQLitePath != path {
			t.Fatalf("pulse.json = %+v (err %v), want configured with %s", boot, err, path)
		}
		if !strings.Contains(logs.String(), "adopted the database "+path) {
			t.Errorf("adoption not logged: %q", logs.String())
		}
	})

	t.Run("an unwritable data directory still serves the database", func(t *testing.T) {
		logs := captureLog(t)
		notADir := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "pulse.db")
		createAdmin(t, path, "old", "original-pass")
		conn, err := adoptDatabase(filepath.Join(notADir, "data"), config.Config{SQLitePath: path})
		if err != nil || conn == nil {
			t.Fatalf("adoptDatabase = %v, %v, want an open database", conn, err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		if !strings.Contains(logs.String(), "adopts it again on the next start") {
			t.Errorf("the failed pulse.json write should be logged: %q", logs.String())
		}
	})

	t.Run("a path that is not a database is an error", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := adoptDatabase(dir, config.Config{SQLitePath: dir}); err == nil {
			t.Fatal("a directory as the database: want an error")
		}
	})

	t.Run("a path that cannot be checked is an error", func(t *testing.T) {
		notADir := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := adoptDatabase(t.TempDir(), config.Config{SQLitePath: filepath.Join(notADir, "pulse.db")}); err == nil {
			t.Fatal("a database under a regular file: want an error")
		}
	})
}

func TestOpenDatabase(t *testing.T) {
	captureLog(t)
	dir := t.TempDir()
	recorded := filepath.Join(dir, "recorded.db")
	createAdmin(t, recorded, "admin", "original-pass")

	// Recorded setup: the recorded file is opened, even if PULSE_DB differs.
	conn, err := openDatabase(dir, bootstrap.Config{Configured: true, SQLitePath: recorded},
		config.Config{SQLitePath: filepath.Join(dir, "other.db"), SQLitePathSet: true})
	if err != nil || conn == nil {
		t.Fatalf("openDatabase = %v, %v", conn, err)
	}
	_ = conn.Close()
	if _, err := os.Stat(filepath.Join(dir, "other.db")); !os.IsNotExist(err) {
		t.Errorf("openDatabase created the PULSE_DB file (stat err %v)", err)
	}

	// No setup and no database: setup mode.
	conn, err = openDatabase(t.TempDir(), bootstrap.Config{}, config.Config{SQLitePath: filepath.Join(t.TempDir(), "pulse.db")})
	if err != nil || conn != nil {
		t.Fatalf("fresh instance: openDatabase = %v, %v, want nil, nil", conn, err)
	}
}
