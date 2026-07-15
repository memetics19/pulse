package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/testutil"
)

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

func TestDataDir(t *testing.T) {
	t.Setenv("PULSE_DATA_DIR", "/custom")
	if got := dataDir(); got != "/custom" {
		t.Fatalf("dataDir=%q want /custom", got)
	}
	t.Setenv("PULSE_DATA_DIR", "")
	t.Setenv("SQLITE_PATH", "/var/lib/pulse/pulse.db")
	if got := dataDir(); got != "/var/lib/pulse" {
		t.Fatalf("dataDir=%q want /var/lib/pulse", got)
	}
	t.Setenv("SQLITE_PATH", "")
	if got := dataDir(); got != "/data" {
		t.Fatalf("dataDir=%q want /data", got)
	}
}

func TestRunWorkerStopsOnCancel(t *testing.T) {
	// Unconfigured app: runWorker polls, then returns on cancel.
	a := app.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { runWorker(ctx, a, config.Config{}); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runWorker (unconfigured) did not return on cancel")
	}

	// Configured app: worker.Run actually runs, then returns on cancel.
	a2 := app.New()
	a2.SetDB(testutil.NewTestDB(t))
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	go func() { runWorker(ctx2, a2, config.Config{}); close(done2) }()
	time.Sleep(50 * time.Millisecond)
	cancel2()
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("runWorker (configured) did not return on cancel")
	}
}

func TestServeStartsAndShutsDown(t *testing.T) {
	a := app.New()
	a.SetDB(testutil.NewTestDB(t))
	a.MarkWorkerAlive()
	cfg := config.Config{Port: freePort(t)}

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- serve(ctx, a, t.TempDir(), cfg) }()

	// Wait for the server to accept connections, then hit /healthz.
	url := "http://127.0.0.1:" + cfg.Port + "/healthz"
	var resp *http.Response
	var err error
	for i := 0; i < 50; i++ {
		resp, err = http.Get(url)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server never came up: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz = %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("serve returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not shut down")
	}
}
