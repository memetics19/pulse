package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/bootstrap"
	"github.com/memetics19/pulse/api/internal/config"
)

func TestSetupStateAndComplete(t *testing.T) {
	dataDir := t.TempDir()
	a := app.New()
	h := NewSetup(a, dataDir, config.Config{})

	rec := httptest.NewRecorder()
	h.State(rec, httptest.NewRequest(http.MethodGet, "/api/setup/state", nil))
	var st struct {
		Configured bool `json:"configured"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if st.Configured {
		t.Fatal("fresh app: configured should be false")
	}

	body, _ := json.Marshal(map[string]any{
		"name": "Shreeda", "email": "s@b.c", "username": "admin", "password": "supersecret",
		"site_name": "Acme", "logo_url": "", "sqlite_path": dataDir + "/pulse.db",
	})
	rec = httptest.NewRecorder()
	h.Complete(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup complete = %d, want 201", rec.Code)
	}
	if !a.Configured() {
		t.Fatal("app should be configured after setup")
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("setup should log the admin in (session cookie)")
	}

	rec = httptest.NewRecorder()
	h.Complete(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", rec.Code)
	}
}

// With PULSE_DB set, the wizard offers that file, marks it fixed, creates it
// when the form leaves the path empty, and refuses a different one.
func TestSetupUsesPULSEDB(t *testing.T) {
	dataDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "sub", "custom.db")
	cfg := config.Config{SQLitePath: dbPath, SQLitePathSet: true}

	a := app.New()
	h := NewSetup(a, dataDir, cfg)
	rec := httptest.NewRecorder()
	h.State(rec, httptest.NewRequest(http.MethodGet, "/api/setup/state", nil))
	var st struct {
		Path  string `json:"default_sqlite_path"`
		Fixed bool   `json:"sqlite_path_fixed"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Path != dbPath || !st.Fixed {
		t.Fatalf("state = %+v, want %s fixed", st, dbPath)
	}

	other, _ := json.Marshal(map[string]any{"username": "admin", "password": "supersecret", "sqlite_path": filepath.Join(dataDir, "pulse.db")})
	rec = httptest.NewRecorder()
	h.Complete(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(other)))
	if rec.Code != http.StatusBadRequest || a.Configured() {
		t.Fatalf("a path other than PULSE_DB = %d (configured %v), want 400", rec.Code, a.Configured())
	}

	empty, _ := json.Marshal(map[string]any{"username": "admin", "password": "supersecret"})
	rec = httptest.NewRecorder()
	h.Complete(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(empty)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d %s, want 201", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("setup did not create PULSE_DB: %v", err)
	}
	boot, err := bootstrap.Load(dataDir)
	if err != nil || boot.SQLitePath != dbPath {
		t.Fatalf("pulse.json records %q (err %v), want %q", boot.SQLitePath, err, dbPath)
	}
}

// Without PULSE_DB the wizard offers pulse.db in the data directory, and the
// form may choose another file, as before.
func TestSetupDefaultsToDataDir(t *testing.T) {
	dataDir := t.TempDir()
	a := app.New()
	h := NewSetup(a, dataDir, config.Config{})
	rec := httptest.NewRecorder()
	h.State(rec, httptest.NewRequest(http.MethodGet, "/api/setup/state", nil))
	var st struct {
		Path  string `json:"default_sqlite_path"`
		Fixed bool   `json:"sqlite_path_fixed"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Path != filepath.Join(dataDir, "pulse.db") || st.Fixed {
		t.Fatalf("state = %+v, want %s/pulse.db not fixed", st, dataDir)
	}
	chosen := filepath.Join(dataDir, "chosen.db")
	body, _ := json.Marshal(map[string]any{"username": "admin", "password": "supersecret", "sqlite_path": chosen})
	rec = httptest.NewRecorder()
	h.Complete(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d %s, want 201", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(chosen); err != nil {
		t.Fatalf("setup did not create the chosen file: %v", err)
	}
}
