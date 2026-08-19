package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/memetics19/pulse/agent/internal/diagnostics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRunner struct{}

func (stubRunner) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	if name == "df" {
		return []byte("Filesystem 1024-blocks Used Available Capacity Mounted on\n" +
			"/dev/sda1 100 99 0 100% /\n"), nil
	}
	return nil, errors.New("not available on this host")
}

type stubPusher struct {
	pushed  bool
	pushErr error
	ctxErr  error
}

func (s *stubPusher) PushDiagnostics(ctx context.Context, _ diagnostics.Bundle) error {
	s.pushed = true
	s.ctxErr = ctx.Err()
	return s.pushErr
}

// Without a configured server the bundle still has to be usable, so it goes to
// stdout. That is the only mode available when Pulse itself is down.
func TestRunDiagnose_PrintsBundleWhenNoPusherConfigured(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, runDiagnose(context.Background(), stubRunner{}, nil, &out))

	var bundle diagnostics.Bundle
	require.NoError(t, json.Unmarshal(out.Bytes(), &bundle))
	assert.Contains(t, bundle.Sections, "disk")
	assert.Empty(t, bundle.Sections["disk"].Error)
	assert.NotEmpty(t, bundle.Sections["proxmox"].Error, "unavailable collectors degrade, not fail")
}

func TestRunDiagnose_UploadsWhenPusherConfigured(t *testing.T) {
	p := &stubPusher{}

	require.NoError(t, runDiagnose(context.Background(), stubRunner{}, p, &bytes.Buffer{}))

	assert.True(t, p.pushed)
}

func TestRunDiagnose_ReturnsUploadFailure(t *testing.T) {
	p := &stubPusher{pushErr: errors.New("server returned 401")}

	err := runDiagnose(context.Background(), stubRunner{}, p, &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

// Collection consumes the shared deadline on exactly the degraded hosts this
// feature exists for. The upload must not inherit an already-expired context,
// or the evidence never leaves the box.
func TestRunDiagnose_UploadsEvenWhenCollectionExhaustedTheDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	<-ctx.Done()

	p := &stubPusher{}
	require.NoError(t, runDiagnose(ctx, stubRunner{}, p, &bytes.Buffer{}))

	assert.True(t, p.pushed, "upload must still be attempted")
	assert.NoError(t, p.ctxErr, "pusher must receive a live context")
}

// Supplying only one credential silently fell back to local-only mode and
// exited 0, so automation would believe evidence reached the server.
func TestCredentialError(t *testing.T) {
	tests := []struct {
		name          string
		server, token string
		wantErr       bool
	}{
		{"neither is local-only mode", "", "", false},
		{"both is upload mode", "https://s", "t", false},
		{"server without token", "https://s", "", true},
		{"token without server", "", "t", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := credentialError(tc.server, tc.token)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
