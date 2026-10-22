package config

import (
	"net"
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
	t.Setenv("API_PORT", "")
	if c := Load(); c.Port != "8080" {
		t.Errorf("default port = %q, want 8080", c.Port)
	}
	t.Setenv("API_PORT", "9999")
	t.Setenv("SQLITE_PATH", "/tmp/x.db")
	t.Setenv("PULSE_ALLOW_PRIVATE_MONITORS", "true")
	t.Setenv("PULSE_CORS_ORIGINS", "https://a.com,https://b.com")
	t.Setenv("PULSE_TRUSTED_PROXIES", "10.0.0.0/8")
	c := Load()
	if c.Port != "9999" || c.SQLitePath != "/tmp/x.db" || !c.AllowPrivateMonitors ||
		len(c.CORSOrigins) != 2 || len(c.TrustedProxies) != 1 {
		t.Fatalf("Load did not populate config from env: %+v", c)
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
