package config

import (
	"bytes"
	"log"
	"net"
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
	c := Load()
	if c.Port != "9999" || c.SQLitePath != "/tmp/x.db" || !c.AllowPrivateMonitors ||
		len(c.CORSOrigins) != 2 || len(c.TrustedProxies) != 1 {
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
	c := Load()
	if c.Port != "9090" || c.SQLitePath != "/srv/pulse.db" || c.DatabaseURL != "postgres://db/pulse" {
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
	t.Setenv("DATABASE_URL", "postgres://old/pulse")
	c := Load()
	if c.Port != "7070" || c.SQLitePath != "/old/pulse.db" || c.DatabaseURL != "postgres://old/pulse" {
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

func mustIP(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s)
	if ip == nil {
		t.Fatalf("bad test IP %q", s)
	}
	return ip
}
