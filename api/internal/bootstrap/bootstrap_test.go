package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if c, _ := Load(dir); c.Configured {
		t.Fatal("fresh dir must be unconfigured")
	}
	want := Config{Configured: true, SQLitePath: filepath.Join(dir, "pulse.db")}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Configured || got.SQLitePath != want.SQLitePath {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestSaveReplacesAndLeavesNoStagedFile(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, Config{Configured: true, SQLitePath: "/old.db"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, Config{Configured: true, SQLitePath: "/new.db"}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || got.SQLitePath != "/new.db" {
		t.Fatalf("after a second save: %+v (err %v)", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "pulse.json" {
		t.Fatalf("data dir holds %v, want only pulse.json", entries)
	}
	info, err := os.Stat(Path(dir))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("pulse.json mode = %v (err %v), want 0600", info.Mode().Perm(), err)
	}
}

func TestStageDiscardWritesNothing(t *testing.T) {
	dir := t.TempDir()
	s, err := Stage(dir, Config{Configured: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Path(dir)); !os.IsNotExist(err) {
		t.Fatalf("Stage wrote pulse.json before Commit (stat err %v)", err)
	}
	s.Discard()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("Discard left %v", entries)
	}
}

func TestSaveErrors(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(filepath.Join(notADir, "data"), Config{}); err == nil {
		t.Error("a data dir under a regular file: want an error")
	}

	// A non-empty directory in place of pulse.json makes the rename fail,
	// and the staged file is removed again.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(Path(dir), "blocker"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, Config{}); err == nil {
		t.Error("pulse.json is a directory: want an error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("a failed save left %v", entries)
	}
}
