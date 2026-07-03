package config

import "os"

type Config struct {
	DatabaseURL     string
	SQLitePath      string
	ResendAPIKey    string
	SlackWebhookURL string
	Port            string
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
	}
}
