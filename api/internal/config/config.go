package config

import (
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
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		SQLitePath:      os.Getenv("SQLITE_PATH"),
		ResendAPIKey:    os.Getenv("RESEND_API_KEY"),
		SlackWebhookURL: os.Getenv("SLACK_WEBHOOK_URL"),
		Port:            port,
		SecureCookies:   envBool("PULSE_SECURE_COOKIES"),
	}
}
