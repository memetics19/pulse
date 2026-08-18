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

// Pusher sends metric snapshots to the Pulse API.
type Pusher struct {
	serverURL string
	token     string
	client    *http.Client
}

// New creates a Pusher targeting serverURL (e.g. "https://status.example.com")
// and authenticating with the given bearer token.
func New(serverURL, token string) *Pusher {
	return &Pusher{
		serverURL: serverURL,
		token:     token,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Push encodes m as JSON and POSTs it to POST /api/ingest/metrics.
// Returns a non-nil error if the request fails or the server returns non-2xx.
func (p *Pusher) Push(ctx context.Context, m collector.Metrics) error {
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
