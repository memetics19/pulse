package bootstrap

import (
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
