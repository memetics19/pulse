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
	pushed   bool
	pushErr  error
	ctxErr   error
	deadline time.Time
	bounded  bool
}

func (s *stubPusher) PushDiagnostics(ctx context.Context, _ diagnostics.Bundle) error {
	s.pushed = true
	s.ctxErr = ctx.Err()
	s.deadline, s.bounded = ctx.Deadline()
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

// diagnosticsPusher promises no timeout of its own, so runDiagnose must give
// the upload a finite budget rather than trusting the adapter.
func TestRunDiagnose_UploadGetsItsOwnBudget(t *testing.T) {
	p := &stubPusher{}

	require.NoError(t, runDiagnose(context.Background(), stubRunner{}, p, &bytes.Buffer{}))

	require.True(t, p.pushed)
	assert.True(t, p.bounded, "upload must be bounded even when the caller sets no deadline")
}

// The upload budget must still be a child of the caller's context, or Ctrl-C
// stops cancelling the run.
func TestRunDiagnose_UploadHonoursCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &stubPusher{}

	_ = runDiagnose(ctx, stubRunner{}, p, &bytes.Buffer{})

	assert.Error(t, p.ctxErr, "a cancelled caller must cancel the upload")
}
