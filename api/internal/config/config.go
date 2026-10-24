package config

import (
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// renamed maps each setting renamed in 3.0 to the name it replaced. The old
// name is still read when the new one is unset, with a warning, until 4.0.
//
// PULSE_DATABASE_URL has no entry: its old name, DATABASE_URL, is a variable
// that hosting platforms (Heroku, Railway, Dokku) inject on their own, and
// nothing uses the value, so reading it only logged a misleading warning.
var renamed = map[string]string{
	"PULSE_PORT": "API_PORT",
	"PULSE_DB":   "SQLITE_PATH",
}

// DefaultDataDir is the data directory when neither PULSE_DATA_DIR nor
// PULSE_DB is set.
const DefaultDataDir = "/data"

// SetupTokenOff is the PULSE_SETUP_TOKEN value that lets setup run without a
// token, for automated installs that complete setup themselves.
const SetupTokenOff = "off"

// DBFileName is the name of the database file Pulse creates in the data
// directory when PULSE_DB is unset.
const DBFileName = "pulse.db"

// DataDir returns the directory that holds the setup state (pulse.json):
// PULSE_DATA_DIR, else the directory of PULSE_DB, else /data. Every release
// has used this rule, so an existing install finds its setup state, and
// through it the database file it was set up with.
func DataDir() string {
	if d := os.Getenv("PULSE_DATA_DIR"); d != "" {
		return d
	}
	if p := Env("PULSE_DB"); p != "" {
		return filepath.Dir(p)
	}
	return DefaultDataDir
}

// DBPath returns the SQLite database file for this instance: PULSE_DB as a
// full path, else pulse.db in PULSE_DATA_DIR, else /data/pulse.db.
func DBPath() string {
	if p := Env("PULSE_DB"); p != "" {
		return p
	}
	if d := os.Getenv("PULSE_DATA_DIR"); d != "" {
		return filepath.Join(d, DBFileName)
	}
	return filepath.Join(DefaultDataDir, DBFileName)
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
	DatabaseURL string
	// SQLitePath is the database file, resolved by DBPath.
	SQLitePath string
	// SQLitePathSet reports that PULSE_DB (or the old SQLITE_PATH) names the
	// file, so a warning can say when it differs from the recorded one.
	SQLitePathSet bool
	// SetupToken is PULSE_SETUP_TOKEN: the token the first-run setup
	// requires. Empty means the server generates one and logs it. The value
	// SetupTokenOff turns the check off.
	SetupToken      string
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
		SQLitePath:           DBPath(),
		SQLitePathSet:        Env("PULSE_DB") != "",
		SetupToken:           strings.TrimSpace(os.Getenv("PULSE_SETUP_TOKEN")),
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
