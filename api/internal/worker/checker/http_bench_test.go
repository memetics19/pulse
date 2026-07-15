package checker_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync/atomic"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/checker"
)

// BenchmarkHTTPChecker exercises the checker against a local server. With the
// shared transport, connections are reused across checks (no per-check TCP
// handshake), which both lowers latency and stops the old per-check transport
// leak. Run with -benchmem to see the allocation drop.
func BenchmarkHTTPChecker(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := checker.NewHTTP(200, "", true)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if res := c.Check(ctx, srv.URL, 5); res.Status != "up" {
			b.Fatalf("unexpected status %q: %s", res.Status, res.ErrorMessage)
		}
	}
}

// TestHTTPCheckerReusesConnections asserts the shared transport actually pools
// connections: after the first check, subsequent checks reuse the connection
// (GotConn.Reused == true) instead of re-handshaking. This is the regression
// guard for the "fresh transport per check" leak/latency bug.
func TestHTTPCheckerReusesConnections(t *testing.T) {
	var reusedAfterFirst atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := checker.NewHTTP(200, "", true)

	// First check: warms the pool.
	if res := c.Check(context.Background(), srv.URL, 5); res.Status != "up" {
		t.Fatalf("first check: %q %s", res.Status, res.ErrorMessage)
	}

	// Second check with a trace: the connection should be reused.
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { reusedAfterFirst.Store(info.Reused) },
	}
	ctx := httptrace.WithClientTrace(context.Background(), trace)
	if res := c.Check(ctx, srv.URL, 5); res.Status != "up" {
		t.Fatalf("second check: %q %s", res.Status, res.ErrorMessage)
	}
	if !reusedAfterFirst.Load() {
		t.Fatal("expected the shared transport to reuse the connection on the second check")
	}
}
