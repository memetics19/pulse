package bootstrap

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config is the pre-database bootstrap state, stored as JSON on disk so the
// binary knows whether setup has run and which SQLite file to open.
type Config struct {
	Configured bool   `json:"configured"`
	SQLitePath string `json:"sqlite_path"`
}

func file(dataDir string) string { return filepath.Join(dataDir, "pulse.json") }

// Load reads the bootstrap config; a missing file yields a zero Config (unconfigured).
func Load(dataDir string) (Config, error) {
	b, err := os.ReadFile(file(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Save writes the bootstrap config, creating the data dir if needed.
func Save(dataDir string, c Config) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file(dataDir), b, 0o600)
}
