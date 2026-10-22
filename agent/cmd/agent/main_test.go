package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveToken(t *testing.T) {
	t.Run("env wins over file and flag", func(t *testing.T) {
		t.Setenv("PULSE_AGENT_TOKEN", "env-token")
		f := filepath.Join(t.TempDir(), "tok")
		os.WriteFile(f, []byte("file-token\n"), 0o600)
		got, err := resolveToken("flag-token", f)
		if err != nil || got != "env-token" {
			t.Fatalf("got %q, %v; want env-token", got, err)
		}
	})

	t.Run("file wins over flag", func(t *testing.T) {
		t.Setenv("PULSE_AGENT_TOKEN", "")
		f := filepath.Join(t.TempDir(), "tok")
		os.WriteFile(f, []byte("  file-token\n"), 0o600)
		got, err := resolveToken("flag-token", f)
		if err != nil || got != "file-token" {
			t.Fatalf("got %q, %v; want file-token", got, err)
		}
	})

	t.Run("flag fallback", func(t *testing.T) {
		t.Setenv("PULSE_AGENT_TOKEN", "")
		got, err := resolveToken("flag-token", "")
		if err != nil || got != "flag-token" {
			t.Fatalf("got %q, %v; want flag-token", got, err)
		}
	})

	t.Run("missing file errors", func(t *testing.T) {
		t.Setenv("PULSE_AGENT_TOKEN", "")
		if _, err := resolveToken("", "/no/such/token/file"); err == nil {
			t.Fatal("expected error for missing token file")
		}
	})
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
	// missing server/token -> 1
	if code := parseAndRun(context.Background(), []string{"--token", "t"}, io.Discard); code != 1 {
		t.Errorf("missing server: code=%d want 1", code)
	}
	// bad interval -> 1
	if code := parseAndRun(context.Background(), []string{"--server", "http://x", "--token", "t", "--interval", "0"}, io.Discard); code != 1 {
		t.Errorf("bad interval: code=%d want 1", code)
	}
	// bad flag -> 2
	if code := parseAndRun(context.Background(), []string{"--nope"}, io.Discard); code != 2 {
		t.Errorf("bad flag: code=%d want 2", code)
	}
	// valid -> runs then returns 0 on cancel
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	if code := parseAndRun(ctx, []string{"--server", srv.URL, "--token", "t", "--interval", "1"}, io.Discard); code != 0 {
		t.Errorf("valid run: code=%d want 0", code)
	}
}
