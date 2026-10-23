package alerter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/stretchr/testify/assert"
)

var downAlert = alerter.Alert{Title: "down", Status: "detected", Severity: "major", Message: "m"}

// Issue #50: a stored webhook (saved before validation existed, or from
// SLACK_WEBHOOK_URL) is checked again before every send.
func TestSlackSenderRefusesUnsafeURLsAtSendTime(t *testing.T) {
	var hits int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// The test server listens on 127.0.0.1: forbidden unless private targets are allowed.
	s := alerter.NewSlackSender("", false, alerter.WithHTTPClient(srv.Client()))
	assert.Error(t, s.Send(context.Background(), map[string]string{"webhook_url": srv.URL}, downAlert))
	assert.Equal(t, 0, hits, "no request may reach a forbidden address")

	// http is refused even when private targets are allowed.
	plain := alerter.NewSlackSender("", true)
	assert.Error(t, plain.Send(context.Background(), map[string]string{"webhook_url": "http://127.0.0.1:1/x"}, downAlert))

	// The env fallback is checked too.
	env := alerter.NewSlackSender("https://169.254.169.254/latest", false)
	assert.Error(t, env.Send(context.Background(), map[string]string{}, downAlert))
}

// The production client blocks private addresses at dial time, whatever the
// URL check said (DNS can change between the check and the connect).
func TestSlackSenderDialGuard(t *testing.T) {
	var hits int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// No WithHTTPClient: the real guarded client. allowPrivate=false refuses 127.0.0.1.
	s := alerter.NewSlackSender("", false)
	assert.Error(t, s.Send(context.Background(), map[string]string{"webhook_url": srv.URL}, downAlert))
	assert.Equal(t, 0, hits)
}
