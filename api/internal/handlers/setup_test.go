package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/bootstrap"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/db"
)

// noToken is the config tests use when the setup token is not under test.
var noToken = config.Config{SetupToken: config.SetupTokenOff}

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

type setupState struct {
	Configured    bool    `json:"configured"`
	SQLitePath    *string `json:"sqlite_path"`
	TokenRequired *bool   `json:"setup_token_required"`
}

func getState(t *testing.T, h *Setup) (setupState, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.State(rec, httptest.NewRequest(http.MethodGet, "/api/setup/state", nil))
	raw := rec.Body.String()
	var st setupState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		t.Fatalf("decode state %q: %v", raw, err)
	}
	return st, raw
}

func postSetup(h *Setup, body map[string]any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	h.Complete(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(b)))
	return rec
}

func admin(extra map[string]any) map[string]any {
	body := map[string]any{"username": "admin", "password": "supersecret"}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func TestSetupStateAndComplete(t *testing.T) {
	dataDir := t.TempDir()
	a := app.New()
	h := NewSetup(a, dataDir, noToken)

	st, _ := getState(t, h)
	if st.Configured {
		t.Fatal("fresh app: configured should be false")
	}
	if st.SQLitePath == nil || *st.SQLitePath != filepath.Join(dataDir, "pulse.db") {
		t.Fatalf("state sqlite_path = %v, want %s/pulse.db", st.SQLitePath, dataDir)
	}
	if st.TokenRequired == nil || *st.TokenRequired {
		t.Fatalf("PULSE_SETUP_TOKEN=off: setup_token_required = %v, want false", st.TokenRequired)
	}

	body := admin(map[string]any{"name": "Shreeda", "email": "s@b.c", "site_name": "Acme", "logo_url": ""})
	rec := postSetup(h, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup complete = %d %s, want 201", rec.Code, rec.Body.String())
	}
	t.Cleanup(func() { _ = a.DB().Close() })
	if !a.Configured() {
		t.Fatal("app should be configured after setup")
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("setup should log the admin in (session cookie)")
	}

	if rec := postSetup(h, body); rec.Code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", rec.Code)
	}

	// After setup the state no longer describes the database file.
	st, raw := getState(t, h)
	if !st.Configured || st.SQLitePath != nil || st.TokenRequired != nil {
		t.Fatalf("configured state = %s, want only configured:true", raw)
	}
}

// With PULSE_DB set, setup creates that file and records it. A request that
// names any file, even the same one, is refused.
func TestSetupUsesPULSEDB(t *testing.T) {
	dataDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "sub", "custom.db")
	cfg := config.Config{SQLitePath: dbPath, SQLitePathSet: true, SetupToken: config.SetupTokenOff}

	a := app.New()
	h := NewSetup(a, dataDir, cfg)
	if st, _ := getState(t, h); st.SQLitePath == nil || *st.SQLitePath != dbPath {
		t.Fatalf("state sqlite_path = %v, want %s", st.SQLitePath, dbPath)
	}

	other := filepath.Join(dataDir, "pulse.db")
	for _, p := range []string{other, dbPath} {
		if rec := postSetup(h, admin(map[string]any{"sqlite_path": p})); rec.Code != http.StatusBadRequest || a.Configured() {
			t.Fatalf("sqlite_path %s = %d (configured %v), want 400", p, rec.Code, a.Configured())
		}
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("a refused request created %s (stat err %v)", other, err)
	}

	if rec := postSetup(h, admin(nil)); rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d %s, want 201", rec.Code, rec.Body.String())
	}
	t.Cleanup(func() { _ = a.DB().Close() })
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("setup did not create PULSE_DB: %v", err)
	}
	boot, err := bootstrap.Load(dataDir)
	if err != nil || boot.SQLitePath != dbPath {
		t.Fatalf("pulse.json records %q (err %v), want %q", boot.SQLitePath, err, dbPath)
	}
}

