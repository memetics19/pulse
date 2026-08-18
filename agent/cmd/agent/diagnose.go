package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/memetics19/pulse/agent/internal/diagnostics"
)

// diagnosticsPusher uploads a collected bundle. It is an interface so
// --diagnose can run with no server configured at all.
type diagnosticsPusher interface {
	PushDiagnostics(ctx context.Context, b diagnostics.Bundle) error
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
	if err := p.PushDiagnostics(ctx, bundle); err != nil {
		return fmt.Errorf("upload bundle: %w", err)
	}
	return nil
}
