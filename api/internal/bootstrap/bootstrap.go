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

// Path returns the bootstrap file in dataDir.
func Path(dataDir string) string { return filepath.Join(dataDir, "pulse.json") }

// Load reads the bootstrap config; a missing file yields a zero Config (unconfigured).
func Load(dataDir string) (Config, error) {
	b, err := os.ReadFile(Path(dataDir))
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

// Save writes the bootstrap config, creating the data dir if needed. The file
// is replaced in one rename, so a crash never leaves a half-written pulse.json.
func Save(dataDir string, c Config) error {
	s, err := Stage(dataDir, c)
	if err != nil {
		return err
	}
	if err := s.Commit(); err != nil {
		s.Discard()
		return err
	}
	return nil
}

// Staged is a bootstrap config written to a temporary file next to
// pulse.json. Commit moves it into place. Discard removes it. Setup stages the
// file before it commits the admin, so a data directory that cannot be written
// fails setup before anything is stored, and pulse.json appears only after the
// admin is committed.
type Staged struct {
	tmp, dst string
}

// Stage writes c to a temporary file in dataDir, creating dataDir if needed.
func Stage(dataDir string, c Config) (*Staged, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dataDir, ".pulse.json-*")
	if err != nil {
		return nil, err
	}
	s := &Staged{tmp: f.Name(), dst: Path(dataDir)}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		s.Discard()
		return nil, err
	}
	return s, nil
}

// Commit renames the staged file to pulse.json.
func (s *Staged) Commit() error {
	return os.Rename(s.tmp, s.dst)
}

// Discard removes the staged file. It does nothing after Commit.
func (s *Staged) Discard() {
	_ = os.Remove(s.tmp)
}
