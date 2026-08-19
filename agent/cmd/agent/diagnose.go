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

// credentialError rejects a half-configured upload. Supplying only one of
// --server or --token used to fall back to local-only mode and exit 0, so
// automation could believe evidence reached the server when it never did.
func credentialError(server, token string) error {
	if (server == "") != (token == "") {
		return errors.New("--server and --token must be given together")
	}
	return nil
}

// runDiagnose collects one diagnostic bundle and either uploads it or, when no
// pusher is configured, writes it to out. The local mode matters: when Pulse
// itself is unreachable, printing the bundle is the only way to get it.
func runDiagnose(ctx context.Context, r diagnostics.Runner, p diagnosticsPusher, out io.Writer) error {
	bundle := diagnostics.Collect(ctx, r)

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(bundle); err != nil {
		return fmt.Errorf("render bundle: %w", err)
	}

	if p == nil {
		return nil
	}
	// The upload gets its own budget rather than whatever collection left over,
	// and it stays a child of ctx so Ctrl-C still cancels the run.
	// diagnosticsPusher promises no timeout of its own.
	uploadCtx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()

	if err := p.PushDiagnostics(uploadCtx, bundle); err != nil {
		return fmt.Errorf("upload bundle: %w", err)
	}
	return nil
}
