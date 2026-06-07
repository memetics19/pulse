package pusher_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/agent/internal/collector"
	"github.com/memetics19/pulse/agent/internal/pusher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPush_SendsCorrectPayloadAndHeaders(t *testing.T) {
	var capturedBody []byte
	var capturedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/ingest/metrics", r.URL.Path)
		capturedAuth = r.Header.Get("Authorization")
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p := pusher.New(srv.URL, "test-token-abc")
	m := collector.Metrics{
		CpuPercent:  24.5,
		MemPercent:  61.2,
		MemUsedMb:   4920.0,
		DiskPercent: 38.1,
		DiskUsedGb:  152.4,
		NetRxBytes:  1234567,
		NetTxBytes:  987654,
	}

	err := p.Push(context.Background(), m)
	require.NoError(t, err)

	assert.Equal(t, "Bearer test-token-abc", capturedAuth)

	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &sent))
	assert.InDelta(t, 24.5, sent["cpu_percent"], 0.001)
	assert.InDelta(t, 61.2, sent["mem_percent"], 0.001)
	assert.InDelta(t, 4920.0, sent["mem_used_mb"], 0.001)
	assert.InDelta(t, 38.1, sent["disk_percent"], 0.001)
	assert.InDelta(t, 152.4, sent["disk_used_gb"], 0.001)
	assert.EqualValues(t, 1234567, sent["net_rx_bytes"])
	assert.EqualValues(t, 987654, sent["net_tx_bytes"])
}

func TestPush_ReturnsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := pusher.New(srv.URL, "bad-token")
	err := p.Push(context.Background(), collector.Metrics{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestPush_ReturnsErrorWhenServerUnreachable(t *testing.T) {
	p := pusher.New("http://127.0.0.1:1", "tok")
	err := p.Push(context.Background(), collector.Metrics{})
	require.Error(t, err)
}