// Without PULSE_DB the request still cannot choose the file: the server uses
// pulse.db in the data directory.
func TestSetupRequestCannotChoosePath(t *testing.T) {
	dataDir := t.TempDir()
	chosen := filepath.Join(t.TempDir(), "elsewhere", "chosen.db")
	a := app.New()
	h := NewSetup(a, dataDir, noToken)

	rec := postSetup(h, admin(map[string]any{"sqlite_path": chosen}))
	if rec.Code != http.StatusBadRequest || a.Configured() {
		t.Fatalf("sqlite_path in the request = %d (configured %v), want 400", rec.Code, a.Configured())
	}
	if !strings.Contains(rec.Body.String(), "PULSE_DB or PULSE_DATA_DIR") {
		t.Errorf("400 should say where the path comes from, got %q", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Dir(chosen)); !os.IsNotExist(err) {
		t.Fatalf("a refused request created %s (stat err %v)", filepath.Dir(chosen), err)
	}
	if _, err := os.Stat(bootstrap.Path(dataDir)); !os.IsNotExist(err) {
		t.Fatalf("a refused request wrote pulse.json (stat err %v)", err)
	}

	if rec := postSetup(h, admin(nil)); rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d %s, want 201", rec.Code, rec.Body.String())
	}
	t.Cleanup(func() { _ = a.DB().Close() })
	want := filepath.Join(dataDir, "pulse.db")
	boot, err := bootstrap.Load(dataDir)
	if err != nil || boot.SQLitePath != want {
		t.Fatalf("pulse.json records %q (err %v), want %q", boot.SQLitePath, err, want)
	}
}

// Without PULSE_SETUP_TOKEN, a fresh instance generates a token, logs it once
// and requires it. The state says a token is needed but never shows it.
func TestSetupGeneratedToken(t *testing.T) {
	logs := captureLog(t)
	a := app.New()
	h := NewSetup(a, t.TempDir(), config.Config{})

	const prefix = "open /setup and enter this token: "
	if n := strings.Count(logs.String(), prefix); n != 1 {
		t.Fatalf("token logged %d times, want 1. Logs: %q", n, logs.String())
	}
	line := logs.String()[strings.Index(logs.String(), prefix)+len(prefix):]
	token := strings.TrimSpace(strings.SplitN(line, "\n", 2)[0])
	if len(token) < 20 {
		t.Fatalf("generated token %q is too short", token)
	}
	if other := NewSetup(app.New(), t.TempDir(), config.Config{}); other.token == token {
		t.Fatal("two servers generated the same token")
	}

	st, raw := getState(t, h)
	if st.TokenRequired == nil || !*st.TokenRequired {
		t.Fatalf("state = %s, want setup_token_required true", raw)
	}
	if strings.Contains(raw, token) {
		t.Fatalf("state leaks the token: %s", raw)
	}

	for name, body := range map[string]map[string]any{
		"no token":     admin(nil),
		"empty token":  admin(map[string]any{"setup_token": ""}),
		"wrong token":  admin(map[string]any{"setup_token": "not-the-token"}),
		"token prefix": admin(map[string]any{"setup_token": token[:len(token)-1]}),
		"token+suffix": admin(map[string]any{"setup_token": token + "x"}),
	} {
		rec := postSetup(h, body)
		if rec.Code != http.StatusUnauthorized || a.Configured() {
			t.Fatalf("%s = %d (configured %v), want 401", name, rec.Code, a.Configured())
		}
		if strings.Contains(rec.Body.String(), token) {
			t.Fatalf("%s: the reply leaks the token", name)
		}
	}

	// The token check comes before any other answer, so a caller without it
	// learns nothing about the path rule either.
	if rec := postSetup(h, admin(map[string]any{"sqlite_path": "/tmp/x.db"})); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sqlite_path without a token = %d, want 401", rec.Code)
	}

	// A token pasted with surrounding spaces still works.
	rec := postSetup(h, admin(map[string]any{"setup_token": "  " + token + "\n"}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("right token = %d %s, want 201", rec.Code, rec.Body.String())
	}
	t.Cleanup(func() { _ = a.DB().Close() })
	if n := strings.Count(logs.String(), token); n != 1 {
		t.Errorf("token appears %d times in the log, want 1", n)
	}
}

// PULSE_SETUP_TOKEN sets the token. Its value never reaches the log.
func TestSetupTokenFromEnv(t *testing.T) {
	logs := captureLog(t)
	const token = "operator-chosen-setup-token"
	a := app.New()
	h := NewSetup(a, t.TempDir(), config.Config{SetupToken: token})
	if strings.Contains(logs.String(), token) {
		t.Fatalf("PULSE_SETUP_TOKEN value logged: %q", logs.String())
	}
	if !strings.Contains(logs.String(), "PULSE_SETUP_TOKEN") {
		t.Errorf("startup should say setup needs PULSE_SETUP_TOKEN, logs: %q", logs.String())
	}
	if st, _ := getState(t, h); st.TokenRequired == nil || !*st.TokenRequired {
		t.Fatal("state should say a token is required")
	}
	if rec := postSetup(h, admin(map[string]any{"setup_token": "wrong"})); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token = %d, want 401", rec.Code)
	}
	if rec := postSetup(h, admin(map[string]any{"setup_token": token})); rec.Code != http.StatusCreated {
		t.Fatalf("right token = %d %s, want 201", rec.Code, rec.Body.String())
	}
	t.Cleanup(func() { _ = a.DB().Close() })
}

