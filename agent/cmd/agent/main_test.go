package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func writeTokenFile(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "tok")
	if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestResolveToken(t *testing.T) {
	t.Run("file wins over env", func(t *testing.T) {
		t.Setenv("PULSE_AGENT_TOKEN", "env-token")
		got, err := resolveToken(writeTokenFile(t, "  file-token\n"))
		if err != nil || got != "file-token" {
			t.Fatalf("got %q, %v; want file-token", got, err)
		}
	})

	t.Run("env fallback", func(t *testing.T) {
		t.Setenv("PULSE_AGENT_TOKEN", " env-token\n")
		got, err := resolveToken("")
		if err != nil || got != "env-token" {
			t.Fatalf("got %q, %v; want env-token", got, err)
		}
	})

	t.Run("no source is empty", func(t *testing.T) {
		t.Setenv("PULSE_AGENT_TOKEN", "")
		got, err := resolveToken("")
		if err != nil || got != "" {
			t.Fatalf("got %q, %v; want empty", got, err)
		}
	})

	t.Run("missing file errors", func(t *testing.T) {
		t.Setenv("PULSE_AGENT_TOKEN", "env-token")
		if _, err := resolveToken("/no/such/token/file"); err == nil {
			t.Fatal("expected error for missing token file")
		}
	})

	t.Run("empty file errors instead of falling back to env", func(t *testing.T) {
		t.Setenv("PULSE_AGENT_TOKEN", "env-token")
		_, err := resolveToken(writeTokenFile(t, " \n"))
		if err == nil || !strings.Contains(err.Error(), "is empty") {
			t.Fatalf("got %v; want an empty token file error", err)
		}
	})
}

func TestUsesRemovedTokenFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"double dash", []string{"--server", "http://x", "--token", "t"}, true},
		{"single dash", []string{"-token", "t"}, true},
		{"double dash with value", []string{"--token=t"}, true},
		{"single dash with value", []string{"-token=t"}, true},
		{"after a positional", []string{"--diagnose", "extra", "--token", "t"}, true},
		{"token-file is fine", []string{"--token-file", "/etc/pulse-agent/token"}, false},
		{"token-file with value is fine", []string{"--token-file=/etc/pulse-agent/token"}, false},
		{"after terminator is positional", []string{"--diagnose", "--", "--token"}, false},
		{"plain word", []string{"token"}, false},
		{"no args", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := usesRemovedTokenFlag(tc.args); got != tc.want {
				t.Fatalf("usesRemovedTokenFlag(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestRunPushesAndStops(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { run(ctx, srv.URL, "tok", 20*time.Millisecond); close(done) }()

	// The collector's CPU sample blocks ~500ms, so allow the immediate push to
	// complete (and hit the server) before cancelling.
	time.Sleep(700 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not stop on cancel")
	}
	if atomic.LoadInt32(&hits) < 1 {
		t.Fatal("expected at least one push")
	}
}

func TestParseAndRun(t *testing.T) {
	t.Setenv("PULSE_AGENT_TOKEN", "")
	tokFile := writeTokenFile(t, "t\n")
	// missing server -> 1
	if code := parseAndRun(context.Background(), []string{"--token-file", tokFile}, io.Discard); code != 1 {
		t.Errorf("missing server: code=%d want 1", code)
	}
	// missing token -> 1
	if code := parseAndRun(context.Background(), []string{"--server", "http://x"}, io.Discard); code != 1 {
		t.Errorf("missing token: code=%d want 1", code)
	}
	// unreadable token file -> 1
	if code := parseAndRun(context.Background(), []string{"--server", "http://x", "--token-file", "/no/such/file"}, io.Discard); code != 1 {
		t.Errorf("missing token file: code=%d want 1", code)
	}
	// bad interval -> 1
	if code := parseAndRun(context.Background(), []string{"--server", "http://x", "--token-file", tokFile, "--interval", "0"}, io.Discard); code != 1 {
		t.Errorf("bad interval: code=%d want 1", code)
	}
	// bad flag -> 2
	if code := parseAndRun(context.Background(), []string{"--nope"}, io.Discard); code != 2 {
		t.Errorf("bad flag: code=%d want 2", code)
	}
	// --diagnose with a server but no token -> 1, before any collection runs
	if code := parseAndRun(context.Background(), []string{"--diagnose", "--server", "http://x"}, io.Discard); code != 1 {
		t.Errorf("diagnose without token: code=%d want 1", code)
	}
	// valid, token from file -> runs then returns 0 on cancel
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	if code := parseAndRun(ctx, []string{"--server", srv.URL, "--token-file", tokFile, "--interval", "1"}, io.Discard); code != 0 {
		t.Errorf("valid run: code=%d want 0", code)
	}
}

func TestParseAndRunEnvToken(t *testing.T) {
	t.Setenv("PULSE_AGENT_TOKEN", "env-token")
	var auth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(204)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	if code := parseAndRun(ctx, []string{"--server", srv.URL}, io.Discard); code != 0 {
		t.Fatalf("env token run: code=%d want 0", code)
	}
	if got, _ := auth.Load().(string); got != "Bearer env-token" {
		t.Fatalf("Authorization = %q, want the env token", got)
	}
}

func TestParseAndRunRejectsTokenFlag(t *testing.T) {
	t.Setenv("PULSE_AGENT_TOKEN", "")
	for _, args := range [][]string{
		{"--server", "http://x", "--token", "secret"},
		{"--token=secret"},
		{"--diagnose", "--server", "http://x", "-token", "secret"},
	} {
		var stderr bytes.Buffer
		if code := parseAndRun(context.Background(), args, &stderr); code != 2 {
			t.Errorf("%q: code=%d want 2", args, code)
		}
		msg := stderr.String()
		for _, want := range []string{"--token was removed in 3.0", "PULSE_AGENT_TOKEN", "--token-file"} {
			if !strings.Contains(msg, want) {
				t.Errorf("%q: stderr %q does not mention %q", args, msg, want)
			}
		}
		if strings.Contains(msg, "secret") {
			t.Errorf("%q: stderr echoes the token: %q", args, msg)
		}
	}
}

func TestUsageNamesTokenSources(t *testing.T) {
	var stderr bytes.Buffer
	if code := parseAndRun(context.Background(), []string{"--help"}, &stderr); code != 2 {
		t.Fatalf("--help: code=%d want 2", code)
	}
	out := stderr.String()
	for _, want := range []string{"PULSE_AGENT_TOKEN", "-token-file", "precedence over PULSE_AGENT_TOKEN"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage %q does not mention %q", out, want)
		}
	}
}
