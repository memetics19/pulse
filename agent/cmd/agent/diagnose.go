package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/memetics19/pulse/agent/internal/diagnostics"
)

// diagnosticsPusher uploads a collected bundle. It is an interface so
// --diagnose can run with no server configured at all.
type diagnosticsPusher interface {
	PushDiagnostics(ctx context.Context, b diagnostics.Bundle) error
}

// uploadTimeout bounds the bundle upload independently of how long collection
// took, so a slow host cannot leave the evidence unsent.
const uploadTimeout = 30 * time.Second

// maxBundleBytes caps the rendered bundle. The server rejects requests over
// 1 MiB, so an oversized bundle should fail here with a message naming the
// cause rather than as an opaque 400 from the far end. Headroom covers the
// request envelope.
const maxBundleBytes = 900 << 10

// validateDiagnoseFlags rejects invocations that would do something other than
// what was asked. Supplying only one of --server or --token used to fall back
// to local-only mode and exit 0, so automation could believe evidence reached
// the server when it never did. A stray positional argument is a typo, not an
// instruction.
func validateDiagnoseFlags(server, token string, nargs int) error {
	if (server == "") != (token == "") {
		return errors.New("--server and --token must be given together")
	}
	if nargs > 0 {
		return errors.New("unexpected positional arguments; --diagnose takes none")
	}
	return nil
}

// runDiagnose collects one diagnostic bundle and either uploads it or, when no
// pusher is configured, writes it to out. The local mode matters: when Pulse
// itself is unreachable, printing the bundle is the only way to get it.
func runDiagnose(ctx context.Context, r diagnostics.Runner, p diagnosticsPusher, out io.Writer) error {
	bundle := diagnostics.Collect(ctx, r)

	rendered, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("render bundle: %w", err)
	}
	// Printing locally and uploading are independent effects. A full disk or a
	// broken stdout redirect is exactly the degraded-host case this serves, so
	// a local write failure must not destroy the only off-host copy.
	var writeErr error
	if _, err := out.Write(append(rendered, '\n')); err != nil {
		writeErr = fmt.Errorf("write bundle: %w", err)
	}

	// An interrupted run is not a success: Collect records "context canceled"
	// in every section it could not reach. The partial bundle is still printed,
	// but the exit status has to say the diagnosis did not complete, or a
	// script will treat cancellation as a clean result.
	if err := ctx.Err(); err != nil {
		return errors.Join(writeErr, fmt.Errorf("collection interrupted: %w", err))
	}

	if p == nil {
		return writeErr
	}

	// Checked after printing: the local copy is the evidence, and losing it
	// because it cannot be uploaded would be the worse outcome.
	if len(rendered) > maxBundleBytes {
		return errors.Join(writeErr, fmt.Errorf(
			"bundle too large to upload: %d bytes exceeds the %d byte limit",
			len(rendered), maxBundleBytes))
	}
	// The upload gets its own budget rather than whatever collection left over,
	// and it stays a child of ctx so Ctrl-C still cancels the run.
	// diagnosticsPusher promises no timeout of its own.
	uploadCtx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()

	if err := p.PushDiagnostics(uploadCtx, bundle); err != nil {
		return errors.Join(writeErr, fmt.Errorf("upload bundle: %w", err))
	}
	return writeErr
}
