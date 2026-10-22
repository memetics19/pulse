package pusher

import (
	"context"
	"fmt"

	"github.com/memetics19/pulse/agent/internal/diagnostics"
)

// diagnosticsRequest mirrors the server's POST /api/ingest/diagnostics body.
type diagnosticsRequest struct {
	Bundle diagnostics.Bundle `json:"bundle"`
}

// PushDiagnostics uploads a diagnostic bundle collected from this host.
func (p *Pusher) PushDiagnostics(ctx context.Context, b diagnostics.Bundle) error {
	if err := p.postJSON(ctx, "/api/ingest/diagnostics", diagnosticsRequest{Bundle: b}); err != nil {
		return fmt.Errorf("pusher: diagnostics: %w", err)
	}
	return nil
}
