package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

// An interrupted run is not a success. Collect records "context canceled" in
// every section, so returning nil would make a script treat a partial
// cancellation bundle as a completed diagnosis.
func TestRunDiagnose_InterruptedRunIsNotSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &stubPusher{}
	var out bytes.Buffer

	err := runDiagnose(ctx, stubRunner{}, p, &out)

	require.Error(t, err, "an interrupted diagnosis must not report success")
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, p.pushed, "a cancelled run must not upload")
	assert.NotEmpty(t, out.String(), "partial evidence is still printed")
}

func TestValidateDiagnoseFlags(t *testing.T) {
	tests := []struct {
		name          string
		server, token string
		nargs         int
		wantErr       bool
	}{
		{"neither credential is local-only mode", "", "", 0, false},
		{"both credentials is upload mode", "https://s", "t", 0, false},
		{"server without token", "https://s", "", 0, true},
		{"token without server", "", "t", 0, true},
		{"stray positional argument", "", "", 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDiagnoseFlags(tc.server, tc.token, tc.nargs)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// The server caps requests at 1 MiB. A host with a very large container or
// mount table is how a bundle realistically gets there. It should fail with a
// message naming the cause rather than as an opaque 400 from the far end, and
// the local copy must still be printed so the evidence is not lost.
func TestRunDiagnose_RejectsAnOversizedBundleButStillPrintsIt(t *testing.T) {
	var containers strings.Builder
	for i := 0; i < 6000; i++ {
		fmt.Fprintf(&containers,
			`{"ID":"abcdef%06d","Image":"registry.example.com/team/service:1.2.3","Names":"service-%06d","State":"running","Status":"Up 3 days"}`+"\n", i, i)
	}

	runner := funcRunnerAgent(func(name string, _ []string) ([]byte, error) {
		if name == "docker" {
			return []byte(containers.String()), nil
		}
		return nil, errors.New("unavailable")
	})
	p := &stubPusher{}
	var out bytes.Buffer

	err := runDiagnose(context.Background(), runner, p, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
	assert.False(t, p.pushed, "an oversized bundle must not be sent")
	assert.Greater(t, out.Len(), maxBundleBytes, "the local copy is still printed in full")
}

type funcRunnerAgent func(name string, args []string) ([]byte, error)

func (f funcRunnerAgent) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	return f(name, args)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("no space left on device") }

// A full disk or a broken stdout redirect is exactly the degraded-host case
// this command serves. Losing the only off-host copy because the local one
// could not be written is the worst possible outcome.
func TestRunDiagnose_UploadsEvenWhenLocalWriteFails(t *testing.T) {
	p := &stubPusher{}

	err := runDiagnose(context.Background(), stubRunner{}, p, failingWriter{})

	assert.True(t, p.pushed, "the upload must still be attempted")
	require.Error(t, err, "the write failure is still reported")
	assert.Contains(t, err.Error(), "no space left on device")
}
