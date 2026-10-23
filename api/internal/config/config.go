package config

import (
	"log"
	"net"
	"os"
	"strings"
	"sync"
)

// renamed maps each setting renamed in 3.0 to the name it replaced. The old
// name is still read when the new one is unset, with a warning, until 4.0.
var renamed = map[string]string{
	"PULSE_PORT":         "API_PORT",
	"PULSE_DB":           "SQLITE_PATH",
	"PULSE_DATABASE_URL": "DATABASE_URL",
}

// warned records the old names already reported, so each warning is logged
// once per process even when settings are read more than once.
var warned sync.Map

// Env returns the value of the named setting. For a setting renamed in 3.0,
// an unset or empty new name falls back to the old name, and the first use of
// that old name logs a deprecation warning. The new name wins when both are set.
func Env(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	old, ok := renamed[name]
	if !ok {
		return ""
	}
	v := os.Getenv(old)
	if v != "" {
		if _, seen := warned.LoadOrStore(old, struct{}{}); !seen {
			log.Printf("%s is deprecated, use %s; the old name stops working in 4.0", old, name)
		}
	}
	return v
}

type Config struct {
	DatabaseURL     string
	SQLitePath      string
	ResendAPIKey    string
	SlackWebhookURL string
	Port            string
	SecureCookies   bool
	CORSOrigins     []string
	// AllowPrivateMonitors permits monitors to target private/internal
	// addresses (loopback, LAN, link-local). Required for homelab setups
	// that monitor LAN services; off by default to prevent SSRF.
	AllowPrivateMonitors bool
	// TrustedProxies are CIDRs of reverse proxies whose X-Forwarded-For header
	// may be trusted for the login rate limiter. Empty (default) means proxy
	// headers are ignored so they cannot be spoofed. Set when Pulse runs behind
	// a known proxy (e.g. the bundled Caddy) so per-client limiting still works.
	TrustedProxies []*net.IPNet
}

// envList splits the named environment variable on commas, trimming spaces
// and dropping empty entries.
func envList(name string) []string {
	var out []string
	for _, v := range strings.Split(os.Getenv(name), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// envBool reports whether the named environment variable is set to a truthy
// value ("1", "true", "yes", case-insensitive).
func envBool(name string) bool {
	switch strings.ToLower(os.Getenv(name)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func Load() Config {
	port := Env("PULSE_PORT")
	if port == "" {
		port = "8080"
	}
	return Config{
		DatabaseURL:          Env("PULSE_DATABASE_URL"),
		SQLitePath:           Env("PULSE_DB"),
		ResendAPIKey:         os.Getenv("RESEND_API_KEY"),
		SlackWebhookURL:      os.Getenv("SLACK_WEBHOOK_URL"),
		Port:                 port,
		SecureCookies:        envBool("PULSE_SECURE_COOKIES"),
		CORSOrigins:          envList("PULSE_CORS_ORIGINS"),
		AllowPrivateMonitors: envBool("PULSE_ALLOW_PRIVATE_MONITORS"),
		TrustedProxies:       parseCIDRs(envList("PULSE_TRUSTED_PROXIES")),
	}
}

// parseCIDRs converts CIDR strings (or bare IPs) into networks, skipping and
// logging any that don't parse rather than failing startup.
func parseCIDRs(entries []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, e := range entries {
		if !strings.Contains(e, "/") {
			if ip := net.ParseIP(e); ip != nil {
				if ip.To4() != nil {
					e += "/32"
				} else {
					e += "/128"
				}
			}
		}
		_, n, err := net.ParseCIDR(e)
		if err != nil {
			log.Printf("config: ignoring invalid PULSE_TRUSTED_PROXIES entry %q: %v", e, err)
			continue
		}
		nets = append(nets, n)
	}
	return nets
}
