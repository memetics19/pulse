package main

import "testing"

func TestRun(t *testing.T) {
	if code := run([]string{"--help"}); code != 0 {
		t.Errorf("--help exit code = %d, want 0", code)
	}
	if code := run([]string{"import", "uptime-kuma"}); code == 0 {
		t.Error("import without required flags should return non-zero")
	}
	if code := run([]string{"no-such-command"}); code == 0 {
		t.Error("unknown command should return non-zero")
	}
}
