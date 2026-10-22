package pusher_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/agent/internal/diagnostics"
	"github.com/memetics19/pulse/agent/internal/pusher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushDiagnostics_SendsBundleToIngestEndpoint(t *testing.T) {
	var path, auth string
	var body []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		var err error
		body, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var bundle diagnostics.Bundle
	bundle.Add("kernel", diagnostics.KernelReport{
		OOMKills: []diagnostics.OOMKill{{Process: "jellyfin", PID: 1234}},
	}, nil)

	require.NoError(t, pusher.New(srv.URL, "test-token-abc").
		PushDiagnostics(context.Background(), bundle))

	assert.Equal(t, "/api/ingest/diagnostics", path)
	assert.Equal(t, "Bearer test-token-abc", auth)

	var sent struct {
		Bundle diagnostics.Bundle `json:"bundle"`
	}
	require.NoError(t, json.Unmarshal(body, &sent))
	assert.Contains(t, sent.Bundle.Sections, "kernel")
}

func TestPushDiagnostics_ErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := pusher.New(srv.URL, "bad").PushDiagnostics(context.Background(), diagnostics.Bundle{})

	require.Error(t, err)
}