// PULSE_SETUP_TOKEN=off (any case) turns the check off and says so in the log.
func TestSetupTokenOff(t *testing.T) {
	logs := captureLog(t)
	a := app.New()
	h := NewSetup(a, t.TempDir(), config.Config{SetupToken: "OFF"})
	if !strings.Contains(logs.String(), "setup token: off") {
		t.Errorf("turning the token off should be logged, logs: %q", logs.String())
	}
	if st, _ := getState(t, h); st.TokenRequired == nil || *st.TokenRequired {
		t.Fatal("state should say no token is required")
	}
	if rec := postSetup(h, admin(nil)); rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d %s, want 201", rec.Code, rec.Body.String())
	}
	t.Cleanup(func() { _ = a.DB().Close() })
}

func TestTokenMatches(t *testing.T) {
	cases := []struct {
		want, got string
		match     bool
	}{
		{"abc123", "abc123", true},
		{"abc123", "abc124", false},
		{"abc123", "abc12", false},
		{"abc123", "abc1234", false},
		{"abc123", "", false},
		{"abc123", "ABC123", false},
	}
	for _, c := range cases {
		if got := tokenMatches(c.want, c.got); got != c.match {
			t.Errorf("tokenMatches(%q, %q) = %v, want %v", c.want, c.got, got, c.match)
		}
	}
}

// An install that is already set up keeps its recorded database: setup
// generates no token, says nothing about paths, and changes nothing on disk.
func TestSetupLeavesConfiguredInstallAlone(t *testing.T) {
	logs := captureLog(t)
	dataDir := t.TempDir()
	recorded := filepath.Join(t.TempDir(), "chosen-by-the-old-wizard.db")
	conn, err := db.Open(recorded)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := bootstrap.Save(dataDir, bootstrap.Config{Configured: true, SQLitePath: recorded}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(bootstrap.Path(dataDir))
	if err != nil {
		t.Fatal(err)
	}

	a := app.New()
	a.SetDB(conn)
	h := NewSetup(a, dataDir, config.Config{SQLitePath: filepath.Join(dataDir, "pulse.db")})
	if logs.Len() != 0 {
		t.Errorf("a configured install should log nothing about setup, got %q", logs.String())
	}
	if _, raw := getState(t, h); strings.Contains(raw, "sqlite_path") || strings.Contains(raw, "token") {
		t.Errorf("configured state = %s, want only configured", raw)
	}
	for _, body := range []map[string]any{admin(nil), admin(map[string]any{"sqlite_path": filepath.Join(dataDir, "new.db")})} {
		if rec := postSetup(h, body); rec.Code != http.StatusConflict {
			t.Fatalf("setup on a configured install = %d, want 409", rec.Code)
		}
	}
	after, err := os.ReadFile(bootstrap.Path(dataDir))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("pulse.json changed (err %v): %s", err, after)
	}
	for _, p := range []string{filepath.Join(dataDir, "pulse.db"), filepath.Join(dataDir, "new.db")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("setup created %s on a configured install (stat err %v)", p, err)
		}
	}
	if a.DB() != conn {
		t.Error("setup replaced the open database")
	}
}

// Failures on the server answer with a generic message and log the detail.
func TestSetupErrorsAreGeneric(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		dataDir, dbPath, detail string
	}{
		"data directory cannot be created": {t.TempDir(), filepath.Join(notADir, "db.sqlite"), "create the data directory"},
		"database cannot be opened":        {t.TempDir(), t.TempDir(), "open the database"},
		"pulse.json cannot be saved":       {filepath.Join(notADir, "data"), filepath.Join(t.TempDir(), "pulse.db"), "save "},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			logs := captureLog(t)
			h := NewSetup(app.New(), c.dataDir, config.Config{SQLitePath: c.dbPath, SetupToken: config.SetupTokenOff})
			rec := postSetup(h, admin(nil))
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("code = %d, want 500", rec.Code)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != setupFailed {
				t.Fatalf("body = %q, want %q", got, setupFailed)
			}
			if !strings.Contains(logs.String(), "setup: "+c.detail) {
				t.Errorf("detail %q not logged: %q", c.detail, logs.String())
			}
		})
	}
}
