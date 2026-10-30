package checker_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingServer answers every request with status and counts the requests.
func countingServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestHTTPChecker_RetryPolicy(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		retry    *checker.Retry
		wantHits int32
	}{
		{name: "default retries a 5xx once", status: http.StatusBadGateway, wantHits: 2},
		{name: "no retries", status: http.StatusBadGateway, retry: &checker.Retry{}, wantHits: 1},
		{name: "three retries", status: http.StatusBadGateway, retry: &checker.Retry{Count: 3}, wantHits: 4},
		{name: "4xx is never retried", status: http.StatusNotFound, retry: &checker.Retry{Count: 3}, wantHits: 1},
		{name: "success is never retried", status: http.StatusOK, retry: &checker.Retry{Count: 3}, wantHits: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, hits := countingServer(t, tt.status)
			var c checker.Checker = checker.NewHTTP(0, "", true)
			if tt.retry != nil {
				c = checker.NewHTTP(0, "", true).WithRetry(*tt.retry)
			}
			c.Check(context.Background(), srv.URL, 5)
			assert.Equal(t, tt.wantHits, hits.Load())
		})
	}
}

func TestHTTPChecker_RetryRecovers(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := checker.NewHTTP(0, "", true).WithRetry(checker.Retry{Count: 2}).Check(context.Background(), srv.URL, 5)
	assert.Equal(t, "up", res.Status)
	assert.Equal(t, int32(3), hits.Load())
}

func TestTCPChecker_RetriesFailedConnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	start := time.Now()
	res := checker.NewTCP(true).WithRetry(checker.Retry{Count: 2, Interval: 30 * time.Millisecond}).
		Check(context.Background(), addr, 1)
	assert.Equal(t, "down", res.Status)
	assert.GreaterOrEqual(t, time.Since(start), 60*time.Millisecond, "two retries wait the interval twice")
}

func TestTCPChecker_UpWithRetry(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	res := checker.NewTCP(true).WithRetry(checker.DefaultRetry).Check(context.Background(), ln.Addr().String(), 5)
	assert.Equal(t, "up", res.Status)
}

func TestSSLChecker_RetriesFailedHandshake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	var accepts atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			_ = conn.Close() // no TLS: the handshake fails
		}
	}()

	res := checker.NewSSL(true).WithRetry(checker.Retry{Count: 2}).Check(context.Background(), ln.Addr().String(), 2)
	assert.Equal(t, "down", res.Status)
	assert.Equal(t, int32(3), accepts.Load())
}
