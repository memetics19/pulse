package config

import (
	"log"
	"net"
	"os"
	"strings"
)

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
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}
	return Config{
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		SQLitePath:           os.Getenv("SQLITE_PATH"),
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
