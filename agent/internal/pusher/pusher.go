package pusher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/memetics19/pulse/agent/internal/collector"
)

// defaultMetricsTimeout bounds a single metrics push. It is short because the
// agent pushes on a ticker and a stalled request would delay the next sample.
const defaultMetricsTimeout = 10 * time.Second

// Pusher sends metric snapshots to the Pulse API.
type Pusher struct {
	serverURL string
	token     string
	client    *http.Client

	// metricsTimeout bounds a metrics push. A field rather than a constant so
	// the bound can be exercised without a ten-second test.
	metricsTimeout time.Duration
}

// New creates a Pusher targeting serverURL (e.g. "https://status.example.com")
// and authenticating with the given bearer token.
func New(serverURL, token string) *Pusher {
	return &Pusher{
		serverURL: serverURL,
		token:     token,
		// No client timeout: it would silently cap every operation, including
		// a bundle upload given a longer budget by its caller. Each method
		// bounds itself through the context instead.
		client:         &http.Client{},
		metricsTimeout: defaultMetricsTimeout,
	}
}

// Push encodes m as JSON and POSTs it to POST /api/ingest/metrics.
// Returns a non-nil error if the request fails or the server returns non-2xx.
func (p *Pusher) Push(ctx context.Context, m collector.Metrics) error {
	// Metrics run on a ticker and the caller supplies no deadline, so this
	// bounds itself rather than risking a push that never returns.
	ctx, cancel := context.WithTimeout(ctx, p.metricsTimeout)
	defer cancel()

	return p.postJSON(ctx, "/api/ingest/metrics", m)
}

// postJSON marshals payload and POSTs it to path with the agent's bearer
// token, treating any non-2xx response as an error.
func (p *Pusher) postJSON(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("pusher: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.serverURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("pusher: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("pusher: do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("pusher: server returned %d", resp.StatusCode)
	}
	return nil
}
