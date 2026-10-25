package config

import (
	"bytes"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvBool(t *testing.T) {
	cases := map[string]bool{"1": true, "true": true, "TRUE": true, "yes": true, "YeS": true,
		"0": false, "false": false, "no": false, "": false, "nope": false}
	for v, want := range cases {
		t.Setenv("X_BOOL", v)
		if got := envBool("X_BOOL"); got != want {
			t.Errorf("envBool(%q)=%v want %v", v, got, want)
		}
	}
}

func TestEnvList(t *testing.T) {
	t.Setenv("X_LIST", " a , b ,,c, ")
	got := envList("X_LIST")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("envList=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("envList[%d]=%q want %q", i, got[i], want[i])
		}
	}
	t.Setenv("X_LIST", "")
	if got := envList("X_LIST"); len(got) != 0 {
		t.Errorf("empty envList should be nil/empty, got %v", got)
	}
}

func TestParseCIDRs(t *testing.T) {
	nets := parseCIDRs([]string{"10.0.0.0/8", "192.168.1.5", "::1", "not-a-cidr", "8.8.8.8/33"})
	// valid: /8, bare IPv4 -> /32, bare IPv6 -> /128. invalid two are skipped.
	if len(nets) != 3 {
		t.Fatalf("parseCIDRs kept %d nets, want 3: %v", len(nets), nets)
	}
	if !nets[0].Contains(mustIP(t, "10.9.9.9")) {
		t.Error("10.0.0.0/8 should contain 10.9.9.9")
	}
	if nets[1].Contains(mustIP(t, "192.168.1.6")) {
		t.Error("bare 192.168.1.5 should be a /32, not contain .6")
	}
}

func TestLoadDefaultsAndEnv(t *testing.T) {
	clearRenamed(t)
	if c := Load(); c.Port != "8080" {
		t.Errorf("default port = %q, want 8080", c.Port)
	}
	t.Setenv("PULSE_PORT", "9999")
	t.Setenv("PULSE_DB", "/tmp/x.db")
	t.Setenv("PULSE_ALLOW_PRIVATE_MONITORS", "true")
	t.Setenv("PULSE_CORS_ORIGINS", "https://a.com,https://b.com")
	t.Setenv("PULSE_TRUSTED_PROXIES", "10.0.0.0/8")
	t.Setenv("PULSE_SETUP_TOKEN", " a-setup-token ")
	c := Load()
	if c.Port != "9999" || c.SQLitePath != "/tmp/x.db" || !c.AllowPrivateMonitors ||
		len(c.CORSOrigins) != 2 || len(c.TrustedProxies) != 1 || c.SetupToken != "a-setup-token" {
		t.Fatalf("Load did not populate config from env: %+v", c)
	}
}

