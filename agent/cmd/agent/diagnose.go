package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/memetics19/pulse/agent/internal/diagnostics"
)

// diagnosticsPusher uploads a collected bundle. It is an interface so
// --diagnose can run with no server configured at all.
type diagnosticsPusher interface {
	PushDiagnostics(ctx context.Context, b diagnostics.Bundle) error
}

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
	// Collection may have consumed the whole deadline — which is likeliest on
	// exactly the degraded hosts worth diagnosing. The upload must not inherit
	// an expired context, or the evidence never leaves the box. Pusher bounds
	// the request with its own client timeout.
	if err := p.PushDiagnostics(context.WithoutCancel(ctx), bundle); err != nil {
		return fmt.Errorf("upload bundle: %w", err)
	}
	return nil
}