// clearRenamed empties every renamed setting, old and new, and forgets which
// deprecation warnings were logged, so each test starts from a clean slate.
func clearRenamed(t *testing.T) {
	t.Helper()
	for newName, old := range renamed {
		t.Setenv(newName, "")
		t.Setenv(old, "")
	}
	for _, name := range []string{"PULSE_DATA_DIR", "PULSE_DATABASE_URL", "DATABASE_URL", "PULSE_SETUP_TOKEN"} {
		t.Setenv(name, "")
	}
	warned.Clear()
	t.Cleanup(func() { warned.Clear() })
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

func TestLoadNewNames(t *testing.T) {
	clearRenamed(t)
	logs := captureLog(t)
	t.Setenv("PULSE_PORT", "9090")
	t.Setenv("PULSE_DB", "/srv/pulse.db")
	t.Setenv("PULSE_DATABASE_URL", "postgres://db/pulse")
	t.Setenv("PULSE_EMAIL_FROM", "Pulse <alerts@example.com>")
	c := Load()
	if c.Port != "9090" || c.SQLitePath != "/srv/pulse.db" || c.DatabaseURL != "postgres://db/pulse" ||
		c.EmailFrom != "Pulse <alerts@example.com>" {
		t.Fatalf("Load ignored the PULSE_* names: %+v", c)
	}
	if logs.Len() != 0 {
		t.Errorf("new names should not warn, got %q", logs.String())
	}
}

func TestLoadFallsBackToOldNames(t *testing.T) {
	clearRenamed(t)
	logs := captureLog(t)
	t.Setenv("API_PORT", "7070")
	t.Setenv("SQLITE_PATH", "/old/pulse.db")
	c := Load()
	if c.Port != "7070" || c.SQLitePath != "/old/pulse.db" || !c.SQLitePathSet {
		t.Fatalf("Load did not fall back to the old names: %+v", c)
	}
	for newName, old := range renamed {
		want := old + " is deprecated, use " + newName + "; the old name stops working in 4.0\n"
		if n := strings.Count(logs.String(), want); n != 1 {
			t.Errorf("warning %q logged %d times, want 1. Logs: %q", want, n, logs.String())
		}
	}
}

func TestNewNameWinsOverOldName(t *testing.T) {
	clearRenamed(t)
	logs := captureLog(t)
	t.Setenv("PULSE_PORT", "9090")
	t.Setenv("API_PORT", "7070")
	t.Setenv("PULSE_DB", "/new/pulse.db")
	t.Setenv("SQLITE_PATH", "/old/pulse.db")
	t.Setenv("PULSE_DATABASE_URL", "postgres://new/pulse")
	t.Setenv("DATABASE_URL", "postgres://old/pulse")
	c := Load()
	if c.Port != "9090" || c.SQLitePath != "/new/pulse.db" || c.DatabaseURL != "postgres://new/pulse" {
		t.Fatalf("new names should win: %+v", c)
	}
	if logs.Len() != 0 {
		t.Errorf("an unused old name should not warn, got %q", logs.String())
	}
}

func TestDeprecationWarningLoggedOnce(t *testing.T) {
	clearRenamed(t)
	logs := captureLog(t)
	t.Setenv("API_PORT", "7070")
	Load()
	Load()
	if got := Env("PULSE_PORT"); got != "7070" {
		t.Fatalf("Env(PULSE_PORT)=%q want 7070", got)
	}
	want := "API_PORT is deprecated, use PULSE_PORT; the old name stops working in 4.0\n"
	if logs.String() != want {
		t.Errorf("logs = %q, want exactly one %q", logs.String(), want)
	}
}

func TestEnvNotRenamed(t *testing.T) {
	clearRenamed(t)
	t.Setenv("PULSE_DATA_DIR", "/var/lib/pulse")
	if got := Env("PULSE_DATA_DIR"); got != "/var/lib/pulse" {
		t.Errorf("Env(PULSE_DATA_DIR)=%q want /var/lib/pulse", got)
	}
	t.Setenv("PULSE_DATA_DIR", "")
	if got := Env("PULSE_DATA_DIR"); got != "" {
		t.Errorf("unset setting should be empty, got %q", got)
	}
}

// A platform such as Heroku, Railway or Dokku injects DATABASE_URL on its own.
// Pulse must neither read it nor warn about it. PULSE_DATABASE_URL still works.
func TestPlatformDatabaseURLIgnored(t *testing.T) {
	clearRenamed(t)
	logs := captureLog(t)
	t.Setenv("DATABASE_URL", "postgres://platform/db")
	if c := Load(); c.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty: DATABASE_URL is not a Pulse setting", c.DatabaseURL)
	}
	if logs.Len() != 0 {
		t.Errorf("DATABASE_URL should not warn, got %q", logs.String())
	}
	t.Setenv("PULSE_DATABASE_URL", "postgres://pulse/db")
	if c := Load(); c.DatabaseURL != "postgres://pulse/db" {
		t.Errorf("DatabaseURL = %q, want the PULSE_DATABASE_URL value", c.DatabaseURL)
	}
}

func TestDataDir(t *testing.T) {
	cases := []struct {
		name, dataDir, db, sqlitePath, want string
	}{
		{"PULSE_DATA_DIR wins", "/custom", "/var/lib/pulse/pulse.db", "", "/custom"},
		{"directory of PULSE_DB", "", "/var/lib/pulse/custom.db", "", "/var/lib/pulse"},
		{"directory of the old SQLITE_PATH", "", "", "/srv/old/pulse.db", "/srv/old"},
		{"default", "", "", "", "/data"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearRenamed(t)
			captureLog(t)
			t.Setenv("PULSE_DATA_DIR", tc.dataDir)
			t.Setenv("PULSE_DB", tc.db)
			t.Setenv("SQLITE_PATH", tc.sqlitePath)
			if got := DataDir(); got != tc.want {
				t.Errorf("DataDir() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDBPathPrecedence(t *testing.T) {
	cases := []struct {
		name, dataDir, db, sqlitePath, want string
		set                                 bool
	}{
		{"PULSE_DB is the full path", "", "/srv/pulse/custom.db", "", "/srv/pulse/custom.db", true},
		{"PULSE_DB wins over PULSE_DATA_DIR", "/data", "/srv/pulse/custom.db", "", "/srv/pulse/custom.db", true},
		{"PULSE_DB wins over SQLITE_PATH", "", "/new/pulse.db", "/old/pulse.db", "/new/pulse.db", true},
		{"old SQLITE_PATH is the full path", "", "", "/srv/old/custom.db", "/srv/old/custom.db", true},
		{"pulse.db in PULSE_DATA_DIR", "/var/lib/pulse", "", "", "/var/lib/pulse/pulse.db", false},
		{"default", "", "", "", "/data/pulse.db", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearRenamed(t)
			captureLog(t)
			t.Setenv("PULSE_DATA_DIR", tc.dataDir)
			t.Setenv("PULSE_DB", tc.db)
			t.Setenv("SQLITE_PATH", tc.sqlitePath)
			if got := DBPath(); got != tc.want {
				t.Errorf("DBPath() = %q, want %q", got, tc.want)
			}
			c := Load()
			if c.SQLitePath != tc.want || c.SQLitePathSet != tc.set {
				t.Errorf("Load() SQLitePath=%q SQLitePathSet=%v, want %q %v", c.SQLitePath, c.SQLitePathSet, tc.want, tc.set)
			}
		})
	}
}

// The settings each shipped way of running Pulse passes must resolve to the
// file those installs use today, pulse.db in their data directory, so an
// upgrade does not move anyone's database.
func TestShippedInstallsKeepTheirDatabase(t *testing.T) {
	cases := []struct {
		name            string
		env             map[string]string
		wantDir, wantDB string
	}{
		{"deb 2.1", map[string]string{"SQLITE_PATH": "/var/lib/pulse/pulse.db", "PULSE_DATA_DIR": "/var/lib/pulse"}, "/var/lib/pulse", "/var/lib/pulse/pulse.db"},
		{"deb 3.0", map[string]string{"PULSE_DB": "/var/lib/pulse/pulse.db", "PULSE_DATA_DIR": "/var/lib/pulse"}, "/var/lib/pulse", "/var/lib/pulse/pulse.db"},
		{"image 2.1", map[string]string{"SQLITE_PATH": "/data/pulse.db", "PULSE_DATA_DIR": "/data"}, "/data", "/data/pulse.db"},
		{"image 3.0", map[string]string{"PULSE_DATA_DIR": "/data"}, "/data", "/data/pulse.db"},
		{"image 3.0 with Compose PULSE_DB", map[string]string{"PULSE_DATA_DIR": "/data", "PULSE_DB": "/data/pulse.db"}, "/data", "/data/pulse.db"},
		{"homelab", map[string]string{"PULSE_DATA_DIR": "/data", "SQLITE_PATH": "/data/pulse.db"}, "/data", "/data/pulse.db"},
		{"installer", map[string]string{"SQLITE_PATH": "/home/u/.pulse/pulse.db", "PULSE_DATA_DIR": "/home/u/.pulse"}, "/home/u/.pulse", "/home/u/.pulse/pulse.db"},
		{"bare binary", map[string]string{}, "/data", "/data/pulse.db"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearRenamed(t)
			captureLog(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := DataDir(); got != tc.wantDir {
				t.Errorf("DataDir() = %q, want %q", got, tc.wantDir)
			}
			if got := DBPath(); got != tc.wantDB {
				t.Errorf("DBPath() = %q, want %q", got, tc.wantDB)
			}
		})
	}
}

func mustIP(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s)
	if ip == nil {
		t.Fatalf("bad test IP %q", s)
	}
	return ip
}

func TestLoadSMTP(t *testing.T) {
	t.Setenv("PULSE_SMTP_HOST", " smtp.example.com ")
	t.Setenv("PULSE_SMTP_PORT", "2525")
	t.Setenv("PULSE_SMTP_USER", "pulse")
	t.Setenv("PULSE_SMTP_PASSWORD", "")
	t.Setenv("PULSE_SMTP_TLS", " TLS ")
	file := filepath.Join(t.TempDir(), "smtp_password")
	if err := os.WriteFile(file, []byte("from-file\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PULSE_SMTP_PASSWORD_FILE", file)
	got := Load().SMTP
	want := SMTP{Host: "smtp.example.com", Port: "2525", User: "pulse", Password: "from-file", TLS: "tls"}
	if got != want {
		t.Fatalf("SMTP = %+v, want %+v", got, want)
	}

	// PULSE_SMTP_PASSWORD wins over the file.
	t.Setenv("PULSE_SMTP_PASSWORD", "from-env")
	if got := loadSMTP().Password; got != "from-env" {
		t.Errorf("password = %q, want the env value", got)
	}

	// An unreadable file is logged by name and leaves the password empty.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	t.Setenv("PULSE_SMTP_PASSWORD", "")
	t.Setenv("PULSE_SMTP_PASSWORD_FILE", filepath.Join(t.TempDir(), "missing"))
	if got := loadSMTP().Password; got != "" {
		t.Errorf("password = %q, want empty", got)
	}
	if !strings.Contains(buf.String(), "PULSE_SMTP_PASSWORD_FILE") {
		t.Errorf("log = %q, want the setting named", buf.String())
	}
}
